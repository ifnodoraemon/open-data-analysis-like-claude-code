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
