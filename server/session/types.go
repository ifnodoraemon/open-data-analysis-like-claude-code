package session

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/ifnodoraemon/openDataAnalysis/agent"
	"github.com/ifnodoraemon/openDataAnalysis/config"
	"github.com/ifnodoraemon/openDataAnalysis/data"
	"github.com/ifnodoraemon/openDataAnalysis/domain"
	"github.com/ifnodoraemon/openDataAnalysis/service"
	"github.com/ifnodoraemon/openDataAnalysis/tools"
)

type RunState struct {
	RunID     string
	Status    string
	Cancel    context.CancelFunc
	StartedAt time.Time
}

type ReportSnapshotLoader interface {
	LoadReportSnapshot(ctx context.Context, sessionID, workspaceID, userID, runID string) (*domain.ReportSnapshot, error)
}

type Session struct {
	ID           string
	WorkspaceID  string
	UserID       string
	CacheRoot    string
	FileService  *service.FileService
	Ingester     *data.Ingester
	Registry     *tools.Registry
	Engine       *agent.Engine
	ReportState  *tools.ReportState
	EditState    *tools.ReportEditState
	FinalizeTool *tools.FinalizeReportTool
	Memory       *agent.WorkingMemory
	Subgoals     *agent.SubgoalManager
	ActiveRun    *RunState
	CreatedAt    time.Time
	LastSeenAt   time.Time
	mu           sync.Mutex
}

func New(id, workspaceID, userID, cacheRoot string, fileService *service.FileService) (*Session, error) {
	ingester := data.NewIngester(cacheRoot)
	if err := ingester.InitDB(id); err != nil {
		return nil, err
	}

	// 当 LLM 配置可用时，将语义预分析器注入 Ingester，导入文件后自动触发
	if config.Cfg != nil && config.Cfg.LLMAPIKey != "" {
		llmClient := agent.NewLLMClient()
		ingester.SemanticEnricher = llmClient.SimpleChatFunc()
	}

	memory := agent.NewWorkingMemory()
	subgoals := agent.NewSubgoalManager()

	s := &Session{
		ID:          id,
		WorkspaceID: workspaceID,
		UserID:      userID,
		CacheRoot:   cacheRoot,
		FileService: fileService,
		Ingester:    ingester,
		ReportState: &tools.ReportState{},
		EditState:   &tools.ReportEditState{},
		Memory:      memory,
		Subgoals:    subgoals,
		CreatedAt:   time.Now(),
		LastSeenAt:  time.Now(),
	}

	ctx := tools.ToolContext{
		Ingester:          s.Ingester,
		ReportState:       s.ReportState,
		EditState:         s.EditState,
		FileFactsProvider: s.BuildFileFacts,
		Memory:            memory,
		Subgoals:          subgoals,
		FileMaterializer: func(fileID string) (*tools.FileReference, error) {
			tempPath, file, err := s.FileService.MaterializeToTemp(context.Background(), fileID)
			if err != nil {
				return nil, err
			}
			return &tools.FileReference{
				FileID:      file.ID,
				DisplayName: file.DisplayName,
				StoredPath:  tempPath,
			}, nil
		},
	}

	masterReg := tools.NewRegistry()
	masterReg.LoadGlobalTools(ctx)

	// 主控和子 Agent 使用同一套工具语义；子 Agent 只是按本次任务裁剪过工具边界的递归实例。
	plannerAllowed := []string{
		"data_load_file",
		"state_session_files_inspect",
		"data_list_tables",
		"data_describe_table",
		"data_query_sql",
		"report_create_chart",
		"report_manage_blocks",
		"report_configure_layout",
		"report_finalize",
		"memory_save_fact",
		"state_memory_inspect",
		"state_goal_inspect",
		"state_report_inspect",
		"state_report_edit_inspect",
		"goal_manage",
		"user_request_input",
		"task_delegate",
	}
	if pt, err := masterReg.Get("code_run_python"); err == nil {
		if runPython, ok := pt.(*tools.RunPythonTool); ok {
			runPython.MCPEndpoint = config.Cfg.PythonMCPURL
			if err := runPython.HealthCheck(context.Background()); err != nil {
				log.Printf("code_run_python disabled for session %s: %v", id, err)
			} else {
				plannerAllowed = append(plannerAllowed, "code_run_python")
			}
		}
	}
	plannerRegistry := masterReg.CloneFiltered(plannerAllowed)

	s.Registry = plannerRegistry
	if ft, err := plannerRegistry.Get("report_finalize"); err == nil {
		s.FinalizeTool = ft.(*tools.FinalizeReportTool)
	}

	if dt, err := plannerRegistry.Get("task_delegate"); err == nil {
		if dtTool, ok := dt.(*agent.DelegateTaskTool); ok {
			dtTool.BaseRegistry = plannerRegistry
			dtTool.Subgoals = subgoals
			dtTool.Memory = memory
		}
	}

	policyPrompt := agent.BuildPolicyPrompt()
	s.Engine = agent.NewEngine(plannerRegistry, policyPrompt)

	return s, nil
}

