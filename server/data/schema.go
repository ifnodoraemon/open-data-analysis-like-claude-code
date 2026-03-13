package data

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const (
	queryTimeout   = 5 * time.Second
	queryRowLimit  = 200
	queryProbeRows = queryRowLimit + 1
)

var forbiddenSQLKeywordPattern = regexp.MustCompile(`\b(INSERT|UPDATE|DELETE|ALTER|DROP|CREATE|ATTACH|DETACH|REINDEX|VACUUM|PRAGMA|REPLACE|MERGE|UPSERT|TRUNCATE)\b`)

// SchemaInfo 表 Schema 信息
type SchemaInfo struct {
	TableName string       `json:"tableName"`
	RowCount  int          `json:"rowCount"`
	Columns   []ColumnInfo `json:"columns"`
}

// ColumnInfo 列信息
type ColumnInfo struct {
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	NonNullRate  float64  `json:"nonNullRate"`
	UniqueCount  int      `json:"uniqueCount"`
	SampleValues []string `json:"sampleValues"`
	Roles        []string `json:"roles,omitempty"` // 语义角色：time, amount, ratio, category, pk_candidate 等
	// 数值列统计
	Min    *float64 `json:"min,omitempty"`
	Max    *float64 `json:"max,omitempty"`
	Avg    *float64 `json:"avg,omitempty"`
	Median *float64 `json:"median,omitempty"`
}

// ExtractSchema 提取表的 Schema 和统计摘要
func ExtractSchema(db *sql.DB, tableName string) (*SchemaInfo, error) {
	schema := &SchemaInfo{TableName: tableName}

	// 获取行数
	var rowCount int
	err := db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM \"%s\"", tableName)).Scan(&rowCount)
	if err != nil {
		return nil, fmt.Errorf("获取行数失败: %w", err)
	}
	schema.RowCount = rowCount

	// 获取列信息
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(\"%s\")", tableName))
	if err != nil {
		return nil, fmt.Errorf("获取列信息失败: %w", err)
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull int
		var defaultVal, pk interface{}
		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultVal, &pk); err != nil {
			continue
		}
		columns = append(columns, name)
	}

	// 分析每一列
	for _, col := range columns {
		colInfo := ColumnInfo{Name: col, Type: "TEXT"}

		// 非空率
		var nonNullCount int
		db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM \"%s\" WHERE \"%s\" IS NOT NULL AND \"%s\" != ''", tableName, col, col)).Scan(&nonNullCount)
		if rowCount > 0 {
			colInfo.NonNullRate = float64(nonNullCount) / float64(rowCount)
		}

		// 唯一值数
		db.QueryRow(fmt.Sprintf("SELECT COUNT(DISTINCT \"%s\") FROM \"%s\" WHERE \"%s\" IS NOT NULL AND \"%s\" != ''", col, tableName, col, col)).Scan(&colInfo.UniqueCount)

		// 采样值 (前 5 个不同值)
		sampleRows, err := db.Query(fmt.Sprintf("SELECT DISTINCT \"%s\" FROM \"%s\" WHERE \"%s\" IS NOT NULL AND \"%s\" != '' LIMIT 5", col, tableName, col, col))
		if err == nil {
			for sampleRows.Next() {
				var val string
				if sampleRows.Scan(&val) == nil {
					colInfo.SampleValues = append(colInfo.SampleValues, val)
				}
			}
			sampleRows.Close()
		}

		// 尝试数值统计
		var minVal, maxVal, avgVal sql.NullFloat64
		err = db.QueryRow(fmt.Sprintf(
			"SELECT MIN(CAST(\"%s\" AS REAL)), MAX(CAST(\"%s\" AS REAL)), AVG(CAST(\"%s\" AS REAL)) FROM \"%s\" WHERE \"%s\" GLOB '[0-9]*'",
			col, col, col, tableName, col)).Scan(&minVal, &maxVal, &avgVal)
		if err == nil && minVal.Valid {
			colInfo.Type = "NUMERIC"
			min := minVal.Float64
			max := maxVal.Float64
			avg := avgVal.Float64
			colInfo.Min = &min
			colInfo.Max = &max
			colInfo.Avg = &avg
		}

		colInfo.Roles = inferColumnRoles(colInfo, rowCount)
		schema.Columns = append(schema.Columns, colInfo)
	}

	return schema, nil
}

