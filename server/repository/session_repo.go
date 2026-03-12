package repository

import (
	"context"

	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/domain"
)

type SessionRepository interface {
	Create(ctx context.Context, session *domain.Session) error
	GetByID(ctx context.Context, sessionID string) (*domain.Session, error)
	ListByUserWorkspace(ctx context.Context, userID, workspaceID string, limit int) ([]domain.Session, error)
	UpdateLastSeen(ctx context.Context, sessionID string) error
	UpdateLastRun(ctx context.Context, sessionID, runID string) error
}
