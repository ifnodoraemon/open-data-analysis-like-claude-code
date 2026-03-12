package domain

import "time"

type RunStatus string

const (
	RunStatusQueued    RunStatus = "queued"
	RunStatusRunning   RunStatus = "running"
	RunStatusCompleted RunStatus = "completed"
	RunStatusFailed    RunStatus = "failed"
	RunStatusCancelled RunStatus = "cancelled"
)

type AnalysisRun struct {
	ID           string
	SessionID    string
	WorkspaceID  string
	UserID       string
	Status       RunStatus
	InputMessage string
	ErrorMessage *string
	ReportFileID *string
	StartedAt    *time.Time
	FinishedAt   *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
