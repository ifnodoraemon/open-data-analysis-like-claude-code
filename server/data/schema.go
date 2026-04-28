package data

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	QueryTimeoutQuick       = 5 * time.Second
	QueryTimeoutLarge       = 30 * time.Second
	queryTimeout            = QueryTimeoutQuick
	queryTimeoutLarge       = QueryTimeoutLarge
	largeTableThreshold     = 100000
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
	Estimated    bool            `json:"estimated"`
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

// ExtractSchema 提取表的 Schema 和统计摘要 (full table scan)
func ExtractSchema(db *sql.DB, tableName string) (*SchemaInfo, error) {
	return extractSchemaInternal(db, tableName, false)
}

// ExtractSchemaSampled 提取表的 Schema 和统计摘要 (bounded sample, max 10000 rows)
// Stats are marked as estimated. Row count is still exact (COUNT(*) is cheap).
func ExtractSchemaSampled(db *sql.DB, tableName string) (*SchemaInfo, error) {
	return extractSchemaInternal(db, tableName, true)
}

func extractSchemaInternal(db *sql.DB, tableName string, sampled bool) (*SchemaInfo, error) {
	if err := ValidateSQLIdent(tableName); err != nil {
		return nil, fmt.Errorf("invalid table name: %w", err)
	}
	schema := &SchemaInfo{TableName: tableName}

	// 获取行数 (always exact — COUNT(*) is a single scan)
	var rowCount int
	err := db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM \"%s\"", tableName)).Scan(&rowCount)
	if err != nil {
		return nil, fmt.Errorf("failed to get row count: %w", err)
	}
	schema.RowCount = rowCount

	// For sampled mode, create a temporary bounded sample view
	sampleTable := ""
	actuallySampled := false
	if sampled && rowCount > 10000 {
		sampleTable = fmt.Sprintf("_oda_sample_%s", sanitizeSampleTableName(tableName))
		_, _ = db.Exec(fmt.Sprintf("DROP VIEW IF EXISTS \"%s\"", sampleTable))
		_, err := db.Exec(fmt.Sprintf("CREATE TEMP VIEW \"%s\" AS SELECT * FROM \"%s\" LIMIT 10000", sampleTable, tableName))
		if err != nil {
			log.Printf("[Warning] Failed to create sample view, falling back to full table: %v", err)
			sampleTable = ""
		} else {
			actuallySampled = true
		}
	}
	queryTable := tableName
	if sampleTable != "" {
		queryTable = sampleTable
	}

	// 获取列信息
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(\"%s\")", tableName))
	if err != nil {
		return nil, fmt.Errorf("failed to get column info: %w", err)
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
		if err := ValidateSQLIdent(col); err != nil {
			continue
		}
		colInfo := ColumnInfo{Name: col, Type: "TEXT", Estimated: actuallySampled}

		// 非空率
		if sampled && sampleTable != "" {
			// Estimated from sample
			var sampleNonNull int
			var sampleTotal int
			db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM \"%s\" WHERE \"%s\" IS NOT NULL AND \"%s\" != ''", queryTable, col, col)).Scan(&sampleNonNull)
			db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM \"%s\"", queryTable)).Scan(&sampleTotal)
			if sampleTotal > 0 {
				colInfo.NonNullRate = float64(sampleNonNull) / float64(sampleTotal)
			}
		} else {
			var nonNullCount int
			db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM \"%s\" WHERE \"%s\" IS NOT NULL AND \"%s\" != ''", queryTable, col, col)).Scan(&nonNullCount)
			if rowCount > 0 {
				colInfo.NonNullRate = float64(nonNullCount) / float64(rowCount)
			}
		}

		// 唯一值数
		db.QueryRow(fmt.Sprintf("SELECT COUNT(DISTINCT \"%s\") FROM \"%s\" WHERE \"%s\" IS NOT NULL AND \"%s\" != ''", col, queryTable, col, col)).Scan(&colInfo.UniqueCount)

		observedValues, err := collectDistinctColumnValues(db, queryTable, col, schemaDistinctProbeRows)
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
			"SELECT MIN(CAST(\"%s\" AS REAL)), MAX(CAST(\"%s\" AS REAL)), AVG(CAST(\"%s\" AS REAL)) FROM \"%s\" WHERE TRIM(CAST(\"%s\" AS TEXT)) GLOB '-[0-9]*' OR TRIM(CAST(\"%s\" AS TEXT)) GLOB '[0-9]*'",
			col, col, col, queryTable, col, col)).Scan(&minVal, &maxVal, &avgVal)
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

	// Clean up sample view
	if sampleTable != "" {
		_, _ = db.Exec(fmt.Sprintf("DROP VIEW IF EXISTS \"%s\"", sampleTable))
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

// QueryTimeoutForDB determines the query timeout based on whether any table
// referenced in the query exceeds the large-table threshold.
// Uses sqlite_stat1 when available to avoid expensive COUNT(*).
func QueryTimeoutForDB(db *sql.DB, query string) time.Duration {
	tables := extractTableReferences(query)
	for _, t := range tables {
		if err := ValidateSQLIdent(t); err != nil {
			continue
		}
		// Try sqlite_stat1 first (populated by ANALYZE, no full scan needed)
		var statRows string
		err := db.QueryRow(`SELECT stat FROM sqlite_stat1 WHERE tbl = ? LIMIT 1`, t).Scan(&statRows)
		if err == nil && statRows != "" {
			// stat format: "1000000 500 10" — first number is row count
			parts := strings.Fields(statRows)
			if len(parts) > 0 {
				if rc, err := strconv.Atoi(parts[0]); err == nil && rc >= largeTableThreshold {
					return queryTimeoutLarge
				}
				continue
			}
		}
		// Fall back to COUNT(*) only if stat1 not available
		var rc int
		err = db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM \"%s\"", t)).Scan(&rc)
		if err == nil && rc >= largeTableThreshold {
			return queryTimeoutLarge
		}
	}
	return queryTimeout
}

