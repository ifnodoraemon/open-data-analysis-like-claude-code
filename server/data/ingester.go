package data

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

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
	dbPath := filepath.Join(ing.CacheDir, sessionID+".db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("创建 SQLite 数据库失败: %w", err)
	}
	ing.db = db
	ing.dbPath = dbPath
	return nil
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

	// 创建表
	if err := ing.createTable(tableName, sanitizedHeaders); err != nil {
		return 0, 0, err
	}

	// 批量插入数据
	rowCount := 0
	batchSize := 500
	var batch [][]string

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
			if err := ing.insertBatch(tableName, sanitizedHeaders, batch); err != nil {
				return rowCount, colCount, err
			}
			rowCount += len(batch)
			batch = batch[:0]
		}
	}

	// 插入剩余数据
	if len(batch) > 0 {
		if err := ing.insertBatch(tableName, sanitizedHeaders, batch); err != nil {
			return rowCount, colCount, err
		}
		rowCount += len(batch)
	}

	return rowCount, colCount, nil
}

// importExcel 导入 Excel 文件
func (ing *Ingester) importExcel(filePath, tableName string) (int, int, error) {
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		return 0, 0, fmt.Errorf("打开 Excel 文件失败: %w", err)
	}
	defer f.Close()

	// 默认使用第一个 sheet
	sheetName := f.GetSheetName(0)
	rows, err := f.GetRows(sheetName)
	if err != nil {
		return 0, 0, fmt.Errorf("读取 Excel 数据失败: %w", err)
	}

	if len(rows) < 1 {
		return 0, 0, fmt.Errorf("Excel 文件为空")
	}

	// 表头
	headers := rows[0]
	colCount := len(headers)
	sanitizedHeaders := make([]string, colCount)
	for i, h := range headers {
		sanitizedHeaders[i] = sanitizeColumnName(h)
	}

	// 创建表
	if err := ing.createTable(tableName, sanitizedHeaders); err != nil {
		return 0, 0, err
	}

	// 批量插入数据 (跳过表头)
	dataRows := rows[1:]
	batchSize := 500
	rowCount := 0

	for i := 0; i < len(dataRows); i += batchSize {
		end := i + batchSize
		if end > len(dataRows) {
			end = len(dataRows)
		}
		batch := dataRows[i:end]

		// 补齐列数
		normalizedBatch := make([][]string, len(batch))
		for j, row := range batch {
			if len(row) < colCount {
				padded := make([]string, colCount)
				copy(padded, row)
				normalizedBatch[j] = padded
			} else {
				normalizedBatch[j] = row[:colCount]
			}
		}

		if err := ing.insertBatch(tableName, sanitizedHeaders, normalizedBatch); err != nil {
			return rowCount, colCount, err
		}
		rowCount += len(normalizedBatch)
	}

	return rowCount, colCount, nil
}

// createTable 创建 SQLite 表
func (ing *Ingester) createTable(tableName string, columns []string) error {
	// 删除已存在的同名表
	_, _ = ing.db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS \"%s\"", tableName))

	var colDefs []string
	for _, col := range columns {
		colDefs = append(colDefs, fmt.Sprintf("\"%s\" TEXT", col))
	}

	createSQL := fmt.Sprintf("CREATE TABLE \"%s\" (%s)", tableName, strings.Join(colDefs, ", "))
	_, err := ing.db.Exec(createSQL)
	if err != nil {
		return fmt.Errorf("创建表失败: %w", err)
	}
	return nil
}

// insertBatch 批量插入数据
func (ing *Ingester) insertBatch(tableName string, columns []string, rows [][]string) error {
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
			if i < len(row) {
				vals[i] = row[i]
			} else {
				vals[i] = ""
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
