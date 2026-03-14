package data

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ifnodoraemon/openDataAnalysis/config"
	"github.com/sashabaranov/go-openai"
	"github.com/xuri/excelize/v2"
	_ "modernc.org/sqlite"
)

// Ingester 数据导入引擎: Excel/CSV → SQLite
type Ingester struct {
	CacheDir string
	db       *sql.DB
	dbPath   string
}

// NewIngester 创建导入引擎
func NewIngester(cacheDir string) *Ingester {
	return &Ingester{CacheDir: cacheDir}
}

// GetDB 获取当前数据库连接
func (ing *Ingester) GetDB() *sql.DB {
	return ing.db
}

// InitDB 初始化 SQLite 缓存数据库
func (ing *Ingester) InitDB(sessionID string) error {
	if err := os.MkdirAll(ing.CacheDir, 0o755); err != nil {
		return fmt.Errorf("创建缓存目录失败: %w", err)
	}
	dbPath := filepath.Join(ing.CacheDir, sessionID+".db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("创建 SQLite 数据库失败: %w", err)
	}
	ing.db = db
	ing.dbPath = dbPath
	return nil
}

func (ing *Ingester) ResetDB(sessionID string) error {
	if ing.db != nil {
		_ = ing.db.Close()
		ing.db = nil
	}
	dbPath := filepath.Join(ing.CacheDir, sessionID+".db")
	if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除旧数据库失败: %w", err)
	}
	return ing.InitDB(sessionID)
}

// ImportFile 导入文件到 SQLite
func (ing *Ingester) ImportFile(filePath string) (tableName string, rowCount int, colCount int, err error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	baseName := strings.TrimSuffix(filepath.Base(filePath), ext)
	tableName = sanitizeTableName(baseName)

	switch ext {
	case ".csv":
		rowCount, colCount, err = ing.importCSV(filePath, tableName)
	case ".xlsx", ".xls":
		rowCount, colCount, err = ing.importExcel(filePath, tableName)
	default:
		err = fmt.Errorf("不支持的文件格式: %s", ext)
	}
	return
}

// importCSV 导入 CSV 文件
func (ing *Ingester) importCSV(filePath, tableName string) (int, int, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return 0, 0, fmt.Errorf("打开 CSV 文件失败: %w", err)
	}
	defer f.Close()

	reader := csv.NewReader(f)

	// 读取表头
	headers, err := reader.Read()
	if err != nil {
		return 0, 0, fmt.Errorf("读取 CSV 表头失败: %w", err)
	}

	colCount := len(headers)
	sanitizedHeaders := make([]string, colCount)
	for i, h := range headers {
		sanitizedHeaders[i] = sanitizeColumnName(h)
	}

	// 读取部分数据进行类型推断（最多扫描 500 行）
	sampleSize := 500
	var sampleRows [][]string
	for i := 0; i < sampleSize; i++ {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue // 跳过错误行
		}
		sampleRows = append(sampleRows, record)
	}

	// 类型推断
	colTypes := inferColumnTypes(sampleRows, colCount)

	// 创建表
	if err := ing.createTableTyped(tableName, sanitizedHeaders, colTypes); err != nil {
		return 0, 0, err
	}

	// 插入已经读出的 sample 行
	rowCount := 0
	if len(sampleRows) > 0 {
		if err := ing.insertBatchTyped(tableName, sanitizedHeaders, colTypes, sampleRows); err != nil {
			return rowCount, colCount, err
		}
		rowCount += len(sampleRows)
	}

	// 流式读取剩余数据并批量插入
	batchSize := 500
	batch := make([][]string, 0, batchSize)

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue // 跳过错误行
		}

		batch = append(batch, record)

		if len(batch) >= batchSize {
			if err := ing.insertBatchTyped(tableName, sanitizedHeaders, colTypes, batch); err != nil {
				return rowCount, colCount, err
			}
			rowCount += len(batch)
			batch = batch[:0] // 复用 slice
		}
	}

	// 插入最后不足一个 batch 的数据
	if len(batch) > 0 {
		if err := ing.insertBatchTyped(tableName, sanitizedHeaders, colTypes, batch); err != nil {
			return rowCount, colCount, err
		}
		rowCount += len(batch)
	}

	// 自动生成系统统计和索引
	_ = ing.GenerateStatsAndIndexes(tableName)

	return rowCount, colCount, nil
}

