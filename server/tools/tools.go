package tools

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/data"
)

type FileReference struct {
	FileID      string `json:"fileId"`
	DisplayName string `json:"displayName"`
	StoredPath  string `json:"storedPath"`
}

type FileMaterializer func(fileID string) (*FileReference, error)

type ReportState struct {
	Sections    []ReportSection `json:"sections"`
	Charts      []ChartData     `json:"charts"`
	FinalTitle  string          `json:"finalTitle,omitempty"`
	FinalAuthor string          `json:"finalAuthor,omitempty"`
}

// LoadDataTool 加载数据文件到 SQLite
type LoadDataTool struct {
	Ingester         *data.Ingester
	FileMaterializer FileMaterializer
}

func (t *LoadDataTool) Name() string { return "load_data" }
func (t *LoadDataTool) Description() string {
	return "加载用户上传的数据文件 (CSV/Excel) 到内部数据库，返回行数、列数和表名。大数据文件也可以处理。"
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

	return toolSuccess("load_data", map[string]interface{}{
		"file_id":      fileRef.FileID,
		"display_name": fileRef.DisplayName,
		"table_name":   tableName,
		"row_count":    rowCount,
		"column_count": colCount,
		"summary_text": fmt.Sprintf("数据已成功导入到表 %s（%d 行，%d 列）", tableName, rowCount, colCount),
		"next_action":  fmt.Sprintf("先调用 describe_data 查看 %s 的结构，再按需使用 query_data", tableName),
	}), nil
}

// ListTablesTool 列出所有已导入的表
type ListTablesTool struct {
	Ingester *data.Ingester
}

func (t *ListTablesTool) Name() string { return "list_tables" }
func (t *ListTablesTool) Description() string {
	return "列出当前数据库中所有已导入的数据表名称和基本信息。"
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
		return toolSuccess("list_tables", map[string]interface{}{
			"table_count":  0,
			"tables":       []string{},
			"empty":        true,
			"next_action":  "先调用 load_data 导入文件，再继续 describe_data 或 query_data",
			"summary_text": "当前没有已导入的数据表",
		}), nil
	}

	return toolSuccess("list_tables", map[string]interface{}{
		"table_count":  len(tables),
		"tables":       tables,
		"empty":        false,
		"summary_text": fmt.Sprintf("已导入 %d 张表", len(tables)),
	}), nil
}

// DescribeDataTool 获取数据 Schema 和统计摘要
type DescribeDataTool struct {
	Ingester *data.Ingester
}

func (t *DescribeDataTool) Name() string { return "describe_data" }
func (t *DescribeDataTool) Description() string {
	return "获取指定数据表的 Schema 元信息和统计摘要，包括列名、数据类型、非空率、唯一值数、数值列的min/max/avg、以及采样值。对于大数据集，这比直接查看数据更高效。"
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
		return toolFailure("describe_data", "schema_lookup_failed", "读取表结构失败", map[string]interface{}{
			"table_name":  params.TableName,
			"detail":      err.Error(),
			"next_action": "检查 table_name 是否正确，必要时先调用 list_tables",
		}), nil
	}

	return toolSuccess("describe_data", map[string]interface{}{
		"table_name":   schema.TableName,
		"row_count":    schema.RowCount,
		"column_count": len(schema.Columns),
		"schema":       schema,
		"summary_text": fmt.Sprintf("表 %s 已完成 schema 分析，共 %d 列、%d 行", schema.TableName, len(schema.Columns), schema.RowCount),
	}), nil
}

// QueryDataTool 执行 SQL 查询
type QueryDataTool struct {
	Ingester *data.Ingester
}

