package agent

import "testing"

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
