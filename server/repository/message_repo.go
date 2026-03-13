package repository

import (
	"context"

	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/domain"
)

type MessageRepository interface {
	Create(ctx context.Context, msg *domain.RunMessage) error
	ListByRun(ctx context.Context, runID string) ([]domain.RunMessage, error)
}