func (t *QueryDataTool) Name() string { return "query_data" }
func (t *QueryDataTool) Description() string {
	return "在数据库上执行单条只读 SQL 查询。仅允许 SELECT / WITH，强制只读、超时保护，结果最多 200 行。用于分析大数据集时通过 SQL 聚合查询获取所需信息。"
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
		return toolFailure("query_data", "query_failed", "SQL 执行失败", map[string]interface{}{
			"sql":         params.SQL,
			"detail":      err.Error(),
			"next_action": "根据错误信息修正 SQL，继续使用单条只读 SELECT/WITH",
		}), nil
	}

	return toolSuccess("query_data", map[string]interface{}{
		"sql":          params.SQL,
		"row_count":    len(rows),
		"columns":      queryResultColumns(rows),
		"rows":         rows,
		"summary_text": fmt.Sprintf("SQL 查询成功，返回 %d 行", len(rows)),
	}), nil
}

// WriteSectionTool 向研报写入章节
type WriteSectionTool struct {
	ReportState *ReportState
}

type ReportSection struct {
	Type    string `json:"type"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

func (t *WriteSectionTool) Name() string { return "write_section" }
func (t *WriteSectionTool) Description() string {
	return "向研究报告中追加一个章节。支持的章节类型: title(报告标题), summary(执行摘要), overview(数据概述), analysis(分析章节，含图表和解读), conclusion(结论与建议)。图表解读应写在 analysis 章节内，紧跟 {{chart:chart_id}} 之后，不要创建单独的图表说明章节。内容使用 Markdown 格式。"
}
func (t *WriteSectionTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"section_type": {"type": "string", "enum": ["title", "summary", "overview", "analysis", "conclusion"], "description": "章节类型"},
			"title": {"type": "string", "description": "章节标题"},
			"content": {"type": "string", "description": "章节内容 (Markdown 格式)"}
		},
		"required": ["section_type", "title", "content"]
	}`)
}

func (t *WriteSectionTool) Execute(args json.RawMessage) (string, error) {
	var params struct {
		SectionType string `json:"section_type"`
		Title       string `json:"title"`
		Content     string `json:"content"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}

	section := ReportSection{
		Type:    params.SectionType,
		Title:   params.Title,
		Content: params.Content,
	}

	t.ReportState.Sections = append(t.ReportState.Sections, section)
	return toolSuccess("write_section", map[string]interface{}{
		"section_type":  section.Type,
		"title":         section.Title,
		"content_chars": len([]rune(strings.TrimSpace(section.Content))),
		"section_count": len(t.ReportState.Sections),
		"summary_text":  fmt.Sprintf("已添加章节 [%s] %s", section.Type, section.Title),
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

// FinalizeReportTool 生成最终报告
type FinalizeReportTool struct {
	ReportState *ReportState
}

func (t *FinalizeReportTool) Name() string { return "finalize_report" }
func (t *FinalizeReportTool) Description() string {
	return "生成最终的完整研究报告 HTML 文件。会将之前通过 write_section 添加的所有章节和 create_chart 创建的图表汇总，生成带封面、目录、交互式图表的完整研报。"
}
func (t *FinalizeReportTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"report_title": {"type": "string", "description": "报告标题"},
			"author": {"type": "string", "description": "作者/分析师名称", "default": "AI 数据分析师"}
		},
		"required": ["report_title"]
	}`)
}

func (t *FinalizeReportTool) Execute(args json.RawMessage) (string, error) {
	var params struct {
		ReportTitle string `json:"report_title"`
		Author      string `json:"author"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}
	if params.Author == "" {
		params.Author = "AI 数据分析师"
	}

	t.ReportState.FinalTitle = params.ReportTitle
	t.ReportState.FinalAuthor = params.Author

	chartCount := len(t.ReportState.Charts)
	sectionCount := len(t.ReportState.Sections)
	return toolSuccess("finalize_report", map[string]interface{}{
		"report_title":  params.ReportTitle,
		"author":        params.Author,
		"section_count": sectionCount,
		"chart_count":   chartCount,
		"summary_text":  fmt.Sprintf("研究报告已生成完成（%d 个章节，%d 个交互式图表）", sectionCount, chartCount),
	}), nil
}
