package handler

import (
	"encoding/json"
	"net/http"

	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/auth"
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
	if err == nil {
		respSessions := make([]map[string]interface{}, 0, len(sessions))
		for _, session := range sessions {
			respSessions = append(respSessions, serializeSession(session))
		}
		resp["sessions"] = respSessions
	}
	if err == nil && len(sessions) > 0 {
		latestSession := sessions[0]
		resp["session"] = serializeSession(latestSession)
		runs, err := runRepo.ListBySession(r.Context(), latestSession.ID, 20)
		if err == nil {
			resp["runs"] = serializeRuns(runs)
		}
		files, err := fileService.GetSessionFiles(r.Context(), latestSession.ID)
		if err == nil {
			respFiles := make([]map[string]interface{}, 0, len(files))
			for _, file := range files {
				respFiles = append(respFiles, serializeFile(file))
			}
			resp["files"] = respFiles
		}
	} else if err == nil {
		session, createErr := ensureSession(r.Context(), identity)
		if createErr == nil && session != nil {
			resp["session"] = serializeSession(*session)
			resp["sessions"] = []map[string]interface{}{serializeSession(*session)}
			resp["files"] = []map[string]interface{}{}
			resp["runs"] = []map[string]interface{}{}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
