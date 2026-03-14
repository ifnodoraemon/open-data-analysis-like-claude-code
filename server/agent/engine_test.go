package agent

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ifnodoraemon/openDataAnalysis/tools"
	openai "github.com/sashabaranov/go-openai"
)

func TestCompactAssistantMessage(t *testing.T) {
	t.Parallel()

	message := openai.ChatCompletionMessage{
		Role: openai.ChatMessageRoleAssistant,
		ToolCalls: []openai.ToolCall{
			{
				ID:   "call_1",
				Type: openai.ToolTypeFunction,
				Function: openai.FunctionCall{
					Name:      "report_create_chart",
					Arguments: `{"chart_id":"chart_a","title":"图A","option":{"series":[{"data":[1,2,3]}]}}`,
				},
			},
			{
				ID:   "call_2",
				Type: openai.ToolTypeFunction,
				Function: openai.FunctionCall{
					Name:      "report_manage_blocks",
					Arguments: `{"block_kind":"markdown","title":"趋势分析","content":"` + strings.Repeat("x", 400) + `"}`,
				},
			},
		},
	}

	compacted := compactAssistantMessage(message)
	if compacted.ToolCalls[0].Function.Arguments != message.ToolCalls[0].Function.Arguments {
		t.Fatal("expected report_create_chart arguments to stay unchanged")
	}
	if compacted.ToolCalls[1].Function.Arguments == message.ToolCalls[1].Function.Arguments {
		t.Fatal("expected report_manage_blocks arguments to be compacted")
	}
}

func TestCompactToolResult(t *testing.T) {
	t.Parallel()

	queryResult := `{
  "ok": true,
  "tool": "data_query_sql",
  "row_count": 2,
  "columns": ["month", "revenue"],
  "rows": [
    {"month":"2025-01","revenue":100},
    {"month":"2025-02","revenue":120}
  ]
}`
	compactedQuery := compactToolResult("data_query_sql", queryResult)
	if strings.Contains(compactedQuery, "\n") {
		t.Fatalf("expected minified query result, got %q", compactedQuery)
	}

	describeResult := `{
  "tableName":"sales",
  "rowCount":2,
  "columns":[{"name":"month"}]
}`
	compactedDescribe := compactToolResult("data_describe_table", describeResult)
	if strings.Contains(compactedDescribe, "\n") {
		t.Fatalf("expected minified describe result, got %q", compactedDescribe)
	}

	listTablesResult := `{
  "ok": true,
  "tool": "data_list_tables",
  "tables": ["sales"],
  "table_count": 1
}`
	compactedTables := compactToolResult("data_list_tables", listTablesResult)
	if strings.Contains(compactedTables, "\n") {
		t.Fatalf("expected minified data_list_tables result, got %q", compactedTables)
	}

	runPythonResult := `{
  "ok": false,
  "tool": "code_run_python",
  "error_code": "execution_failed",
  "detail": "NameError"
}`
	compactedPython := compactToolResult("code_run_python", runPythonResult)
	if strings.Contains(compactedPython, "\n") {
		t.Fatalf("expected minified code_run_python result, got %q", compactedPython)
	}
}

func TestToolCallSucceeded(t *testing.T) {
	t.Parallel()

	if toolCallSucceeded("", errors.New("boom")) {
		t.Fatal("expected exec error to mark tool call as failed")
	}
	if toolCallSucceeded(`{"ok":false,"error_code":"missing_option"}`, nil) {
		t.Fatal("expected ok=false payload to mark tool call as failed")
	}
	if !toolCallSucceeded(`{"ok":true}`, nil) {
		t.Fatal("expected ok=true payload to mark tool call as successful")
	}
	if !toolCallSucceeded("plain text result", nil) {
		t.Fatal("expected plain text result to remain successful")
	}
}