func (s *Session) Touch() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LastSeenAt = time.Now()
}

func (s *Session) FilesForClient() []agent.UploadedFile {
	files, err := s.FileService.GetSessionFiles(context.Background(), s.ID)
	if err != nil {
		return nil
	}

	clientFiles := make([]agent.UploadedFile, 0, len(files))
	for _, file := range files {
		clientFiles = append(clientFiles, agent.UploadedFile{
			FileID: file.ID,
			Name:   file.DisplayName,
			Size:   file.SizeBytes,
		})
	}
	return clientFiles
}

func (s *Session) BuildFileFacts() ([]tools.SessionFileFact, error) {
	files, err := s.FileService.GetSessionFiles(context.Background(), s.ID)
	if err != nil {
		return nil, err
	}
	facts := make([]tools.SessionFileFact, 0, len(files))
	for _, file := range files {
		fact := tools.SessionFileFact{
			FileID:      file.ID,
			DisplayName: file.DisplayName,
		}
		tableName := strings.TrimSuffix(file.DisplayName, filepath.Ext(file.DisplayName))
		tableName = sanitizeTableName(tableName)
		if tableName != "" {
			fact.TableName = tableName
		}
		if s.Ingester != nil && tableName != "" {
			schemaJSON, metaErr := s.Ingester.GetTableMetadata(tableName)
			if metaErr == nil && strings.TrimSpace(schemaJSON) != "" {
				fact.SchemaAvailable = true
				var schemaPayload interface{}
				if err := json.Unmarshal([]byte(schemaJSON), &schemaPayload); err == nil {
					fact.SchemaSummary = schemaPayload
				} else {
					fact.SchemaSummary = schemaJSON
				}
			}
		}
		facts = append(facts, fact)
	}
	return facts, nil
}

// sanitizeTableName 保持与 Ingester 中相同的逻辑以便匹配表名
func sanitizeTableName(name string) string {
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, "-", "_")
	return strings.ToLower(name)
}

func (s *Session) StartRun(parent context.Context) (string, context.Context, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.ActiveRun != nil {
		return "", nil, fmt.Errorf("已有任务正在行中或尚未清理，请稍候停止后再试")
	}

	runID := "r_" + uuid.New().String()[:8]
	ctx, cancel := context.WithCancel(parent)
	s.ActiveRun = &RunState{
		RunID:     runID,
		Status:    "running",
		Cancel:    cancel,
		StartedAt: time.Now(),
	}
	s.LastSeenAt = time.Now()
	return runID, ctx, nil
}

func (s *Session) FinishRun(runID, status string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.ActiveRun != nil && s.ActiveRun.RunID == runID {
		s.ActiveRun.Status = status
		s.ActiveRun.Cancel = nil
		s.ActiveRun = nil
	}
	if s.EditState != nil {
		s.EditState.Reset()
	}
	s.LastSeenAt = time.Now()
}

func (s *Session) SuspendRun(runID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ActiveRun != nil && s.ActiveRun.RunID == runID {
		s.ActiveRun.Status = "waiting_user_input"
	}
	s.LastSeenAt = time.Now()
}

