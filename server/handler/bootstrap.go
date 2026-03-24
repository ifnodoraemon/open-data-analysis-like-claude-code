package handler

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/ifnodoraemon/openDataAnalysis/auth"
	"github.com/ifnodoraemon/openDataAnalysis/domain"
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
		http.Error(w, "获取历史会话失败", http.StatusInternalServerError)
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
			runs, err := runRepo.ListBySession(r.Context(), latestSession.ID, 20)
			if err != nil {
				log.Printf("bootstrap: list runs error session_id=%s err=%v", latestSession.ID, err)
				http.Error(w, "获取运行记录失败", http.StatusInternalServerError)
				return
			}
			// 回收 stale runs：服务重启后 DB 中残留的 running/waiting_user_input 状态
			// 如果 session 不在内存中（无活跃引擎），这些 run 永远无法恢复，标记为 failed
			if !sessionManager.IsSessionLive(latestSession.ID) {
				for i := range runs {
					if runs[i].Status == domain.RunStatusRunning || runs[i].Status == domain.RunStatusWaitingUserInput {
						errMsg := "服务重启后任务无法恢复，已自动标记为失败"
						log.Printf("bootstrap: recovered stale run run_id=%s old_status=%s", runs[i].ID, runs[i].Status)
						runs[i].Status = domain.RunStatusFailed
						runs[i].ErrorMessage = &errMsg
						_ = runRepo.UpdateStatus(r.Context(), runs[i].ID, domain.RunStatusFailed, &errMsg)
					}
				}
			}
			resp["runs"] = serializeRuns(r.Context(), runs)

			files, err := fileService.GetSessionFiles(r.Context(), latestSession.ID)
			if err != nil {
				log.Printf("bootstrap: get session files error session_id=%s err=%v", latestSession.ID, err)
				http.Error(w, "获取会话文件失败", http.StatusInternalServerError)
				return
			}
			respFiles := make([]map[string]interface{}, 0, len(files))
			for _, file := range files {
				respFiles = append(respFiles, serializeFile(file))
			}
			resp["files"] = respFiles
			attachRuntimeState(r.Context(), resp, identity.WorkspaceID, identity.UserID, latestSession.ID)
		} else {
			session, createErr := ensureSession(r.Context(), identity)
			if createErr != nil {
				log.Printf("bootstrap: create session error workspace_id=%s err=%v", identity.WorkspaceID, createErr)
				http.Error(w, "创建初始会话失败", http.StatusInternalServerError)
				return
			}
			if session != nil {
				resp["session"] = serializeSession(*session)
				resp["sessions"] = []map[string]interface{}{serializeSession(*session)}
				resp["files"] = []map[string]interface{}{}
				resp["runs"] = []map[string]interface{}{}
				attachRuntimeState(r.Context(), resp, identity.WorkspaceID, identity.UserID, session.ID)
			}
		}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
