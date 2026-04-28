package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ifnodoraemon/openDataAnalysis/auth"
	"github.com/ifnodoraemon/openDataAnalysis/config"
	"github.com/ifnodoraemon/openDataAnalysis/domain"
	repomemory "github.com/ifnodoraemon/openDataAnalysis/repository/memory"
	"github.com/ifnodoraemon/openDataAnalysis/tools"
)

func TestProxyPythonFileHandlerRequiresSignedRunScope(t *testing.T) {
	prevRunRepo := runRepo
	prevCfg := config.Cfg
	runRepo = repomemory.NewRunRepository()
	config.Cfg = &config.Config{AuthSecret: "abcdefghijklmnopqrstuvwxyz123456"}
	t.Cleanup(func() {
		runRepo = prevRunRepo
		config.Cfg = prevCfg
	})

	now := time.Now()
	if err := runRepo.Create(context.Background(), &domain.AnalysisRun{
		ID:          "r_1",
		SessionID:   "s_1",
		WorkspaceID: "w_1",
		UserID:      "u_1",
		RunKind:     domain.RunKindRoot,
		Status:      domain.RunStatusCompleted,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("create run: %v", err)
	}

	const filename = "req_12345678_plot.png"
	var proxyCalls atomic.Int32
	executor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyCalls.Add(1)
		if r.URL.Path != "/files/"+filename {
			t.Fatalf("unexpected executor path: %s", r.URL.Path)
		}
		if r.Header.Get("X-Proxy-Token") != "proxy-token" {
			t.Fatalf("missing proxy token header")
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("file-data"))
	}))
	t.Cleanup(executor.Close)
	t.Setenv("PYTHON_MCP_URL", executor.URL)
	t.Setenv("PROXY_TOKEN", "proxy-token")

	meta := tools.ExecutionMetadata{WorkspaceID: "w_1", SessionID: "s_1", RunID: "r_1"}
	sig := tools.SignPythonFileAccess(filename, meta, config.Cfg.AuthSecret)
	validTarget := "/api/python-files/" + filename + "?session_id=s_1&run_id=r_1&sig=" + url.QueryEscape(sig)
	identity := auth.Identity{UserID: "u_1", WorkspaceID: "w_1"}

	rec := servePythonFileRequest(validTarget, identity)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected valid signed request to succeed, got status=%d body=%q", rec.Code, rec.Body.String())
	}
	if strings.TrimSpace(rec.Body.String()) != "file-data" {
		t.Fatalf("unexpected proxy body: %q", rec.Body.String())
	}

	badSig := servePythonFileRequest("/api/python-files/"+filename+"?session_id=s_1&run_id=r_1&sig=bad", identity)
	if badSig.Code != http.StatusForbidden {
		t.Fatalf("expected bad signature to be forbidden, got %d", badSig.Code)
	}
	otherUser := servePythonFileRequest(validTarget, auth.Identity{UserID: "u_2", WorkspaceID: "w_1"})
	if otherUser.Code != http.StatusForbidden {
		t.Fatalf("expected other user to be forbidden, got %d", otherUser.Code)
	}
	if proxyCalls.Load() != 1 {
		t.Fatalf("expected only the authorized request to reach executor, got %d calls", proxyCalls.Load())
	}
}

func servePythonFileRequest(target string, identity auth.Identity) *httptest.ResponseRecorder {
	router := chi.NewRouter()
	router.Get("/api/python-files/{filename}", ProxyPythonFileHandler)
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req = req.WithContext(auth.WithIdentity(req.Context(), identity))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}
