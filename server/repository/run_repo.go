package repository

import (
	"context"

	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/domain"
)

type RunRepository interface {
	Create(ctx context.Context, run *domain.AnalysisRun) error
	GetByID(ctx context.Context, runID string) (*domain.AnalysisRun, error)
	UpdateStatus(ctx context.Context, runID string, status domain.RunStatus, errMsg *string) error
	BindReportFile(ctx context.Context, runID, reportFileID string) error
}
