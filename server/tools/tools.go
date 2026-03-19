package tools

import (
	"encoding/json"
	"fmt"
	"regexp"
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

type ReportState struct {
	Blocks        []ReportBlock `json:"blocks"`
	Charts        []ChartData   `json:"charts"`
	FinalTitle    string        `json:"finalTitle,omitempty"`
	FinalAuthor   string        `json:"finalAuthor,omitempty"`
	Layout        ReportLayout  `json:"layout,omitempty"`
	NeedsFinalize bool          `json:"needsFinalize,omitempty"`
}

type ReportBlock struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Title   string `json:"title,omitempty"`
	Content string `json:"content,omitempty"`
	ChartID string `json:"chartId,omitempty"`
}

type ReportLayout struct {
	CustomHTMLShell string `json:"customHtmlShell,omitempty"`
	CustomCSS       string `json:"customCss,omitempty"`
	CustomJS        string `json:"customJs,omitempty"`
	BodyClass       string `json:"bodyClass,omitempty"`
	HideCover       bool   `json:"hideCover,omitempty"`
	HideTOC         bool   `json:"hideToc,omitempty"`
}

type ReportEditState struct {
	Mode                string              `json:"mode,omitempty"`
	TargetRunID         string              `json:"targetRunId,omitempty"`
	TargetBlockID       string              `json:"targetBlockId,omitempty"`
	SelectionText       string              `json:"selectionText,omitempty"`
	PreserveOtherBlocks bool                `json:"preserveOtherBlocks,omitempty"`
	AllowedChartIDs     map[string]struct{} `json:"-"`
}

type ReportDeliveryState struct {
	HasContent    bool   `json:"has_content"`
	IsFinalized   bool   `json:"is_finalized"`
	NeedsFinalize bool   `json:"needs_finalize"`
	DeliveryState string `json:"delivery_state"`
	BlockCount    int    `json:"block_count"`
	ChartCount    int    `json:"chart_count"`
	FinalTitle    string `json:"final_title,omitempty"`
	FinalAuthor   string `json:"final_author,omitempty"`
}

func (s *ReportEditState) Reset() {
	if s == nil {
		return
	}
	s.Mode = ""
	s.TargetRunID = ""
	s.TargetBlockID = ""
	s.SelectionText = ""
	s.PreserveOtherBlocks = false
	s.AllowedChartIDs = nil
}

func (s *ReportEditState) Active() bool {
	return s != nil && strings.TrimSpace(s.Mode) != ""
}

func (s *ReportEditState) RefreshFromReportState(state *ReportState) {
	if s == nil {
		return
	}
	s.AllowedChartIDs = collectEditableChartIDs(state, s.TargetBlockID)
}

func (s *ReportEditState) BlockMutationAllowed(action, blockID string) bool {
	if !s.Active() || !s.PreserveOtherBlocks {
		return true
	}
	target := strings.TrimSpace(s.TargetBlockID)
	id := strings.TrimSpace(blockID)
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "upsert":
		return target != "" && id == target
	default:
		return false
	}
}

func (s *ReportEditState) ChartMutationAllowed(chartID string) bool {
	if !s.Active() || !s.PreserveOtherBlocks {
		return true
	}
	_, ok := s.AllowedChartIDs[strings.TrimSpace(chartID)]
	return ok
}

func (s *ReportEditState) Snapshot() map[string]interface{} {
	if s == nil {
		return map[string]interface{}{}
	}
	charts := make([]string, 0, len(s.AllowedChartIDs))
	for chartID := range s.AllowedChartIDs {
		charts = append(charts, chartID)
	}
	sort.Strings(charts)
	return map[string]interface{}{
		"mode":                  s.Mode,
		"target_run_id":         s.TargetRunID,
		"target_block_id":       s.TargetBlockID,
		"selection_text":        s.SelectionText,
		"preserve_other_blocks": s.PreserveOtherBlocks,
		"allowed_chart_ids":     charts,
		"active":                s.Active(),
	}
}

