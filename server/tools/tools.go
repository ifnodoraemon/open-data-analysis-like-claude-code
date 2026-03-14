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
}

type FileMaterializer func(fileID string) (*FileReference, error)

type ReportState struct {
	Blocks      []ReportBlock `json:"blocks"`
	Charts      []ChartData   `json:"charts"`
	FinalTitle  string        `json:"finalTitle,omitempty"`
	FinalAuthor string        `json:"finalAuthor,omitempty"`
	Layout      ReportLayout  `json:"layout,omitempty"`
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

// LoadDataTool 加载数据文件到 SQLite
type LoadDataTool struct {
	Ingester         *data.Ingester
	FileMaterializer FileMaterializer
}

func (t *LoadDataTool) Name() string { return "data_load_file" }
func (t *LoadDataTool) Description() string {
	return "加载用户上传的 CSV 或 Excel 文件到内部数据库，并返回表名、行数和列数。"
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
		"summary_text": fmt.Sprintf("数据已成功导入到表 %s（%d 行，%d 列）", tableName, rowCount, colCount),
		"next_action":  fmt.Sprintf("先调用 data_describe_table 查看 %s 的结构，再按需使用 data_query_sql", tableName),
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
			"table_count":  0,
			"tables":       []string{},
			"empty":        true,
			"next_action":  "先调用 data_load_file 导入文件，再继续 data_describe_table 或 data_query_sql",
			"summary_text": "当前没有已导入的数据表",
		}), nil
	}

	return toolSuccess("data_list_tables", map[string]interface{}{
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

func (t *DescribeDataTool) Name() string { return "data_describe_table" }
func (t *DescribeDataTool) Description() string {
	return "返回指定表的 schema 和统计摘要，包括列信息、行数、基础统计和采样值。"
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
			"table_name":  params.TableName,
			"detail":      err.Error(),
			"next_action": "检查 table_name 是否正确，必要时先调用 data_list_tables",
		}), nil
	}

	return toolSuccess("data_describe_table", map[string]interface{}{
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
			"sql":         params.SQL,
			"detail":      err.Error(),
			"next_action": "根据错误信息修正 SQL，继续使用单条只读 SELECT/WITH",
		}), nil
	}

	return toolSuccess("data_query_sql", map[string]interface{}{
		"sql":          params.SQL,
		"row_count":    len(rows),
		"columns":      queryResultColumns(rows),
		"rows":         rows,
		"summary_text": fmt.Sprintf("SQL 查询成功，返回 %d 行", len(rows)),
	}), nil
}

