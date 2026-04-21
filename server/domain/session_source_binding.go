package domain

import "time"

type SessionSourceBinding struct {
	SessionID        string
	SourceID         string
	ActiveSnapshotID string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}
