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

// Snapshot 返回当前工作记忆的副本。
func (m *WorkingMemory) Snapshot() map[string]string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	res := make(map[string]string, len(m.Facts))
	for k, v := range m.Facts {
		res[k] = v
	}
	return res
}

// Reset 清空所有工作记忆。
func (m *WorkingMemory) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Facts = make(map[string]string)
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
	ID           string        `json:"id"`
	ParentGoalID string        `json:"parentGoalId,omitempty"`
	Description  string        `json:"description"`
	Status       SubgoalStatus `json:"status"`
	Result       string        `json:"result,omitempty"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

// SubgoalManager 维护 Agent 当前计划的待解决问题树
type SubgoalManager struct {
	Goals []Subgoal `json:"goals"`
	mu    sync.RWMutex
}

type finalizeSnapshot struct {
	roots    []Subgoal
	children map[string][]Subgoal
}

func NewSubgoalManager() *SubgoalManager {
	return &SubgoalManager{
		Goals: make([]Subgoal, 0),
	}
}

// AddGoal 增加一个新的子任务
func (s *SubgoalManager) AddGoal(description, parentGoalID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := "goal_" + uuid.New().String()[:8]
	s.Goals = append(s.Goals, Subgoal{
		ID:           id,
		ParentGoalID: strings.TrimSpace(parentGoalID),
		Description:  description,
		Status:       StatusPending,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
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

func isTerminalSubgoalStatus(status SubgoalStatus) bool {
	return status == StatusComplete || status == StatusRejected
}

func (s *SubgoalManager) snapshotLocked() finalizeSnapshot {
	byID := make(map[string]Subgoal, len(s.Goals))
	for _, g := range s.Goals {
		byID[g.ID] = g
	}

	children := make(map[string][]Subgoal, len(s.Goals))
	roots := make([]Subgoal, 0)
	for _, g := range s.Goals {
		parentID := strings.TrimSpace(g.ParentGoalID)
		if parentID == "" {
			roots = append(roots, g)
			continue
		}
		if _, ok := byID[parentID]; !ok {
			roots = append(roots, g)
			continue
		}
		children[parentID] = append(children[parentID], g)
	}

	return finalizeSnapshot{
		roots:    roots,
		children: children,
	}
}

func (s *SubgoalManager) collectActiveBranchLines(snapshot finalizeSnapshot) []string {
	if len(snapshot.roots) == 0 {
		return nil
	}

	branches := make([]string, 0)
	var dfs func(goal Subgoal, path []string)
	dfs = func(goal Subgoal, path []string) {
		if isTerminalSubgoalStatus(goal.Status) {
			return
		}

		step := fmt.Sprintf("%s[%s]", goal.Description, goal.Status)
		path = append(path, step)

		hasNonTerminalChild := false
		for _, child := range snapshot.children[goal.ID] {
			if isTerminalSubgoalStatus(child.Status) {
				continue
			}
			hasNonTerminalChild = true
			dfs(child, path)
		}
		if !hasNonTerminalChild {
			branches = append(branches, strings.Join(path, " -> "))
		}
	}

	for _, root := range snapshot.roots {
		if isTerminalSubgoalStatus(root.Status) {
			continue
		}
		dfs(root, nil)
	}
	return branches
}

// CanFinalize 检查当前是否允许结束。
// 判定只基于根目标是否闭环；已闭环根目标下面遗留的旧子步骤不会继续阻塞结束。
func (s *SubgoalManager) CanFinalize() (bool, []string) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.Goals) == 0 {
		return true, nil
	}

	snapshot := s.snapshotLocked()
	blockers := s.collectActiveBranchLines(snapshot)
	return len(blockers) == 0, blockers
}

// AutoCompleteReportGoals 在进入最终收尾前自动闭合明显属于“产出图表/报告”的根目标。
func (s *SubgoalManager) AutoCompleteReportGoals(result string) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	completed := 0
	now := time.Now()
	for i := range s.Goals {
		goal := &s.Goals[i]
		if strings.TrimSpace(goal.ParentGoalID) != "" {
			continue
		}
		if isTerminalSubgoalStatus(goal.Status) {
			continue
		}
		if !looksLikeReportGoal(goal.Description) {
			continue
		}
		goal.Status = StatusComplete
		if strings.TrimSpace(result) != "" {
			goal.Result = result
		}
		goal.UpdatedAt = now
		completed++
	}
	return completed
}

func looksLikeReportGoal(description string) bool {
	hint := strings.ToLower(strings.TrimSpace(description))
	if hint == "" {
		return false
	}
	keywords := []string{"报告", "研报", "图表", "report", "chart", "dashboard", "finalize"}
	for _, keyword := range keywords {
		if strings.Contains(hint, keyword) {
			return true
		}
	}
	return false
}

// GetSummary 格式化输出供 LLM 使用的当前子任务树状态
func (s *SubgoalManager) GetSummary() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.Goals) == 0 {
		return "当前没有进行中的子任务。"
	}

	var builder strings.Builder
	builder.WriteString("【当前待解决的目标清单】\n")
	snapshot := s.snapshotLocked()
	for _, root := range snapshot.roots {
		s.renderSummary(&builder, snapshot.children, root, 0)
	}
	if blockers := s.collectActiveBranchLines(snapshot); len(blockers) > 0 {
		builder.WriteString("\n【当前阻塞收尾的 Active Branch】\n")
		for _, branch := range blockers {
			builder.WriteString("- ")
			builder.WriteString(branch)
			builder.WriteString("\n")
		}
	}
	return builder.String()
}

func (s *SubgoalManager) renderSummary(builder *strings.Builder, children map[string][]Subgoal, goal Subgoal, depth int) {
	mark := "[ ]"
	switch goal.Status {
	case StatusRunning:
		mark = "[~]"
	case StatusComplete:
		mark = "[x]"
	case StatusRejected:
		mark = "[-]"
	}
	indent := strings.Repeat("  ", depth)
	builder.WriteString(fmt.Sprintf("%s%s ID:%s | %s\n", indent, mark, goal.ID, goal.Description))
	if goal.Result != "" {
		builder.WriteString(fmt.Sprintf("%s    -> 结论/原因: %s\n", indent, goal.Result))
	}
	for _, child := range children[goal.ID] {
		s.renderSummary(builder, children, child, depth+1)
	}
}

// ListAll 导出当前所有的目标（返回副本，避免外部由于并发修改切片导致 panic）
func (s *SubgoalManager) ListAll() []Subgoal {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res := make([]Subgoal, len(s.Goals))
	copy(res, s.Goals)
	return res
}

// Reset 清空全部子目标。
func (s *SubgoalManager) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Goals = make([]Subgoal, 0)
}
