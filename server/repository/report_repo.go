package repository

import (
	"context"

	"github.com/ifnodoraemon/openDataAnalysis/domain"
)

type ReportRepository interface {
	Create(ctx context.Context, report *domain.Report) error
	GetByRunID(ctx context.Context, runID string) (*domain.Report, error)
}
