package handler

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
)

var (
	exportScriptBlockRe   = regexp.MustCompile(`(?is)<\s*script\b[^>]*>.*?<\s*/\s*script\s*>`)
	exportDangerousTagRe  = regexp.MustCompile(`(?is)<\s*/?\s*(iframe|object|embed|link|meta|base)\b[^>]*>`)
	exportEventAttrRe     = regexp.MustCompile(`(?i)\s+on[a-z]+\s*=\s*(?:"[^"]*"|'[^']*'|[^\s>]*)`)
	exportRemoteSrcHrefRe = regexp.MustCompile(`(?i)\s+(src|href)\s*=\s*(?:"[^"]*(?:https?|file|data|blob|javascript|vbscript)\s*:[^"]*"|'[^']*(?:https?|file|data|blob|javascript|vbscript)\s*:[^']*'|(?:https?|file|data|blob|javascript|vbscript)\s*:[^\s>]+)`)
)

func sanitizeExportHTML(html string) string {
	html = exportScriptBlockRe.ReplaceAllString(html, "")
	html = exportDangerousTagRe.ReplaceAllString(html, "")
	html = exportEventAttrRe.ReplaceAllString(html, "")
	html = exportRemoteSrcHrefRe.ReplaceAllString(html, "")
	return strings.TrimSpace(html)
}

func ConvertReportDOCXHandler(w http.ResponseWriter, r *http.Request) {
	type request struct {
		Title string `json:"title"`
		HTML  string `json:"html"`
	}

	var req request

	r.Body = http.MaxBytesReader(w, r.Body, 5*1024*1024)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid export request or body too large", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.HTML) == "" {
		http.Error(w, "missing report HTML", http.StatusBadRequest)
		return
	}
	req.HTML = sanitizeExportHTML(req.HTML)
	if strings.TrimSpace(req.HTML) == "" {
		http.Error(w, "export HTML became empty after sanitization", http.StatusBadRequest)
		return
	}

	body, filename, err := fileService.ConvertHTMLToDOCX(r.Context(), req.Title, req.HTML)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	w.Header().Set("Content-Disposition", `attachment; filename="`+safeHeaderFilename(filename)+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}