// importExcel 导入 Excel 文件（流式处理，带行数硬上限）
func (ing *Ingester) importExcel(filePath, tableName string) (int, int, error) {
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		return 0, 0, fmt.Errorf("打开 Excel 文件失败: %w", err)
	}
	defer f.Close()

	// 默认使用第一个 sheet
	sheetName := f.GetSheetName(0)
	rows, err := f.Rows(sheetName)
	if err != nil {
		return 0, 0, fmt.Errorf("读取 Excel 数据失败: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return 0, 0, fmt.Errorf("Excel 文件为空")
	}

	// 表头
	headers, err := rows.Columns()
	if err != nil {
		return 0, 0, fmt.Errorf("读取 Excel 表头失败: %w", err)
	}
	colCount := len(headers)
	if colCount == 0 {
		return 0, 0, fmt.Errorf("Excel 文件没有表头")
	}

	sanitizedHeaders := make([]string, colCount)
	for i, h := range headers {
		sanitizedHeaders[i] = sanitizeColumnName(h)
	}

	// 明确产品上限：最多支持 10 万行
	const maxExcelRows = 100000

	// 读取部分数据进行类型推断（最多扫描 500 行）
	sampleSize := 500
	var sampleRows [][]string
	rowsRead := 0

	for i := 0; i < sampleSize; i++ {
		if !rows.Next() {
			break
		}
		row, err := rows.Columns()
		if err != nil {
			continue
		}
		// 补齐或截断列数
		padded := make([]string, colCount)
		copy(padded, row)
		sampleRows = append(sampleRows, padded)
		rowsRead++
	}

	if rowsRead == 0 {
		return 0, 0, fmt.Errorf("Excel 数据区为空")
	}

	// 类型推断
	colTypes := inferColumnTypes(sampleRows, colCount)

	// 创建表
	if err := ing.createTableTyped(tableName, sanitizedHeaders, colTypes); err != nil {
		return 0, 0, err
	}

	// 插入已经读出的 sample 行
	rowCount := 0
	if len(sampleRows) > 0 {
		if err := ing.insertBatchTyped(tableName, sanitizedHeaders, colTypes, sampleRows); err != nil {
			return rowCount, colCount, err
		}
		rowCount += len(sampleRows)
	}

	// 流式读取剩余数据并批量插入
	batchSize := 500
	batch := make([][]string, 0, batchSize)

	for rows.Next() {
		if rowCount+len(batch) >= maxExcelRows {
			return 0, 0, fmt.Errorf("Excel 文件行数超过 %d 行的上限，若是较大数据集建议转换为 CSV 格式后再上传", maxExcelRows)
		}

		row, err := rows.Columns()
		if err != nil {
			continue // 跳过错误行
		}

		// 补齐或截断列数
		padded := make([]string, colCount)
		copy(padded, row)
		batch = append(batch, padded)

		if len(batch) >= batchSize {
			if err := ing.insertBatchTyped(tableName, sanitizedHeaders, colTypes, batch); err != nil {
				return rowCount, colCount, err
			}
			rowCount += len(batch)
			batch = batch[:0] // 复用 slice
		}
	}

	// 插入最后不足一个 batch 的数据
	if len(batch) > 0 {
		if err := ing.insertBatchTyped(tableName, sanitizedHeaders, colTypes, batch); err != nil {
			return rowCount, colCount, err
		}
		rowCount += len(batch)
	}

	// 自动生成系统统计和索引
	_ = ing.GenerateStatsAndIndexes(tableName)

	return rowCount, colCount, nil
}

// ColumnType 列类型
type ColumnType int

const (
	TypeText    ColumnType = iota // 默认文本
	TypeInteger                   // 整数
	TypeReal                      // 浮点数
)

func (t ColumnType) SQLType() string {
	switch t {
	case TypeInteger:
		return "INTEGER"
	case TypeReal:
		return "REAL"
	default:
		return "TEXT"
	}
}

