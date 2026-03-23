package session

import (
	"context"
	"strings"
	"testing"

	"github.com/ifnodoraemon/openDataAnalysis/agent"
	"github.com/ifnodoraemon/openDataAnalysis/domain"
	"github.com/ifnodoraemon/openDataAnalysis/tools"
)

type stubSnapshotLoader struct {
	snapshot *domain.ReportSnapshot
	err      error
	calls    int
	runID    string
}

func (s *stubSnapshotLoader) LoadReportSnapshot(_ context.Context, _, _, _, runID string) (*domain.ReportSnapshot, error) {
	s.calls++
	s.runID = runID
	return s.snapshot, s.err
}

func TestPrepareUserRunLoadsSnapshotThroughLoader(t *testing.T) {
	t.Parallel()

	sess := &Session{
		ID:          "sess_1",
		WorkspaceID: "ws_1",
		UserID:      "user_1",
		ReportState: &tools.ReportState{},
		EditState:   &tools.ReportEditState{},
	}
	loader := &stubSnapshotLoader{
		snapshot: &domain.ReportSnapshot{
			Title:  "旧报告",
			Author: "tester",
			Blocks: []domain.ReportSnapshotBlock{
				{ID: "b1", Kind: "chart", Title: "趋势", ChartID: "chart_1"},
			},
			Charts: []domain.ReportSnapshotChart{
				{ID: "chart_1"},
			},
		},
	}

	err := sess.PrepareUserRun(context.Background(), agent.UserMessage{
		Content: "重写这一段",
		EditContext: &agent.ReportEditContext{
			Mode:                "regenerate_block",
			TargetRunID:         "run_123",
			BlockID:             "b1",
			PreserveOtherBlocks: true,
		},
	}, loader)
	if err != nil {
		t.Fatalf("prepare user run: %v", err)
	}
	if loader.calls != 1 || loader.runID != "run_123" {
		t.Fatalf("expected loader to be called for target run, calls=%d runID=%q", loader.calls, loader.runID)
	}
	if sess.ReportState.FinalTitle != "旧报告" || len(sess.ReportState.Blocks) != 1 {
		t.Fatalf("expected snapshot to be loaded into report state: %#v", sess.ReportState)
	}
	if sess.EditState.TargetBlockID != "b1" || len(sess.EditState.AllowedChartIDs) != 1 {
		t.Fatalf("expected edit state to refresh from loaded snapshot: %#v", sess.EditState)
	}
}

func TestPrepareUserRunWithoutEditTargetDoesNotLoadSnapshot(t *testing.T) {
	t.Parallel()

	sess := &Session{
		ID:          "sess_1",
		WorkspaceID: "ws_1",
		UserID:      "user_1",
		ReportState: &tools.ReportState{},
		EditState:   &tools.ReportEditState{},
	}
	loader := &stubSnapshotLoader{}

	err := sess.PrepareUserRun(context.Background(), agent.UserMessage{
		Content: "继续分析",
	}, loader)
	if err != nil {
		t.Fatalf("prepare user run: %v", err)
	}
	if loader.calls != 0 {
		t.Fatalf("expected loader to remain unused, calls=%d", loader.calls)
	}
}

func TestSessionRuntimeVars(t *testing.T) {
	t.Parallel()

	sess := &Session{
		ID: "test-sess",
	}

	// 1. Test EditScope
	sess.EditState = &tools.ReportEditState{
		Mode:          "append",
		TargetBlockID: "b1",
		SelectionText: "hello",
	}
	vars := sess.RuntimeVars()
	if len(vars) != 1 || vars[0].Name != "active_edit_scope" {
		t.Fatalf("expected 1 active_edit_scope, got %v", vars)
	}
	if !strings.Contains(vars[0].Content, "Mode: append") {
		t.Errorf("missing Mode in edit scope fact: %s", vars[0].Content)
	}
	if strings.Contains(vars[0].Content, "请仅在允许的边界") {
		t.Errorf("imperative hint should not be present in edit scope")
	}

	// 2. Test Subgoals
	sess.EditState = nil
	sess.Subgoals = agent.NewSubgoalManager()
	sess.Subgoals.AddGoal("do work", "")
	vars = sess.RuntimeVars()
	if len(vars) != 1 || vars[0].Name != "active_subgoals" {
		t.Fatalf("expected 1 active_subgoals, got %v", vars)
	}
	if !strings.Contains(vars[0].Content, "] do work (pending)") {
		t.Errorf("missing subgoal in fact: %s", vars[0].Content)
	}
	if strings.Contains(vars[0].Content, "请记得更新状态") {
		t.Errorf("imperative hint should not be present in subgoals")
	}
}