// extractTableReferences does a heuristic extraction of table names from a SQL query.
func extractTableReferences(query string) []string {
	tables := extractTableReferencesFromQuery(strings.ToUpper(query), query)
	return tables
}

func extractTableReferencesFromQuery(upper, original string) []string {
	var tables []string
	keywords := []string{"FROM", "JOIN"}
	for _, keyword := range keywords {
		remaining := upper
		origRemaining := original
		for {
			idx := strings.Index(remaining, keyword)
			if idx == -1 {
				break
			}
			rest := strings.TrimSpace(origRemaining[idx+len(keyword):])
			if len(rest) > 0 {
				if rest[0] == '"' {
					end := strings.Index(rest[1:], "\"")
					if end > 0 {
						tables = append(tables, rest[1:end+1])
					}
				} else if rest[0] == '`' {
					end := strings.Index(rest[1:], "`")
					if end > 0 {
						tables = append(tables, rest[1:end+1])
					}
				} else {
					// Unquoted identifier: read until whitespace or comma or closing paren
					name := readUnquotedIdent(rest)
					if name != "" && !isSQLKeyword(name) {
						tables = append(tables, name)
					}
				}
			}
			remaining = remaining[idx+len(keyword):]
			origRemaining = origRemaining[idx+len(keyword):]
		}
	}
	return tables
}

func readUnquotedIdent(s string) string {
	var b strings.Builder
	for i, ch := range s {
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == ',' || ch == ')' || ch == '(' || ch == ';' {
			break
		}
		if i > 63 {
			break
		}
		b.WriteRune(ch)
	}
	return b.String()
}

func isSQLKeyword(s string) bool {
	switch strings.ToUpper(s) {
	case "SELECT", "WHERE", "GROUP", "ORDER", "HAVING", "LIMIT", "OFFSET",
		"AS", "ON", "AND", "OR", "NOT", "IN", "IS", "NULL",
		"LEFT", "RIGHT", "INNER", "OUTER", "CROSS", "FULL",
		"UNION", "INTERSECT", "EXCEPT", "WITH":
		return true
	}
	return false
}

func sanitizeSampleTableName(tableName string) string {
	name := strings.ToLower(tableName)
	name = invalidSchemaSQLIdent.ReplaceAllString(name, "_")
	if len(name) > 50 {
		name = name[:50]
	}
	return name
}

var invalidSchemaSQLIdent = regexp.MustCompile(`[^a-zA-Z0-9_]`)

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
	return ExecuteQueryWithTimeout(db, query, 0)
}

// ExecuteQueryWithTimeout 执行 SQL 查询，可指定超时。如果 timeout 为 0，自动检测：大表用 30s，否则 5s。
func ExecuteQueryWithTimeout(db *sql.DB, query string, timeout time.Duration) ([]map[string]interface{}, error) {
	normalizedQuery, err := normalizeReadOnlyQuery(query)
	if err != nil {
		return nil, err
	}

	if timeout == 0 {
		timeout = queryTimeout
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get database connection: %w", err)
	}
	defer func() {
		resetCtx, resetCancel := context.WithTimeout(context.Background(), time.Second)
		defer resetCancel()
		_, _ = conn.ExecContext(resetCtx, "PRAGMA query_only = OFF")
		_ = conn.Close()
	}()

	if _, err := conn.ExecContext(ctx, "PRAGMA query_only = ON"); err != nil {
		return nil, fmt.Errorf("failed to enable read-only query mode: %w", err)
	}

	wrappedQuery := fmt.Sprintf("SELECT * FROM (%s) AS _oda_query LIMIT %d", normalizedQuery, queryProbeRows)
	rows, err := conn.QueryContext(ctx, wrappedQuery)
	if err != nil {
		return nil, fmt.Errorf("SQL execution failed: %w", err)
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
			return nil, fmt.Errorf("query result exceeds %d row limit", queryRowLimit)
		}
	}

	if err := rows.Err(); err != nil {
		if errorsIsDeadline(err) || ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("SQL query timeout (>%ds)", int(timeout/time.Second))
		}
		return nil, fmt.Errorf("failed to read SQL results: %w", err)
	}
	return result, nil
}

func normalizeReadOnlyQuery(query string) (string, error) {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return "", fmt.Errorf("SQL cannot be empty")
	}

	trimmed = strings.TrimSuffix(trimmed, ";")
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" {
		return "", fmt.Errorf("SQL cannot be empty")
	}

	if hasMultipleStatements(trimmed) {
		return "", fmt.Errorf("only single SQL statement allowed")
	}

	inspection := stripSQLStringsAndComments(trimmed)
	upper := strings.ToUpper(strings.TrimSpace(inspection))
	if !strings.HasPrefix(upper, "SELECT") && !strings.HasPrefix(upper, "WITH") {
		return "", fmt.Errorf("only read-only SELECT / WITH queries allowed")
	}
	if forbiddenSQLKeywordPattern.MatchString(upper) {
		return "", fmt.Errorf("non-read-only SQL keyword detected, only read-only queries allowed")
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