func init() {
	RegisterGlobalTool(func(ctx ToolContext) Tool {
		return &ManageReportBlocksTool{ReportState: ctx.ReportState}
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
}

func (t *ConfigureReportTool) Name() string { return "report_configure_layout" }
func (t *ConfigureReportTool) Description() string {
	return "配置报告布局和页面外壳。支持更新或重置 layout 配置。"
}
func (t *ConfigureReportTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {"type": "string", "enum": ["merge", "reset"], "description": "merge（默认，更新布局配置）或 reset（恢复默认模板）"},
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
		return toolSuccess("report_configure_layout", map[string]interface{}{
			"action":       action,
			"summary_text": "已恢复默认报告模板",
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
		return toolSuccess("report_configure_layout", map[string]interface{}{
			"action":           action,
			"has_custom_shell": t.ReportState.Layout.CustomHTMLShell != "",
			"has_custom_css":   t.ReportState.Layout.CustomCSS != "",
			"has_custom_js":    t.ReportState.Layout.CustomJS != "",
			"body_class":       t.ReportState.Layout.BodyClass,
			"hide_cover":       t.ReportState.Layout.HideCover,
			"hide_toc":         t.ReportState.Layout.HideTOC,
			"summary_text":     "已更新报告布局配置",
		}), nil
	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

func (t *ManageReportBlocksTool) Name() string { return "report_manage_blocks" }
func (t *ManageReportBlocksTool) Description() string {
	return "创建、更新、删除或重排报告 block。支持 title、markdown、html 和 chart 四类 block；markdown block 的 title 会作为章节标题渲染，chart block 的 content 会作为图下说明渲染。"
}
func (t *ManageReportBlocksTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {"type": "string", "enum": ["append", "upsert", "remove", "move"], "description": "append（默认）、upsert、remove、move"},
			"block_id": {"type": "string", "description": "block 稳定 ID。upsert/remove/move 必填；append 可选，不填则自动生成。"},
			"block_kind": {"type": "string", "enum": ["title", "markdown", "html", "chart"], "description": "block 类型。"},
			"title": {"type": "string", "description": "可选标题，用于目录和区块展示。markdown block 通常不需要在 content 里重复相同标题。"},
			"content": {"type": "string", "description": "markdown/html block 内容；chart block 时会渲染为图下说明。"},
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
		existingIndex := findReportBlockIndex(t.ReportState.Blocks, block.ID)
		insertHintIndex := -1
		summaryText := fmt.Sprintf("已添加 block [%s] %s", block.Kind, block.ID)
		if existingIndex >= 0 {
			t.ReportState.Blocks = append(t.ReportState.Blocks[:existingIndex], t.ReportState.Blocks[existingIndex+1:]...)
			insertHintIndex = existingIndex
			summaryText = fmt.Sprintf("已更新 block [%s] %s", block.Kind, block.ID)
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
		return toolSuccess("report_manage_blocks", map[string]interface{}{
			"action":       action,
			"block_id":     block.ID,
			"block_kind":   block.Kind,
			"block_count":  len(t.ReportState.Blocks),
			"summary_text": summaryText,
		}), nil
	case "remove":
		index := findReportBlockIndex(t.ReportState.Blocks, blockID)
		if index < 0 {
			return "", fmt.Errorf("block_id %s not found", blockID)
		}
		removed := t.ReportState.Blocks[index]
		t.ReportState.Blocks = append(t.ReportState.Blocks[:index], t.ReportState.Blocks[index+1:]...)
		return toolSuccess("report_manage_blocks", map[string]interface{}{
			"action":       action,
			"block_id":     blockID,
			"block_count":  len(t.ReportState.Blocks),
			"summary_text": fmt.Sprintf("已删除 block [%s] %s", removed.Kind, removed.ID),
		}), nil
	case "move":
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
		return toolSuccess("report_manage_blocks", map[string]interface{}{
			"action":       action,
			"block_id":     blockID,
			"block_count":  len(t.ReportState.Blocks),
			"summary_text": fmt.Sprintf("已重排 block [%s] %s", block.Kind, block.ID),
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
	re := regexp.MustCompile(`\{\{chart:(\w+)\}\}`)
	refs := make(map[string]struct{})
	for _, block := range blocks {
		if strings.EqualFold(strings.TrimSpace(block.Kind), "chart") {
			continue
		}
		for _, match := range re.FindAllStringSubmatch(block.Content, -1) {
			if len(match) > 1 {
				refs[match[1]] = struct{}{}
			}
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
	return "生成最终报告。调用前要求报告状态结构有效，且不存在未闭环的根目标。"
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
			return "", fmt.Errorf("Action Denied: 当前仍有未闭环的根目标 / active branch，暂不允许生成最终报告。请优先完成或放弃这些分支后再收尾：%s", strings.Join(blockers, " | "))
		}
	}
	if issues := reportFinalizeIssues(t.ReportState); len(issues) > 0 {
		return "", fmt.Errorf("Action Denied: 当前报告状态未通过最终收尾校验：%s", strings.Join(issues, ", "))
	}

	t.ReportState.FinalTitle = params.ReportTitle
	t.ReportState.FinalAuthor = params.Author

	chartCount := len(t.ReportState.Charts)
	blockCount := len(t.ReportState.Blocks)
	return toolSuccess("report_finalize", map[string]interface{}{
		"report_title": params.ReportTitle,
		"author":       params.Author,
		"block_count":  blockCount,
		"chart_count":  chartCount,
		"summary_text": fmt.Sprintf("研究报告已生成完成（%d 个内容块，%d 个交互式图表）", blockCount, chartCount),
	}), nil
}
