package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/auth"
)

func ListSessionsHandler(w http.ResponseWriter, r *http.Request) {
	identity, _ := auth.FromContext(r.Context())
	sessions, err := sessionRepo.ListByUserWorkspace(r.Context(), identity.UserID, identity.WorkspaceID, 20)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	respSessions := make([]map[string]interface{}, 0, len(sessions))
	for _, session := range sessions {
		respSessions = append(respSessions, serializeSession(session))
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"sessions": respSessions,
	})
}

func GetSessionHandler(w http.ResponseWriter, r *http.Request) {
	identity, _ := auth.FromContext(r.Context())
	sessionID := chi.URLParam(r, "sessionID")
	session, err := sessionRepo.GetByID(r.Context(), sessionID)
	if err != nil {
		http.Error(w, "会话不存在", http.StatusNotFound)
		return
	}
	if session.UserID != identity.UserID || session.WorkspaceID != identity.WorkspaceID {
		http.Error(w, "无权访问该会话", http.StatusForbidden)
		return
	}

	files, err := fileService.GetSessionFiles(r.Context(), session.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	runs, err := runRepo.ListBySession(r.Context(), session.ID, 20)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	respFiles := make([]map[string]interface{}, 0, len(files))
	for _, file := range files {
		respFiles = append(respFiles, serializeFile(file))
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"session": serializeSession(*session),
		"files":   respFiles,
		"runs":    serializeRuns(r.Context(), runs),
	})
}

func CreateSessionHandler(w http.ResponseWriter, r *http.Request) {
	identity, _ := auth.FromContext(r.Context())
	session, err := ensureSession(r.Context(), identity)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"session": serializeSession(*session),
		"files":   []map[string]interface{}{},
		"runs":    []map[string]interface{}{},
	})
}
