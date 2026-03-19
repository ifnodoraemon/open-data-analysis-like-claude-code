package handler

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ifnodoraemon/openDataAnalysis/agent"
	"github.com/ifnodoraemon/openDataAnalysis/domain"
)

type captureMessageRepo struct {
	createCtxErr error
	created      []*domain.RunMessage
}

func (r *captureMessageRepo) Create(ctx context.Context, msg *domain.RunMessage) error {
	r.createCtxErr = ctx.Err()
	copy := *msg
	r.created = append(r.created, &copy)
	return nil
}

func (r *captureMessageRepo) ListByRun(context.Context, string) ([]domain.RunMessage, error) {
	return nil, nil
}

func (r *captureMessageRepo) ListRecentByRun(context.Context, string, int) ([]domain.RunMessage, error) {
	return nil, nil
}

func TestSaveEventToDBDetachesCancelledContext(t *testing.T) {
	previous := messageRepo
	repo := &captureMessageRepo{}
	messageRepo = repo
	t.Cleanup(func() {
		messageRepo = previous
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	saveEventToDB(ctx, "ws_1", "sess_1", "run_1", agent.WSEvent{
		Type: agent.EventRunCompleted,
		Data: agent.CompleteData{Summary: "done"},
	})

	if len(repo.created) != 1 {
		t.Fatalf("expected one persisted message, got %d", len(repo.created))
	}
	if repo.createCtxErr != nil {
		t.Fatalf("expected detached context, got %v", repo.createCtxErr)
	}
	if repo.created[0].Content != "done" {
		t.Fatalf("unexpected persisted content: %#v", repo.created[0])
	}
}

func TestWriteRepoLookupErrorRecognizesNotFound(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	handled := writeRepoLookupError(recorder, sql.ErrNoRows, "任务不存在")
	if !handled {
		t.Fatal("expected helper to handle sql.ErrNoRows")
	}
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", recorder.Code)
	}
}

func TestWriteRepoLookupErrorRecognizesInternalError(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	handled := writeRepoLookupError(recorder, errors.New("database is locked"), "任务不存在")
	if !handled {
		t.Fatal("expected helper to handle internal error")
	}
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", recorder.Code)
	}
}
