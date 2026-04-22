package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gorilla/websocket"
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

func TestWebSocketUpgraderSelectsAuthSubprotocol(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade failed: %v", err)
			return
		}
		defer conn.Close()
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	headers := http.Header{}
	headers.Set("Origin", "http://localhost")
	dialer := websocket.Dialer{Subprotocols: []string{"mcp-token", "token-example"}}

	conn, resp, err := dialer.Dial(wsURL, headers)
	if err != nil {
		if resp != nil {
			t.Fatalf("dial failed status=%s err=%v", resp.Status, err)
		}
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	if got := conn.Subprotocol(); got != "mcp-token" {
		t.Fatalf("expected selected subprotocol mcp-token, got %q", got)
	}
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

func TestSaveEventToDBSyncPersistsReportUpdatePayload(t *testing.T) {
	previous := messageRepo
	repo := &captureMessageRepo{}
	messageRepo = repo
	t.Cleanup(func() {
		messageRepo = previous
	})

	saveEventToDBSync("ws_1", "sess_1", "run_1", agent.WSEvent{
		Type: agent.EventReportUpdate,
		Data: agent.ReportUpdateData{
			HTML:  "<p>draft</p>",
			Title: "Draft",
			ReportSnapshot: &domain.ReportSnapshot{
				Title:         "Draft",
				NeedsFinalize: true,
				Blocks: []domain.ReportSnapshotBlock{
					{ID: "blk_1", Kind: "markdown", Content: "draft body"},
				},
			},
		},
	})

	created, _ := repo.snapshot()
	if len(created) != 1 {
		t.Fatalf("expected one persisted message, got %d", len(created))
	}

	var payload agent.ReportUpdateData
	if err := json.Unmarshal([]byte(created[0].Content), &payload); err != nil {
		t.Fatalf("expected report update payload JSON, got err=%v content=%q", err, created[0].Content)
	}
	if payload.HTML != "<p>draft</p>" || payload.Title != "Draft" {
		t.Fatalf("unexpected persisted report payload: %#v", payload)
	}
	if payload.ReportSnapshot == nil || !payload.ReportSnapshot.NeedsFinalize || len(payload.ReportSnapshot.Blocks) != 1 {
		t.Fatalf("expected structured report snapshot to be persisted, got %#v", payload.ReportSnapshot)
	}
}
