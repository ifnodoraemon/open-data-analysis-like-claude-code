package repository

import (
	"context"

	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/domain"
)

type ReportRepository interface {
	Create(ctx context.Context, report *domain.Report) error
	GetByRunID(ctx context.Context, runID string) (*domain.Report, error)
}
