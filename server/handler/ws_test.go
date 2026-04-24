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
	"github.com/ifnodoraemon/openDataAnalysis/config"
	"github.com/ifnodoraemon/openDataAnalysis/domain"
	"github.com/ifnodoraemon/openDataAnalysis/session"
	"github.com/ifnodoraemon/openDataAnalysis/tools"
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

func TestIsExpectedWebSocketReadCloseRecognizesAbnormalBrowserDisconnects(t *testing.T) {
	t.Parallel()

	tests := []error{
		&websocket.CloseError{Code: websocket.CloseNoStatusReceived, Text: ""},
		&websocket.CloseError{Code: websocket.CloseAbnormalClosure, Text: "unexpected EOF"},
		errors.New("websocket: close 1006 (abnormal closure): unexpected EOF"),
	}

	for _, err := range tests {
		if !isExpectedWebSocketReadClose(err) {
			t.Fatalf("expected read close to be classified as expected: %v", err)
		}
	}
}

func TestIsExpectedWebSocketWriteCloseRecognizesClosedConnectionNoise(t *testing.T) {
	t.Parallel()

	tests := []error{
		&websocket.CloseError{Code: websocket.CloseGoingAway, Text: ""},
		errors.New("write tcp 127.0.0.1:8080->127.0.0.1:12345: write: broken pipe"),
		errors.New("websocket: close 1006 (abnormal closure): unexpected EOF"),
	}

	for _, err := range tests {
		if !isExpectedWebSocketWriteClose(err) {
			t.Fatalf("expected write close to be classified as expected: %v", err)
		}
	}
}

func TestResolvePreparedUserMessageMaterializesWholeReportEditContext(t *testing.T) {
	t.Parallel()

	prevCfg := config.Cfg
	config.Cfg = &config.Config{LLMProvider: "openai", LLMModel: "gpt-4o"}
	t.Cleanup(func() { config.Cfg = prevCfg })

	sess := &session.Session{
		Engine: agent.NewEngine(tools.NewRegistry(), ""),
		ReportState: &tools.ReportState{
			Blocks: []tools.ReportBlock{
				{ID: "b1", Kind: "markdown", Title: "Overview", Content: "body"},
			},
			NeedsFinalize: true,
		},
		EditState: &tools.ReportEditState{},
	}
	sess.Engine.SetTurnResolver(func(context.Context, *agent.PromptBundle) (agent.TurnResolution, error) {
		return agent.TurnResolution{
			Artifact:          agent.TurnArtifactReport,
			Operation:         agent.TurnOperationRevise,
			Scope:             agent.TurnScopeWholeReport,
			MutationRequested: true,
			Confidence:        0.93,
		}, nil
	})

	prepared, extra, err := resolvePreparedUserMessage(context.Background(), sess, agent.UserMessage{Content: "把当前报告整体整理一下"})
	if err != nil {
		t.Fatalf("resolve prepared user message: %v", err)
	}
	if prepared.EditContext == nil || prepared.EditContext.Mode != "revise_report" {
		t.Fatalf("expected whole-report edit context, got %#v", prepared.EditContext)
	}
	if len(extra) != 1 || extra[0].Name != "current_turn_resolution" {
		t.Fatalf("expected current_turn_resolution runtime block, got %#v", extra)
	}
}

func TestResolvePreparedUserMessageLeavesBlockScopeUnmaterialized(t *testing.T) {
	t.Parallel()

	prevCfg := config.Cfg
	config.Cfg = &config.Config{LLMProvider: "openai", LLMModel: "gpt-4o"}
	t.Cleanup(func() { config.Cfg = prevCfg })

	sess := &session.Session{
		Engine: agent.NewEngine(tools.NewRegistry(), ""),
		ReportState: &tools.ReportState{
			Blocks: []tools.ReportBlock{
				{ID: "b1", Kind: "markdown", Title: "结论", Content: "body"},
			},
			NeedsFinalize: true,
		},
		EditState: &tools.ReportEditState{},
	}
	sess.Engine.SetTurnResolver(func(context.Context, *agent.PromptBundle) (agent.TurnResolution, error) {
		return agent.TurnResolution{
			Artifact:          agent.TurnArtifactReport,
			Operation:         agent.TurnOperationRevise,
			Scope:             agent.TurnScopeBlock,
			TargetRefHint:     "结论部分",
			MutationRequested: true,
			Confidence:        0.92,
		}, nil
	})

	prepared, extra, err := resolvePreparedUserMessage(context.Background(), sess, agent.UserMessage{Content: "把结论部分改一下"})
	if err != nil {
		t.Fatalf("resolve prepared user message: %v", err)
	}
	if prepared.EditContext != nil {
		t.Fatalf("did not expect block-scope resolution to auto-materialize edit context: %#v", prepared.EditContext)
	}
	if len(extra) != 1 || extra[0].Name != "current_turn_resolution" || !strings.Contains(extra[0].Content, "Scope: block") {
		t.Fatalf("expected block-scope current_turn_resolution runtime block, got %#v", extra)
	}
}
