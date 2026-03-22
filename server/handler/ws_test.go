package handler

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/ifnodoraemon/openDataAnalysis/agent"
	"github.com/ifnodoraemon/openDataAnalysis/domain"
)

type captureMessageRepo struct {
	mu           sync.Mutex
	createCtxErr error
	created      []*domain.RunMessage
}

func (r *captureMessageRepo) Create(ctx context.Context, msg *domain.RunMessage) error {
	r.mu.Lock()
	defer r.mu.Unlock()
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

func (r *captureMessageRepo) snapshot() ([]*domain.RunMessage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*domain.RunMessage, len(r.created))
	copy(out, r.created)
	return out, r.createCtxErr
}

func TestSaveEventToDBDetachesCancelledContext(t *testing.T) {
	previous := messageRepo
	prevQ := eventPersistQueue
	repo := &captureMessageRepo{}
	messageRepo = repo
	t.Cleanup(func() {
		messageRepo = previous
		eventPersistQueue = prevQ
	})

	// 启动专属测试用的异步持久化 worker，并获取 shutdown 函数
	shutdown := startEventPersistWorker()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 已取消的 context

	saveEventToDB(ctx, "ws_1", "sess_1", "run_1", agent.WSEvent{
		Type: agent.EventRunCompleted,
		Data: agent.CompleteData{Summary: "done"},
	})

	// shutdown 关闭队列并等待 worker goroutine 完全退出（含 saveEventToDBSync 返回）
	shutdown()

	created, ctxErr := repo.snapshot()

	if len(created) != 1 {
		t.Fatalf("expected one persisted message, got %d", len(created))
	}
	if ctxErr != nil {
		t.Fatalf("expected detached context (nil Err), got %v", ctxErr)
	}
	if created[0].Content != "done" {
		t.Fatalf("unexpected persisted content: %#v", created[0])
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