func collectEditableChartIDs(state *ReportState, blockID string) map[string]struct{} {
	refs := make(map[string]struct{})
	if state == nil || strings.TrimSpace(blockID) == "" {
		return refs
	}
	index := findReportBlockIndex(state.Blocks, strings.TrimSpace(blockID))
	if index < 0 {
		return refs
	}
	block := state.Blocks[index]
	if strings.TrimSpace(block.ChartID) != "" {
		refs[strings.TrimSpace(block.ChartID)] = struct{}{}
	}
	for _, ref := range chartRefsOutsideChartBlock(block.Content) {
		refs[ref] = struct{}{}
	}
	return refs
}

func DescribeReportDeliveryState(state *ReportState) ReportDeliveryState {
	delivery := ReportDeliveryState{
		DeliveryState: "empty",
	}
	if state == nil {
		return delivery
	}
	delivery.BlockCount = len(state.Blocks)
	delivery.ChartCount = len(state.Charts)
	delivery.FinalTitle = strings.TrimSpace(state.FinalTitle)
	delivery.FinalAuthor = strings.TrimSpace(state.FinalAuthor)
	delivery.HasContent = delivery.BlockCount > 0 || delivery.ChartCount > 0
	delivery.NeedsFinalize = state.NeedsFinalize
	delivery.IsFinalized = delivery.HasContent && !state.NeedsFinalize && delivery.FinalTitle != ""
	if delivery.HasContent {
		if delivery.IsFinalized {
			delivery.DeliveryState = "finalized"
		} else {
			delivery.DeliveryState = "draft"
		}
	}
	return delivery
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

func init() {
	RegisterGlobalTool(func(ctx ToolContext) Tool {
		return &ManageReportBlocksTool{ReportState: ctx.ReportState, EditState: ctx.EditState}
	})
	RegisterGlobalTool(func(ctx ToolContext) Tool {
		return &ConfigureReportTool{ReportState: ctx.ReportState}
	})
	RegisterGlobalTool(func(ctx ToolContext) Tool {
		return &FinalizeReportTool{
			ReportState: ctx.ReportState,
			Subgoals:    ctx.Subgoals,
		}
	})
}

type ConfigureReportTool struct {
	ReportState *ReportState
}

type ManageReportBlocksTool struct {
	ReportState *ReportState
	EditState   *ReportEditState
}

func (t *ConfigureReportTool) Name() string { return "report_configure_layout" }
func (t *ConfigureReportTool) Description() string {
	return "读取并修改报告布局配置。可用于更新或重置 HTML 壳、CSS、JS 和封面/目录显示选项；会修改 report layout 状态，但不会直接修改 block 或 chart。执行后若当前报告已有内容，delivery_state 仍会保持 draft，只有 report_finalize 才会把当前报告变成最终可交付状态。"
}
func (t *ConfigureReportTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {"type": "string", "enum": ["merge", "reset"], "description": "merge（默认）或 reset。"},
			"custom_html_shell": {"type": "string", "description": "可选。完整 HTML 壳子模板，支持 {{title}} {{author}} {{date}} {{toc}} {{content}} {{chart_scripts}} {{custom_css}} {{custom_js}} {{body_class}} 占位符。"},
			"custom_css": {"type": "string", "description": "追加到页面中的自定义 CSS。"},
			"custom_js": {"type": "string", "description": "追加到页面底部的自定义 JS。"},
			"body_class": {"type": "string", "description": "附加到 body 的 class。"},
			"hide_cover": {"type": "boolean", "description": "是否隐藏默认封面。"},
			"hide_toc": {"type": "boolean", "description": "是否隐藏默认目录。"}
		},
		"required": []
	}`)
}

func (t *ConfigureReportTool) Execute(args json.RawMessage) (string, error) {
	var params struct {
		Action          string `json:"action"`
		CustomHTMLShell string `json:"custom_html_shell"`
		CustomCSS       string `json:"custom_css"`
		CustomJS        string `json:"custom_js"`
		BodyClass       string `json:"body_class"`
		HideCover       *bool  `json:"hide_cover"`
		HideTOC         *bool  `json:"hide_toc"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}
	action := strings.TrimSpace(params.Action)
	if action == "" {
		action = "merge"
	}
	if t.ReportState == nil {
		return "", fmt.Errorf("report state is not initialized")
	}

	switch action {
	case "reset":
		t.ReportState.Layout = ReportLayout{}
		t.ReportState.NeedsFinalize = true
		delivery := DescribeReportDeliveryState(t.ReportState)
		return toolSuccess("report_configure_layout", map[string]interface{}{
			"action":                         action,
			"delivery_state":                 delivery.DeliveryState,
			"is_finalized":                   delivery.IsFinalized,
			"needs_finalize":                 delivery.NeedsFinalize,
			"requires_finalize_for_delivery": delivery.HasContent,
			"message":                        "当前报告仍处于草稿态，尚未生成最终报告文件。",
			"ui_summary":                     "已恢复默认报告模板，当前仍是报告草稿",
		}), nil
	case "merge":
		if params.CustomHTMLShell != "" {
			t.ReportState.Layout.CustomHTMLShell = params.CustomHTMLShell
		}
		if params.CustomCSS != "" {
			t.ReportState.Layout.CustomCSS = params.CustomCSS
		}
		if params.CustomJS != "" {
			t.ReportState.Layout.CustomJS = params.CustomJS
		}
		if params.BodyClass != "" {
			t.ReportState.Layout.BodyClass = strings.TrimSpace(params.BodyClass)
		}
		if params.HideCover != nil {
			t.ReportState.Layout.HideCover = *params.HideCover
		}
		if params.HideTOC != nil {
			t.ReportState.Layout.HideTOC = *params.HideTOC
		}
		t.ReportState.NeedsFinalize = true
		delivery := DescribeReportDeliveryState(t.ReportState)
		return toolSuccess("report_configure_layout", map[string]interface{}{
			"action":                         action,
			"has_custom_shell":               t.ReportState.Layout.CustomHTMLShell != "",
			"has_custom_css":                 t.ReportState.Layout.CustomCSS != "",
			"has_custom_js":                  t.ReportState.Layout.CustomJS != "",
			"body_class":                     t.ReportState.Layout.BodyClass,
			"hide_cover":                     t.ReportState.Layout.HideCover,
			"hide_toc":                       t.ReportState.Layout.HideTOC,
			"delivery_state":                 delivery.DeliveryState,
			"is_finalized":                   delivery.IsFinalized,
			"needs_finalize":                 delivery.NeedsFinalize,
			"requires_finalize_for_delivery": delivery.HasContent,
			"message":                        "当前报告仍处于草稿态，尚未生成最终报告文件。",
			"ui_summary":                     "已更新报告布局配置，当前仍是报告草稿",
		}), nil
	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

func (t *ManageReportBlocksTool) Name() string { return "report_manage_blocks" }
func (t *ManageReportBlocksTool) Description() string {
	return "修改报告中的 block 结构。支持 append、upsert、remove、move，作用对象是 title、markdown、html、chart 四类 block；会直接修改报告内容结构，但执行后 report delivery_state 仍会保持 draft，只有 report_finalize 才会把当前报告变成最终可交付状态。在局部编辑范围存在时，此工具只允许修改被授权的 block。"
}
func (t *ManageReportBlocksTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {"type": "string", "enum": ["append", "upsert", "remove", "move"], "description": "append（默认）、upsert、remove、move"},
			"block_id": {"type": "string", "description": "block 稳定 ID。upsert/remove/move 必填；append 可选，不填则自动生成。"},
			"block_kind": {"type": "string", "enum": ["title", "markdown", "html", "chart"], "description": "block 类型。"},
			"title": {"type": "string", "description": "标题。"},
			"content": {"type": "string", "description": "block 内容。chart block 时作为图下说明。"},
			"chart_id": {"type": "string", "description": "chart block 引用的图表 ID。"},
			"before_block_id": {"type": "string", "description": "插入到某个 block 之前。"},
			"after_block_id": {"type": "string", "description": "插入到某个 block 之后。"}
		},
		"required": []
	}`)
}

func (t *ManageReportBlocksTool) Execute(args json.RawMessage) (string, error) {
	var params struct {
		Action        string `json:"action"`
		BlockID       string `json:"block_id"`
		BlockKind     string `json:"block_kind"`
		Title         string `json:"title"`
		Content       string `json:"content"`
		ChartID       string `json:"chart_id"`
		BeforeBlockID string `json:"before_block_id"`
		AfterBlockID  string `json:"after_block_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}
	if t.ReportState == nil {
		return "", fmt.Errorf("report state is not initialized")
	}
	action := strings.TrimSpace(params.Action)
	if action == "" {
		action = "append"
	}
	blockID := strings.TrimSpace(params.BlockID)
	if blockID == "" && action != "append" {
		return "", fmt.Errorf("block_id is required for %s action", action)
	}

	switch action {
	case "append", "upsert":
		kind := strings.TrimSpace(params.BlockKind)
		if kind == "" {
			kind = "markdown"
		}
		block, err := buildReportBlock(kind, blockID, strings.TrimSpace(params.Title), params.Content, strings.TrimSpace(params.ChartID), len(t.ReportState.Blocks)+1)
		if err != nil {
			return "", err
		}
		if t.EditState != nil && !t.EditState.BlockMutationAllowed(action, block.ID) {
			return toolFailure("report_manage_blocks", "edit_scope_violation", "当前编辑范围不允许修改该 block", map[string]interface{}{
				"action":     action,
				"block_id":   block.ID,
				"ui_summary": fmt.Sprintf("block %s 不在当前局部编辑范围内", block.ID),
			}), nil
		}
		existingIndex := findReportBlockIndex(t.ReportState.Blocks, block.ID)
		insertHintIndex := -1
		summaryText := fmt.Sprintf("已将 block [%s] %s 写入报告草稿", block.Kind, block.ID)
		if existingIndex >= 0 {
			t.ReportState.Blocks = append(t.ReportState.Blocks[:existingIndex], t.ReportState.Blocks[existingIndex+1:]...)
			insertHintIndex = existingIndex
			summaryText = fmt.Sprintf("已更新报告草稿中的 block [%s] %s", block.Kind, block.ID)
		}
		insertAt := len(t.ReportState.Blocks)
		if strings.TrimSpace(params.BeforeBlockID) == "" && strings.TrimSpace(params.AfterBlockID) == "" && insertHintIndex >= 0 {
			insertAt = insertHintIndex
		} else {
			insertAt, err = resolveReportBlockInsertIndex(t.ReportState.Blocks, strings.TrimSpace(params.BeforeBlockID), strings.TrimSpace(params.AfterBlockID))
			if err != nil {
				return "", err
			}
		}
		t.ReportState.Blocks = insertReportBlockAt(t.ReportState.Blocks, block, insertAt)
		t.ReportState.NeedsFinalize = true
		delivery := DescribeReportDeliveryState(t.ReportState)
		return toolSuccess("report_manage_blocks", map[string]interface{}{
			"action":                         action,
			"block_id":                       block.ID,
			"block_kind":                     block.Kind,
			"block_count":                    len(t.ReportState.Blocks),
			"delivery_state":                 delivery.DeliveryState,
			"is_finalized":                   delivery.IsFinalized,
			"needs_finalize":                 delivery.NeedsFinalize,
			"requires_finalize_for_delivery": delivery.HasContent,
			"message":                        "当前报告仍处于草稿态，尚未生成最终报告文件。",
			"ui_summary":                     summaryText,
		}), nil
	case "remove":
		if t.EditState != nil && !t.EditState.BlockMutationAllowed(action, blockID) {
			return toolFailure("report_manage_blocks", "edit_scope_violation", "当前编辑范围不允许删除该 block", map[string]interface{}{
				"action":     action,
				"block_id":   blockID,
				"ui_summary": fmt.Sprintf("block %s 不在当前局部编辑范围内", blockID),
			}), nil
		}
		index := findReportBlockIndex(t.ReportState.Blocks, blockID)
		if index < 0 {
			return "", fmt.Errorf("block_id %s not found", blockID)
		}
		removed := t.ReportState.Blocks[index]
		t.ReportState.Blocks = append(t.ReportState.Blocks[:index], t.ReportState.Blocks[index+1:]...)
		t.ReportState.NeedsFinalize = true
		delivery := DescribeReportDeliveryState(t.ReportState)
		return toolSuccess("report_manage_blocks", map[string]interface{}{
			"action":                         action,
			"block_id":                       blockID,
			"block_count":                    len(t.ReportState.Blocks),
			"delivery_state":                 delivery.DeliveryState,
			"is_finalized":                   delivery.IsFinalized,
			"needs_finalize":                 delivery.NeedsFinalize,
			"requires_finalize_for_delivery": delivery.HasContent,
			"message":                        "当前报告仍处于草稿态，尚未生成最终报告文件。",
			"ui_summary":                     fmt.Sprintf("已从报告草稿删除 block [%s] %s", removed.Kind, removed.ID),
		}), nil
	case "move":
		if t.EditState != nil && !t.EditState.BlockMutationAllowed(action, blockID) {
			return toolFailure("report_manage_blocks", "edit_scope_violation", "当前编辑范围不允许重排该 block", map[string]interface{}{
				"action":     action,
				"block_id":   blockID,
				"ui_summary": fmt.Sprintf("block %s 不在当前局部编辑范围内", blockID),
			}), nil
		}
		index := findReportBlockIndex(t.ReportState.Blocks, blockID)
		if index < 0 {
			return "", fmt.Errorf("block_id %s not found", blockID)
		}
		block := t.ReportState.Blocks[index]
		blocks := append([]ReportBlock{}, t.ReportState.Blocks[:index]...)
		blocks = append(blocks, t.ReportState.Blocks[index+1:]...)
		insertAt, err := resolveReportBlockInsertIndex(blocks, strings.TrimSpace(params.BeforeBlockID), strings.TrimSpace(params.AfterBlockID))
		if err != nil {
			return "", err
		}
		t.ReportState.Blocks = insertReportBlockAt(blocks, block, insertAt)
		t.ReportState.NeedsFinalize = true
		delivery := DescribeReportDeliveryState(t.ReportState)
		return toolSuccess("report_manage_blocks", map[string]interface{}{
			"action":                         action,
			"block_id":                       blockID,
			"block_count":                    len(t.ReportState.Blocks),
			"delivery_state":                 delivery.DeliveryState,
			"is_finalized":                   delivery.IsFinalized,
			"needs_finalize":                 delivery.NeedsFinalize,
			"requires_finalize_for_delivery": delivery.HasContent,
			"message":                        "当前报告仍处于草稿态，尚未生成最终报告文件。",
			"ui_summary":                     fmt.Sprintf("已重排报告草稿中的 block [%s] %s", block.Kind, block.ID),
		}), nil
	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

func buildBlockID(title string, fallbackIndex int) string {
	base := strings.ToLower(strings.TrimSpace(title))
	base = strings.ReplaceAll(base, " ", "-")
	base = strings.ReplaceAll(base, "_", "-")
	base = strings.ReplaceAll(base, "/", "-")
	base = strings.ReplaceAll(base, "\\", "-")
	base = strings.Trim(base, "-")
	if base == "" {
		base = fmt.Sprintf("section-%d", fallbackIndex)
	}
	return base
}

func buildReportBlock(kind, blockID, title, content, chartID string, fallbackIndex int) (ReportBlock, error) {
	if blockID == "" {
		switch {
		case title != "":
			blockID = buildBlockID(title, fallbackIndex)
		case chartID != "":
			blockID = buildBlockID(chartID, fallbackIndex)
		default:
			blockID = fmt.Sprintf("block-%d", fallbackIndex)
		}
	}
	block := ReportBlock{
		ID:      blockID,
		Kind:    kind,
		Title:   title,
		Content: content,
		ChartID: chartID,
	}
	switch kind {
	case "title":
		if strings.TrimSpace(title) == "" {
			return ReportBlock{}, fmt.Errorf("title is required for title block")
		}
	case "markdown", "html":
		if strings.TrimSpace(content) == "" {
			return ReportBlock{}, fmt.Errorf("content is required for %s block", kind)
		}
	case "chart":
		if strings.TrimSpace(chartID) == "" {
			return ReportBlock{}, fmt.Errorf("chart_id is required for chart block")
		}
	default:
		return ReportBlock{}, fmt.Errorf("unsupported block_kind: %s", kind)
	}
	return block, nil
}

func findReportBlockIndex(blocks []ReportBlock, blockID string) int {
	for i, block := range blocks {
		if block.ID == blockID {
			return i
		}
	}
	return -1
}

func resolveReportBlockInsertIndex(blocks []ReportBlock, beforeBlockID, afterBlockID string) (int, error) {
	if beforeBlockID != "" && afterBlockID != "" {
		return 0, fmt.Errorf("before_block_id and after_block_id cannot both be set")
	}
	if beforeBlockID != "" {
		index := findReportBlockIndex(blocks, beforeBlockID)
		if index < 0 {
			return 0, fmt.Errorf("before_block_id %s not found", beforeBlockID)
		}
		return index, nil
	}
	if afterBlockID != "" {
		index := findReportBlockIndex(blocks, afterBlockID)
		if index < 0 {
			return 0, fmt.Errorf("after_block_id %s not found", afterBlockID)
		}
		return index + 1, nil
	}
	return len(blocks), nil
}

func insertReportBlockAt(blocks []ReportBlock, block ReportBlock, index int) []ReportBlock {
	if index < 0 {
		index = 0
	}
	if index > len(blocks) {
		index = len(blocks)
	}
	blocks = append(blocks, ReportBlock{})
	copy(blocks[index+1:], blocks[index:])
	blocks[index] = block
	return blocks
}

func referencedChartsOutsideChartBlocks(blocks []ReportBlock) map[string]struct{} {
	refs := make(map[string]struct{})
	for _, block := range blocks {
		if strings.EqualFold(strings.TrimSpace(block.Kind), "chart") {
			continue
		}
		for _, ref := range chartRefsOutsideChartBlock(block.Content) {
			refs[ref] = struct{}{}
		}
	}
	return refs
}

func chartRefsOutsideChartBlock(content string) []string {
	re := regexp.MustCompile(`\{\{chart:(\w+)\}\}`)
	matches := re.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}
	refs := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 && strings.TrimSpace(match[1]) != "" {
			refs = append(refs, strings.TrimSpace(match[1]))
		}
	}
	return refs
}

