package tools

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/ifnodoraemon/openDataAnalysis/data"
)

type FileReference struct {
	FileID      string `json:"fileId"`
	DisplayName string `json:"displayName"`
	StoredPath  string `json:"storedPath"`
}

type SessionFileFact struct {
	FileID          string      `json:"file_id"`
	DisplayName     string      `json:"display_name"`
	TableName       string      `json:"table_name,omitempty"`
	SchemaSummary   interface{} `json:"schema_summary,omitempty"`
	SchemaAvailable bool        `json:"schema_available"`
}

type FileFactsProvider func() ([]SessionFileFact, error)

func init() {
	RegisterGlobalTool(func(ctx ToolContext) Tool {
		return &ListTablesTool{Ingester: ctx.Ingester}
	})
	RegisterGlobalTool(func(ctx ToolContext) Tool {
		return &DescribeDataTool{
			Ingester:                  ctx.Ingester,
			ConfirmedOverridesProvider: ctx.ConfirmedOverridesProvider,
		}
	})
	RegisterGlobalTool(func(ctx ToolContext) Tool {
		return &QueryDataTool{Ingester: ctx.Ingester}
	})
}

type FileMaterializer func(fileID string) (*FileReference, error)

type InspectSessionFilesTool struct {
	Provider FileFactsProvider
}

// LoadDataTool 加载数据文件到 SQLite
type LoadDataTool struct {
	Ingester         *data.Ingester
	FileMaterializer FileMaterializer
}

func (t *LoadDataTool) Name() string { return "data_load_file" }
func (t *LoadDataTool) Description() string {
	return "Import a user-uploaded CSV or Excel file into the internal SQLite database. Returns table_name, row_count, column_count. Side effect: creates a new table in the internal database; overwrites if a table with the same name already exists. Reads uploaded file list state; writes to database state. Failure conditions: file ID does not exist, unsupported file format, file content cannot be parsed. Limitations: only CSV and Excel formats are supported."
}

func (t *InspectSessionFilesTool) Name() string { return "state_session_files_inspect" }
func (t *InspectSessionFilesTool) Description() string {
	return "Read the fact state of uploaded files in the current session. Returns file ID, display name, inferred table name, and available schema summary. Does not modify any state."
}
func (t *InspectSessionFilesTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"required":[]}`)
}

func (t *InspectSessionFilesTool) Execute(args json.RawMessage) (string, error) {
	if t.Provider == nil {
		return "", fmt.Errorf("file facts provider is not initialized")
	}
	files, err := t.Provider()
	if err != nil {
		return "", err
	}
	payload := map[string]interface{}{
		"file_count": len(files),
		"files":      files,
		"ui_summary": fmt.Sprintf("session has %d uploaded files.", len(files)),
	}
	return toolSuccess("state_session_files_inspect", payload), nil
}
func (t *LoadDataTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"file_id": {"type": "string", "description": "Unique identifier of the uploaded file"}
		},
		"required": ["file_id"]
	}`)
}

func (t *LoadDataTool) Execute(args json.RawMessage) (string, error) {
	var params struct {
		FileID string `json:"file_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("failed to parse parameters: %w", err)
	}
	if t.FileMaterializer == nil {
		return "", fmt.Errorf("file materializer not configured")
	}

	fileRef, err := t.FileMaterializer(params.FileID)
	if err != nil {
		return "", err
	}

	tableName, rowCount, colCount, err := t.Ingester.ImportFile(fileRef.StoredPath)
	if err != nil {
		return "", err
	}

	return toolSuccess("data_load_file", map[string]interface{}{
		"file_id":      fileRef.FileID,
		"display_name": fileRef.DisplayName,
		"table_name":   tableName,
		"row_count":    rowCount,
		"column_count": colCount,
		"ui_summary":   fmt.Sprintf("data imported to table %s (%d rows, %d columns)", tableName, rowCount, colCount),
	}), nil
}

// ListTablesTool 列出所有已导入的表
type ListTablesTool struct {
	Ingester *data.Ingester
}

func (t *ListTablesTool) Name() string { return "data_list_tables" }
func (t *ListTablesTool) Description() string {
	return "Return a list of all imported table names in the internal database. Returns table_count, tables list, and empty flag. Does not modify any state. Failure conditions: database not initialized."
}
func (t *ListTablesTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type": "object", "properties": {}}`)
}

