package session

import (
	"context"
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
