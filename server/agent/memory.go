package agent

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// WorkingMemory 存储已经确认的事实、口径和核心发现
type WorkingMemory struct {
	Facts map[string]string `json:"facts"`
	mu    sync.RWMutex
}

func NewWorkingMemory() *WorkingMemory {
	return &WorkingMemory{
		Facts: make(map[string]string),
	}
}

// SaveFact 保存一条核心记忆，若 key 已存在则覆盖更新
func (m *WorkingMemory) SaveFact(key, fact string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Facts[key] = fact
}

// RemoveFact 移除一条核心记忆
func (m *WorkingMemory) RemoveFact(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.Facts, key)
}

// GetSummary 输出格式化的大模型提示词上下文
func (m *WorkingMemory) GetSummary() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.Facts) == 0 {
		return "Working Memory: <empty>"
	}

	var builder strings.Builder
	builder.WriteString("=== 已确认的工作记忆 (Working Memory) ===\n")
	builder.WriteString("这里存放了你之前确定的关键口径、字段含义和中间结论。请在后续分析中优先使用这些信息，避免重复查证：\n")
	for k, v := range m.Facts {
		builder.WriteString(fmt.Sprintf("- [%s]: %s\n", k, v))
	}
	builder.WriteString("=====================================\n")
	return builder.String()
}

type SubgoalStatus string

const (
	StatusPending  SubgoalStatus = "pending"
	StatusRunning  SubgoalStatus = "running"
	StatusComplete SubgoalStatus = "complete"
	StatusRejected SubgoalStatus = "rejected"
)

type Subgoal struct {
	ID          string        `json:"id"`
	Description string        `json:"description"`
	Status      SubgoalStatus `json:"status"`
	Result      string        `json:"result,omitempty"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
}

// SubgoalManager 维护 Agent 当前计划的待解决问题树
type SubgoalManager struct {
	Goals []Subgoal `json:"goals"`
	mu    sync.RWMutex
}

func NewSubgoalManager() *SubgoalManager {
	return &SubgoalManager{
		Goals: make([]Subgoal, 0),
	}
}

// AddGoal 增加一个新的子任务
func (s *SubgoalManager) AddGoal(description string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := "goal_" + uuid.New().String()[:8]
	s.Goals = append(s.Goals, Subgoal{
		ID:          id,
		Description: description,
		Status:      StatusPending,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	})
	return id
}

// UpdateGoalStatus 更新任务状态（完成、拒绝等）
func (s *SubgoalManager) UpdateGoalStatus(id string, status SubgoalStatus, result string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.Goals {
		if s.Goals[i].ID == id {
			s.Goals[i].Status = status
			if result != "" {
				s.Goals[i].Result = result
			}
			s.Goals[i].UpdatedAt = time.Now()
			return nil
		}
	}
	return fmt.Errorf("subgoal with ID %s not found", id)
}

// IsAllCompleted 检查所有的目标是否都已经终态
func (s *SubgoalManager) IsAllCompleted() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.Goals) == 0 {
		return true // 如果没有任何目标，也可以视为完成，或者按需调整
	}

	for _, g := range s.Goals {
		if g.Status == StatusPending || g.Status == StatusRunning {
			return false
		}
	}
	return true
}

// GetSummary 格式化输出供 LLM 使用的当前子任务树状态
func (s *SubgoalManager) GetSummary() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.Goals) == 0 {
		return "Subgoals Tree: <empty>"
	}

	var builder strings.Builder
	builder.WriteString("=== 动态子目标树 (Subgoals / Tree of Tasks) ===\n")
	builder.WriteString("这是你当前正在推进的任务清单。你必须依靠这些状态来决定下一步做什么，或者什么时候可以生成报告：\n")

	for _, g := range s.Goals {
		statusIcon := "[ ]"
		switch g.Status {
		case StatusRunning:
			statusIcon = "[~]"
		case StatusComplete:
			statusIcon = "[x]"
		case StatusRejected:
			statusIcon = "[-]"
		}

		builder.WriteString(fmt.Sprintf("%s ID: %s | 目标: %s\n", statusIcon, g.ID, g.Description))
		if g.Result != "" {
			builder.WriteString(fmt.Sprintf("    -> 结论: %s\n", g.Result))
		}
	}
	builder.WriteString("==================================================\n")
	return builder.String()
}
