package data

import (
	"database/sql"
	"fmt"
	"strings"
)

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

		schema.Columns = append(schema.Columns, colInfo)
	}

	return schema, nil
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
	// 安全检查：只允许 SELECT 查询
	trimmed := strings.TrimSpace(strings.ToUpper(query))
	if !strings.HasPrefix(trimmed, "SELECT") && !strings.HasPrefix(trimmed, "WITH") {
		return nil, fmt.Errorf("只允许 SELECT 查询")
	}

	// 自动添加 LIMIT (如果没有的话)
	if !strings.Contains(strings.ToUpper(query), "LIMIT") {
		query = query + " LIMIT 200"
	}

	rows, err := db.Query(query)
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
	}

	return result, nil
}
