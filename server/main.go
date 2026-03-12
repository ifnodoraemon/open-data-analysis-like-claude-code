package main

import (
	"fmt"
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
	r.Use(middleware.Logger)
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
		protected.Get("/api/bootstrap", handler.BootstrapHandler)
		protected.Post("/api/upload", handler.UploadHandler)
		protected.Get("/ws", handler.WSHandler)
	})

	port := config.Cfg.ServerPort
	fmt.Printf("🚀 数据分析智能体后端启动在 http://localhost:%s\n", port)
	fmt.Printf("   WebSocket: ws://localhost:%s/ws\n", port)
	fmt.Printf("   LLM Model: %s\n", config.Cfg.LLMModel)
	fmt.Printf("   LLM Base URL: %s\n", config.Cfg.LLMBaseURL)

	log.Fatal(http.ListenAndServe(":"+port, r))
}
