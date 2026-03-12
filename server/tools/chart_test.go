package tools

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCreateChartToolReturnsStructuredValidationFeedback(t *testing.T) {
	t.Parallel()

	tool := &CreateChartTool{ReportState: &ReportState{}}
	result, err := tool.Execute(json.RawMessage(`{"chart_id":"chart_sales","title":"销售趋势","option":{}}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(result, "请重新调用 create_chart") {
		t.Fatalf("expected structured feedback instead of retry prompt, got %q", result)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("expected json feedback, got %v", err)
	}
	if payload["ok"] != false {
		t.Fatalf("expected ok=false, got %#v", payload["ok"])
	}
	if payload["error_code"] != "missing_option" {
		t.Fatalf("unexpected error_code: %#v", payload["error_code"])
	}
}

func TestCreateChartToolRejectsInvalidOptionObject(t *testing.T) {
	t.Parallel()

	tool := &CreateChartTool{ReportState: &ReportState{}}
	result, err := tool.Execute(json.RawMessage(`{"chart_id":"chart_sales","title":"销售趋势","option":"oops"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("expected json feedback, got %v", err)
	}
	if payload["error_code"] != "invalid_option" {
		t.Fatalf("unexpected error_code: %#v", payload["error_code"])
	}
	if payload["detail"] == "" {
		t.Fatalf("expected validation detail in payload: %#v", payload)
	}
}

func TestCreateChartToolReturnsStructuredSuccess(t *testing.T) {
	t.Parallel()

	tool := &CreateChartTool{ReportState: &ReportState{}}
	result, err := tool.Execute(json.RawMessage(`{
		"chart_id":"chart_sales",
		"title":"销售趋势",
		"option":{"title":{"text":"销售趋势"},"xAxis":{"type":"category","data":["1月"]},"yAxis":{"type":"value"},"series":[{"type":"bar","data":[100]}]}
	}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("expected json feedback, got %v", err)
	}
	if payload["ok"] != true {
		t.Fatalf("expected ok=true, got %#v", payload["ok"])
	}
	if payload["chart_ref"] != "{{chart:chart_sales}}" {
		t.Fatalf("unexpected chart_ref: %#v", payload["chart_ref"])
	}
}
