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
		return &LoadDataTool{
			Ingester:         ctx.Ingester,
			FileMaterializer: ctx.FileMaterializer,
		}
	})
	RegisterGlobalTool(func(ctx ToolContext) Tool {
		return &ListTablesTool{Ingester: ctx.Ingester}
	})
	RegisterGlobalTool(func(ctx ToolContext) Tool {
		return &DescribeDataTool{Ingester: ctx.Ingester}
	})
	RegisterGlobalTool(func(ctx ToolContext) Tool {
		return &QueryDataTool{Ingester: ctx.Ingester}
	})
	RegisterGlobalTool(func(ctx ToolContext) Tool {
		if ctx.FileFactsProvider == nil {
			return nil
		}
		return &InspectSessionFilesTool{Provider: ctx.FileFactsProvider}
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
	return "加载用户上传的 CSV 或 Excel 文件到内部数据库，并返回表名、行数和列数。"
}

func (t *InspectSessionFilesTool) Name() string { return "state_session_files_inspect" }
func (t *InspectSessionFilesTool) Description() string {
	return "读取当前会话上传文件的事实状态。返回文件标识、文件名、推断表名和可用的 schema 摘要；不修改任何状态。当任务依赖已上传文件或需要确认可分析的数据对象时可调用。"
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
		"ui_summary": fmt.Sprintf("当前会话共有 %d 个上传文件。", len(files)),
	}
	return toolSuccess("state_session_files_inspect", payload), nil
}
func (t *LoadDataTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"file_id": {"type": "string", "description": "上传文件的唯一标识"}
		},
		"required": ["file_id"]
	}`)
}

func (t *LoadDataTool) Execute(args json.RawMessage) (string, error) {
	var params struct {
		FileID string `json:"file_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}
	if t.FileMaterializer == nil {
		return "", fmt.Errorf("文件物化器未配置")
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
		"ui_summary":   fmt.Sprintf("数据已成功导入到表 %s（%d 行，%d 列）", tableName, rowCount, colCount),
	}), nil
}

// ListTablesTool 列出所有已导入的表
type ListTablesTool struct {
	Ingester *data.Ingester
}

func (t *ListTablesTool) Name() string { return "data_list_tables" }
func (t *ListTablesTool) Description() string {
	return "返回当前数据库中的已导入表。"
}
func (t *ListTablesTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type": "object", "properties": {}}`)
}

func (t *ListTablesTool) Execute(args json.RawMessage) (string, error) {
	db := t.Ingester.GetDB()
	if db == nil {
		return "", fmt.Errorf("数据库未初始化")
	}

	rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
	if err != nil {
		return "", fmt.Errorf("查询表列表失败: %w", err)
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
			"ui_summary":  "当前没有已导入的数据表",
		}), nil
	}

	return toolSuccess("data_list_tables", map[string]interface{}{
		"table_count": len(tables),
		"tables":      tables,
		"empty":       false,
		"ui_summary":  fmt.Sprintf("已导入 %d 张表", len(tables)),
	}), nil
}

// DescribeDataTool 获取数据 Schema 和统计摘要
type DescribeDataTool struct {
	Ingester *data.Ingester
}

func (t *DescribeDataTool) Name() string { return "data_describe_table" }
func (t *DescribeDataTool) Description() string {
	return "返回指定表的 schema 和统计摘要，包括列信息、行数、基础统计、采样值以及可观察到的潜在口径歧义候选。"
}
func (t *DescribeDataTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"table_name": {"type": "string", "description": "表名"}
		},
		"required": ["table_name"]
	}`)
}

func (t *DescribeDataTool) Execute(args json.RawMessage) (string, error) {
	var params struct {
		TableName string `json:"table_name"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}

	db := t.Ingester.GetDB()
	if db == nil {
		return "", fmt.Errorf("数据库未初始化")
	}

	schema, err := data.ExtractSchema(db, params.TableName)
	if err != nil {
		return toolFailure("data_describe_table", "schema_lookup_failed", "读取表结构失败", map[string]interface{}{
			"table_name": params.TableName,
			"detail":     err.Error(),
		}), nil
	}
	ambiguousMetricGroups := inferAmbiguousMetricGroups(schema.Columns)
	uiSummary := fmt.Sprintf("表 %s 已完成 schema 分析，共 %d 列、%d 行", schema.TableName, len(schema.Columns), schema.RowCount)
	if len(ambiguousMetricGroups) > 0 {
		uiSummary += fmt.Sprintf("；发现 %d 组可能影响口径的指标候选", len(ambiguousMetricGroups))
	}
	primaryTimeColumn := choosePrimaryTimeColumn(schema.TimeColumns)
	if primaryTimeColumn != nil && primaryTimeColumn.CoverageStart != "" && primaryTimeColumn.CoverageEnd != "" {
		uiSummary += fmt.Sprintf("；时间字段 %s 为 %s 粒度，覆盖 %s 到 %s", primaryTimeColumn.Name, primaryTimeColumn.Grain, primaryTimeColumn.CoverageStart, primaryTimeColumn.CoverageEnd)
	}

	return toolSuccess("data_describe_table", map[string]interface{}{
		"table_name":                   schema.TableName,
		"row_count":                    schema.RowCount,
		"column_count":                 len(schema.Columns),
		"schema":                       schema,
		"time_column_count":            len(schema.TimeColumns),
		"time_columns":                 schema.TimeColumns,
		"primary_time_column":          primaryTimeColumn,
		"ambiguous_metric_group_count": len(ambiguousMetricGroups),
		"ambiguous_metric_groups":      ambiguousMetricGroups,
		"ui_summary":                   uiSummary,
	}), nil
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
	return "执行单条只读 SQL 查询。仅允许 SELECT 或 WITH，结果最多返回 200 行。"
}
func (t *QueryDataTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"sql": {"type": "string", "description": "要执行的 SQL SELECT 查询语句"}
		},
		"required": ["sql"]
	}`)
}

func (t *QueryDataTool) Execute(args json.RawMessage) (string, error) {
	var params struct {
		SQL string `json:"sql"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}

	db := t.Ingester.GetDB()
	if db == nil {
		return "", fmt.Errorf("数据库未初始化")
	}

	rows, err := data.ExecuteQuery(db, params.SQL)
	if err != nil {
		return toolFailure("data_query_sql", "query_failed", "SQL 执行失败", map[string]interface{}{
			"sql":    params.SQL,
			"detail": err.Error(),
		}), nil
	}

	return toolSuccess("data_query_sql", map[string]interface{}{
		"sql":        params.SQL,
		"row_count":  len(rows),
		"columns":    queryResultColumns(rows),
		"rows":       rows,
		"ui_summary": fmt.Sprintf("SQL 查询成功，返回 %d 行", len(rows)),
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
