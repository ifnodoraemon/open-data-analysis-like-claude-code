package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/config"
)

func TestDebugWriterWritesMetaAndIndexes(t *testing.T) {
	t.Parallel()

	previousCfg := config.Cfg
	config.Cfg = &config.Config{
		LLMDebug:    true,
		LLMDebugDir: t.TempDir(),
	}
	t.Cleanup(func() {
		config.Cfg = previousCfg
	})

	writer := &debugWriter{
		runSequence:    make(map[string]int),
		lastRunTraceID: make(map[string]string),
		traceMetaReady: make(map[string]bool),
	}

	trace := writer.StartTrace(TraceMetadata{
		WorkspaceID: "ws_1",
		SessionID:   "s_1",
		RunID:       "r_1",
	})
	if trace.TraceID == "" {
		t.Fatal("expected trace id")
	}

	payload := []byte(`{"ok":true}`)
	requestPath := writer.WriteBlob(trace, "request.json", payload)
	if requestPath == "" {
		t.Fatal("expected request path")
	}

	writer.WriteRecord(trace, "llm.request", map[string]interface{}{
		"request_path":  requestPath,
		"request_bytes": len(payload),
	})

	metaPath := filepath.Join(config.Cfg.LLMDebugDir, trace.Date, trace.TraceID, "meta.json")
	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("read meta.json: %v", err)
	}

	var meta map[string]interface{}
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		t.Fatalf("unmarshal meta.json: %v", err)
	}
	if meta["run_id"] != "r_1" {
		t.Fatalf("expected run_id r_1, got %#v", meta["run_id"])
	}

	traceIndexPath := filepath.Join(config.Cfg.LLMDebugDir, trace.Date, trace.TraceID, "index.jsonl")
	traceIndexBytes, err := os.ReadFile(traceIndexPath)
	if err != nil {
		t.Fatalf("read trace index: %v", err)
	}
	if !strings.Contains(string(traceIndexBytes), `"trace_id":"r_1-llm-001"`) {
		t.Fatalf("expected trace index to contain trace id, got %s", string(traceIndexBytes))
	}

	dailyIndexPath := filepath.Join(config.Cfg.LLMDebugDir, trace.Date, "index.jsonl")
	dailyIndexBytes, err := os.ReadFile(dailyIndexPath)
	if err != nil {
		t.Fatalf("read daily index: %v", err)
	}
	if !strings.Contains(string(dailyIndexBytes), `"request_bytes":11`) {
		t.Fatalf("expected daily index to contain compact payload summary, got %s", string(dailyIndexBytes))
	}
}
