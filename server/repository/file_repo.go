package repository

import (
	"context"

	"github.com/ifnodoraemon/openDataAnalysis/domain"
)

type FileRepository interface {
	Create(ctx context.Context, file *domain.File) error
	GetByID(ctx context.Context, fileID string) (*domain.File, error)
	ListBySession(ctx context.Context, sessionID string) ([]domain.File, error)
	AttachFilesToSession(ctx context.Context, sessionID string, fileIDs []string) error
}
