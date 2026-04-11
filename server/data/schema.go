package data

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	queryTimeout            = 5 * time.Second
	queryRowLimit           = 200
	queryProbeRows          = queryRowLimit + 1
	schemaDistinctProbeRows = 200
)

var forbiddenSQLKeywordPattern = regexp.MustCompile(`\b(INSERT|UPDATE|DELETE|ALTER|DROP|CREATE|ATTACH|DETACH|REINDEX|VACUUM|PRAGMA|REPLACE|MERGE|UPSERT|TRUNCATE)\b`)
var timeColumnNameHintPattern = regexp.MustCompile(`(^|_)(date|dt|day|week|month|year|quarter|period|ym|yyyymm|yyyymmdd)(_|$)`)
var (
	isoDatePattern      = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	slashDatePattern    = regexp.MustCompile(`^\d{4}/\d{2}/\d{2}$`)
	compactDatePattern  = regexp.MustCompile(`^\d{8}$`)
	isoMonthPattern     = regexp.MustCompile(`^\d{4}-\d{2}$`)
	slashMonthPattern   = regexp.MustCompile(`^\d{4}/\d{2}$`)
	compactMonthPattern = regexp.MustCompile(`^\d{6}$`)
	quarterPattern      = regexp.MustCompile(`^\d{4}[-/ ]?Q[1-4]$`)
	yearPattern         = regexp.MustCompile(`^\d{4}$`)
)

// SchemaInfo 表 Schema 信息
type SchemaInfo struct {
	TableName   string           `json:"tableName"`
	RowCount    int              `json:"rowCount"`
	Columns     []ColumnInfo     `json:"columns"`
	TimeColumns []TimeColumnInfo `json:"timeColumns,omitempty"`
}

// ColumnInfo 列信息
type ColumnInfo struct {
	Name         string          `json:"name"`
	Type         string          `json:"type"`
	NonNullRate  float64         `json:"nonNullRate"`
	UniqueCount  int             `json:"uniqueCount"`
	SampleValues []string        `json:"sampleValues"`
	TimeProfile  *TimeColumnInfo `json:"timeProfile,omitempty"`
	// 数值列统计
	Min    *float64 `json:"min,omitempty"`
	Max    *float64 `json:"max,omitempty"`
	Avg    *float64 `json:"avg,omitempty"`
	Median *float64 `json:"median,omitempty"`
}

type TimeColumnInfo struct {
	Name                string   `json:"name"`
	ValueKind           string   `json:"valueKind"`
	Grain               string   `json:"grain"`
	CoverageStart       string   `json:"coverageStart,omitempty"`
	CoverageEnd         string   `json:"coverageEnd,omitempty"`
	DistinctPeriodCount int      `json:"distinctPeriodCount"`
	SamplePeriods       []string `json:"samplePeriods,omitempty"`
	RollupGrains        []string `json:"rollupGrains,omitempty"`
}

type normalizedTimeValue struct {
	grain     string
	canonical string
	sortKey   string
}

