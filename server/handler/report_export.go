package handler

import (
	"encoding/json"
	"net/http"
	"strings"
)

func ConvertReportDOCXHandler(w http.ResponseWriter, r *http.Request) {
	type request struct {
		Title string `json:"title"`
		HTML  string `json:"html"`
	}

	var req request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "无效的导出请求", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.HTML) == "" {
		http.Error(w, "缺少报告 HTML", http.StatusBadRequest)
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