// inferColumnTypes 扫描前 N 行推断列类型
func inferColumnTypes(rows [][]string, colCount int) []ColumnType {
	types := make([]ColumnType, colCount)

	// 初始假设所有列是整数
	for i := range types {
		types[i] = TypeInteger
	}

	sampleSize := 100
	if len(rows) < sampleSize {
		sampleSize = len(rows)
	}

	for _, row := range rows[:sampleSize] {
		for i := 0; i < colCount && i < len(row); i++ {
			val := strings.TrimSpace(row[i])

			// 空值不影响判断
			if val == "" || val == "-" || val == "N/A" || val == "null" || val == "NULL" {
				continue
			}

			switch types[i] {
			case TypeInteger:
				// 先尝试整数
				if _, err := strconv.ParseInt(val, 10, 64); err != nil {
					// 再尝试浮点
					if _, err := strconv.ParseFloat(val, 64); err != nil {
						types[i] = TypeText // 不是数字 → TEXT
					} else {
						types[i] = TypeReal // 是浮点 → REAL
					}
				}
			case TypeReal:
				// 已确定为浮点，检查是否还是数字
				if _, err := strconv.ParseFloat(val, 64); err != nil {
					types[i] = TypeText // 不是数字 → TEXT
				}
			case TypeText:
				// 已确定为文本，不再变化
			}
		}
	}

	return types
}

// createTableTyped 创建带类型的 SQLite 表
func (ing *Ingester) createTableTyped(tableName string, columns []string, types []ColumnType) error {
	_, _ = ing.db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS \"%s\"", tableName))

	var colDefs []string
	for i, col := range columns {
		sqlType := "TEXT"
		if i < len(types) {
			sqlType = types[i].SQLType()
		}
		colDefs = append(colDefs, fmt.Sprintf("\"%s\" %s", col, sqlType))
	}

	createSQL := fmt.Sprintf("CREATE TABLE \"%s\" (%s)", tableName, strings.Join(colDefs, ", "))
	_, err := ing.db.Exec(createSQL)
	if err != nil {
		return fmt.Errorf("创建表失败: %w", err)
	}
	return nil
}

// insertBatchTyped 批量插入（带类型转换）
func (ing *Ingester) insertBatchTyped(tableName string, columns []string, types []ColumnType, rows [][]string) error {
	if len(rows) == 0 {
		return nil
	}

	tx, err := ing.db.Begin()
	if err != nil {
		return err
	}

	placeholders := make([]string, len(columns))
	for i := range placeholders {
		placeholders[i] = "?"
	}

	insertSQL := fmt.Sprintf("INSERT INTO \"%s\" (%s) VALUES (%s)",
		tableName,
		"\""+strings.Join(columns, "\", \"")+"\"",
		strings.Join(placeholders, ", "))

	stmt, err := tx.Prepare(insertSQL)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	for _, row := range rows {
		vals := make([]interface{}, len(columns))
		for i := range columns {
			if i >= len(row) || strings.TrimSpace(row[i]) == "" {
				vals[i] = nil // 空值 → NULL
				continue
			}

			val := strings.TrimSpace(row[i])

			if i < len(types) {
				switch types[i] {
				case TypeInteger:
					if v, err := strconv.ParseInt(val, 10, 64); err == nil {
						vals[i] = v
					} else {
						vals[i] = val // fallback 到文本
					}
				case TypeReal:
					if v, err := strconv.ParseFloat(val, 64); err == nil {
						vals[i] = v
					} else {
						vals[i] = val
					}
				default:
					vals[i] = val
				}
			} else {
				vals[i] = val
			}
		}
		if _, err := stmt.Exec(vals...); err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit()
}

// sanitizeTableName 清理表名
func sanitizeTableName(name string) string {
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, "-", "_")
	return strings.ToLower(name)
}

// sanitizeColumnName 清理列名
func sanitizeColumnName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "unnamed"
	}
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, "-", "_")
	return strings.ToLower(name)
}

