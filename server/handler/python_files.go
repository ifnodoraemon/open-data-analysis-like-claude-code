package handler

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
)

func ProxyPythonFileHandler(w http.ResponseWriter, r *http.Request) {
	filename := chi.URLParam(r, "filename")
	if filename == "" {
		http.Error(w, "缺少文件名参数", http.StatusBadRequest)
		return
	}

	endpoint := os.Getenv("PYTHON_MCP_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://python-executor:8081"
	}
	proxyToken := os.Getenv("PROXY_TOKEN")
	if proxyToken == "" {
		http.Error(w, "文件代理服务未配置", http.StatusServiceUnavailable)
		return
	}

	url := fmt.Sprintf("%s/files/%s", endpoint, filename)
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, url, nil)
	if err != nil {
		http.Error(w, "内部服务错误", http.StatusInternalServerError)
		return
	}
	req.Header.Set("X-Proxy-Token", proxyToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "代理请求失败", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
		return
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.WriteHeader(http.StatusOK)
	io.Copy(w, resp.Body)
}
