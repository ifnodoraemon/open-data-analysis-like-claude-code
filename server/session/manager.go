package session

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/ifnodoraemon/openDataAnalysis/data"
	"github.com/ifnodoraemon/openDataAnalysis/domain"
	"github.com/ifnodoraemon/openDataAnalysis/repository"
	"github.com/ifnodoraemon/openDataAnalysis/service"
)

// dbTimeout 是单次数据库查询的最大等待时间，防止慢 DB 永久阻塞 goroutine（BUG2 修复）。
const dbTimeout = 10 * time.Second

// dbContext 返回一个带固定超时、与父 ctx 取消无关的衍生 context，
// 用于 Manager 内部的 DB 查询。使用 context.WithoutCancel 防止请求 cancel
// 传播到 DB 写操作（如 UpdateLastSeen）。
func dbContext(parent context.Context) (context.Context, context.CancelFunc) {
	base := context.WithoutCancel(parent)
	return context.WithTimeout(base, dbTimeout)
}

const sessionStopTimeout = 10 * time.Second

type Manager struct {
	cacheRoot      string
	fileService    *service.FileService
	sessionRepo    repository.SessionRepository
	sessions       map[string]*Session
	mu             sync.Mutex
	// FullDeleteFunc 是全链路删除函数，由 handler.Initialize 设置。
	// 如果为 nil，则退化为 Session.Destroy + sessionRepo.Delete。
	FullDeleteFunc func(ctx context.Context, sessionID string) error
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

// SetFullDeleteFunc 设置全链路删除回调，用于 TTL 入口的全呼到。
func (m *Manager) SetFullDeleteFunc(fn func(ctx context.Context, sessionID string) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.FullDeleteFunc = fn
}

// GetOrCreate 查找或创建 session。
// ctx 用于限制 DB 查询时长（BUG2 修复），不传播取消语义到写操作。
func (m *Manager) GetOrCreate(ctx context.Context, sessionID, workspaceID, userID string) (*Session, bool, error) {
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
		qCtx, cancel := dbContext(ctx)
		record, err := m.sessionRepo.GetByID(qCtx, sessionID)
		cancel()
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
		wCtx, cancel := dbContext(ctx)
		err := m.sessionRepo.Create(wCtx, &domain.Session{
			ID:          id,
			WorkspaceID: workspaceID,
			UserID:      userID,
			Title:       "未命名分析",
			Status:      domain.SessionStatusActive,
			CreatedAt:   now,
			UpdatedAt:   now,
			LastSeenAt:  now,
		})
		cancel()
		if err != nil {
			return nil, false, err
		}
	}
	if m.sessionRepo != nil {
		wCtx, cancel := dbContext(ctx)
		_ = m.sessionRepo.UpdateLastSeen(wCtx, id)
		cancel()
	}
	m.sessions[id] = sess
	return sess, created, nil
}

// Get 返回指定 session（仅内存中存在的，或从 DB 记录重建的空 session）。
// ctx 用于限制 DB 查询时长（BUG2 修复）。
// ISSUE10 注意：若 session 不在内存中（例如服务重启后），将从 DB 重建一个空状态 session，
// WorkingMemory、SubgoalManager、Engine 消息历史均为初始状态，这是设计上的权衡（无法持久化运行时状态）。
// 调用方应意识到重建的 session 与原始 session 行为不同，勿在无持久化的重建 session 上执行复杂操作。
func (m *Manager) Get(ctx context.Context, sessionID, workspaceID, userID string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	sess, ok := m.sessions[sessionID]
	if !ok {
		if m.sessionRepo == nil {
			return nil, fmt.Errorf("会话不存在: %s", sessionID)
		}
		qCtx, cancel := dbContext(ctx)
		record, err := m.sessionRepo.GetByID(qCtx, sessionID)
		cancel()
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
		// ISSUE10 警告：从 DB 重建的 session 运行时状态（Memory/Subgoals/Engine 历史）已重置。
		// 这在服务重启后属于预期行为，但若在 session 仍活跃时发生则可能导致状态不一致。
		log.Printf("[warn] session %s not in memory, rebuilt from DB (runtime state reset) workspace=%s", sessionID, workspaceID)
		m.sessions[sessionID] = sess
	}
	if sess.WorkspaceID != workspaceID || sess.UserID != userID {
		return nil, fmt.Errorf("无权访问该会话")
	}
	sess.Touch()
	if m.sessionRepo != nil {
		wCtx, cancel := dbContext(ctx)
		_ = m.sessionRepo.UpdateLastSeen(wCtx, sessionID)
		cancel()
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

func (m *Manager) Delete(sessionID, workspaceID, userID string) error {
	m.mu.Lock()
	sess, ok := m.sessions[sessionID]
	if ok {
		if sess.WorkspaceID != workspaceID || sess.UserID != userID {
			m.mu.Unlock()
			return fmt.Errorf("无权访问该会话")
		}
		delete(m.sessions, sessionID)
	}
	m.mu.Unlock()

	if ok {
		return sess.Destroy()
	}
	return data.DestroySessionDB(m.cacheRoot, sessionID)
}

func (m *Manager) Stop(sessionID, workspaceID, userID string) error {
	m.mu.Lock()
	sess, ok := m.sessions[sessionID]
	m.mu.Unlock()
	if !ok {
		return nil
	}
	if sess.WorkspaceID != workspaceID || sess.UserID != userID {
		return fmt.Errorf("无权访问该会话")
	}
	sess.CancelRun("")
	if !sess.WaitUntilIdle(sessionStopTimeout) {
		return fmt.Errorf("会话仍有任务在运行，无法删除")
	}
	return nil
}

// IsSessionLive 判断 session 是否存在于内存中（有活跃引擎 / 等待态 run）。
// 用于 bootstrap 阶段识别 stale run（DB 中仍为 running/waiting_user_input 但无引擎持有）。
func (m *Manager) IsSessionLive(sessionID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.sessions[sessionID]
	return ok
}
