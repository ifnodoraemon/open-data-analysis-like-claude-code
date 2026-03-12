package session

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/domain"
	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/repository"
	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/service"
)

type Manager struct {
	cacheRoot   string
	fileService *service.FileService
	workspaceID string
	userID      string
	sessionRepo repository.SessionRepository
	sessions    map[string]*Session
	mu          sync.Mutex
}

func NewManager(cacheRoot, workspaceID, userID string, fileService *service.FileService) *Manager {
	return &Manager{
		cacheRoot:   cacheRoot,
		fileService: fileService,
		workspaceID: workspaceID,
		userID:      userID,
		sessions:    make(map[string]*Session),
	}
}

func (m *Manager) SetSessionRepository(repo repository.SessionRepository) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessionRepo = repo
}

func (m *Manager) GetOrCreate(sessionID string) (*Session, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if sessionID != "" {
		if sess, ok := m.sessions[sessionID]; ok {
			sess.Touch()
			return sess, false, nil
		}
	}

	id := sessionID
	if id == "" {
		id = "s_" + uuid.New().String()[:8]
	}

	workspaceID := m.workspaceID
	userID := m.userID
	created := true
	if sessionID != "" && m.sessionRepo != nil {
		record, err := m.sessionRepo.GetByID(context.Background(), sessionID)
		if err == nil {
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

func (m *Manager) Get(sessionID string) (*Session, error) {
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
		sess, err = New(record.ID, record.WorkspaceID, record.UserID, m.cacheRoot, m.fileService)
		if err != nil {
			return nil, err
		}
		m.sessions[sessionID] = sess
	}
	sess.Touch()
	if m.sessionRepo != nil {
		_ = m.sessionRepo.UpdateLastSeen(context.Background(), sessionID)
	}
	return sess, nil
}
