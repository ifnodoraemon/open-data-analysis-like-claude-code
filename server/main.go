package main

import (
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/config"
	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/handler"
)

func main() {
	// 加载配置
	config.Load()
	handler.Initialize()

	r := chi.NewRouter()

	// 中间件
	r.Use(middleware.RequestID)
	r.Use(handler.RequestLoggingMiddleware)
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:5173", "http://127.0.0.1:5173"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: true,
	}))

	// 公开接口
	r.Post("/api/auth/login", handler.LoginHandler)
	r.Get("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"ok"}`))
	})

	// 鉴权接口
	r.Group(func(protected chi.Router) {
		protected.Use(handler.AuthMiddleware)
		protected.Post("/api/auth/switch-workspace", handler.SwitchWorkspaceHandler)
		protected.Get("/api/bootstrap", handler.BootstrapHandler)
		protected.Post("/api/sessions", handler.CreateSessionHandler)
		protected.Get("/api/sessions", handler.ListSessionsHandler)
		protected.Get("/api/sessions/{sessionID}", handler.GetSessionHandler)
		protected.Get("/api/runs", handler.ListRunsHandler)
		protected.Get("/api/runs/{runID}", handler.GetRunHandler)
		protected.Get("/api/runs/{runID}/report", handler.GetRunReportHandler)
		protected.Post("/api/upload", handler.UploadHandler)
		protected.Get("/ws", handler.WSHandler)
	})

	port := config.Cfg.ServerPort
	log.Printf("server started addr=0.0.0.0:%s ws_path=/ws model=%s endpoint=%s llm_debug=%t llm_debug_dir=%s",
		port,
		config.Cfg.LLMModel,
		config.Cfg.LLMAPIEndpoint,
		config.Cfg.LLMDebug,
		config.Cfg.LLMDebugDir,
	)

	log.Fatal(http.ListenAndServe(":"+port, r))
}
