package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/ifnodoraemon/openDataAnalysis/agent"
	"github.com/ifnodoraemon/openDataAnalysis/auth"
)

func ListSessionsHandler(w http.ResponseWriter, r *http.Request) {
	identity, _ := auth.FromContext(r.Context())
	if ok, _ := workspaceRepo.IsMember(r.Context(), identity.WorkspaceID, identity.UserID); !ok {
		http.Error(w, "user not authorized to access workspace", http.StatusForbidden)
		return
	}
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

func CreateSessionHandler(w http.ResponseWriter, r *http.Request) {
	identity, _ := auth.FromContext(r.Context())
	if ok, _ := workspaceRepo.IsMember(r.Context(), identity.WorkspaceID, identity.UserID); !ok {
		http.Error(w, "user not authorized to access workspace", http.StatusForbidden)
		return
	}
	session, err := ensureSession(r.Context(), identity)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"session":      serializeSession(*session),
		"files":        []map[string]interface{}{},
		"runs":         []map[string]interface{}{},
		"runtimeState": serializeRuntimeState(map[string]string{}, []agent.Subgoal{}, ""),
	})
}

func GetSessionHandler(w http.ResponseWriter, r *http.Request) {
	identity, _ := auth.FromContext(r.Context())
	if ok, _ := workspaceRepo.IsMember(r.Context(), identity.WorkspaceID, identity.UserID); !ok {
		http.Error(w, "user not authorized to access workspace", http.StatusForbidden)
		return
	}
	sessionID := chi.URLParam(r, "sessionID")
	session, err := sessionRepo.GetByID(r.Context(), sessionID)
	if writeRepoLookupError(w, err, "session does not exist") {
		return
	}
	if session.UserID != identity.UserID || session.WorkspaceID != identity.WorkspaceID {
		http.Error(w, "not authorized to access this session", http.StatusForbidden)
		return
	}

	if err := recoverStaleSessionRuns(r.Context(), session.ID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	runs, err := runRepo.ListBySession(r.Context(), session.ID, 20)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	resp := map[string]interface{}{
		"session": serializeSession(*session),
		"runs":    serializeRuns(r.Context(), runs),
	}
	attachRuntimeState(r.Context(), resp, identity.WorkspaceID, identity.UserID, session.ID)

	sources, srcErr := sourceService.GetSessionSources(r.Context(), session.ID)
	if srcErr == nil {
		resp["sessionSources"] = sources
	}
	profiles, profErr := sourceService.GetSessionProfiles(r.Context(), session.ID)
	if profErr == nil {
		resp["pendingSemanticProfiles"] = filterPendingProfiles(profiles)
	}

	_ = json.NewEncoder(w).Encode(resp)
}

func UpdateSessionHandler(w http.ResponseWriter, r *http.Request) {
	identity, _ := auth.FromContext(r.Context())
	if ok, _ := workspaceRepo.IsMember(r.Context(), identity.WorkspaceID, identity.UserID); !ok {
		http.Error(w, "user not authorized to access workspace", http.StatusForbidden)
		return
	}
	sessionID := chi.URLParam(r, "sessionID")

	session, err := sessionRepo.GetByID(r.Context(), sessionID)
	if writeRepoLookupError(w, err, "session does not exist") {
		return
	}
	if session.UserID != identity.UserID || session.WorkspaceID != identity.WorkspaceID {
		http.Error(w, "not authorized to modify this session", http.StatusForbidden)
		return
	}

	var req struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Title != "" {
		if err := sessionRepo.UpdateTitle(r.Context(), sessionID, req.Title); err != nil {
			http.Error(w, "failed to update title", http.StatusInternalServerError)
			return
		}
	}

	session.Title = req.Title
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"session": serializeSession(*session),
	})
}

func DeleteSessionHandler(w http.ResponseWriter, r *http.Request) {
	identity, _ := auth.FromContext(r.Context())
	if ok, _ := workspaceRepo.IsMember(r.Context(), identity.WorkspaceID, identity.UserID); !ok {
		http.Error(w, "user not authorized to access workspace", http.StatusForbidden)
		return
	}
	sessionID := chi.URLParam(r, "sessionID")

	session, err := sessionRepo.GetByID(r.Context(), sessionID)
	if writeRepoLookupError(w, err, "session does not exist") {
		return
	}
	if session.UserID != identity.UserID || session.WorkspaceID != identity.WorkspaceID {
		http.Error(w, "not authorized to delete this session", http.StatusForbidden)
		return
	}

	if err := deleteSessionResources(r.Context(), *session); err != nil {
		http.Error(w, "failed to delete session", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
