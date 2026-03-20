package handler

import (
	"context"
	"log"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/ifnodoraemon/openDataAnalysis/agent"
	"github.com/ifnodoraemon/openDataAnalysis/auth"
	"github.com/ifnodoraemon/openDataAnalysis/domain"
	"github.com/ifnodoraemon/openDataAnalysis/session"
)

type beforeUserRunHook func(context.Context, *session.Session, agent.UserMessage) error

func runBeforeUserRunHooks(ctx context.Context, sess *session.Session, userMsg agent.UserMessage, hooks ...beforeUserRunHook) error {
	for _, hook := range hooks {
		if hook == nil {
			continue
		}
		if err := hook(ctx, sess, userMsg); err != nil {
			return err
		}
	}
	return nil
}

func prepareUserRunHook(loader session.ReportSnapshotLoader) beforeUserRunHook {
	return func(ctx context.Context, sess *session.Session, userMsg agent.UserMessage) error {
		return sess.PrepareUserRun(ctx, userMsg, loader)
	}
}

type runtimeEventHook func(runtimeEventScope, agent.WSEvent)

type runtimeEventScope struct {
	session           *session.Session
	runID             string
	emitReportPreview func()
	finalizeReport    func()
	setRunStatus      func(domain.RunStatus, *string)
	setRunSummary     func(string)
}

type runtimeEventDispatcher struct {
	deliver func(agent.WSEvent)
	scope   runtimeEventScope
	hooks   []runtimeEventHook
}

func newRuntimeEventDispatcher(ctx context.Context, conn *websocket.Conn, writeMu *sync.Mutex, sess *session.Session, identity auth.Identity, runID string) runtimeEventDispatcher {
	deliver := func(ev agent.WSEvent) {
		sendSessionEvent(conn, writeMu, sess.ID, runID, ev)
		saveEventToDB(ctx, sess.WorkspaceID, sess.ID, runID, ev)
	}

	scope := runtimeEventScope{
		session: sess,
		runID:   runID,
		emitReportPreview: func() {
			emitReportPreviewUpdate(ctx, conn, writeMu, sess.ID, sess.WorkspaceID, runID, sess.ReportState)
		},
		finalizeReport: func() {
			finalizeAndPersistReport(ctx, conn, writeMu, sess, identity, runID)
		},
		setRunStatus: func(status domain.RunStatus, errMsg *string) {
			_ = withPersistenceContext(ctx, func(persistCtx context.Context) error {
				return runRepo.UpdateStatus(persistCtx, runID, status, errMsg)
			})
		},
		setRunSummary: func(summary string) {
			trimmed := strings.TrimSpace(summary)
			_ = withPersistenceContext(ctx, func(persistCtx context.Context) error {
				return runRepo.UpdateSummary(persistCtx, runID, trimmed)
			})
		},
	}

	return runtimeEventDispatcher{
		deliver: deliver,
		scope:   scope,
		hooks: []runtimeEventHook{
			reportLifecycleHook,
			runLifecycleHook,
			runLoggingHook,
		},
	}
}

func (d runtimeEventDispatcher) Emit(ev agent.WSEvent) {
	d.deliver(ev)
	for _, hook := range d.hooks {
		hook(d.scope, ev)
	}
}

func reportLifecycleHook(scope runtimeEventScope, ev agent.WSEvent) {
	if ev.Type != agent.EventToolResult {
		return
	}
	result, ok := ev.Data.(agent.ToolResultData)
	if !ok {
		return
	}
	if shouldEmitReportPreview(result.Name) && scope.emitReportPreview != nil {
		scope.emitReportPreview()
	}
	if result.Name == "report_finalize" && result.Success && scope.finalizeReport != nil {
		scope.finalizeReport()
	}
}

func runLifecycleHook(scope runtimeEventScope, ev agent.WSEvent) {
	if scope.session == nil || strings.TrimSpace(scope.runID) == "" {
		return
	}

	switch ev.Type {
	case agent.EventUserRequestInput:
		scope.session.SuspendRun(scope.runID)
		if scope.setRunStatus != nil {
			scope.setRunStatus(domain.RunStatusWaitingUserInput, nil)
		}
	case agent.EventRunCompleted:
		scope.session.FinishRun(scope.runID, "completed")
		if scope.setRunStatus != nil {
			scope.setRunStatus(domain.RunStatusCompleted, nil)
		}
		if complete, ok := ev.Data.(agent.CompleteData); ok && scope.setRunSummary != nil {
			scope.setRunSummary(complete.Summary)
		}
	case agent.EventRunCancelled:
		scope.session.FinishRun(scope.runID, "cancelled")
		if scope.setRunStatus != nil {
			scope.setRunStatus(domain.RunStatusCancelled, nil)
		}
	case agent.EventError:
		scope.session.FinishRun(scope.runID, "failed")
		if scope.setRunStatus == nil {
			return
		}
		if errData, ok := ev.Data.(agent.ErrorData); ok {
			msg := errData.Message
			scope.setRunStatus(domain.RunStatusFailed, &msg)
			return
		}
		scope.setRunStatus(domain.RunStatusFailed, nil)
	}
}

func runLoggingHook(scope runtimeEventScope, ev agent.WSEvent) {
	if scope.session == nil || strings.TrimSpace(scope.runID) == "" {
		return
	}

	switch ev.Type {
	case agent.EventUserRequestInput:
		log.Printf("run suspended waiting_user_input run_id=%s session_id=%s", scope.runID, scope.session.ID)
	case agent.EventRunCompleted:
		if complete, ok := ev.Data.(agent.CompleteData); ok {
			log.Printf("run completed run_id=%s session_id=%s summary_chars=%d", scope.runID, scope.session.ID, len([]rune(strings.TrimSpace(complete.Summary))))
			return
		}
		log.Printf("run completed run_id=%s session_id=%s", scope.runID, scope.session.ID)
	case agent.EventRunCancelled:
		log.Printf("run cancelled run_id=%s session_id=%s", scope.runID, scope.session.ID)
	case agent.EventError:
		if errData, ok := ev.Data.(agent.ErrorData); ok {
			log.Printf("run failed run_id=%s session_id=%s error=%q", scope.runID, scope.session.ID, clipLogText(errData.Message, 240))
			return
		}
		log.Printf("run failed run_id=%s session_id=%s", scope.runID, scope.session.ID)
	}
}
