package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// UploadHandler 文件上传处理
func UploadHandler(w http.ResponseWriter, r *http.Request) {
	// 限制文件大小 100MB
	r.ParseMultipartForm(100 << 20)

	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "获取文件失败", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// 确保上传目录存在
	uploadDir := "./uploads"
	os.MkdirAll(uploadDir, 0755)

	// 保存文件
	destPath := filepath.Join(uploadDir, fileHeader.Filename)
	dest, err := os.Create(destPath)
	if err != nil {
		http.Error(w, "保存文件失败", http.StatusInternalServerError)
		return
	}
	defer dest.Close()

	written, err := io.Copy(dest, file)
	if err != nil {
		http.Error(w, "写入文件失败", http.StatusInternalServerError)
		return
	}

	resp := map[string]interface{}{
		"filename": fileHeader.Filename,
		"size":     written,
		"path":     destPath,
		"message":  fmt.Sprintf("文件 %s 上传成功 (%.2f MB)", fileHeader.Filename, float64(written)/(1024*1024)),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
