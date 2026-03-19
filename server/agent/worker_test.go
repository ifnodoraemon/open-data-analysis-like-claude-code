package agent

import (
	"encoding/json"
	"testing"

	"github.com/ifnodoraemon/openDataAnalysis/tools"
)

func TestDelegateTaskToolReturnsStructuredFailureWhenAllowedToolsUnresolved(t *testing.T) {
	t.Parallel()

	tool := &DelegateTaskTool{
		BaseRegistry: tools.NewRegistry(),
	}

	result, err := tool.Execute(json.RawMessage(`{
		"role_name":"researcher",
		"task_instruction":"检查销售异常",
		"allowed_tools":["missing_tool"]
	}`))
	if err != nil {
		t.Fatalf("expected structured failure instead of error, got %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("expected json payload: %v", err)
	}
	if payload["tool"] != "task_delegate" || payload["ok"] != false {
		t.Fatalf("unexpected delegate failure payload: %#v", payload)
	}
	if payload["error_code"] != "no_allowed_tools_resolved" {
		t.Fatalf("unexpected error_code: %#v", payload["error_code"])
	}
	if payload["delegate_role"] != "researcher" {
		t.Fatalf("expected delegate_role in payload: %#v", payload)
	}
}

func TestDelegateTaskToolReturnsStructuredFailureForMissingRoleName(t *testing.T) {
	t.Parallel()

	tool := &DelegateTaskTool{
		BaseRegistry: tools.NewRegistry(),
	}

	result, err := tool.Execute(json.RawMessage(`{
		"task_instruction":"检查销售异常",
		"allowed_tools":["data_query_sql"]
	}`))
	if err != nil {
		t.Fatalf("expected structured failure instead of error, got %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("expected json payload: %v", err)
	}
	if payload["error_code"] != "missing_role_name" {
		t.Fatalf("unexpected error_code: %#v", payload["error_code"])
	}
	if payload["tool"] != "task_delegate" {
		t.Fatalf("unexpected tool: %#v", payload["tool"])
	}
}

func TestDelegateChildToolFailureReturnsStructuredPayload(t *testing.T) {
	t.Parallel()

	result := delegateChildToolFailure("data_query_sql", "boom")

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("expected json payload: %v", err)
	}
	if payload["tool"] != "data_query_sql" || payload["ok"] != false {
		t.Fatalf("unexpected child tool failure payload: %#v", payload)
	}
	if payload["error_code"] != "execution_error" || payload["message"] != "boom" {
		t.Fatalf("unexpected child tool failure fields: %#v", payload)
	}
}