// GenerateStatsAndIndexes 针对表生成统计信息并按基数自动创建索引
func (ing *Ingester) GenerateStatsAndIndexes(tableName string) error {
	if ing.db == nil {
		return fmt.Errorf("数据库未初始化")
	}

	// 1. 生成系统级统计 (用于 SQL Query Planner)
	_, err := ing.db.Exec(fmt.Sprintf("ANALYZE \"%s\"", tableName))
	if err != nil {
		fmt.Printf("[Warning] Failed to analyze table %s: %v\n", tableName, err)
	}

	// 2. 根据 `ExtractSchema` 或自定义逻辑，给具有适当基数特性的列添加索引
	schema, err := ExtractSchema(ing.db, tableName)
	if err != nil {
		return fmt.Errorf("提取概要失败: %w", err)
	}

	// 2.1 获取目前环境中的其他表名供大模型做 Join 探测
	var activeTables []string
	tableRows, _ := ing.db.Query(`SELECT table_name FROM _oda_table_metadata`)
	if tableRows != nil {
		for tableRows.Next() {
			var tName string
			if err := tableRows.Scan(&tName); err == nil {
                if tName != schema.TableName {
				    activeTables = append(activeTables, tName)
                }
			}
		}
		tableRows.Close()
	}

	// 2.2 小样本语义大模型预分析 (Item 10)
	if config.Cfg == nil || config.Cfg.LLMAPIKey == "" {
		return fmt.Errorf("系统未配置 LLMAPIKey，无法执行小样本语义预分析")
	}

	client := openai.NewClient(config.Cfg.LLMAPIKey) // 或其它途径初始化，此处假定能全局读到配置
	if config.Cfg.LLMBaseURL != "" {
		cfg := openai.DefaultConfig(config.Cfg.LLMAPIKey)
		cfg.BaseURL = config.Cfg.LLMBaseURL
		client = openai.NewClientWithConfig(cfg)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	profile, metaErr := AnalyzeTableSemantics(ctx, client, schema, activeTables)
	if metaErr != nil {
		return fmt.Errorf("大模型语义分析失败: %w", metaErr)
	}

	fmt.Printf("[Info] AI 语义分析完成，提取了业务别名和关联关系预测表: %s\n", tableName)
	finalMetadataBytes, _ := json.Marshal(profile)

	// 2.3 将结果保存到内置元数据表
	_, _ = ing.db.Exec(`
		CREATE TABLE IF NOT EXISTS _oda_table_metadata (
			table_name TEXT PRIMARY KEY,
			schema_json TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)

	if len(finalMetadataBytes) > 0 {
		_, err = ing.db.Exec(
			`INSERT INTO _oda_table_metadata (table_name, schema_json, updated_at) 
			 VALUES (?, ?, CURRENT_TIMESTAMP)
			 ON CONFLICT(table_name) DO UPDATE SET schema_json=excluded.schema_json, updated_at=excluded.updated_at`,
			tableName, string(finalMetadataBytes),
		)
		if err != nil {
			fmt.Printf("[Warning] Failed to save schema metadata for %s: %v\n", tableName, err)
		}
	}

	// 3. 构建索引尝试
	for _, col := range schema.Columns {
		// 如果唯一值数量 > 1 并且非空，且唯一值占比小于 20%（说明有大量重复的类别）
		// 或者列名为常见 id (如 user_id, org_id)，自动建立索引
		isNominal := float64(col.UniqueCount)/float64(schema.RowCount) < 0.20 && col.UniqueCount > 1
		isID := strings.HasSuffix(strings.ToLower(col.Name), "_id") || strings.ToLower(col.Name) == "id"

		if isNominal || isID {
			idxName := fmt.Sprintf("idx_%s_%s", sanitizeTableName(tableName), sanitizeColumnName(col.Name))
			// 因为 SQLite 的标识符长度有限但此处通常够用，直接创建
			createIdxSQL := fmt.Sprintf("CREATE INDEX IF NOT EXISTS \"%s\" ON \"%s\" (\"%s\")", idxName, tableName, col.Name)
			_, err := ing.db.Exec(createIdxSQL)
			if err != nil {
				fmt.Printf("[Warning] Failed to create index %s on table %s: %v\n", idxName, tableName, err)
			}
		}
	}

	return nil
}

// GetTableMetadata 从内置表读取预先提取的 Schema 返回 JSON
func (ing *Ingester) GetTableMetadata(tableName string) (string, error) {
	if ing.db == nil {
		return "", fmt.Errorf("数据库未初始化")
	}
	var schemaJSON string
	err := ing.db.QueryRow(`SELECT schema_json FROM _oda_table_metadata WHERE table_name = ?`, tableName).Scan(&schemaJSON)
	if err != nil {
		return "", err
	}
	return schemaJSON, nil
}