// inferColumnRoles 基于字段名和列统计进行简单的语义角色识别
func inferColumnRoles(info ColumnInfo, rowCount int) []string {
	var roles []string
	nameLower := strings.ToLower(info.Name)

	// 1. time: 时间字段
	isTime := strings.Contains(nameLower, "time") || strings.Contains(nameLower, "date") ||
		strings.Contains(nameLower, "year") || strings.Contains(nameLower, "month") ||
		strings.Contains(nameLower, "day") || strings.HasSuffix(nameLower, "_at") ||
		strings.Contains(nameLower, "日期") || strings.Contains(nameLower, "时间")
	if isTime {
		roles = append(roles, "time")
	}

	// 2. amount: 金额字段
	isAmount := strings.Contains(nameLower, "price") || strings.Contains(nameLower, "amount") ||
		strings.Contains(nameLower, "cost") || strings.Contains(nameLower, "revenue") ||
		strings.Contains(nameLower, "fee") || strings.Contains(nameLower, "金额") ||
		strings.Contains(nameLower, "费用") || strings.Contains(nameLower, "钱") ||
		strings.Contains(nameLower, "总价") || strings.Contains(nameLower, "单价")
	if isAmount && info.Type == "NUMERIC" {
		roles = append(roles, "amount")
	}

	// 3. ratio: 比例字段
	isRatio := strings.Contains(nameLower, "rate") || strings.Contains(nameLower, "ratio") ||
		strings.Contains(nameLower, "pct") || strings.Contains(nameLower, "占比") ||
		strings.Contains(nameLower, "率") || strings.Contains(nameLower, "%") ||
		strings.Contains(nameLower, "百分比")
	if isRatio && info.Type == "NUMERIC" {
		roles = append(roles, "ratio")
	}

	// 4. category: 类别字段
	// 唯一值小于某个阈值，并且总行数较大时，视为 category
	uniqueRatio := 1.0
	if rowCount > 0 {
		uniqueRatio = float64(info.UniqueCount) / float64(rowCount)
	}
	isCategoryKw := strings.Contains(nameLower, "type") || strings.Contains(nameLower, "category") ||
		strings.Contains(nameLower, "status") || strings.Contains(nameLower, "类别") ||
		strings.Contains(nameLower, "类型") || strings.Contains(nameLower, "状态")

	// heuristics: 唯一值占比 < 20% 或者绝对唯一值 < 100，或者是相关关键词，并且不是数值 ID
	if (uniqueRatio < 0.2 || info.UniqueCount < 100 || isCategoryKw) && !strings.HasSuffix(nameLower, "id") && info.Type != "NUMERIC" && info.UniqueCount > 0 && info.UniqueCount < rowCount {
		roles = append(roles, "category")
	}

	// 5. pk_candidate: 主键候选
	isPK := strings.HasSuffix(nameLower, "id") || strings.HasSuffix(nameLower, "_no") || nameLower == "id" ||
		strings.Contains(nameLower, "编号") || strings.Contains(nameLower, "代码")
	if isPK && uniqueRatio > 0.90 && info.NonNullRate > 0.95 {
		roles = append(roles, "pk_candidate")
	}

	return roles
}

// GetSampleRows 获取采样行数据
func GetSampleRows(db *sql.DB, tableName string, limit int) ([]map[string]interface{}, error) {
	query := fmt.Sprintf("SELECT * FROM \"%s\" LIMIT %d", tableName, limit)
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns, _ := rows.Columns()
	var result []map[string]interface{}

	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			continue
		}

		row := make(map[string]interface{})
		for i, col := range columns {
			row[col] = values[i]
		}
		result = append(result, row)
	}

	return result, nil
}

// ExecuteQuery 执行 SQL 查询 (带安全限制)
func ExecuteQuery(db *sql.DB, query string) ([]map[string]interface{}, error) {
	normalizedQuery, err := normalizeReadOnlyQuery(query)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("获取数据库连接失败: %w", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "PRAGMA query_only = ON"); err != nil {
		return nil, fmt.Errorf("启用只读查询模式失败: %w", err)
	}

	wrappedQuery := fmt.Sprintf("SELECT * FROM (%s) AS _oda_query LIMIT %d", normalizedQuery, queryProbeRows)
	rows, err := conn.QueryContext(ctx, wrappedQuery)
	if err != nil {
		return nil, fmt.Errorf("SQL 执行失败: %w", err)
	}
	defer rows.Close()

	columns, _ := rows.Columns()
	var result []map[string]interface{}

	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			continue
		}

		row := make(map[string]interface{})
		for i, col := range columns {
			val := values[i]
			// 将 []byte 转为 string
			if b, ok := val.([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = val
			}
		}
		result = append(result, row)
		if len(result) >= queryProbeRows {
			return nil, fmt.Errorf("查询结果超过 %d 行，请增加 WHERE 条件或更小的 LIMIT", queryRowLimit)
		}
	}

	if err := rows.Err(); err != nil {
		if errorsIsDeadline(err) || ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("SQL 查询超时（>%ds），请简化语句或缩小范围", int(queryTimeout/time.Second))
		}
		return nil, fmt.Errorf("读取 SQL 结果失败: %w", err)
	}
	return result, nil
}

