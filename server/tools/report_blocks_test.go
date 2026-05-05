package tools

import (
	"encoding/json"
	"testing"
)

func TestApplyReportBlockMutationPreservesExistingSourcesOnUpsert(t *testing.T) {
	t.Parallel()

	state := &ReportState{
		Blocks: []ReportBlock{
			{
				ID:      "summary",
				Kind:    "markdown",
				Title:   "摘要",
				Content: "原内容",
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

func TestApplyReportBlockMutationPartialSelectionKeepsMetadata(t *testing.T) {
	t.Parallel()

	state := &ReportState{
		Blocks: []ReportBlock{
			{
				ID:      "summary",
				Kind:    "markdown",
				Title:   "摘要",
				Content: "前文。需要改写的句子。后文。",
				Sources: []EvidenceRef{{Kind: "sql", SQL: "select 1"}},
			},
		},
	}
	editState := &ReportEditState{
		Mode:                "regenerate_selection",
		TargetBlockID:       "summary",
		SelectionText:       "需要改写的句子",
		SelectionStart:      3,
		SelectionEnd:        10,
		SelectionRangeSet:   true,
		PreserveOtherBlocks: true,
	}
	editState.RefreshFromReportState(state)

	_, err := applyReportBlockMutation(state, editState, reportBlockMutationParams{
		Action:    "upsert",
		BlockID:   "summary",
		BlockKind: "markdown",
		Title:     "改名",
		Content:   "前文。新的句子。后文。",
	})
	if err == nil {
		t.Fatal("expected title mutation outside partial selection to be rejected")
	}

	result, err := applyReportBlockMutation(state, editState, reportBlockMutationParams{
		Action:    "upsert",
		BlockID:   "summary",
		BlockKind: "markdown",
		Title:     "摘要",
		Content:   "前文。新的句子。后文。",
	})
	if err != nil {
		t.Fatalf("expected content-only selection mutation to succeed: %v", err)
	}
	if result.BlockID != "summary" || state.Blocks[0].Content != "前文。新的句子。后文。" {
		t.Fatalf("unexpected mutation result=%#v state=%#v", result, state.Blocks)
	}
	if len(state.Blocks[0].Sources) != 1 || state.Blocks[0].Sources[0].SQL != "select 1" {
		t.Fatalf("expected existing sources to be preserved before scope check, got %#v", state.Blocks[0].Sources)
	}
}

func TestManageReportBlocksScopeFailureReturnsTargetFacts(t *testing.T) {
	t.Parallel()

	state := &ReportState{
		Blocks: []ReportBlock{
			{ID: "summary", Kind: "markdown", Title: "摘要", Content: "前文。需要改写的句子。后文。"},
		},
	}
	editState := &ReportEditState{
		Mode:                "regenerate_selection",
		TargetBlockID:       "summary",
		SelectionText:       "需要改写的句子",
		SelectionStart:      3,
		SelectionEnd:        10,
		SelectionRangeSet:   true,
		PreserveOtherBlocks: true,
	}
	editState.RefreshFromReportState(state)

	tool := &ManageReportBlocksTool{ReportState: state, EditState: editState}
	result, err := tool.Execute(json.RawMessage(`{
		"action":"append",
		"block_kind":"markdown",
		"title":"新段落",
		"content":"新的句子。"
	}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("expected json payload: %v", err)
	}
	if payload["ok"] != false || payload["error_code"] != "edit_scope_violation" {
		t.Fatalf("unexpected scope payload: %#v", payload)
	}
	actions, ok := payload["allowed_block_actions"].([]interface{})
	if payload["scope_kind"] != "partial_selection" || payload["target_block_id"] != "summary" || !ok || len(actions) != 1 || actions[0] != "upsert" {
		t.Fatalf("expected target selection facts in payload: %#v", payload)
	}
}

func TestManageReportBlocksToolAcceptsStringifiedSources(t *testing.T) {
	t.Parallel()

	state := &ReportState{}
	tool := &ManageReportBlocksTool{ReportState: state}
	result, err := tool.Execute(json.RawMessage(`{
		"action":"append",
		"block_kind":"markdown",
		"title":"摘要",
		"content":"结论内容",
		"sources":"[{\"kind\":\"sql\",\"sql\":\"select 1\",\"summary\":\"测试查询\"}]"
	}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("expected json payload: %v", err)
	}
	if payload["ok"] != true {
		t.Fatalf("expected ok=true, got %#v", payload["ok"])
	}
	if len(state.Blocks) != 1 || len(state.Blocks[0].Sources) != 1 {
		t.Fatalf("expected normalized sources on block, got %#v", state.Blocks)
	}
	if state.Blocks[0].Sources[0].SQL != "select 1" {
		t.Fatalf("unexpected source: %#v", state.Blocks[0].Sources[0])
	}
}
