package agent

import (
	"errors"
	"strings"
	"testing"

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
					Name:      "create_chart",
					Arguments: `{"chart_id":"chart_a","title":"图A","option":{"series":[{"data":[1,2,3]}]}}`,
				},
			},
			{
				ID:   "call_2",
				Type: openai.ToolTypeFunction,
				Function: openai.FunctionCall{
					Name:      "write_section",
					Arguments: `{"section_type":"analysis","title":"趋势分析","content":"` + strings.Repeat("x", 400) + `"}`,
				},
			},
		},
	}

	compacted := compactAssistantMessage(message)
	if compacted.ToolCalls[0].Function.Arguments != message.ToolCalls[0].Function.Arguments {
		t.Fatal("expected create_chart arguments to stay unchanged")
	}
	if compacted.ToolCalls[1].Function.Arguments == message.ToolCalls[1].Function.Arguments {
		t.Fatal("expected write_section arguments to be compacted")
	}
}

func TestCompactToolResult(t *testing.T) {
	t.Parallel()

	queryResult := `{
  "ok": true,
  "tool": "query_data",
  "row_count": 2,
  "columns": ["month", "revenue"],
  "rows": [
    {"month":"2025-01","revenue":100},
    {"month":"2025-02","revenue":120}
  ]
}`
	compactedQuery := compactToolResult("query_data", queryResult)
	if strings.Contains(compactedQuery, "\n") {
		t.Fatalf("expected minified query result, got %q", compactedQuery)
	}

	describeResult := `{
  "tableName":"sales",
  "rowCount":2,
  "columns":[{"name":"month"}]
}`
	compactedDescribe := compactToolResult("describe_data", describeResult)
	if strings.Contains(compactedDescribe, "\n") {
		t.Fatalf("expected minified describe result, got %q", compactedDescribe)
	}

	listTablesResult := `{
  "ok": true,
  "tool": "list_tables",
  "tables": ["sales"],
  "table_count": 1
}`
	compactedTables := compactToolResult("list_tables", listTablesResult)
	if strings.Contains(compactedTables, "\n") {
		t.Fatalf("expected minified list_tables result, got %q", compactedTables)
	}

	runPythonResult := `{
  "ok": false,
  "tool": "run_python",
  "error_code": "execution_failed",
  "detail": "NameError"
}`
	compactedPython := compactToolResult("run_python", runPythonResult)
	if strings.Contains(compactedPython, "\n") {
		t.Fatalf("expected minified run_python result, got %q", compactedPython)
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
