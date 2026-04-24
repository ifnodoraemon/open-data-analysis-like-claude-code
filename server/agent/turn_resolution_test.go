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