// ExtractSchema 提取表的 Schema 和统计摘要
func ExtractSchema(db *sql.DB, tableName string) (*SchemaInfo, error) {
	if err := validateSQLIdent(tableName); err != nil {
		return nil, fmt.Errorf("invalid table name: %w", err)
	}
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
		if err := validateSQLIdent(col); err != nil {
			continue
		}
		colInfo := ColumnInfo{Name: col, Type: "TEXT"}

		// 非空率
		var nonNullCount int
		db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM \"%s\" WHERE \"%s\" IS NOT NULL AND \"%s\" != ''", tableName, col, col)).Scan(&nonNullCount)
		if rowCount > 0 {
			colInfo.NonNullRate = float64(nonNullCount) / float64(rowCount)
		}

		// 唯一值数
		db.QueryRow(fmt.Sprintf("SELECT COUNT(DISTINCT \"%s\") FROM \"%s\" WHERE \"%s\" IS NOT NULL AND \"%s\" != ''", col, tableName, col, col)).Scan(&colInfo.UniqueCount)

		observedValues, err := collectDistinctColumnValues(db, tableName, col, schemaDistinctProbeRows)
		if err == nil {
			if len(observedValues) > 5 {
				colInfo.SampleValues = append(colInfo.SampleValues, observedValues[:5]...)
			} else {
				colInfo.SampleValues = append(colInfo.SampleValues, observedValues...)
			}
			if timeInfo := detectTimeColumnInfo(col, observedValues, colInfo.UniqueCount); timeInfo != nil {
				colInfo.Type = "TIME"
				colInfo.TimeProfile = timeInfo
				schema.TimeColumns = append(schema.TimeColumns, *timeInfo)
				schema.Columns = append(schema.Columns, colInfo)
				continue
			}
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

func collectDistinctColumnValues(db *sql.DB, tableName, column string, limit int) ([]string, error) {
	rows, err := db.Query(fmt.Sprintf(
		"SELECT DISTINCT CAST(\"%s\" AS TEXT) FROM \"%s\" WHERE \"%s\" IS NOT NULL AND CAST(\"%s\" AS TEXT) != '' ORDER BY 1 LIMIT %d",
		column, tableName, column, column, limit,
	))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	values := make([]string, 0, limit)
	for rows.Next() {
		var value string
		if rows.Scan(&value) == nil {
			values = append(values, strings.TrimSpace(value))
		}
	}
	return values, rows.Err()
}

func detectTimeColumnInfo(columnName string, observedValues []string, uniqueCount int) *TimeColumnInfo {
	if len(observedValues) == 0 {
		return nil
	}

	nameHint := looksLikeTimeColumnName(columnName)
	matched := make([]normalizedTimeValue, 0, len(observedValues))
	grainCounts := make(map[string]int)
	for _, raw := range observedValues {
		value, ok := normalizeTimeValue(columnName, raw)
		if !ok {
			continue
		}
		matched = append(matched, value)
		grainCounts[value.grain]++
	}
	if len(matched) == 0 {
		return nil
	}

	dominantGrain := ""
	dominantCount := 0
	for grain, count := range grainCounts {
		if count > dominantCount {
			dominantGrain = grain
			dominantCount = count
		}
	}
	if dominantGrain == "" {
		return nil
	}

	matchRatio := float64(dominantCount) / float64(len(observedValues))
	if !nameHint && matchRatio < 0.8 {
		return nil
	}
	if nameHint && matchRatio < 0.6 {
		return nil
	}

	filtered := make([]normalizedTimeValue, 0, dominantCount)
	seen := make(map[string]struct{}, dominantCount)
	for _, item := range matched {
		if item.grain != dominantGrain {
			continue
		}
		if _, ok := seen[item.canonical]; ok {
			continue
		}
		seen[item.canonical] = struct{}{}
		filtered = append(filtered, item)
	}
	if len(filtered) == 0 {
		return nil
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].sortKey == filtered[j].sortKey {
			return filtered[i].canonical < filtered[j].canonical
		}
		return filtered[i].sortKey < filtered[j].sortKey
	})

	samplePeriods := make([]string, 0, minInt(5, len(filtered)))
	for _, item := range filtered {
		samplePeriods = append(samplePeriods, item.canonical)
		if len(samplePeriods) >= 5 {
			break
		}
	}

	distinctPeriods := uniqueCount
	if distinctPeriods <= 0 {
		distinctPeriods = len(filtered)
	}

	return &TimeColumnInfo{
		Name:                columnName,
		ValueKind:           dominantGrain,
		Grain:               dominantGrain,
		CoverageStart:       filtered[0].canonical,
		CoverageEnd:         filtered[len(filtered)-1].canonical,
		DistinctPeriodCount: distinctPeriods,
		SamplePeriods:       samplePeriods,
		RollupGrains:        rollupGrainsForTimeGrain(dominantGrain),
	}
}

func looksLikeTimeColumnName(columnName string) bool {
	normalized := strings.ToLower(strings.TrimSpace(columnName))
	normalized = strings.ReplaceAll(normalized, " ", "_")
	normalized = strings.ReplaceAll(normalized, "-", "_")
	return timeColumnNameHintPattern.MatchString(normalized)
}

func normalizeTimeValue(columnName, raw string) (normalizedTimeValue, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return normalizedTimeValue{}, false
	}

	switch {
	case isoDatePattern.MatchString(trimmed):
		return normalizedTimeValue{grain: "day", canonical: trimmed, sortKey: trimmed}, true
	case slashDatePattern.MatchString(trimmed):
		canonical := strings.ReplaceAll(trimmed, "/", "-")
		return normalizedTimeValue{grain: "day", canonical: canonical, sortKey: canonical}, true
	case compactDatePattern.MatchString(trimmed) && looksLikeTimeColumnName(columnName):
		canonical := fmt.Sprintf("%s-%s-%s", trimmed[:4], trimmed[4:6], trimmed[6:8])
		return normalizedTimeValue{grain: "day", canonical: canonical, sortKey: canonical}, true
	case isoMonthPattern.MatchString(trimmed):
		return normalizedTimeValue{grain: "month", canonical: trimmed, sortKey: trimmed}, true
	case slashMonthPattern.MatchString(trimmed):
		canonical := strings.ReplaceAll(trimmed, "/", "-")
		return normalizedTimeValue{grain: "month", canonical: canonical, sortKey: canonical}, true
	case compactMonthPattern.MatchString(trimmed) && looksLikeTimeColumnName(columnName):
		canonical := fmt.Sprintf("%s-%s", trimmed[:4], trimmed[4:6])
		return normalizedTimeValue{grain: "month", canonical: canonical, sortKey: canonical}, true
	case quarterPattern.MatchString(strings.ToUpper(trimmed)):
		upper := strings.ToUpper(trimmed)
		year := upper[:4]
		quarter := upper[len(upper)-1:]
		canonical := fmt.Sprintf("%s-Q%s", year, quarter)
		sortKey := fmt.Sprintf("%s-%s", year, quarter)
		return normalizedTimeValue{grain: "quarter", canonical: canonical, sortKey: sortKey}, true
	case yearPattern.MatchString(trimmed) && looksLikeTimeColumnName(columnName):
		year, err := strconv.Atoi(trimmed)
		if err != nil || year < 1900 || year > 2100 {
			return normalizedTimeValue{}, false
		}
		return normalizedTimeValue{grain: "year", canonical: trimmed, sortKey: trimmed}, true
	default:
		return normalizedTimeValue{}, false
	}
}

func rollupGrainsForTimeGrain(grain string) []string {
	switch grain {
	case "day":
		return []string{"month", "quarter", "year"}
	case "month":
		return []string{"quarter", "year"}
	case "quarter":
		return []string{"year"}
	default:
		return nil
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
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
