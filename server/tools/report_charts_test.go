package tools

import (
	"encoding/json"
	"testing"
)

func TestApplyReportChartMutationReplacesExistingChart(t *testing.T) {
	t.Parallel()

	state := &ReportState{
		Charts: []ChartData{
			{ID: "chart_sales", Option: json.RawMessage(`{"series":[{"data":[1]}]}`)},
		},
	}

	result, err := applyReportChartMutation(state, nil, createChartParams{
		ChartID:    "chart_sales",
		Title:      "销售趋势",
		ChartType:  "bar",
		Categories: []string{"1月"},
		Series: []chartSeriesInput{
			{Data: []float64{100}},
		},
	})
	if err != nil {
		t.Fatalf("applyReportChartMutation: %v", err)
	}
	if !result.Replaced {
		t.Fatalf("expected chart replacement, got %#v", result)
	}
	if len(state.Charts) != 1 {
		t.Fatalf("expected chart list to stay at 1, got %d", len(state.Charts))
	}
	if !state.NeedsFinalize {
		t.Fatal("expected chart mutation to mark report as needing finalize")
	}
}

func TestCreateChartToolRejectsEditScopeViolation(t *testing.T) {
	t.Parallel()

	tool := &CreateChartTool{
		ReportState: &ReportState{},
		EditState: &ReportEditState{
			Mode:                "regenerate_block",
			PreserveOtherBlocks: true,
			AllowedChartIDs: map[string]struct{}{
				"chart_allowed": {},
			},
		},
	}

	result, err := tool.Execute(json.RawMessage(`{
		"chart_id":"chart_blocked",
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
		t.Fatalf("expected json payload, got %v", err)
	}
	if payload["ok"] != false || payload["error_code"] != "edit_scope_violation" {
		t.Fatalf("unexpected scope payload: %#v", payload)
	}
}
