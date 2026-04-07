package agent

import "github.com/ifnodoraemon/openDataAnalysis/domain"

// WSEvent WebSocket 事件类型
type WSEvent struct {
	Type      string      `json:"type"`
	SessionID string      `json:"sessionId,omitempty"`
	RunID     string      `json:"runId,omitempty"`
	Data      interface{} `json:"data"`
}

// 事件类型常量
const (
	EventSessionReady          = "session_ready"
	EventSessionReset          = "session_reset"
	EventUserMessage           = "user_message"
	EventRunStarted            = "run_started"
	EventThinking              = "thinking"
	EventToolCall              = "tool_call"
	EventToolResult            = "tool_result"
	EventReportUpdate          = "report_update"
	EventReportFinal           = "report_final"
	EventError                 = "error"
	EventRunCompleted          = "run_completed"
	EventRunCancelled          = "run_cancelled"
	EventStop                  = "stop_run"
	EventReset                 = "reset_session"
	EventUserRequestInput      = "user_request_input"
	EventStateSubgoalsUpdated  = "state_subgoals_updated"
	EventStateMemoryUpdated    = "state_memory_updated"
	EventStateChildRunsUpdated = "state_child_runs_updated"
)

// UserMessage 用户输入
type UserMessage struct {
	Content     string             `json:"content"`
	EditContext *ReportEditContext `json:"editContext,omitempty"`
}

type ReportEditContext struct {
	Mode                string `json:"mode,omitempty"`
	TargetRunID         string `json:"targetRunId,omitempty"`
	BlockID             string `json:"blockId,omitempty"`
	SelectionText       string `json:"selectionText,omitempty"`
	PreserveOtherBlocks bool   `json:"preserveOtherBlocks,omitempty"`
}

type StopRunRequest struct {
	RunID string `json:"runId,omitempty"`
}

type ResetSessionRequest struct {
	KeepFiles bool `json:"keepFiles"`
}

type SessionReadyData struct {
	SessionID      string                 `json:"sessionId"`
	Files          []UploadedFile         `json:"files,omitempty"`
	Subgoals       []Subgoal              `json:"subgoals,omitempty"`
	Memory         map[string]string      `json:"memory,omitempty"`
	ReportHTML     string                 `json:"report_html,omitempty"`
	ReportSnapshot *domain.ReportSnapshot `json:"report_snapshot,omitempty"`
}

type SessionResetData struct {
	KeepFiles bool           `json:"keepFiles"`
	Files     []UploadedFile `json:"files,omitempty"`
}

type RunStartedData struct {
	RunID string `json:"runId"`
}

type UploadedFile struct {
	FileID string `json:"fileId"`
	Name   string `json:"name"`
	Size   int64  `json:"size"`
}

// ThinkingData 思考事件
type ThinkingData struct {
	Content string `json:"content"`
}

// ToolCallData 工具调用事件
type ToolCallData struct {
	ID        string      `json:"id"`
	Name      string      `json:"name"`
	Arguments interface{} `json:"arguments"`
}

// ToolResultData 工具结果事件
type ToolResultData struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Result   string `json:"result"`
	Duration int64  `json:"duration"` // 毫秒
	Success  bool   `json:"success"`
}

// AskUserOption 确认协议的结构化选项
type AskUserOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Hint  string `json:"hint,omitempty"`
}

// AskUserData 等待用户回答事件（结构化确认协议）
type AskUserData struct {
	Question      string          `json:"question"`
	Reason        string          `json:"reason,omitempty"`      // 确认原因：为什么需要用户确认
	Scope         string          `json:"scope,omitempty"`       // 作用域: join_key | metric | time_grain | filter | general
	ContextRef    string          `json:"context_ref,omitempty"` // 关联上下文（表名、列名等）
	Required      bool            `json:"required"`
	AllowMultiple bool            `json:"allow_multiple,omitempty"` // 是否允许多选
	Options       []AskUserOption `json:"options,omitempty"`        // 结构化选项
}

type MemoryUpdatedData struct {
	Facts map[string]string `json:"facts"`
}

type ChildRunsUpdatedData struct {
	ParentRunID string                   `json:"parentRunId"`
	ChildRuns   []map[string]interface{} `json:"childRuns"`
}

// ReportUpdateData 研报更新事件
type ReportUpdateData struct {
	HTML           string                 `json:"html"`
	SectionID      string                 `json:"sectionId,omitempty"`
	Title          string                 `json:"title,omitempty"`
	ReportFileID   string                 `json:"reportFileId,omitempty"`
	ReportSnapshot *domain.ReportSnapshot `json:"report_snapshot,omitempty"`
}

// ErrorData 错误事件
type ErrorData struct {
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

// CompleteData 完成事件
type CompleteData struct {
	Summary     string `json:"summary"`
	DownloadURL string `json:"downloadUrl,omitempty"`
}
