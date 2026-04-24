package handler

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"

	"github.com/ifnodoraemon/openDataAnalysis/agent"
	"github.com/ifnodoraemon/openDataAnalysis/domain"
	"github.com/ifnodoraemon/openDataAnalysis/session"
	"github.com/ifnodoraemon/openDataAnalysis/tools"
)

func TestRunBeforeUserRunHooksStopsOnError(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	order := make([]string, 0, 3)

	err := runBeforeUserRunHooks(context.Background(), &session.Session{}, agent.UserMessage{Content: "hi"},
		func(context.Context, *session.Session, agent.UserMessage) error {
			order = append(order, "first")
			return nil
		},
		func(context.Context, *session.Session, agent.UserMessage) error {
			order = append(order, "second")
			return boom
		},
		func(context.Context, *session.Session, agent.UserMessage) error {
			order = append(order, "third")
			return nil
		},
	)

	if !errors.Is(err, boom) {
		t.Fatalf("expected hook error to bubble up, got %v", err)
	}
	if strings.Join(order, ",") != "first,second" {
		t.Fatalf("expected hooks to stop on first error, got %v", order)
	}
}

func TestReportLifecycleHookTriggersDerivedReportEffects(t *testing.T) {
	t.Parallel()

	var previewCount, finalizeCount int
	scope := runtimeEventScope{
		emitReportPreview: func() { previewCount++ },
		finalizeReport:    func() error { finalizeCount++; return nil },
	}

	reportLifecycleHook(scope, agent.WSEvent{
		Type: agent.EventToolResult,
		Data: agent.ToolResultData{Name: "report_manage_blocks", Success: true},
	})
	reportLifecycleHook(scope, agent.WSEvent{
		Type: agent.EventToolResult,
		Data: agent.ToolResultData{Name: "report_finalize", Success: true},
	})
	reportLifecycleHook(scope, agent.WSEvent{
		Type: agent.EventToolResult,
		Data: agent.ToolResultData{Name: "report_finalize", Success: false},
	})

	if previewCount != 1 {
		t.Fatalf("expected preview hook to run once, got %d", previewCount)
	}
	if finalizeCount != 1 {
		t.Fatalf("expected finalize hook to run once, got %d", finalizeCount)
	}
}

func TestReportLifecycleHookFailsRunOnFinalizeError(t *testing.T) {
	t.Parallel()

	boom := errors.New("storage offline")
	sess := &session.Session{
		ActiveRun:   &session.RunState{RunID: "run_99", Status: "running"},
		ReportState: &tools.ReportState{NeedsFinalize: false},
	}

	var updatedStatus domain.RunStatus
	var updatedMsg string
	scope := runtimeEventScope{
		session:        sess,
		runID:          "run_99",
		finalizeReport: func() error { return boom },
		setRunStatus: func(status domain.RunStatus, msg *string) {
			updatedStatus = status
			if msg != nil {
				updatedMsg = *msg
			}
		},
	}

	reportLifecycleHook(scope, agent.WSEvent{
		Type: agent.EventToolResult,
		Data: agent.ToolResultData{Name: "report_finalize", Success: true, Result: `{"report_title":"零售分析","author":"AI"}`},
	})

	if sess.ActiveRun != nil && sess.ActiveRun.Status != "failed" {
		t.Fatalf("expected session run status to be marked failed in memory, got %v", sess.ActiveRun.Status)
	}
	if updatedStatus != domain.RunStatusFailed {
		t.Fatalf("expected persistence status to be Failed, got %v", updatedStatus)
	}
	if !strings.Contains(updatedMsg, "storage offline") {
		t.Fatalf("expected error message to contain 'storage offline', got %q", updatedMsg)
	}
	if sess.ReportState == nil || !sess.ReportState.NeedsFinalize {
		t.Fatalf("expected finalize failure to roll report state back to draft")
	}
	if sess.ReportState.FinalTitle != "零售分析" || sess.ReportState.FinalAuthor != "AI" {
		t.Fatalf("expected finalize metadata to be preserved for retry, got %#v", sess.ReportState)
	}
}

