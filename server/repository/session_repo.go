package repository

import (
	"context"
	"time"

	"github.com/ifnodoraemon/openDataAnalysis/domain"
)

type SessionRepository interface {
	Create(ctx context.Context, session *domain.Session) error
	GetByID(ctx context.Context, sessionID string) (*domain.Session, error)
	ListByUserWorkspace(ctx context.Context, userID, workspaceID string, limit int) ([]domain.Session, error)
	UpdateTitle(ctx context.Context, sessionID, title string) error
	UpdateLastSeen(ctx context.Context, sessionID string) error
	UpdateLastRun(ctx context.Context, sessionID, runID string) error
	ListExpired(ctx context.Context, cutoff time.Time, limit int) ([]domain.Session, error)
	Delete(ctx context.Context, sessionID string) error
}
