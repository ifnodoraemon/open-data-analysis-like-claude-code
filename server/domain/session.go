package domain

import "time"

type SessionStatus string

const (
	SessionStatusActive SessionStatus = "active"
	SessionStatusClosed SessionStatus = "closed"
)

type Session struct {
	ID          string
	WorkspaceID string
	UserID      string
	Title       string
	Status      SessionStatus
	LastRunID   *string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	LastSeenAt  time.Time
}