func TestRunLifecycleHookUpdatesSessionStateAndPersistenceCallbacks(t *testing.T) {
	t.Parallel()

	statuses := make([]domain.RunStatus, 0, 2)
	summaries := make([]string, 0, 1)
	sess := &session.Session{
		ActiveRun: &session.RunState{RunID: "run_1", Status: "running"},
		EditState: &tools.ReportEditState{},
	}
	scope := runtimeEventScope{
		session: sess,
		runID:   "run_1",
		setRunStatus: func(status domain.RunStatus, _ *string) {
			statuses = append(statuses, status)
		},
		setRunSummary: func(summary string) {
			summaries = append(summaries, strings.TrimSpace(summary))
		},
	}

	runLifecycleHook(scope, agent.WSEvent{
		Type: agent.EventUserRequestInput,
		Data: agent.AskUserData{Question: "next?"},
	})
	if runID, waiting := sess.GetWaitingRunID(); !waiting || runID != "run_1" {
		t.Fatalf("expected run to enter waiting state, got waiting=%t runID=%q", waiting, runID)
	}

	runLifecycleHook(scope, agent.WSEvent{
		Type: agent.EventRunCompleted,
		Data: agent.CompleteData{Summary: " done "},
	})
	if len(statuses) != 2 || statuses[0] != domain.RunStatusWaitingUserInput || statuses[1] != domain.RunStatusCompleted {
		t.Fatalf("unexpected status updates: %v", statuses)
	}
	if len(summaries) != 1 || summaries[0] != "done" {
		t.Fatalf("unexpected summary updates: %v", summaries)
	}
	if sess.ActiveRun != nil {
		t.Fatalf("expected run to be cleared after completion, got %#v", sess.ActiveRun)
	}
}

func TestRunLoggingHookWritesEventLogs(t *testing.T) {
	var buf bytes.Buffer
	originalWriter := log.Writer()
	originalFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	defer log.SetOutput(originalWriter)
	defer log.SetFlags(originalFlags)

	scope := runtimeEventScope{
		session: &session.Session{ID: "sess_1"},
		runID:   "run_1",
	}

	runLoggingHook(scope, agent.WSEvent{
		Type: agent.EventUserRequestInput,
		Data: agent.AskUserData{Question: "next?"},
	})
	runLoggingHook(scope, agent.WSEvent{
		Type: agent.EventRunCompleted,
		Data: agent.CompleteData{Summary: " done "},
	})
	runLoggingHook(scope, agent.WSEvent{
		Type: agent.EventError,
		Data: agent.ErrorData{Message: "boom"},
	})

	output := buf.String()
	if !strings.Contains(output, "run suspended waiting_user_input run_id=run_1 session_id=sess_1") {
		t.Fatalf("expected suspend log, got %q", output)
	}
	if !strings.Contains(output, "run completed run_id=run_1 session_id=sess_1 summary_chars=4") {
		t.Fatalf("expected completion log, got %q", output)
	}
	if !strings.Contains(output, `run failed run_id=run_1 session_id=sess_1 error="boom"`) {
		t.Fatalf("expected failure log, got %q", output)
	}
}

func TestRuntimeEventDispatcherRoutesChildRunEventsWithoutRootHooks(t *testing.T) {
	t.Parallel()

	delivered := make([]string, 0, 2)
	hooked := false
	dispatcher := runtimeEventDispatcher{
		deliver: func(ev agent.WSEvent) {
			delivered = append(delivered, "root:"+ev.Type)
		},
		deliverToRun: func(runID string, ev agent.WSEvent) {
			delivered = append(delivered, runID+":"+ev.Type)
		},
		scope: runtimeEventScope{
			runID: "root_run",
		},
		hooks: []runtimeEventHook{
			func(runtimeEventScope, agent.WSEvent) {
				hooked = true
			},
		},
	}

	dispatcher.Emit(agent.WSEvent{
		Type:  agent.EventAssistantStatus,
		RunID: "child_run",
		Data:  agent.AssistantStatusData{Content: "child"},
	})

	if len(delivered) != 1 || delivered[0] != "child_run:assistant_status" {
		t.Fatalf("unexpected deliveries: %v", delivered)
	}
	if hooked {
		t.Fatal("expected child run events to bypass root hooks")
	}
}

func TestRuntimeEventDispatcherStillEmitsChildReportPreview(t *testing.T) {
	t.Parallel()

	previewRuns := make([]string, 0, 1)
	dispatcher := runtimeEventDispatcher{
		deliver:      func(agent.WSEvent) {},
		deliverToRun: func(string, agent.WSEvent) {},
		emitChildPreview: func(runID string) {
			previewRuns = append(previewRuns, runID)
		},
		scope: runtimeEventScope{
			runID: "root_run",
		},
	}

	dispatcher.Emit(agent.WSEvent{
		Type:  agent.EventToolResult,
		RunID: "child_run",
		Data:  agent.ToolResultData{Name: "report_manage_blocks", Success: true},
	})

	if len(previewRuns) != 1 || previewRuns[0] != "child_run" {
		t.Fatalf("expected child preview emission, got %v", previewRuns)
	}
}
