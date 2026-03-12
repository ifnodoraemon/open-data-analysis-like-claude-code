package handler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/auth"
	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/domain"
)

type loginRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	WorkspaceID string `json:"workspaceId,omitempty"`
}

func LoginHandler(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "请求格式错误", http.StatusBadRequest)
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" || req.Password == "" {
		http.Error(w, "邮箱和密码不能为空", http.StatusBadRequest)
		return
	}

	user, err := userRepo.GetByEmail(r.Context(), req.Email)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "邮箱或密码错误", http.StatusUnauthorized)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if !auth.VerifyPassword(req.Password, user.PasswordHash) {
		http.Error(w, "邮箱或密码错误", http.StatusUnauthorized)
		return
	}

	workspaces, err := workspaceRepo.ListByUser(r.Context(), user.ID)
	if err != nil || len(workspaces) == 0 {
		http.Error(w, "用户没有可用工作区", http.StatusForbidden)
		return
	}

	activeWorkspace := selectWorkspace(workspaces, req.WorkspaceID)
	identity := auth.Identity{
		UserID:      user.ID,
		UserName:    user.Name,
		UserEmail:   user.Email,
		WorkspaceID: activeWorkspace.ID,
		Workspace:   activeWorkspace.Name,
	}
	token, err := tokenManager.Sign(identity, 7*24*time.Hour)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	responseWorkspaces := make([]map[string]string, 0, len(workspaces))
	for _, workspace := range workspaces {
		responseWorkspaces = append(responseWorkspaces, map[string]string{
			"id":   workspace.ID,
			"name": workspace.Name,
		})
	}

	resp := map[string]interface{}{
		"token": token,
		"user": map[string]string{
			"id":    user.ID,
			"name":  user.Name,
			"email": user.Email,
		},
		"workspace": map[string]string{
			"id":   activeWorkspace.ID,
			"name": activeWorkspace.Name,
		},
		"workspaces": responseWorkspaces,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func selectWorkspace(workspaces []domain.Workspace, workspaceID string) domain.Workspace {
	if workspaceID != "" {
		for _, workspace := range workspaces {
			if workspace.ID == workspaceID {
				return workspace
			}
		}
	}
	return workspaces[0]
}