func normalizeReadOnlyQuery(query string) (string, error) {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return "", fmt.Errorf("SQL 不能为空")
	}

	trimmed = strings.TrimSuffix(trimmed, ";")
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" {
		return "", fmt.Errorf("SQL 不能为空")
	}

	if hasMultipleStatements(trimmed) {
		return "", fmt.Errorf("只允许单条 SQL 查询")
	}

	inspection := stripSQLStringsAndComments(trimmed)
	upper := strings.ToUpper(strings.TrimSpace(inspection))
	if !strings.HasPrefix(upper, "SELECT") && !strings.HasPrefix(upper, "WITH") {
		return "", fmt.Errorf("只允许只读 SELECT / WITH 查询")
	}
	if forbiddenSQLKeywordPattern.MatchString(upper) {
		return "", fmt.Errorf("检测到非只读 SQL 关键字，只允许只读查询")
	}

	return trimmed, nil
}

func hasMultipleStatements(query string) bool {
	inSingleQuote := false
	inDoubleQuote := false
	inLineComment := false
	inBlockComment := false

	for i := 0; i < len(query); i++ {
		ch := query[i]

		if inLineComment {
			if ch == '\n' {
				inLineComment = false
			}
			continue
		}
		if inBlockComment {
			if ch == '*' && i+1 < len(query) && query[i+1] == '/' {
				inBlockComment = false
				i++
			}
			continue
		}
		if inSingleQuote {
			if ch == '\'' {
				if i+1 < len(query) && query[i+1] == '\'' {
					i++
					continue
				}
				inSingleQuote = false
			}
			continue
		}
		if inDoubleQuote {
			if ch == '"' {
				if i+1 < len(query) && query[i+1] == '"' {
					i++
					continue
				}
				inDoubleQuote = false
			}
			continue
		}

		if ch == '-' && i+1 < len(query) && query[i+1] == '-' {
			inLineComment = true
			i++
			continue
		}
		if ch == '/' && i+1 < len(query) && query[i+1] == '*' {
			inBlockComment = true
			i++
			continue
		}
		if ch == '\'' {
			inSingleQuote = true
			continue
		}
		if ch == '"' {
			inDoubleQuote = true
			continue
		}
		if ch == ';' {
			return true
		}
	}

	return false
}

func stripSQLStringsAndComments(query string) string {
	var b strings.Builder
	inSingleQuote := false
	inDoubleQuote := false
	inLineComment := false
	inBlockComment := false

	for i := 0; i < len(query); i++ {
		ch := query[i]

		if inLineComment {
			if ch == '\n' {
				inLineComment = false
				b.WriteByte(' ')
			}
			continue
		}
		if inBlockComment {
			if ch == '*' && i+1 < len(query) && query[i+1] == '/' {
				inBlockComment = false
				i++
				b.WriteByte(' ')
			}
			continue
		}
		if inSingleQuote {
			if ch == '\'' {
				if i+1 < len(query) && query[i+1] == '\'' {
					i++
					continue
				}
				inSingleQuote = false
				b.WriteByte(' ')
			}
			continue
		}
		if inDoubleQuote {
			if ch == '"' {
				if i+1 < len(query) && query[i+1] == '"' {
					i++
					continue
				}
				inDoubleQuote = false
				b.WriteByte(' ')
			}
			continue
		}

		if ch == '-' && i+1 < len(query) && query[i+1] == '-' {
			inLineComment = true
			i++
			continue
		}
		if ch == '/' && i+1 < len(query) && query[i+1] == '*' {
			inBlockComment = true
			i++
			continue
		}
		if ch == '\'' {
			inSingleQuote = true
			b.WriteByte(' ')
			continue
		}
		if ch == '"' {
			inDoubleQuote = true
			b.WriteByte(' ')
			continue
		}
		b.WriteByte(ch)
	}

	return b.String()
}

func errorsIsDeadline(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "deadline exceeded") || strings.Contains(strings.ToLower(err.Error()), "interrupted")
}
