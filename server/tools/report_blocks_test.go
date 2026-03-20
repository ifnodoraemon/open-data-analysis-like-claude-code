package tools

import "testing"

func TestApplyReportBlockMutationPreservesExistingSourcesOnUpsert(t *testing.T) {
	t.Parallel()

	state := &ReportState{
		Blocks: []ReportBlock{
			{
				ID:      "summary",
				Kind:    "markdown",
				Title:   "摘要",
				Content: "旧内容",
				Sources: []EvidenceRef{
					{Kind: "sql", SQL: "select 1"},
				},
			},
		},
	}

	result, err := applyReportBlockMutation(state, nil, reportBlockMutationParams{
		Action:    "upsert",
		BlockID:   "summary",
		BlockKind: "markdown",
		Title:     "摘要",
		Content:   "新内容",
	})
	if err != nil {
		t.Fatalf("applyReportBlockMutation: %v", err)
	}

	if result.BlockID != "summary" || result.BlockKind != "markdown" {
		t.Fatalf("unexpected mutation result: %#v", result)
	}
	if len(state.Blocks) != 1 || state.Blocks[0].Content != "新内容" {
		t.Fatalf("expected block to be updated in place, got %#v", state.Blocks)
	}
	if len(state.Blocks[0].Sources) != 1 || state.Blocks[0].Sources[0].SQL != "select 1" {
		t.Fatalf("expected existing sources to be preserved, got %#v", state.Blocks[0].Sources)
	}
	if !state.NeedsFinalize {
		t.Fatal("expected mutation to mark report as needing finalize")
	}
}
