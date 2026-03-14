package session

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/ifnodoraemon/openDataAnalysis/domain"
	"github.com/ifnodoraemon/openDataAnalysis/repository"
	"github.com/ifnodoraemon/openDataAnalysis/service"
)

type Manager struct {
	cacheRoot   string
	fileService *service.FileService
	sessionRepo repository.SessionRepository
	sessions    map[string]*Session
	mu          sync.Mutex
}

func NewManager(cacheRoot string, fileService *service.FileService) *Manager {
	return &Manager{
		cacheRoot:   cacheRoot,
		fileService: fileService,
		sessions:    make(map[string]*Session),
	}
}

func (m *Manager) SetSessionRepository(repo repository.SessionRepository) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessionRepo = repo
}

func (m *Manager) GetOrCreate(sessionID, workspaceID, userID string) (*Session, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if sessionID != "" {
		if sess, ok := m.sessions[sessionID]; ok {
			if sess.WorkspaceID != workspaceID || sess.UserID != userID {
				return nil, false, fmt.Errorf("无权访问该会话")
			}
			sess.Touch()
			return sess, false, nil
		}
	}

	id := sessionID
	if id == "" {
		id = "s_" + uuid.New().String()[:8]
	}

	created := true
	if sessionID != "" && m.sessionRepo != nil {
		record, err := m.sessionRepo.GetByID(context.Background(), sessionID)
		if err == nil {
			if record.WorkspaceID != workspaceID || record.UserID != userID {
				return nil, false, fmt.Errorf("无权访问该会话")
			}
			workspaceID = record.WorkspaceID
			userID = record.UserID
			created = false
		} else if err != sql.ErrNoRows {
			return nil, false, err
		}
	}

	sess, err := New(id, workspaceID, userID, m.cacheRoot, m.fileService)
	if err != nil {
		return nil, false, err
	}
	if created && m.sessionRepo != nil {
		now := time.Now()
		if err := m.sessionRepo.Create(context.Background(), &domain.Session{
			ID:          id,
			WorkspaceID: workspaceID,
			UserID:      userID,
			Title:       "未命名分析",
			Status:      domain.SessionStatusActive,
			CreatedAt:   now,
			UpdatedAt:   now,
			LastSeenAt:  now,
		}); err != nil {
			return nil, false, err
		}
	}
	if m.sessionRepo != nil {
		_ = m.sessionRepo.UpdateLastSeen(context.Background(), id)
	}
	m.sessions[id] = sess
	return sess, created, nil
}

func (m *Manager) Get(sessionID, workspaceID, userID string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sess, ok := m.sessions[sessionID]
	if !ok {
		if m.sessionRepo == nil {
			return nil, fmt.Errorf("会话不存在: %s", sessionID)
		}
		record, err := m.sessionRepo.GetByID(context.Background(), sessionID)
		if err != nil {
			return nil, fmt.Errorf("会话不存在: %s", sessionID)
		}
		if record.WorkspaceID != workspaceID || record.UserID != userID {
			return nil, fmt.Errorf("无权访问该会话")
		}
		sess, err = New(record.ID, record.WorkspaceID, record.UserID, m.cacheRoot, m.fileService)
		if err != nil {
			return nil, err
		}
		m.sessions[sessionID] = sess
	}
	if sess.WorkspaceID != workspaceID || sess.UserID != userID {
		return nil, fmt.Errorf("无权访问该会话")
	}
	sess.Touch()
	if m.sessionRepo != nil {
		_ = m.sessionRepo.UpdateLastSeen(context.Background(), sessionID)
	}
	return sess, nil
}

func (m *Manager) Peek(sessionID, workspaceID, userID string) (*Session, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sess, ok := m.sessions[sessionID]
	if !ok {
		return nil, false, nil
	}
	if sess.WorkspaceID != workspaceID || sess.UserID != userID {
		return nil, false, fmt.Errorf("无权访问该会话")
	}
	return sess, true, nil
}
