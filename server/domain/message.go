package domain

import "time"

type RunMessage struct {
	ID        string      `json:"id"`
	RunID     string      `json:"runId"`
	SessionID string      `json:"sessionId"`
	WorkspaceID string    `json:"workspaceId"`
	Type      string      `json:"type"` // user, thinking, tool_call, tool_result, complete, error
	Name      string      `json:"name,omitempty"` // for tools
	Content   string      `json:"content"` // JSON stringification of Arguments or simple content
	Success   *bool       `json:"success,omitempty"`
	Duration  *int64      `json:"duration,omitempty"`
	CreatedAt time.Time   `json:"createdAt"`
}
