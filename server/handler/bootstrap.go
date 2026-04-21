package handler

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/ifnodoraemon/openDataAnalysis/auth"
	"github.com/ifnodoraemon/openDataAnalysis/service"
)

func BootstrapHandler(w http.ResponseWriter, r *http.Request) {
	identity, _ := auth.FromContext(r.Context())

	workspaces, _ := workspaceRepo.ListByUser(r.Context(), identity.UserID)
	respWorkspaces := make([]map[string]string, 0, len(workspaces))
	for _, workspace := range workspaces {
		respWorkspaces = append(respWorkspaces, map[string]string{
			"id":   workspace.ID,
			"name": workspace.Name,
		})
	}

	resp := map[string]interface{}{
		"user": map[string]string{
			"id":    identity.UserID,
			"name":  identity.UserName,
			"email": identity.UserEmail,
		},
		"workspace": map[string]string{
			"id":   identity.WorkspaceID,
			"name": identity.Workspace,
		},
		"workspaces": respWorkspaces,
	}

	sessions, err := sessionRepo.ListByUserWorkspace(r.Context(), identity.UserID, identity.WorkspaceID, 20)
	if err != nil {
		log.Printf("bootstrap: list sessions error workspace_id=%s user_id=%s err=%v", identity.WorkspaceID, identity.UserID, err)
		http.Error(w, "failed to get session history", http.StatusInternalServerError)
		return
	}
	respSessions := make([]map[string]interface{}, 0, len(sessions))
	for _, session := range sessions {
		respSessions = append(respSessions, serializeSession(session))
	}
	resp["sessions"] = respSessions

	if len(sessions) > 0 {
		latestSession := sessions[0]
		resp["session"] = serializeSession(latestSession)
		if err := recoverStaleSessionRuns(r.Context(), latestSession.ID); err != nil {
			log.Printf("bootstrap: recover stale runs error session_id=%s err=%v", latestSession.ID, err)
			http.Error(w, "failed to recover stale runs", http.StatusInternalServerError)
			return
		}
		runs, err := runRepo.ListBySession(r.Context(), latestSession.ID, 20)
		if err != nil {
			log.Printf("bootstrap: list runs error session_id=%s err=%v", latestSession.ID, err)
			http.Error(w, "failed to get run history", http.StatusInternalServerError)
			return
		}
		resp["runs"] = serializeRuns(r.Context(), runs)

		attachRuntimeState(r.Context(), resp, identity.WorkspaceID, identity.UserID, latestSession.ID)

		sources, srcErr := sourceService.GetSessionSources(r.Context(), latestSession.ID)
		if srcErr == nil {
			resp["sessionSources"] = sources
		}

		profiles, profErr := sourceService.GetSessionProfiles(r.Context(), latestSession.ID)
		if profErr == nil {
			resp["pendingSemanticProfiles"] = filterPendingProfiles(profiles)
		}
	} else {
		session, createErr := ensureSession(r.Context(), identity)
		if createErr != nil {
			log.Printf("bootstrap: create session error workspace_id=%s err=%v", identity.WorkspaceID, createErr)
			http.Error(w, "failed to create initial session", http.StatusInternalServerError)
			return
		}
		if session != nil {
			resp["session"] = serializeSession(*session)
			resp["sessions"] = []map[string]interface{}{serializeSession(*session)}
			resp["files"] = []map[string]interface{}{}
			resp["runs"] = []map[string]interface{}{}
			attachRuntimeState(r.Context(), resp, identity.WorkspaceID, identity.UserID, session.ID)
			resp["sessionSources"] = []interface{}{}
			resp["pendingSemanticProfiles"] = []interface{}{}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func filterPendingProfiles(profiles []service.SemanticProfileSummary) []service.SemanticProfileSummary {
	var pending []service.SemanticProfileSummary
	for _, p := range profiles {
		if p.ProfileStatus != "confirmed" && p.ProfileStatus != "rejected" {
			pending = append(pending, p)
		}
	}
	return pending
}
