package repository

import (
	"context"

	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/domain"
)

type WorkspaceRepository interface {
	GetByID(ctx context.Context, workspaceID string) (*domain.Workspace, error)
	IsMember(ctx context.Context, workspaceID, userID string) (bool, error)
	CreateWorkspace(ctx context.Context, workspace *domain.Workspace) error
	AddMember(ctx context.Context, member *domain.WorkspaceMember) error
}
