package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/config"
)

var llmDebugWriter = &debugWriter{}

type debugWriter struct {
	mu sync.Mutex
}

func (w *debugWriter) NewTraceID() string {
	if !config.Cfg.LLMDebug {
		return ""
	}
	return fmt.Sprintf("%s-%s", time.Now().Format("150405.000000000"), uuid.NewString()[:8])
}

func (w *debugWriter) WriteRecord(traceID, event string, payload map[string]interface{}) {
	if !config.Cfg.LLMDebug {
		return
	}

	record := map[string]interface{}{
		"ts":       time.Now().Format(time.RFC3339Nano),
		"trace_id": traceID,
		"event":    event,
		"data":     payload,
	}
	line, err := json.Marshal(record)
	if err != nil {
		return
	}

	path := filepath.Join(config.Cfg.LLMDebugDir, time.Now().Format("2006-01-02"), "index.jsonl")

	w.mu.Lock()
	defer w.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()

	_, _ = fmt.Fprintln(f, string(line))
}

func (w *debugWriter) WriteBlob(traceID, name string, payload []byte) string {
	if !config.Cfg.LLMDebug || traceID == "" {
		return ""
	}

	path := filepath.Join(config.Cfg.LLMDebugDir, time.Now().Format("2006-01-02"), traceID, name)

	w.mu.Lock()
	defer w.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return ""
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		return ""
	}
	return path
}
