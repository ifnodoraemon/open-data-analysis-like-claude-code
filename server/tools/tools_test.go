package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/config"
	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/data"
	"github.com/sashabaranov/go-openai"
)

type stubSubgoalChecker struct {
	canFinalize bool
	blockers    []string
}

func (s stubSubgoalChecker) CanFinalize() (bool, []string) {
	return s.canFinalize, s.blockers
}

func (s stubSubgoalChecker) AutoCompleteReportGoals(result string) int {
	return 0
}

func TestMain(m *testing.M) {
	config.Cfg = &config.Config{
		LLMAPIKey: "mock-key",
	}
	data.AnalyzeTableSemantics = func(ctx context.Context, client *openai.Client, schema *data.SchemaInfo, activeTables []string) (*data.SemanticProfile, error) {
		return &data.SemanticProfile{
			TableSummary: "Mock test semantics",
		}, nil
	}
	os.Exit(m.Run())
}

func TestListTablesToolReturnsStructuredEmptyState(t *testing.T) {
	t.Parallel()

	ing := data.NewIngester(t.TempDir())
	if err := ing.InitDB("session_empty"); err != nil {
		t.Fatalf("init db: %v", err)
	}

	tool := &ListTablesTool{Ingester: ing}
	result, err := tool.Execute(nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("expected json payload: %v", err)
	}
	if payload["ok"] != true {
		t.Fatalf("expected ok=true, got %#v", payload["ok"])
	}
	if payload["empty"] != true {
		t.Fatalf("expected empty=true, got %#v", payload["empty"])
	}
}

