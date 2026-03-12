package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/auth"
	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/service"
)

// UploadHandler 文件上传处理
func UploadHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(r.URL.Query().Get("session_id"))
	if sessionID == "" {
		http.Error(w, "缺少 session_id", http.StatusBadRequest)
		return
	}

	identity, _ := auth.FromContext(r.Context())
	sess, err := sessionManager.Get(sessionID, identity.WorkspaceID, identity.UserID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// 限制文件大小 100MB
	if err := r.ParseMultipartForm(100 << 20); err != nil {
		http.Error(w, "解析上传请求失败", http.StatusBadRequest)
		return
	}

	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "获取文件失败", http.StatusBadRequest)
		return
	}
	defer file.Close()

	uploaded, err := fileService.Upload(r.Context(), service.UploadFileInput{
		UserID:      identity.UserID,
		WorkspaceID: sess.WorkspaceID,
		SessionID:   sess.ID,
		FileName:    fileHeader.Filename,
		ContentType: fileHeader.Header.Get("Content-Type"),
		Size:        fileHeader.Size,
		Body:        file,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := map[string]interface{}{
		"file_id":  uploaded.ID,
		"filename": uploaded.DisplayName,
		"size":     uploaded.SizeBytes,
		"message":  fmt.Sprintf("文件 %s 上传成功 (%.2f MB)", uploaded.DisplayName, float64(uploaded.SizeBytes)/(1024*1024)),
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