func (s *Session) ResumeRun(runID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ActiveRun != nil && s.ActiveRun.RunID == runID {
		s.ActiveRun.Status = "running"
	}
	s.LastSeenAt = time.Now()
}

// GetWaitingRunID 返回正在等待用户输入的 run ID。
func (s *Session) GetWaitingRunID() (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ActiveRun != nil && s.ActiveRun.Status == "waiting_user_input" {
		return s.ActiveRun.RunID, true
	}
	return "", false
}

// ConsumeWaitingRun 原子地检查并清除 waiting_user_input 状态。
// 若当前确实处于等待状态，则将状态改为 running 并返回 runID。
// 返回 empty string 表示当前不处于等待状态（已被其它 goroutine 消费）。
// 用于替代原来的 GetWaitingRunID + ResumeRun 两步操作，将第二次重复提交的竞态消除。
func (s *Session) ConsumeWaitingRun() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ActiveRun == nil || s.ActiveRun.Status != "waiting_user_input" {
		return ""
	}
	runID := s.ActiveRun.RunID
	s.ActiveRun.Status = "running"
	s.LastSeenAt = time.Now()
	return runID
}

func (s *Session) CancelRun(runID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.ActiveRun == nil {
		return false
	}
	if runID != "" && s.ActiveRun.RunID != runID {
		return false
	}
	if s.ActiveRun.Cancel != nil {
		s.ActiveRun.Status = "cancelling"
		s.ActiveRun.Cancel()
	}
	s.LastSeenAt = time.Now()
	return true
}