func TestInspectGoalsToolReturnsFactsOnly(t *testing.T) {
	t.Parallel()

	sm := NewSubgoalManager()
	sm.AddGoal("分析销售", "")
	sm.AddGoal("分析营销", "")

	tool := &InspectGoalsTool{Subgoals: sm}
	result, err := tool.Execute(nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload["tool"] != "state_goal_inspect" {
		t.Fatalf("unexpected tool: %#v", payload["tool"])
	}
	if payload["goal_count"].(float64) != 2 {
		t.Fatalf("unexpected goal count: %#v", payload["goal_count"])
	}
	if payload["active_branch_count"].(float64) == 0 {
		t.Fatalf("expected active branches in payload: %#v", payload)
	}
}

func TestInspectMemoryToolReturnsFactsOnly(t *testing.T) {
	t.Parallel()

	mem := NewWorkingMemory()
	mem.SaveFact("roi_definition", "ROI = attributed_revenue / ad_spend")

	tool := &InspectMemoryTool{Memory: mem}
	result, err := tool.Execute(nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload["tool"] != "state_memory_inspect" {
		t.Fatalf("unexpected tool: %#v", payload["tool"])
	}
	if payload["fact_count"].(float64) != 1 {
		t.Fatalf("unexpected fact count: %#v", payload["fact_count"])
	}
	facts := payload["facts"].(map[string]interface{})
	if facts["roi_definition"] != "ROI = attributed_revenue / ad_spend" {
		t.Fatalf("unexpected facts: %#v", facts)
	}
}

func TestInspectReportStateToolReturnsFactsOnly(t *testing.T) {
	t.Parallel()

	tool := &InspectReportStateTool{
		ReportState: &tools.ReportState{
			Blocks: []tools.ReportBlock{
				{ID: "analysis", Kind: "markdown", Title: "销售分析", Content: "## 一、销售分析\n\n{{chart:chart_sales}}\n\n{{chart:chart_missing}}"},
				{ID: "sales_chart", Kind: "chart", ChartID: "chart_sales"},
			},
			Charts: []tools.ChartData{
				{ID: "chart_sales"},
				{ID: "chart_unused"},
			},
		},
	}

	result, err := tool.Execute(nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload["tool"] != "state_report_inspect" {
		t.Fatalf("unexpected tool: %#v", payload["tool"])
	}
	if payload["chart_count"].(float64) != 2 {
		t.Fatalf("unexpected chart count: %#v", payload["chart_count"])
	}
	if len(payload["missing_chart_references"].([]interface{})) != 1 {
		t.Fatalf("expected one missing chart ref, got %#v", payload["missing_chart_references"])
	}
	if len(payload["unreferenced_charts"].([]interface{})) != 1 {
		t.Fatalf("expected one unreferenced chart, got %#v", payload["unreferenced_charts"])
	}
	if len(payload["blocks_with_duplicate_heading"].([]interface{})) != 1 {
		t.Fatalf("expected one duplicate heading block, got %#v", payload["blocks_with_duplicate_heading"])
	}
	if len(payload["chart_blocks_missing_caption"].([]interface{})) != 1 {
		t.Fatalf("expected one chart block missing caption, got %#v", payload["chart_blocks_missing_caption"])
	}
}

func TestCompactMessagesByMeasuredPromptTokens(t *testing.T) {
	t.Parallel()

	engine := &Engine{
		systemPrompt: "system",
		messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: "system"},
		},
	}

	for i := 0; i < 16; i++ {
		engine.messages = append(engine.messages,
			openai.ChatCompletionMessage{Role: openai.ChatMessageRoleUser, Content: strings.Repeat("a", 40000)},
			openai.ChatCompletionMessage{Role: openai.ChatMessageRoleAssistant, Content: "ok"},
		)
	}

	engine.compactMessagesLocked(contextCompactTriggerTokens + 1)

	if len(engine.messages) >= 33 {
		t.Fatalf("expected messages to be compacted, got %d", len(engine.messages))
	}
	if !isHistoryDigestMessage(engine.messages[1]) {
		t.Fatalf("expected digest message at position 1")
	}
}

func TestCompactMessagesLockedSkipsWithoutMeasuredTokens(t *testing.T) {
	t.Parallel()

	engine := &Engine{
		systemPrompt: "system",
		messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: "system"},
			{Role: openai.ChatMessageRoleUser, Content: "user"},
			{Role: openai.ChatMessageRoleAssistant, Content: "assistant"},
			{Role: openai.ChatMessageRoleUser, Content: strings.Repeat("x", 1000)},
		},
	}

	before := len(engine.messages)
	engine.compactMessagesLocked(0)
	if len(engine.messages) != before {
		t.Fatalf("expected no compaction without measured tokens")
	}
}
