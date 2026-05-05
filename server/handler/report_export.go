package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/ifnodoraemon/openDataAnalysis/auth"
)

const ReportExportMaxBodyBytes int64 = 50 * 1024 * 1024

var (
	exportScriptBlockRe   = regexp.MustCompile(`(?is)<\s*script\b[^>]*>.*?<\s*/\s*script\s*>`)
	exportDangerousTagRe  = regexp.MustCompile(`(?is)<\s*/?\s*(iframe|object|embed|link|meta|base)\b[^>]*>`)
	exportEventAttrRe     = regexp.MustCompile(`(?i)\s+on[a-z]+\s*=\s*(?:"[^"]*"|'[^']*'|[^\s>]*)`)
	exportRemoteSrcHrefRe = regexp.MustCompile(`(?i)\s+(src|href)\s*=\s*(?:"[^"]*(?:https?|file|data|blob|javascript|vbscript)\s*:[^"]*"|'[^']*(?:https?|file|data|blob|javascript|vbscript)\s*:[^']*'|(?:https?|file|data|blob|javascript|vbscript)\s*:[^\s>]+)`)
	exportSafeDataImageRe = regexp.MustCompile(`(?i)\s+src\s*=\s*("data:image/(?:png|jpe?g);base64,[a-z0-9+/=]+")`)
)

func RegisterReportExportRoutes(r chi.Router) {
	r.Group(func(exports chi.Router) {
		exports.Use(MaxBodySizeMiddleware(ReportExportMaxBodyBytes))
		exports.Post("/api/report-exports/docx", ConvertReportDOCXHandler)
	})
}

func sanitizeExportHTML(html string) string {
	protectedImages := map[string]string{}
	html = exportSafeDataImageRe.ReplaceAllStringFunc(html, func(match string) string {
		key := "__ODA_SAFE_EXPORT_IMAGE_" + strconv.Itoa(len(protectedImages)) + "__"
		protectedImages[key] = match
		return " " + key
	})
	html = exportScriptBlockRe.ReplaceAllString(html, "")
	html = exportDangerousTagRe.ReplaceAllString(html, "")
	html = exportEventAttrRe.ReplaceAllString(html, "")
	html = exportRemoteSrcHrefRe.ReplaceAllString(html, "")
	for key, src := range protectedImages {
		html = strings.ReplaceAll(html, key, strings.TrimSpace(src))
	}
	return strings.TrimSpace(html)
}

func ConvertReportDOCXHandler(w http.ResponseWriter, r *http.Request) {
	identity, _ := auth.FromContext(r.Context())
	workspaceID := identity.WorkspaceID
	userID := identity.UserID

	type request struct {
		Title string `json:"title"`
		RunID string `json:"runId"`
		HTML  string `json:"html"`
	}

	var req request

	r.Body = http.MaxBytesReader(w, r.Body, ReportExportMaxBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid export request or body too large", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.RunID) == "" {
		http.Error(w, "missing runId", http.StatusBadRequest)
		return
	}

	run, err := runRepo.GetByID(r.Context(), req.RunID)
	if err != nil {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}
	if run.WorkspaceID != workspaceID || run.UserID != userID {
		http.Error(w, "run not found", http.StatusNotFound)
		return
	}
	if run.ReportFileID == nil || *run.ReportFileID == "" {
		http.Error(w, "report not finalized yet", http.StatusBadRequest)
		return
	}

	html := strings.TrimSpace(req.HTML)
	if html == "" {
		reader, _, err := fileService.OpenForDownload(r.Context(), userID, workspaceID, *run.ReportFileID)
		if err != nil {
			http.Error(w, "failed to read report file", http.StatusInternalServerError)
			return
		}
		defer reader.Close()
		htmlBytes, err := io.ReadAll(reader)
		if err != nil {
			http.Error(w, "failed to read report content", http.StatusInternalServerError)
			return
		}
		html = string(htmlBytes)
	}

	html = sanitizeExportHTML(html)
	if strings.TrimSpace(html) == "" {
		http.Error(w, "report content is empty or invalid", http.StatusBadRequest)
		return
	}

	body, filename, err := fileService.ConvertHTMLToDOCX(r.Context(), req.Title, html)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	w.Header().Set("Content-Disposition", `attachment; filename="`+safeHeaderFilename(filename)+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
