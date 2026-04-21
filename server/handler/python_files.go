package handler

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"time"

	"github.com/go-chi/chi/v5"
)

var safeFilenamePattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

func ProxyPythonFileHandler(w http.ResponseWriter, r *http.Request) {
	filename := chi.URLParam(r, "filename")
	if filename == "" {
		http.Error(w, "missing filename parameter", http.StatusBadRequest)
		return
	}
	if !safeFilenamePattern.MatchString(filename) {
		http.Error(w, "invalid filename", http.StatusBadRequest)
		return
	}

	endpoint := os.Getenv("PYTHON_MCP_URL")
	if endpoint == "" {
		endpoint = "http://python-executor:8081"
	}
	proxyToken := os.Getenv("PROXY_TOKEN")
	if proxyToken == "" {
		http.Error(w, "file proxy service not configured", http.StatusServiceUnavailable)
		return
	}

	url := fmt.Sprintf("%s/files/%s", endpoint, filename)
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, url, nil)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	req.Header.Set("X-Proxy-Token", proxyToken)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "proxy request failed", http.StatusBadGateway)
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
