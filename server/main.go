package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/ifnodoraemon/openDataAnalysis/config"
	"github.com/ifnodoraemon/openDataAnalysis/handler"
)

func main() {
	config.Load()
	handler.Initialize()

	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(handler.RequestLoggingMiddleware)
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:5173", "http://127.0.0.1:5173"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type", "X-Proxy-Token"},
		AllowCredentials: true,
	}))

	r.Post("/api/auth/login", handler.LoginHandler)
	r.Get("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"ok"}`))
	})

	r.Group(func(protected chi.Router) {
		protected.Use(handler.AuthMiddleware)
		protected.Use(handler.MaxBodySizeMiddleware(1 << 20))
		protected.Post("/api/auth/switch-workspace", handler.SwitchWorkspaceHandler)
		protected.Get("/api/bootstrap", handler.BootstrapHandler)
		protected.Post("/api/sessions", handler.CreateSessionHandler)
		protected.Get("/api/sessions", handler.ListSessionsHandler)
		protected.Get("/api/sessions/{sessionID}", handler.GetSessionHandler)
		protected.Put("/api/sessions/{sessionID}", handler.UpdateSessionHandler)
		protected.Delete("/api/sessions/{sessionID}", handler.DeleteSessionHandler)
		protected.Get("/api/runs", handler.ListRunsHandler)
		protected.Get("/api/runs/{runID}", handler.GetRunHandler)
		protected.Get("/api/runs/{runID}/report", handler.GetRunReportHandler)
		protected.Post("/api/report-exports/docx", handler.ConvertReportDOCXHandler)
		protected.Get("/api/python-files/{filename}", handler.ProxyPythonFileHandler)
		protected.Post("/api/upload", handler.UploadHandler)
		protected.Get("/ws", handler.WSHandler)

		protected.Get("/api/sessions/{sessionID}/sources", handler.SessionSourcesHandler)
		protected.Get("/api/semantic-profiles/{profileID}", handler.SemanticProfileDetailHandler)
		protected.Post("/api/semantic-profiles/{profileID}/confirm", handler.ConfirmProfileHandler)
		protected.Post("/api/data-sources", handler.CreateDataSourceHandler)
		protected.Get("/api/data-sources", handler.ListDataSourcesHandler)
		protected.Post("/api/data-sources/{sourceID}/test", handler.TestDataSourceHandler)
		protected.Get("/api/data-sources/{sourceID}/catalog", handler.CatalogDataSourceHandler)
		protected.Post("/api/data-sources/{sourceID}/import", handler.ImportDataSourceHandler)
	})

	port := config.Cfg.ServerPort
	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Printf("server started addr=0.0.0.0:%s ws_path=/ws model=%s endpoint=%s llm_debug=%t llm_debug_dir=%s",
			port,
			config.Cfg.LLMModel,
			config.Cfg.LLMAPIEndpoint,
			config.Cfg.LLMDebug,
			config.Cfg.LLMDebugDir,
		)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("server forced shutdown: %v", err)
	}

	if handler.ShutdownEventPersistWorker != nil {
		handler.ShutdownEventPersistWorker()
	}
	handler.StopLoginCleanup()
	handler.StopSessionCleanup()

	log.Println("server exited")
}