func reportFinalizeIssues(state *ReportState) []string {
	if state == nil {
		return []string{"report_state_missing"}
	}

	chartSet := make(map[string]struct{}, len(state.Charts))
	for _, chart := range state.Charts {
		chartID := strings.TrimSpace(chart.ID)
		if chartID != "" {
			chartSet[chartID] = struct{}{}
		}
	}

	refCounts := make(map[string]int)
	for _, block := range state.Blocks {
		if strings.EqualFold(strings.TrimSpace(block.Kind), "chart") && strings.TrimSpace(block.ChartID) != "" {
			refCounts[strings.TrimSpace(block.ChartID)]++
		}
		for chartID := range referencedChartsOutsideChartBlocks([]ReportBlock{block}) {
			refCounts[chartID]++
		}
	}

	var issues []string
	if len(state.Blocks) == 0 {
		issues = append(issues, "report_has_no_blocks")
	}
	for _, block := range state.Blocks {
		if hasDuplicateLeadingHeading(block) {
			issues = append(issues, "duplicate_block_heading:"+block.ID)
		}
		if strings.EqualFold(strings.TrimSpace(block.Kind), "chart") && strings.TrimSpace(block.ChartID) != "" && strings.TrimSpace(block.Content) == "" {
			issues = append(issues, "chart_block_missing_caption:"+block.ID)
		}
	}

	var missingCharts []string
	for chartID := range refCounts {
		if _, ok := chartSet[chartID]; !ok {
			missingCharts = append(missingCharts, chartID)
		}
	}
	sort.Strings(missingCharts)
	for _, chartID := range missingCharts {
		issues = append(issues, "missing_chart:"+chartID)
	}

	var duplicateCharts []string
	for chartID, count := range refCounts {
		if count > 1 {
			duplicateCharts = append(duplicateCharts, fmt.Sprintf("%s(x%d)", chartID, count))
		}
	}
	sort.Strings(duplicateCharts)
	for _, item := range duplicateCharts {
		issues = append(issues, "duplicate_chart:"+item)
	}

	return issues
}

