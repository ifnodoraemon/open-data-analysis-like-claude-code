package handler

import (
	"context"
	"database/sql"
	"strings"

	"github.com/ifnodoraemon/openDataAnalysis/domain"
	"github.com/ifnodoraemon/openDataAnalysis/session"
)

func sessionExists(ctx context.Context, sessionID string) (bool, error) {
	if sessionRepo == nil || strings.TrimSpace(sessionID) == "" {
		return false, nil
	}
	_, err := sessionRepo.GetByID(ctx, sessionID)
	if err == nil {
		return true, nil
	}
	if err == sql.ErrNoRows {
		return false, nil
	}
	return false, err
}

func shouldHydrateSessionFromPersistence(ctx context.Context, workspaceID, userID, sessionID string) (bool, error) {
	if strings.TrimSpace(sessionID) == "" || sessionManager == nil {
		return false, nil
	}
	if _, ok, err := sessionManager.Peek(sessionID, workspaceID, userID); err != nil {
		return false, err
	} else if ok {
		return false, nil
	}
	return sessionExists(ctx, sessionID)
}

func hydrateSessionFromPersistence(ctx context.Context, sess *session.Session) error {
	if sess == nil {
		return nil
	}
	memory, subgoals, reportSnapshot, _, editState := loadSessionRuntimeStateFromPersistence(ctx, sess.ID)
	sess.LoadRuntimeState(memory, subgoals)
	if reportSnapshot != nil {
		sess.LoadReportSnapshot(reportSnapshot)
	}
	if editState != nil && editState.Active && editState.EditContext != nil {
		sess.ConfigureEditState(editState.EditContext)
	} else {
		sess.ClearEditState()
	}
	return nil
}

func recoverStaleSessionRuns(ctx context.Context, sessionID string) error {
	if runRepo == nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	if sessionManager != nil && sessionManager.IsSessionLive(sessionID) {
		return nil
	}

	roots, err := runRepo.ListBySession(ctx, sessionID, -1)
	if err != nil {
		return err
	}
	for _, run := range roots {
		if err := recoverStaleRunTree(ctx, run); err != nil {
			return err
		}
	}
	return nil
}

func recoverStaleRunTree(ctx context.Context, run domain.AnalysisRun) error {
	if run.Status == domain.RunStatusRunning || run.Status == domain.RunStatusWaitingUserInput {
		errMsg := "task cannot be restored after service restart, automatically marked as failed"
		if run.ErrorMessage == nil || strings.TrimSpace(*run.ErrorMessage) != errMsg || run.Status != domain.RunStatusFailed {
			if err := runRepo.UpdateStatus(ctx, run.ID, domain.RunStatusFailed, &errMsg); err != nil {
				return err
			}
		}
	}

	children, err := runRepo.ListByParent(ctx, run.ID)
	if err != nil {
		return err
	}
	for _, child := range children {
		if err := recoverStaleRunTree(ctx, child); err != nil {
			return err
		}
	}
	return nil
}