func TestListTablesToolReturnsStructuredTableList(t *testing.T) {
	t.Parallel()

	ing := data.NewIngester(t.TempDir())
	if err := ing.InitDB("session_tables"); err != nil {
		t.Fatalf("init db: %v", err)
	}
	if _, err := ing.GetDB().Exec(`CREATE TABLE sales (id INTEGER)`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	tool := &ListTablesTool{Ingester: ing}
	result, err := tool.Execute(nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	var payload struct {
		OK         bool     `json:"ok"`
		TableCount float64  `json:"table_count"`
		Tables     []string `json:"tables"`
	}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("expected json payload: %v", err)
	}
	if !payload.OK || payload.TableCount != 1 {
		t.Fatalf("unexpected payload: %#v", payload)
	}
	if len(payload.Tables) != 1 || payload.Tables[0] != "sales" {
		t.Fatalf("unexpected tables: %#v", payload.Tables)
	}
}

func TestLoadDataToolReturnsStructuredPayload(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "sales.csv")
	if err := os.WriteFile(csvPath, []byte("month,revenue\n2025-01,100\n2025-02,120\n"), 0o644); err != nil {
		t.Fatalf("write csv: %v", err)
	}

	ing := data.NewIngester(tmpDir)
	if err := ing.InitDB("session_load"); err != nil {
		t.Fatalf("init db: %v", err)
	}

	tool := &LoadDataTool{
		Ingester: ing,
		FileMaterializer: func(fileID string) (*FileReference, error) {
			return &FileReference{
				FileID:      fileID,
				DisplayName: "sales.csv",
				StoredPath:  csvPath,
			}, nil
		},
	}

	result, err := tool.Execute(json.RawMessage(`{"file_id":"file_1"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	var payload struct {
		OK          bool   `json:"ok"`
		Tool        string `json:"tool"`
		TableName   string `json:"table_name"`
		RowCount    int    `json:"row_count"`
		ColumnCount int    `json:"column_count"`
	}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("expected json payload: %v", err)
	}
	if !payload.OK || payload.Tool != "data_load_file" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
	if payload.TableName != "sales" || payload.RowCount != 2 || payload.ColumnCount != 2 {
		t.Fatalf("unexpected load result: %#v", payload)
	}
}

func TestDescribeDataToolReturnsStructuredSchema(t *testing.T) {
	t.Parallel()

	ing := data.NewIngester(t.TempDir())
	if err := ing.InitDB("session_describe"); err != nil {
		t.Fatalf("init db: %v", err)
	}
	if _, err := ing.GetDB().Exec(`CREATE TABLE sales (month TEXT, revenue INTEGER)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := ing.GetDB().Exec(`INSERT INTO sales (month, revenue) VALUES ('2025-01', 100), ('2025-02', 120)`); err != nil {
		t.Fatalf("insert rows: %v", err)
	}

	tool := &DescribeDataTool{Ingester: ing}
	result, err := tool.Execute(json.RawMessage(`{"table_name":"sales"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	var payload struct {
		OK          bool            `json:"ok"`
		TableName   string          `json:"table_name"`
		ColumnCount int             `json:"column_count"`
		Schema      data.SchemaInfo `json:"schema"`
	}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("expected json payload: %v", err)
	}
	if !payload.OK || payload.TableName != "sales" || payload.ColumnCount != 2 {
		t.Fatalf("unexpected payload: %#v", payload)
	}
	if payload.Schema.RowCount != 2 || len(payload.Schema.Columns) != 2 {
		t.Fatalf("unexpected schema: %#v", payload.Schema)
	}
}

func TestDescribeDataToolReturnsStructuredFailure(t *testing.T) {
	t.Parallel()

	ing := data.NewIngester(t.TempDir())
	if err := ing.InitDB("session_describe_fail"); err != nil {
		t.Fatalf("init db: %v", err)
	}

	tool := &DescribeDataTool{Ingester: ing}
	result, err := tool.Execute(json.RawMessage(`{"table_name":"missing_table"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("expected json payload: %v", err)
	}
	if payload["ok"] != false || payload["error_code"] != "schema_lookup_failed" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestQueryDataToolReturnsStructuredSuccess(t *testing.T) {
	t.Parallel()

	ing := data.NewIngester(t.TempDir())
	if err := ing.InitDB("session_query"); err != nil {
		t.Fatalf("init db: %v", err)
	}
	if _, err := ing.GetDB().Exec(`CREATE TABLE sales (month TEXT, revenue INTEGER)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := ing.GetDB().Exec(`INSERT INTO sales (month, revenue) VALUES ('2025-01', 100), ('2025-02', 120)`); err != nil {
		t.Fatalf("insert rows: %v", err)
	}

	tool := &QueryDataTool{Ingester: ing}
	result, err := tool.Execute(json.RawMessage(`{"sql":"SELECT month, revenue FROM sales ORDER BY month"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	var payload struct {
		OK       bool                     `json:"ok"`
		RowCount int                      `json:"row_count"`
		Columns  []string                 `json:"columns"`
		Rows     []map[string]interface{} `json:"rows"`
	}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("expected json payload: %v", err)
	}
	if !payload.OK || payload.RowCount != 2 || len(payload.Rows) != 2 {
		t.Fatalf("unexpected payload: %#v", payload)
	}
	if len(payload.Columns) != 2 || payload.Columns[0] != "month" || payload.Columns[1] != "revenue" {
		t.Fatalf("unexpected columns: %#v", payload.Columns)
	}
}

func TestQueryDataToolReturnsStructuredFailure(t *testing.T) {
	t.Parallel()

	ing := data.NewIngester(t.TempDir())
	if err := ing.InitDB("session_query_fail"); err != nil {
		t.Fatalf("init db: %v", err)
	}

	tool := &QueryDataTool{Ingester: ing}
	result, err := tool.Execute(json.RawMessage(`{"sql":"SELECT * FROM missing_table"}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("expected json payload: %v", err)
	}
	if payload["ok"] != false || payload["error_code"] != "query_failed" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestManageReportBlocksAndFinalizeReturnStructuredPayloads(t *testing.T) {
	t.Parallel()

	state := &ReportState{}
	blockTool := &ManageReportBlocksTool{ReportState: state}
	finalizeTool := &FinalizeReportTool{ReportState: state}

	blockResult, err := blockTool.Execute(json.RawMessage(`{"block_id":"summary","block_kind":"markdown","title":"执行摘要","content":"收入增长 20%"}`))
	if err != nil {
		t.Fatalf("block execute: %v", err)
	}
	var blockPayload struct {
		OK         bool   `json:"ok"`
		BlockKind  string `json:"block_kind"`
		BlockCount int    `json:"block_count"`
	}
	if err := json.Unmarshal([]byte(blockResult), &blockPayload); err != nil {
		t.Fatalf("expected block json payload: %v", err)
	}
	if !blockPayload.OK || blockPayload.BlockKind != "markdown" || blockPayload.BlockCount != 1 {
		t.Fatalf("unexpected block payload: %#v", blockPayload)
	}

	finalizeResult, err := finalizeTool.Execute(json.RawMessage(`{"report_title":"销售分析","author":"AI 数据分析师"}`))
	if err != nil {
		t.Fatalf("finalize execute: %v", err)
	}
	var finalizePayload struct {
		OK          bool   `json:"ok"`
		ReportTitle string `json:"report_title"`
		BlockCount  int    `json:"block_count"`
		ChartCount  int    `json:"chart_count"`
	}
	if err := json.Unmarshal([]byte(finalizeResult), &finalizePayload); err != nil {
		t.Fatalf("expected finalize json payload: %v", err)
	}
	if !finalizePayload.OK || finalizePayload.ReportTitle != "销售分析" || finalizePayload.BlockCount != 1 || finalizePayload.ChartCount != 0 {
		t.Fatalf("unexpected finalize payload: %#v", finalizePayload)
	}
}

func TestFinalizeReportRejectsOpenActiveBranch(t *testing.T) {
	t.Parallel()

	tool := &FinalizeReportTool{
		ReportState: &ReportState{},
		Subgoals: stubSubgoalChecker{
			canFinalize: false,
			blockers:    []string{"验证销售波动[pending] -> 拆分 East 区域[running]"},
		},
	}

	_, err := tool.Execute(json.RawMessage(`{"report_title":"销售分析"}`))
	if err == nil {
		t.Fatal("expected finalize to be rejected")
	}
	if !strings.Contains(err.Error(), "active branch") {
		t.Fatalf("expected active branch hint in error, got %v", err)
	}
	if !strings.Contains(err.Error(), "East") {
		t.Fatalf("expected blocker details in error, got %v", err)
	}
}

func TestConfigureReportToolMergeAndReset(t *testing.T) {
	t.Parallel()

	state := &ReportState{}
	tool := &ConfigureReportTool{ReportState: state}

	result, err := tool.Execute(json.RawMessage(`{
		"custom_css":".hero{color:red;}",
		"custom_js":"console.log('x')",
		"body_class":"magazine",
		"hide_cover":true,
		"hide_toc":true
	}`))
	if err != nil {
		t.Fatalf("merge execute: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("expected report_configure_layout json payload: %v", err)
	}
	if payload["ok"] != true {
		t.Fatalf("expected ok=true, got %#v", payload["ok"])
	}
	if !state.Layout.HideCover || !state.Layout.HideTOC || state.Layout.BodyClass != "magazine" {
		t.Fatalf("unexpected layout after merge: %#v", state.Layout)
	}

	if _, err := tool.Execute(json.RawMessage(`{"action":"reset"}`)); err != nil {
		t.Fatalf("reset execute: %v", err)
	}
	if state.Layout != (ReportLayout{}) {
		t.Fatalf("expected empty layout after reset, got %#v", state.Layout)
	}
}

func TestManageReportBlocksToolSupportsCRUD(t *testing.T) {
	t.Parallel()

	state := &ReportState{}
	tool := &ManageReportBlocksTool{ReportState: state}

	if _, err := tool.Execute(json.RawMessage(`{"block_id":"intro","block_kind":"markdown","title":"导言","content":"第一段"}`)); err != nil {
		t.Fatalf("append intro: %v", err)
	}
	if _, err := tool.Execute(json.RawMessage(`{"block_id":"chart-1","block_kind":"chart","title":"趋势图","chart_id":"chart_sales"}`)); err != nil {
		t.Fatalf("append chart block: %v", err)
	}
	if len(state.Blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(state.Blocks))
	}

	if _, err := tool.Execute(json.RawMessage(`{"action":"upsert","block_id":"intro","block_kind":"markdown","title":"导言","content":"更新后的第一段"}`)); err != nil {
		t.Fatalf("upsert intro: %v", err)
	}
	if state.Blocks[0].Content != "更新后的第一段" {
		t.Fatalf("expected updated intro block, got %#v", state.Blocks[0])
	}

	if _, err := tool.Execute(json.RawMessage(`{"action":"move","block_id":"chart-1","before_block_id":"intro"}`)); err != nil {
		t.Fatalf("move chart block: %v", err)
	}
	if state.Blocks[0].ID != "chart-1" {
		t.Fatalf("expected chart block moved first, got %#v", state.Blocks)
	}

	if _, err := tool.Execute(json.RawMessage(`{"action":"remove","block_id":"intro"}`)); err != nil {
		t.Fatalf("remove intro block: %v", err)
	}
	if len(state.Blocks) != 1 || state.Blocks[0].ID != "chart-1" {
		t.Fatalf("expected only chart block remaining, got %#v", state.Blocks)
	}
}
