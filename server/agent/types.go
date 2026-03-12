package agent

// WSEvent WebSocket 事件类型
type WSEvent struct {
	Type      string      `json:"type"`
	SessionID string      `json:"sessionId,omitempty"`
	RunID     string      `json:"runId,omitempty"`
	Data      interface{} `json:"data"`
}

// 事件类型常量
const (
	EventSessionReady = "session_ready"
	EventSessionReset = "session_reset"
	EventUserMessage  = "user_message"
	EventRunStarted   = "run_started"
	EventThinking     = "thinking"
	EventToolCall     = "tool_call"
	EventToolResult   = "tool_result"
	EventReportUpdate = "report_update"
	EventReportFinal  = "report_final"
	EventError        = "error"
	EventRunCompleted = "run_completed"
	EventRunCancelled = "run_cancelled"
	EventStop         = "stop_run"
	EventReset        = "reset_session"
)

// UserMessage 用户输入
type UserMessage struct {
	Content string `json:"content"`
}

type StopRunRequest struct {
	RunID string `json:"runId,omitempty"`
}

type ResetSessionRequest struct {
	KeepFiles bool `json:"keepFiles"`
}

type SessionReadyData struct {
	SessionID string         `json:"sessionId"`
	Files     []UploadedFile `json:"files,omitempty"`
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

// ReportUpdateData 研报更新事件
type ReportUpdateData struct {
	HTML      string `json:"html"`
	SectionID string `json:"sectionId,omitempty"`
	Title     string `json:"title,omitempty"`
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
