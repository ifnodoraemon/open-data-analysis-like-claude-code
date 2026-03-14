package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/ifnodoraemon/openDataAnalysis/auth"
	"github.com/ifnodoraemon/openDataAnalysis/domain"
)

const runPreviewLimit = 3

func ListRunsHandler(w http.ResponseWriter, r *http.Request) {
	identity, _ := auth.FromContext(r.Context())
	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
	if sessionID == "" {
		http.Error(w, "缺少 session_id", http.StatusBadRequest)
		return
	}

	session, err := sessionRepo.GetByID(r.Context(), sessionID)
	if err != nil {
		http.Error(w, "会话不存在", http.StatusNotFound)
		return
	}
	if session.UserID != identity.UserID || session.WorkspaceID != identity.WorkspaceID {
		http.Error(w, "无权访问该会话", http.StatusForbidden)
		return
	}

	runs, err := runRepo.ListBySession(r.Context(), sessionID, 20)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := map[string]interface{}{
		"runs": serializeRuns(r.Context(), runs),
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func GetRunHandler(w http.ResponseWriter, r *http.Request) {
	identity, _ := auth.FromContext(r.Context())
	runID := chi.URLParam(r, "runID")
	run, err := runRepo.GetByID(r.Context(), runID)
	if err != nil {
		http.Error(w, "任务不存在", http.StatusNotFound)
		return
	}
	if run.UserID != identity.UserID || run.WorkspaceID != identity.WorkspaceID {
		http.Error(w, "无权访问该任务", http.StatusForbidden)
		return
	}

	resp := map[string]interface{}{
		"run": serializeRun(r.Context(), *run),
	}

	messages, err := messageRepo.ListByRun(r.Context(), runID)
	if err == nil {
		resp["messages"] = messages
	} else {
		resp["messages"] = []domain.RunMessage{}
	}
	attachRunRuntimeState(r.Context(), resp, *run)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func GetRunReportHandler(w http.ResponseWriter, r *http.Request) {
	identity, _ := auth.FromContext(r.Context())
	runID := chi.URLParam(r, "runID")
	run, err := runRepo.GetByID(r.Context(), runID)
	if err != nil {
		http.Error(w, "任务不存在", http.StatusNotFound)
		return
	}
	if run.UserID != identity.UserID || run.WorkspaceID != identity.WorkspaceID {
		http.Error(w, "无权访问该任务", http.StatusForbidden)
		return
	}

	report, reportErr := reportRepo.GetByRunID(r.Context(), runID)
	if reportErr == nil && report != nil {
		reader, err := fileService.OpenStoredObject(r.Context(), identity.UserID, identity.WorkspaceID, report.HTMLStorageKey)
		if err == nil {
			defer reader.Close()
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Content-Disposition", `inline; filename="`+safeHeaderFilename(reportFilename(report.Title, runID))+`"`)
			_, _ = io.Copy(w, reader)
			return
		}
		if errors.Is(err, os.ErrNotExist) {
			http.Error(w, "任务报告文件不存在", http.StatusNotFound)
			return
		}
	}
	if reportErr != nil && reportErr != sql.ErrNoRows {
		http.Error(w, reportErr.Error(), http.StatusInternalServerError)
		return
	}
	if run.ReportFileID == nil || strings.TrimSpace(*run.ReportFileID) == "" {
		http.Error(w, "任务尚未生成报告", http.StatusNotFound)
		return
	}

	reader, file, err := fileService.OpenForDownload(r.Context(), identity.UserID, identity.WorkspaceID, *run.ReportFileID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.Error(w, "任务报告文件不存在", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Type", defaultContentType(file.ContentType))
	w.Header().Set("Content-Disposition", `inline; filename="`+safeHeaderFilename(file.DisplayName)+`"`)
	_, _ = io.Copy(w, reader)
}

func serializeRuns(ctx context.Context, runs []domain.AnalysisRun) []map[string]interface{} {
	resp := make([]map[string]interface{}, 0, len(runs))
	for _, run := range runs {
		resp = append(resp, serializeRun(ctx, run))
	}
	return resp
}

func serializeRun(ctx context.Context, run domain.AnalysisRun) map[string]interface{} {
	item := map[string]interface{}{
		"id":           run.ID,
		"sessionId":    run.SessionID,
		"workspaceId":  run.WorkspaceID,
		"runKind":      run.RunKind,
		"delegateRole": run.DelegateRole,
		"status":       run.Status,
		"inputMessage": run.InputMessage,
		"summary":      run.Summary,
		"createdAt":    run.CreatedAt,
		"updatedAt":    run.UpdatedAt,
	}
	if run.ParentRunID != nil {
		item["parentRunId"] = *run.ParentRunID
	}
	if run.GoalID != nil {
		item["goalId"] = *run.GoalID
	}
	if run.ErrorMessage != nil {
		item["errorMessage"] = *run.ErrorMessage
	}
	if run.ReportFileID != nil {
		item["reportFileId"] = *run.ReportFileID
	}
	if reportRepo != nil {
		if report, err := reportRepo.GetByRunID(ctx, run.ID); err == nil && report != nil {
			item["report"] = serializeReport(*report)
		}
	}
	if run.StartedAt != nil {
		item["startedAt"] = *run.StartedAt
	}
	if run.FinishedAt != nil {
		item["finishedAt"] = *run.FinishedAt
	}
	if preview := buildRunPreview(ctx, run.ID); len(preview) > 0 {
		item["previewMessages"] = preview
	}
	if childRuns, err := runRepo.ListByParent(ctx, run.ID); err == nil && len(childRuns) > 0 {
		item["childRuns"] = serializeRuns(ctx, childRuns)
	}
	return item
}

func buildRunPreview(ctx context.Context, runID string) []map[string]interface{} {
	if messageRepo == nil {
		return nil
	}
	messages, err := messageRepo.ListRecentByRun(ctx, runID, runPreviewLimit)
	if err != nil || len(messages) == 0 {
		return nil
	}
	items := make([]map[string]interface{}, 0, len(messages))
	for _, msg := range messages {
		summary := summarizeRunMessage(msg)
		if summary == "" {
			continue
		}
		items = append(items, map[string]interface{}{
			"type":    msg.Type,
			"name":    msg.Name,
			"summary": summary,
		})
	}
	return items
}

func summarizeRunMessage(msg domain.RunMessage) string {
	content := strings.TrimSpace(msg.Content)
	switch msg.Type {
	case "thinking":
		return clipPreviewText(content, 120)
	case "tool_call":
		return msg.Name
	case "tool_result":
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(content), &payload); err == nil {
			if summary, ok := payload["summary_text"].(string); ok && strings.TrimSpace(summary) != "" {
				return clipPreviewText(summary, 120)
			}
			if summary, ok := payload["delegate_summary"].(string); ok && strings.TrimSpace(summary) != "" {
				return clipPreviewText(summary, 120)
			}
			if message, ok := payload["message"].(string); ok && strings.TrimSpace(message) != "" {
				return clipPreviewText(message, 120)
			}
		}
		if msg.Name != "" {
			return clipPreviewText(msg.Name+": "+content, 120)
		}
		return clipPreviewText(content, 120)
	case "run_completed":
		return clipPreviewText(content, 120)
	case "error":
		return clipPreviewText(content, 120)
	default:
		return clipPreviewText(content, 120)
	}
}

func clipPreviewText(input string, max int) string {
	input = strings.Join(strings.Fields(strings.TrimSpace(input)), " ")
	if max <= 0 {
		return ""
	}
	runes := []rune(input)
	if len(runes) <= max {
		return input
	}
	return string(runes[:max]) + "..."
}

func serializeReport(report domain.Report) map[string]interface{} {
	return map[string]interface{}{
		"id":        report.ID,
		"runId":     report.RunID,
		"title":     report.Title,
		"author":    report.Author,
		"createdAt": report.CreatedAt,
	}
}

func reportFilename(title, runID string) string {
	name := strings.TrimSpace(title)
	if name == "" {
		name = "report-" + runID
	}
	if !strings.HasSuffix(strings.ToLower(name), ".html") {
		name += ".html"
	}
	return name
}

func defaultContentType(contentType string) string {
	if strings.TrimSpace(contentType) == "" {
		return "application/octet-stream"
	}
	return contentType
}

func safeHeaderFilename(name string) string {
	replacer := strings.NewReplacer("\n", "", "\r", "", `"`, "")
	safe := strings.TrimSpace(replacer.Replace(name))
	if safe == "" {
		return "report.html"
	}
	return safe
}
