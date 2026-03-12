package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/auth"
	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/domain"
)

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
		"runs": serializeRuns(runs),
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
		"run": serializeRun(*run),
	}
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
	if run.ReportFileID == nil || strings.TrimSpace(*run.ReportFileID) == "" {
		http.Error(w, "任务尚未生成报告", http.StatusNotFound)
		return
	}

	reader, file, err := fileService.OpenForDownload(r.Context(), identity.UserID, identity.WorkspaceID, *run.ReportFileID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer reader.Close()

	w.Header().Set("Content-Type", defaultContentType(file.ContentType))
	w.Header().Set("Content-Disposition", `inline; filename="`+safeHeaderFilename(file.DisplayName)+`"`)
	_, _ = io.Copy(w, reader)
}

func serializeRuns(runs []domain.AnalysisRun) []map[string]interface{} {
	resp := make([]map[string]interface{}, 0, len(runs))
	for _, run := range runs {
		resp = append(resp, serializeRun(run))
	}
	return resp
}

func serializeRun(run domain.AnalysisRun) map[string]interface{} {
	item := map[string]interface{}{
		"id":           run.ID,
		"sessionId":    run.SessionID,
		"workspaceId":  run.WorkspaceID,
		"status":       run.Status,
		"inputMessage": run.InputMessage,
		"createdAt":    run.CreatedAt,
		"updatedAt":    run.UpdatedAt,
	}
	if run.ErrorMessage != nil {
		item["errorMessage"] = *run.ErrorMessage
	}
	if run.ReportFileID != nil {
		item["reportFileId"] = *run.ReportFileID
	}
	if run.StartedAt != nil {
		item["startedAt"] = *run.StartedAt
	}
	if run.FinishedAt != nil {
		item["finishedAt"] = *run.FinishedAt
	}
	return item
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