func hasDuplicateLeadingHeading(block ReportBlock) bool {
	kind := strings.ToLower(strings.TrimSpace(block.Kind))
	if kind != "markdown" && kind != "html" {
		return false
	}
	if strings.TrimSpace(block.Title) == "" || strings.TrimSpace(block.Content) == "" {
		return false
	}
	lines := strings.Split(block.Content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, "#") {
			return false
		}
		heading := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
		return normalizeSectionTitle(heading) == normalizeSectionTitle(block.Title)
	}
	return false
}

func HasDuplicateLeadingHeadingForAgent(block ReportBlock) bool {
	return hasDuplicateLeadingHeading(block)
}

func normalizeSectionTitle(value string) string {
	normalized := strings.TrimSpace(value)
	normalized = regexp.MustCompile(`^(第[一二三四五六七八九十百千0-9]+[章节部分篇]\s*)`).ReplaceAllString(normalized, "")
	normalized = regexp.MustCompile(`^([一二三四五六七八九十百千0-9]+[.、)）]\s*)`).ReplaceAllString(normalized, "")
	normalized = regexp.MustCompile(`\s+`).ReplaceAllString(normalized, "")
	return normalized
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
	Subgoals    SubgoalChecker
}

func (t *FinalizeReportTool) Name() string { return "report_finalize" }
func (t *FinalizeReportTool) Description() string {
	return "将当前 report state 从 draft 收尾为 finalized，并写入最终标题/作者。调用时会校验报告结构和未闭环目标；如果状态不合法会拒绝执行。该工具不负责补全缺失内容，只负责在当前状态可落地时完成收尾；未调用时，当前 block/chart 只停留在中间状态，不会落地为最终报告文件。"
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

	if t.Subgoals != nil {
		canFinalize, blockers := t.Subgoals.CanFinalize()
		if !canFinalize {
			delivery := DescribeReportDeliveryState(t.ReportState)
			return toolFailure("report_finalize", "active_goals_block_finalize", "当前仍有未闭环的根目标 / active branch，暂不允许生成最终报告。", map[string]interface{}{
				"delivery_state":      delivery.DeliveryState,
				"is_finalized":        delivery.IsFinalized,
				"needs_finalize":      delivery.NeedsFinalize,
				"active_branch_count": len(blockers),
				"active_branches":     blockers,
				"can_finalize":        false,
				"message":             "当前仍有未闭环的根目标 / active branch，暂不允许生成最终报告。",
				"ui_summary":          fmt.Sprintf("报告暂不能 finalize：仍有 %d 条活跃分支。", len(blockers)),
			}), nil
		}
	}
	if issues := reportFinalizeIssues(t.ReportState); len(issues) > 0 {
		delivery := DescribeReportDeliveryState(t.ReportState)
		return toolFailure("report_finalize", "report_state_invalid", "当前报告状态未通过最终收尾校验。", map[string]interface{}{
			"delivery_state":       delivery.DeliveryState,
			"is_finalized":         delivery.IsFinalized,
			"needs_finalize":       delivery.NeedsFinalize,
			"can_finalize":         false,
			"finalize_issue_count": len(issues),
			"finalize_issues":      issues,
			"message":              "当前报告状态未通过最终收尾校验。",
			"ui_summary":           fmt.Sprintf("报告暂不能 finalize：还有 %d 个结构问题。", len(issues)),
		}), nil
	}

	t.ReportState.FinalTitle = params.ReportTitle
	t.ReportState.FinalAuthor = params.Author
	t.ReportState.NeedsFinalize = false

	chartCount := len(t.ReportState.Charts)
	blockCount := len(t.ReportState.Blocks)
	return toolSuccess("report_finalize", map[string]interface{}{
		"report_title":   params.ReportTitle,
		"author":         params.Author,
		"block_count":    blockCount,
		"chart_count":    chartCount,
		"delivery_state": "finalized",
		"is_finalized":   true,
		"needs_finalize": false,
		"message":        "当前报告已完成最终收尾，并可作为最终报告交付。",
		"ui_summary":     fmt.Sprintf("研究报告已生成完成（%d 个内容块，%d 个交互式图表）", blockCount, chartCount),
	}), nil
}

func ReportFinalizeIssuesForAgent(state *ReportState) []string {
	return reportFinalizeIssues(state)
}
