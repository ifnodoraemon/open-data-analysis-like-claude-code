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
