package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/config"
)

var llmDebugWriter = &debugWriter{
	runSequence:    make(map[string]int),
	lastRunTraceID: make(map[string]string),
	traceMetaReady: make(map[string]bool),
}

type TraceInfo struct {
	Date          string
	TraceID       string
	ParentTraceID string
	Sequence      int
	WorkspaceID   string
	SessionID     string
	RunID         string
}

type debugWriter struct {
	mu             sync.Mutex
	runSequence    map[string]int
	lastRunTraceID map[string]string
	traceMetaReady map[string]bool
}

func (w *debugWriter) StartTrace(meta TraceMetadata) TraceInfo {
	if !config.Cfg.LLMDebug {
		return TraceInfo{}
	}

	now := time.Now()
	info := TraceInfo{
		Date:        now.Format("2006-01-02"),
		WorkspaceID: meta.WorkspaceID,
		SessionID:   meta.SessionID,
		RunID:       meta.RunID,
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if meta.RunID != "" {
		info.Sequence = w.runSequence[meta.RunID] + 1
		w.runSequence[meta.RunID] = info.Sequence
		info.ParentTraceID = w.lastRunTraceID[meta.RunID]
		info.TraceID = fmt.Sprintf("%s-llm-%03d", meta.RunID, info.Sequence)
		w.lastRunTraceID[meta.RunID] = info.TraceID
		return info
	}

	info.TraceID = fmt.Sprintf("%s-%s", now.Format("150405.000000000"), uuid.NewString()[:8])
	return info
}

func (w *debugWriter) WriteRecord(trace TraceInfo, event string, payload map[string]interface{}) {
	if !config.Cfg.LLMDebug || trace.TraceID == "" {
		return
	}

	record := map[string]interface{}{
		"ts":              time.Now().Format(time.RFC3339Nano),
		"trace_id":        trace.TraceID,
		"parent_trace_id": trace.ParentTraceID,
		"sequence":        trace.Sequence,
		"workspace_id":    trace.WorkspaceID,
		"session_id":      trace.SessionID,
		"run_id":          trace.RunID,
		"event":           event,
		"data":            payload,
	}
	line, err := json.Marshal(record)
	if err != nil {
		return
	}

	dailyPath := filepath.Join(config.Cfg.LLMDebugDir, trace.Date, "index.jsonl")
	tracePath := filepath.Join(config.Cfg.LLMDebugDir, trace.Date, trace.TraceID, "index.jsonl")

	w.mu.Lock()
	defer w.mu.Unlock()

	w.ensureTraceMetaLocked(trace)
	_ = appendJSONL(dailyPath, line)
	_ = appendJSONL(tracePath, line)
}

func (w *debugWriter) WriteBlob(trace TraceInfo, name string, payload []byte) string {
	if !config.Cfg.LLMDebug || trace.TraceID == "" {
		return ""
	}

	path := filepath.Join(config.Cfg.LLMDebugDir, trace.Date, trace.TraceID, name)

	w.mu.Lock()
	defer w.mu.Unlock()

	w.ensureTraceMetaLocked(trace)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return ""
	}
	if err := os.WriteFile(path, payload, 0o644); err != nil {
		return ""
	}
	return path
}

func appendJSONL(path string, line []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = fmt.Fprintln(f, string(line))
	return err
}

func (w *debugWriter) ensureTraceMetaLocked(trace TraceInfo) {
	if trace.TraceID == "" || w.traceMetaReady[trace.TraceID] {
		return
	}

	meta := map[string]interface{}{
		"trace_id":        trace.TraceID,
		"parent_trace_id": trace.ParentTraceID,
		"sequence":        trace.Sequence,
		"workspace_id":    trace.WorkspaceID,
		"session_id":      trace.SessionID,
		"run_id":          trace.RunID,
		"date":            trace.Date,
		"created_at":      time.Now().Format(time.RFC3339Nano),
	}
	metaBytes, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return
	}
	metaPath := filepath.Join(config.Cfg.LLMDebugDir, trace.Date, trace.TraceID, "meta.json")
	if err := os.MkdirAll(filepath.Dir(metaPath), 0o755); err != nil {
		return
	}
	if err := os.WriteFile(metaPath, metaBytes, 0o644); err != nil {
		return
	}
	w.traceMetaReady[trace.TraceID] = true
}

func blobSHA256(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
