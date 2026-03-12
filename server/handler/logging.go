package handler

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

func RequestLoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		next.ServeHTTP(ww, r)

		if shouldSkipAccessLog(r.URL.Path) {
			return
		}

		log.Printf(
			"http method=%s path=%s status=%d bytes=%d duration_ms=%d remote=%s",
			r.Method,
			r.URL.Path,
			ww.Status(),
			ww.BytesWritten(),
			time.Since(start).Milliseconds(),
			clientIP(r),
		)
	})
}

func shouldSkipAccessLog(path string) bool {
	return path == "/api/health"
}

func clientIP(r *http.Request) string {
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		return realIP
	}
	return r.RemoteAddr
}
