package tools

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/data"
)

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
	if !payload.OK || payload.Tool != "load_data" {
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

func TestWriteSectionAndFinalizeReportReturnStructuredPayloads(t *testing.T) {
	t.Parallel()

	state := &ReportState{}
	writeTool := &WriteSectionTool{ReportState: state}
	finalizeTool := &FinalizeReportTool{ReportState: state}

	writeResult, err := writeTool.Execute(json.RawMessage(`{"section_type":"summary","title":"执行摘要","content":"收入增长 20%"}`))
	if err != nil {
		t.Fatalf("write execute: %v", err)
	}
	var writePayload struct {
		OK           bool   `json:"ok"`
		SectionType  string `json:"section_type"`
		SectionCount int    `json:"section_count"`
	}
	if err := json.Unmarshal([]byte(writeResult), &writePayload); err != nil {
		t.Fatalf("expected write json payload: %v", err)
	}
	if !writePayload.OK || writePayload.SectionType != "summary" || writePayload.SectionCount != 1 {
		t.Fatalf("unexpected write payload: %#v", writePayload)
	}

	finalizeResult, err := finalizeTool.Execute(json.RawMessage(`{"report_title":"销售分析","author":"AI 数据分析师"}`))
	if err != nil {
		t.Fatalf("finalize execute: %v", err)
	}
	var finalizePayload struct {
		OK           bool   `json:"ok"`
		ReportTitle  string `json:"report_title"`
		SectionCount int    `json:"section_count"`
		ChartCount   int    `json:"chart_count"`
	}
	if err := json.Unmarshal([]byte(finalizeResult), &finalizePayload); err != nil {
		t.Fatalf("expected finalize json payload: %v", err)
	}
	if !finalizePayload.OK || finalizePayload.ReportTitle != "销售分析" || finalizePayload.SectionCount != 1 || finalizePayload.ChartCount != 0 {
		t.Fatalf("unexpected finalize payload: %#v", finalizePayload)
	}
}