func (s *Session) WaitUntilIdle(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		s.mu.Lock()
		idle := s.ActiveRun == nil
		s.mu.Unlock()
		if idle {
			return true
		}
		if timeout <= 0 || time.Now().After(deadline) {
			return false
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func (s *Session) ConfigureEditState(edit *agent.ReportEditContext) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.EditState == nil {
		s.EditState = &tools.ReportEditState{}
	}
	if edit == nil {
		s.EditState.Reset()
		return
	}
	s.EditState.Mode = strings.TrimSpace(edit.Mode)
	s.EditState.TargetRunID = strings.TrimSpace(edit.TargetRunID)
	s.EditState.TargetBlockID = strings.TrimSpace(edit.BlockID)
	s.EditState.SelectionText = strings.TrimSpace(edit.SelectionText)
	s.EditState.PreserveOtherBlocks = edit.PreserveOtherBlocks
	s.EditState.RefreshFromReportState(s.ReportState)
}

func (s *Session) ClearEditState() {
	s.ConfigureEditState(nil)
}

func (s *Session) LoadReportSnapshot(snapshot *domain.ReportSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if snapshot == nil {
		return
	}
	s.ReportState.FinalTitle = strings.TrimSpace(snapshot.Title)
	s.ReportState.FinalAuthor = strings.TrimSpace(snapshot.Author)
	s.ReportState.Layout = tools.ReportLayout{
		CustomHTMLShell: snapshot.Layout.CustomHTMLShell,
		CustomCSS:       snapshot.Layout.CustomCSS,
		CustomJS:        snapshot.Layout.CustomJS,
		BodyClass:       snapshot.Layout.BodyClass,
		HideCover:       snapshot.Layout.HideCover,
		HideTOC:         snapshot.Layout.HideTOC,
	}
	s.ReportState.NeedsFinalize = false
	s.ReportState.Blocks = make([]tools.ReportBlock, 0, len(snapshot.Blocks))
	for _, block := range snapshot.Blocks {
		rb := tools.ReportBlock{
			ID:      block.ID,
			Kind:    block.Kind,
			Title:   block.Title,
			Content: block.Content,
			ChartID: block.ChartID,
		}
		if len(block.Sources) > 0 {
			var sources []tools.EvidenceRef
			if err := json.Unmarshal(block.Sources, &sources); err == nil {
				rb.Sources = sources
			}
		}
		s.ReportState.Blocks = append(s.ReportState.Blocks, rb)
	}
	s.ReportState.Charts = make([]tools.ChartData, 0, len(snapshot.Charts))
	for _, chart := range snapshot.Charts {
		s.ReportState.Charts = append(s.ReportState.Charts, tools.ChartData{
			ID:     chart.ID,
			Option: chart.Option,
			Width:  chart.Width,
			Height: chart.Height,
		})
	}
	if s.EditState != nil {
		s.EditState.RefreshFromReportState(s.ReportState)
	}
}

func (s *Session) PrepareUserRun(ctx context.Context, userMsg agent.UserMessage, loader ReportSnapshotLoader) error {
	var snapshot *domain.ReportSnapshot
	if userMsg.EditContext != nil && strings.TrimSpace(userMsg.EditContext.TargetRunID) != "" {
		if loader == nil {
			return fmt.Errorf("缺少报告快照加载器")
		}
		loaded, err := loader.LoadReportSnapshot(ctx, s.ID, s.WorkspaceID, s.UserID, strings.TrimSpace(userMsg.EditContext.TargetRunID))
		if err != nil {
			return err
		}
		snapshot = loaded
	}
	if snapshot != nil {
		s.LoadReportSnapshot(snapshot)
	}
	s.ConfigureEditState(userMsg.EditContext)
	return nil
}

func (s *Session) Reset(keepFiles bool) error {
	s.CancelRun("")
	s.Engine.ResetMessages()
	if s.Memory != nil {
		s.Memory.Reset()
	}
	if s.Subgoals != nil {
		s.Subgoals.Reset()
	}

	s.mu.Lock()
	s.ReportState.Blocks = nil
	s.ReportState.Charts = nil
	s.ReportState.FinalTitle = ""
	s.ReportState.FinalAuthor = ""
	s.ReportState.Layout = tools.ReportLayout{}
	s.ReportState.NeedsFinalize = false
	if s.EditState != nil {
		s.EditState.Reset()
	}
	s.LastSeenAt = time.Now()
	s.mu.Unlock()

	if err := s.Ingester.ResetDB(s.ID); err != nil {
		return err
	}

	// 文件元数据已经通过 FileRepository 与 session 关联，当前阶段 keepFiles=false 仅重置分析状态。
	_ = keepFiles
	return os.MkdirAll(s.FileService.TempDir, 0o755)
}

func (s *Session) Destroy() error {
	if s == nil {
		return nil
	}
	s.CancelRun("")
	if s.Ingester != nil {
		return s.Ingester.Destroy()
	}
	return nil
}

func (s *Session) RuntimeState() (map[string]string, []agent.Subgoal) {
	var memory map[string]string
	var subgoals []agent.Subgoal
	if s.Memory != nil {
		memory = s.Memory.Snapshot()
	}
	if s.Subgoals != nil {
		subgoals = s.Subgoals.ListAll()
	}
	return memory, subgoals
}

func (s *Session) RuntimeVars() []agent.RuntimeContextBlock {
	var vars []agent.RuntimeContextBlock
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. Active Edit Scope
	if s.EditState != nil && s.EditState.Active() {
		content := fmt.Sprintf("Mode: %s\n", s.EditState.Mode)
		if s.EditState.TargetBlockID != "" {
			content += fmt.Sprintf("TargetBlockID: %s\n", s.EditState.TargetBlockID)
		}
		if s.EditState.SelectionText != "" {
			content += fmt.Sprintf("SelectionText: %s\n", s.EditState.SelectionText)
		}
		vars = append(vars, agent.RuntimeContextBlock{
			Name:    "active_edit_scope",
			Content: strings.TrimSpace(content),
		})
	}

	// 2. Active Subgoals
	if s.Subgoals != nil {
		var activeGoals []string
		for _, g := range s.Subgoals.ListAll() {
			if g.Status == "pending" || g.Status == "running" {
				activeGoals = append(activeGoals, fmt.Sprintf("- [%s] %s (%s)", g.ID, g.Description, g.Status))
			}
		}
		if len(activeGoals) > 0 {
			vars = append(vars, agent.RuntimeContextBlock{
				Name:    "active_subgoals",
				Content: strings.Join(activeGoals, "\n"),
			})
		}
	}

	return vars
}
