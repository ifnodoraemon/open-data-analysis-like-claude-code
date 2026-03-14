package tools

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCreateChartToolReturnsStructuredValidationFeedback(t *testing.T) {
	t.Parallel()

	tool := &CreateChartTool{ReportState: &ReportState{}}
	result, err := tool.Execute(json.RawMessage(`{"chart_id":"chart_sales","title":"销售趋势","chart_type":"line","series":[{"data":[100,120]}]}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(result, "请重新调用 report_create_chart") {
		t.Fatalf("expected structured feedback instead of retry prompt, got %q", result)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("expected json feedback, got %v", err)
	}
	if payload["ok"] != false {
		t.Fatalf("expected ok=false, got %#v", payload["ok"])
	}
	if payload["error_code"] != "invalid_chart_spec" {
		t.Fatalf("unexpected error_code: %#v", payload["error_code"])
	}
}

func TestCreateChartToolRejectsUnknownChartType(t *testing.T) {
	t.Parallel()

	tool := &CreateChartTool{ReportState: &ReportState{}}
	result, err := tool.Execute(json.RawMessage(`{"chart_id":"chart_sales","title":"销售趋势","chart_type":"scatter","categories":["1月"],"series":[{"data":[100]}]}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("expected json feedback, got %v", err)
	}
	if payload["error_code"] != "invalid_chart_spec" {
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
		"chart_type":"bar",
		"categories":["1月"],
		"series":[{"data":[100]}]
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

func TestCreateChartToolBuildsOptionFromDSL(t *testing.T) {
	t.Parallel()

	tool := &CreateChartTool{ReportState: &ReportState{}}
	result, err := tool.Execute(json.RawMessage(`{
		"chart_id":"chart_sales",
		"title":"销售趋势",
		"chart_type":"line",
		"categories":["1月","2月","3月"],
		"series":[{"name":"销售额","data":[100,120,140],"smooth":true}]
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
	if len(tool.ReportState.Charts) != 1 {
		t.Fatalf("expected 1 chart, got %d", len(tool.ReportState.Charts))
	}

	var option map[string]interface{}
	if err := json.Unmarshal(tool.ReportState.Charts[0].Option, &option); err != nil {
		t.Fatalf("unmarshal option: %v", err)
	}
	if option["tooltip"] == nil {
		t.Fatalf("expected default tooltip in option: %#v", option)
	}
}

func TestCreateChartToolAcceptsRawOption(t *testing.T) {
	t.Parallel()

	tool := &CreateChartTool{ReportState: &ReportState{}}
	result, err := tool.Execute(json.RawMessage(`{
		"chart_id":"chart_custom",
		"title":"自定义图",
		"option":{
			"tooltip":{"trigger":"item"},
			"xAxis":{"type":"category","data":["A","B"]},
			"yAxis":{"type":"value"},
			"series":[{"type":"line","data":[1,2]}]
		}
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
	if len(tool.ReportState.Charts) != 1 {
		t.Fatalf("expected 1 chart, got %d", len(tool.ReportState.Charts))
	}

	var option map[string]interface{}
	if err := json.Unmarshal(tool.ReportState.Charts[0].Option, &option); err != nil {
		t.Fatalf("unmarshal option: %v", err)
	}
	if option["tooltip"] == nil || option["xAxis"] == nil {
		t.Fatalf("expected raw option to be preserved: %#v", option)
	}
}