func (t *ListTablesTool) Execute(args json.RawMessage) (string, error) {
	db := t.Ingester.GetDB()
	if db == nil {
		return "", fmt.Errorf("database not initialized")
	}

	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
	if err != nil {
		return "", fmt.Errorf("failed to query table list: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if rows.Scan(&name) == nil {
			tables = append(tables, name)
		}
	}

	if len(tables) == 0 {
		return toolSuccess("data_list_tables", map[string]interface{}{
			"table_count": 0,
			"tables":      []string{},
			"empty":       true,
			"ui_summary":  "no imported data tables yet",
		}), nil
	}

	return toolSuccess("data_list_tables", map[string]interface{}{
		"table_count": len(tables),
		"tables":      tables,
		"empty":       false,
		"ui_summary":  fmt.Sprintf("%d tables imported", len(tables)),
	}), nil
}

type ConfirmedOverridesProvider func(tableName string) map[string]interface{}

// DescribeDataTool 获取数据 Schema 和统计摘要
type DescribeDataTool struct {
	Ingester                 *data.Ingester
	ConfirmedOverridesProvider ConfirmedOverridesProvider
}

func (t *DescribeDataTool) Name() string { return "data_describe_table" }
func (t *DescribeDataTool) Description() string {
	return "Return the schema and statistical summary for a specified table, including column info, row count, basic statistics, sample values, and confirmed overrides. Does not include unconfirmed semantic candidates."
}
func (t *DescribeDataTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"		table_name": {"type": "string", "description": "Table name"}
		},
		"required": ["table_name"]
	}`)
}

func (t *DescribeDataTool) Execute(args json.RawMessage) (string, error) {
	var params struct {
		TableName string `json:"table_name"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("failed to parse parameters: %w", err)
	}

	db := t.Ingester.GetDB()
	if db == nil {
		return "", fmt.Errorf("database not initialized")
	}

	if err := data.ValidateSQLIdent(params.TableName); err != nil {
		return toolFailure("data_describe_table", "invalid_table_name", err.Error(), map[string]interface{}{
			"table_name": params.TableName,
		}), nil
	}

	var rowCount int
	db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM \"%s\"", params.TableName)).Scan(&rowCount)

	var schema *data.SchemaInfo
	var err error
	if rowCount > 10000 {
		schema, err = data.ExtractSchemaSampled(db, params.TableName)
	} else {
		schema, err = data.ExtractSchema(db, params.TableName)
	}
	if err != nil {
		return toolFailure("data_describe_table", "schema_lookup_failed", "failed to read table structure", map[string]interface{}{
			"table_name": params.TableName,
			"detail":     err.Error(),
		}), nil
	}
	ambiguousMetricGroups := inferAmbiguousMetricGroups(schema.Columns)
	uiSummary := fmt.Sprintf("table %s schema analysis complete, %d columns, %d rows", schema.TableName, len(schema.Columns), schema.RowCount)
	if len(ambiguousMetricGroups) > 0 {
		uiSummary += fmt.Sprintf("; %d ambiguous metric candidate groups found", len(ambiguousMetricGroups))
	}
	primaryTimeColumn := choosePrimaryTimeColumn(schema.TimeColumns)
	if primaryTimeColumn != nil && primaryTimeColumn.CoverageStart != "" && primaryTimeColumn.CoverageEnd != "" {
		uiSummary += fmt.Sprintf("; time field %s is %s grain, covering %s to %s", primaryTimeColumn.Name, primaryTimeColumn.Grain, primaryTimeColumn.CoverageStart, primaryTimeColumn.CoverageEnd)
	}

	result := map[string]interface{}{
		"table_name":                   schema.TableName,
		"row_count":                    schema.RowCount,
		"column_count":                 len(schema.Columns),
		"schema":                       schema,
		"time_column_count":            len(schema.TimeColumns),
		"time_columns":                 schema.TimeColumns,
		"primary_time_column":          primaryTimeColumn,
		"ambiguous_metric_group_count": len(ambiguousMetricGroups),
		"ambiguous_metric_groups":      ambiguousMetricGroups,
		"note":                         "time_columns and ambiguous_metric_groups are inferred candidates, not confirmed facts; check confirmed_overrides for user-validated selections",
		"ui_summary":                   uiSummary,
	}

	if t.ConfirmedOverridesProvider != nil {
		if overrides := t.ConfirmedOverridesProvider(params.TableName); len(overrides) > 0 {
			result["confirmed_overrides"] = overrides
		}
	}

	return toolSuccess("data_describe_table", result), nil
}

