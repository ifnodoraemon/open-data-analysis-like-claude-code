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
	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/agent"
	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/config"
	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/data"
	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/service"
	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/tools"
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

	s := &Session{
		ID:          id,
		WorkspaceID: workspaceID,
		UserID:      userID,
		CacheRoot:   cacheRoot,
		FileService: fileService,
		Ingester:    ingester,
		ReportState: &tools.ReportState{},
		CreatedAt:   time.Now(),
		LastSeenAt:  time.Now(),
	}

	registry := tools.NewRegistry()
	registry.Register(&tools.LoadDataTool{
		Ingester: s.Ingester,
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
	})
	registry.Register(&tools.ListTablesTool{Ingester: s.Ingester})
	registry.Register(&tools.DescribeDataTool{Ingester: s.Ingester})
	registry.Register(&tools.QueryDataTool{Ingester: s.Ingester})
	registry.Register(&tools.CreateChartTool{ReportState: s.ReportState})
	pythonTool := &tools.RunPythonTool{MCPEndpoint: config.Cfg.PythonMCPURL}
	pythonEnabled := true
	if err := pythonTool.HealthCheck(context.Background()); err != nil {
		pythonEnabled = false
		log.Printf("run_python disabled for session %s: %v", id, err)
	} else {
		registry.Register(pythonTool)
	}
	registry.Register(&tools.WriteSectionTool{ReportState: s.ReportState})
	finalizeTool := &tools.FinalizeReportTool{ReportState: s.ReportState}
	registry.Register(finalizeTool)

	s.Registry = registry
	s.FinalizeTool = finalizeTool
	s.Engine = agent.NewEngine(registry, agent.BuildSystemPrompt(pythonEnabled))

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

	s.mu.Lock()
	s.ReportState.Sections = nil
	s.ReportState.Charts = nil
	s.ReportState.FinalTitle = ""
	s.ReportState.FinalAuthor = ""
	s.LastSeenAt = time.Now()
	s.mu.Unlock()

	if err := s.Ingester.ResetDB(s.ID); err != nil {
		return err
	}

	// 文件元数据已经通过 FileRepository 与 session 关联，当前阶段 keepFiles=false 仅重置分析状态。
	_ = keepFiles
	return os.MkdirAll(s.FileService.TempDir, 0o755)
}
