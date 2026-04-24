package agent

import (
	"testing"

	"github.com/ifnodoraemon/openDataAnalysis/tools"
)

func TestParseTurnResolutionFromCodeFence(t *testing.T) {
	t.Parallel()

	raw := "```json\n{\"artifact\":\"report\",\"operation\":\"revise\",\"scope\":\"whole_report\",\"mutation_requested\":true,\"needs_clarification\":false,\"confidence\":0.92}\n```"
	res, err := parseTurnResolution(raw)
	if err != nil {
		t.Fatalf("parse turn resolution: %v", err)
	}
	if res.Artifact != TurnArtifactReport || res.Operation != TurnOperationRevise || res.Scope != TurnScopeWholeReport {
		t.Fatalf("unexpected resolution: %#v", res)
	}
	if !res.MutationRequested || res.NeedsClarification {
		t.Fatalf("unexpected flags: %#v", res)
	}
}

func TestTurnResolutionMaterializeEditContext(t *testing.T) {
	t.Parallel()

	res := TurnResolution{
		Artifact:          TurnArtifactReport,
		Operation:         TurnOperationRevise,
		Scope:             TurnScopeWholeReport,
		MutationRequested: true,
		Confidence:        0.91,
	}
	edit := res.MaterializeEditContext()
	if edit == nil {
		t.Fatal("expected whole-report revise to materialize edit context")
	}
	if edit.Mode != "revise_report" || edit.PreserveOtherBlocks {
		t.Fatalf("unexpected edit context: %#v", edit)
	}

	blockRes := TurnResolution{
		Artifact:          TurnArtifactReport,
		Operation:         TurnOperationRevise,
		Scope:             TurnScopeBlock,
		MutationRequested: true,
		Confidence:        0.95,
	}
	if blockEdit := blockRes.MaterializeEditContext(); blockEdit != nil {
		t.Fatalf("did not expect block-scope resolution to auto-materialize edit context: %#v", blockEdit)
	}

	layoutRes := TurnResolution{
		Artifact:          TurnArtifactReport,
		Operation:         TurnOperationConfigureLayout,
		Scope:             TurnScopeLayout,
		MutationRequested: true,
		Confidence:        0.9,
	}
	layoutEdit := layoutRes.MaterializeEditContext()
	if layoutEdit == nil || layoutEdit.Mode != "configure_layout" {
		t.Fatalf("expected layout configure to materialize edit context, got %#v", layoutEdit)
	}
}

func TestTurnResolutionRuntimeContextBlock(t *testing.T) {
	t.Parallel()

	res := TurnResolution{
		Artifact:          TurnArtifactReport,
		Operation:         TurnOperationRevise,
		Scope:             TurnScopeBlock,
		TargetRefHint:     "结论部分",
		MutationRequested: true,
		Confidence:        0.84,
	}
	block := res.RuntimeContextBlock()
	if block == nil {
		t.Fatal("expected runtime block")
	}
	if block.Name != "current_turn_resolution" || block.Role != "developer" {
		t.Fatalf("unexpected runtime block: %#v", block)
	}
	if block.Content == "" {
		t.Fatalf("expected non-empty runtime block content")
	}
}

func TestGroundTurnResolutionBlockByHint(t *testing.T) {
	t.Parallel()

	res := TurnResolution{
		Artifact:          TurnArtifactReport,
		Operation:         TurnOperationRevise,
		Scope:             TurnScopeBlock,
		TargetRefHint:     "结论部分",
		MutationRequested: true,
		Confidence:        0.88,
	}
	state := &tools.ReportState{
		Blocks: []tools.ReportBlock{
			{ID: "blk_intro", Title: "引言"},
			{ID: "blk_conclusion", Title: "结论"},
		},
	}
	grounded := GroundTurnResolution(res, state)
	if grounded.TargetBlockID != "blk_conclusion" || grounded.Ambiguous {
		t.Fatalf("unexpected grounded resolution: %#v", grounded)
	}
	edit := grounded.MaterializeEditContext(nil)
	if edit == nil || edit.BlockID != "blk_conclusion" || !edit.PreserveOtherBlocks {
		t.Fatalf("unexpected grounded edit context: %#v", edit)
	}
}

func TestGroundTurnResolutionBlockAmbiguous(t *testing.T) {
	t.Parallel()

	res := TurnResolution{
		Artifact:          TurnArtifactReport,
		Operation:         TurnOperationRevise,
		Scope:             TurnScopeBlock,
		TargetRefHint:     "结论部分",
		MutationRequested: true,
		Confidence:        0.88,
	}
	state := &tools.ReportState{
		Blocks: []tools.ReportBlock{
			{ID: "blk_conclusion_a", Title: "结论"},
			{ID: "blk_conclusion_b", Title: "结论"},
		},
	}
	grounded := GroundTurnResolution(res, state)
	if !grounded.Ambiguous || grounded.TargetBlockID != "" {
		t.Fatalf("expected ambiguous grounding, got %#v", grounded)
	}
	if edit := grounded.MaterializeEditContext(nil); edit != nil {
		t.Fatalf("did not expect ambiguous grounding to materialize edit context: %#v", edit)
	}
}

func TestGroundTurnResolutionChartByHint(t *testing.T) {
	t.Parallel()

	res := TurnResolution{
		Artifact:          TurnArtifactReport,
		Operation:         TurnOperationReviseChart,
		Scope:             TurnScopeChart,
		TargetRefHint:     "销售趋势图",
		MutationRequested: true,
		Confidence:        0.9,
	}
	state := &tools.ReportState{
		Blocks: []tools.ReportBlock{
			{ID: "blk_sales", Kind: "chart", Title: "销售趋势图", ChartID: "chart_sales"},
		},
		Charts: []tools.ChartData{
			{ID: "chart_sales"},
			{ID: "chart_other"},
		},
	}

	grounded := GroundTurnResolution(res, state)
	if grounded.TargetChartID != "chart_sales" || grounded.Ambiguous {
		t.Fatalf("unexpected grounded chart resolution: %#v", grounded)
	}
	edit := grounded.MaterializeEditContext(&TurnContext{ReportTargetRunID: "run_chart_1"})
	if edit == nil {
		t.Fatal("expected chart grounding to materialize edit context")
	}
	if edit.Mode != "revise_chart" || edit.ChartID != "chart_sales" || !edit.PreserveOtherBlocks {
		t.Fatalf("unexpected chart edit context: %#v", edit)
	}
	if edit.BlockID != "" {
		t.Fatalf("did not expect chart-scoped edit to materialize block target: %#v", edit)
	}
	if edit.TargetRunID != "run_chart_1" {
		t.Fatalf("expected target run to carry through, got %#v", edit)
	}
}