func choosePrimaryTimeColumn(columns []data.TimeColumnInfo) *data.TimeColumnInfo {
	if len(columns) == 0 {
		return nil
	}
	best := columns[0]
	for _, item := range columns[1:] {
		if item.DistinctPeriodCount > best.DistinctPeriodCount {
			best = item
			continue
		}
		if item.DistinctPeriodCount == best.DistinctPeriodCount && strings.TrimSpace(item.Name) < strings.TrimSpace(best.Name) {
			best = item
		}
	}
	return &best
}

var metricQualifierTokens = map[string]struct{}{
	"actual":      {},
	"adjusted":    {},
	"booked":      {},
	"confirmed":   {},
	"estimated":   {},
	"est":         {},
	"final":       {},
	"forecast":    {},
	"gross":       {},
	"net":         {},
	"planned":     {},
	"plan":        {},
	"projected":   {},
	"raw":         {},
	"recognized":  {},
	"target":      {},
	"tentative":   {},
	"unconfirmed": {},
}

func inferAmbiguousMetricGroups(columns []data.ColumnInfo) map[string][]string {
	grouped := make(map[string][]string)
	for _, column := range columns {
		if !strings.EqualFold(strings.TrimSpace(column.Type), "NUMERIC") {
			continue
		}
		tokens := tokenizeColumnName(column.Name)
		if len(tokens) < 2 {
			continue
		}
		core := make([]string, 0, len(tokens))
		qualifierCount := 0
		for _, token := range tokens {
			if _, ok := metricQualifierTokens[token]; ok {
				qualifierCount++
				continue
			}
			core = append(core, token)
		}
		if qualifierCount == 0 || len(core) == 0 {
			continue
		}
		key := strings.Join(core, "_")
		grouped[key] = append(grouped[key], column.Name)
	}

	result := make(map[string][]string)
	for key, names := range grouped {
		if len(names) < 2 {
			continue
		}
		sort.Strings(names)
		result[key] = names
	}
	return result
}

func tokenizeColumnName(name string) []string {
	return strings.FieldsFunc(strings.ToLower(strings.TrimSpace(name)), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	})
}

// QueryDataTool 执行 SQL 查询
type QueryDataTool struct {
	Ingester *data.Ingester
}

func (t *QueryDataTool) Name() string { return "data_query_sql" }
func (t *QueryDataTool) Description() string {
	return "Execute a single read-only SQL query on the internal database. Only SELECT or WITH statements are allowed; INSERT/UPDATE/DELETE/DDL are forbidden. Side effects: none (read-only). Returns sql, row_count, columns, and rows. Maximum 200 rows returned; queries exceeding this must add LIMIT. Failure conditions: SQL syntax error, reference to nonexistent table or column, execution timeout. Limitations: only SQLite dialect supported. Large tables (>100K rows) get a 30s timeout; others get 5s."
}
func (t *QueryDataTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"sql": {"type": "string", "description": "The SQL SELECT query to execute"}
		},
		"required": ["sql"]
	}`)
}

func (t *QueryDataTool) Execute(args json.RawMessage) (string, error) {
	var params struct {
		SQL string `json:"sql"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("failed to parse parameters: %w", err)
	}

	db := t.Ingester.GetDB()
	if db == nil {
		return "", fmt.Errorf("database not initialized")
	}

	timeout := data.QueryTimeoutForDB(db, params.SQL)
	rows, err := data.ExecuteQueryWithTimeout(db, params.SQL, timeout)
	if err != nil {
		return toolFailure("data_query_sql", "query_failed", "SQL execution failed", map[string]interface{}{
			"sql":    params.SQL,
			"detail": err.Error(),
		}), nil
	}

	return toolSuccess("data_query_sql", map[string]interface{}{
		"sql":        params.SQL,
		"row_count":  len(rows),
		"columns":    queryResultColumns(rows),
		"rows":       rows,
		"ui_summary": fmt.Sprintf("SQL query succeeded, %d rows returned", len(rows)),
	}), nil
}

func queryResultColumns(rows []map[string]interface{}) []string {
	if len(rows) == 0 {
		return []string{}
	}

	columns := make([]string, 0, len(rows[0]))
	for name := range rows[0] {
		columns = append(columns, name)
	}
	sort.Strings(columns)
	return columns
}
