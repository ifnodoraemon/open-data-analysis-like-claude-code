package session

import (
	"context"
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
	"github.com/ifnodoraemon/openDataAnalysis/service"
	"github.com/ifnodoraemon/openDataAnalysis/tools"
)

type RunState struct {
	RunID     string
	Status    string
	Cancel    context.CancelFunc
	StartedAt time.Time
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
		Memory:      memory,
		Subgoals:    subgoals,
		CreatedAt:   time.Now(),
		LastSeenAt:  time.Now(),
	}

	ctx := tools.ToolContext{
		Ingester:    s.Ingester,
		ReportState: s.ReportState,
		Memory:      memory,
		Subgoals:    subgoals,
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

	plannerPrompt := agent.BuildPlannerPrompt(plannerRegistry)
	s.Engine = agent.NewEngine(plannerRegistry, plannerPrompt)

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

func (s *Session) BuildFileContext() string {
	files, err := s.FileService.GetSessionFiles(context.Background(), s.ID)
	if err != nil || len(files) == 0 {
		return ""
	}

	lines := "当前会话已上传文件与数据语义概况:\n"
	for _, file := range files {
		lines += fmt.Sprintf("=== 文件: %s (ID: %s) ===\n", file.DisplayName, file.ID)

		// 尝试从 Ingester 获取该文件（表名）的预分析语义摘要
		tableName := strings.TrimSuffix(file.DisplayName, filepath.Ext(file.DisplayName))
		tableName = sanitizeTableName(tableName)

		if s.Ingester != nil {
			schemaJSON, metaErr := s.Ingester.GetTableMetadata(tableName)
			if metaErr == nil {
				lines += fmt.Sprintf("AI 语义分析结果:\n%s\n", schemaJSON)
			} else {
				lines += "语义分析结果: 尚未生成或读取失败\n"
			}
		}
		lines += "\n"
	}

	return lines
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

func (s *Session) GetWaitingRunID() (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ActiveRun != nil && s.ActiveRun.Status == "waiting_user_input" {
		return s.ActiveRun.RunID, true
	}
	return "", false
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
	s.LastSeenAt = time.Now()
	s.mu.Unlock()

	if err := s.Ingester.ResetDB(s.ID); err != nil {
		return err
	}

	// 文件元数据已经通过 FileRepository 与 session 关联，当前阶段 keepFiles=false 仅重置分析状态。
	_ = keepFiles
	return os.MkdirAll(s.FileService.TempDir, 0o755)
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
