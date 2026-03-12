package session

import (
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/service"
)

type Manager struct {
	cacheRoot   string
	fileService *service.FileService
	workspaceID string
	userID      string
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

	sess, err := New(id, m.workspaceID, m.userID, m.cacheRoot, m.fileService)
	if err != nil {
		return nil, false, err
	}
	m.sessions[id] = sess
	return sess, true, nil
}

func (m *Manager) Get(sessionID string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sess, ok := m.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("会话不存在: %s", sessionID)
	}
	sess.Touch()
	return sess, nil
}
