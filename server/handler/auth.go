package handler

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ifnodoraemon/openDataAnalysis/auth"
	"github.com/ifnodoraemon/openDataAnalysis/domain"
)

type loginRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	WorkspaceID string `json:"workspaceId,omitempty"`
}

type switchWorkspaceRequest struct {
	WorkspaceID string `json:"workspaceId"`
}

var (
	loginRateMu     sync.Mutex
	loginAttempts   = make(map[string][]time.Time)
	loginRateLimit  = 5
	loginRateWindow = 5 * time.Minute
)

func checkLoginRate(email string) bool {
	loginRateMu.Lock()
	defer loginRateMu.Unlock()
	now := time.Now()
	cutoff := now.Add(-loginRateWindow)
	attempts := loginAttempts[email]
	valid := make([]time.Time, 0, len(attempts))
	for _, t := range attempts {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}
	loginAttempts[email] = valid
	return len(valid) < loginRateLimit
}

func recordLoginAttempt(email string) {
	loginRateMu.Lock()
	defer loginRateMu.Unlock()
	loginAttempts[email] = append(loginAttempts[email], time.Now())
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

	if !checkLoginRate(req.Email) {
		http.Error(w, "登录尝试过于频繁，请稍后重试", http.StatusTooManyRequests)
		return
	}

	user, err := userRepo.GetByEmail(r.Context(), req.Email)
	if err != nil {
		if err == sql.ErrNoRows {
			recordLoginAttempt(req.Email)
			http.Error(w, "邮箱或密码错误", http.StatusUnauthorized)
			return
		}
		http.Error(w, "内部服务错误", http.StatusInternalServerError)
		return
	}

	if !auth.VerifyPassword(req.Password, user.PasswordHash) {
		recordLoginAttempt(req.Email)
		http.Error(w, "邮箱或密码错误", http.StatusUnauthorized)
		return
	}

	workspaces, err := workspaceRepo.ListByUser(r.Context(), user.ID)
	if err != nil || len(workspaces) == 0 {
		http.Error(w, "内部服务错误", http.StatusInternalServerError)
		return
	}

	activeWorkspace, err := selectWorkspace(workspaces, req.WorkspaceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	identity := auth.Identity{
		UserID:      user.ID,
		UserName:    user.Name,
		UserEmail:   user.Email,
		WorkspaceID: activeWorkspace.ID,
		Workspace:   activeWorkspace.Name,
	}
	token, err := tokenManager.Sign(identity, 7*24*time.Hour)
	if err != nil {
		http.Error(w, "内部服务错误", http.StatusInternalServerError)
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

func selectWorkspace(workspaces []domain.Workspace, workspaceID string) (domain.Workspace, error) {
	if workspaceID != "" {
		for _, workspace := range workspaces {
			if workspace.ID == workspaceID {
				return workspace, nil
			}
		}
		return domain.Workspace{}, fmt.Errorf("指定的工作区不存在或无权访问")
	}
	return workspaces[0], nil
}

func SwitchWorkspaceHandler(w http.ResponseWriter, r *http.Request) {
	identity, _ := auth.FromContext(r.Context())

	var req switchWorkspaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "请求格式错误", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.WorkspaceID) == "" {
		http.Error(w, "workspaceId 不能为空", http.StatusBadRequest)
		return
	}

	ok, err := workspaceRepo.IsMember(r.Context(), req.WorkspaceID, identity.UserID)
	if err != nil {
		http.Error(w, "内部服务错误", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "无权访问该工作区", http.StatusForbidden)
		return
	}

	workspace, err := workspaceRepo.GetByID(r.Context(), req.WorkspaceID)
	if err != nil {
		http.Error(w, "内部服务错误", http.StatusInternalServerError)
		return
	}

	newIdentity := auth.Identity{
		UserID:      identity.UserID,
		UserName:    identity.UserName,
		UserEmail:   identity.UserEmail,
		WorkspaceID: workspace.ID,
		Workspace:   workspace.Name,
	}
	token, err := tokenManager.Sign(newIdentity, 7*24*time.Hour)
	if err != nil {
		http.Error(w, "内部服务错误", http.StatusInternalServerError)
		return
	}

	resp := map[string]interface{}{
		"token": token,
		"workspace": map[string]string{
			"id":   workspace.ID,
			"name": workspace.Name,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
