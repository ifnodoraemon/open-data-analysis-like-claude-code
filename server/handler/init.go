package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/auth"
	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/config"
	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/domain"
	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/metadata"
	sqliterepo "github.com/ifnodoraemon/open-data-analysis-like-claude-code/repository/sqlite"
	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/service"
	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/session"
	localstorage "github.com/ifnodoraemon/open-data-analysis-like-claude-code/storage/local"
)

var (
	defaultIdentity auth.Identity
	fileService     *service.FileService
	metadataStore   *metadata.Store
)

func Initialize() {
	defaultIdentity = auth.Identity{
		UserID:      config.Cfg.DefaultUserID,
		UserName:    config.Cfg.DefaultUserName,
		UserEmail:   config.Cfg.DefaultUserEmail,
		WorkspaceID: config.Cfg.DefaultWorkspaceID,
		Workspace:   config.Cfg.DefaultWorkspaceName,
	}

	store, err := metadata.Open(config.Cfg.MetadataDBPath)
	if err != nil {
		panic(err)
	}
	metadataStore = store

	userRepo := sqliterepo.NewUserRepository(store.DB)
	workspaceRepo := sqliterepo.NewWorkspaceRepository(store.DB)
	fileRepo := sqliterepo.NewFileRepository(store.DB)
	sessionRepo := sqliterepo.NewSessionRepository(store.DB)

	now := time.Now()
	_ = userRepo.Create(context.Background(), &domain.User{
		ID:        defaultIdentity.UserID,
		Email:     defaultIdentity.UserEmail,
		Name:      defaultIdentity.UserName,
		Status:    domain.UserStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	})
	_ = workspaceRepo.CreateWorkspace(context.Background(), &domain.Workspace{
		ID:          defaultIdentity.WorkspaceID,
		Name:        defaultIdentity.Workspace,
		Slug:        defaultIdentity.WorkspaceID,
		OwnerUserID: defaultIdentity.UserID,
		Status:      domain.WorkspaceStatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	_ = workspaceRepo.AddMember(context.Background(), &domain.WorkspaceMember{
		WorkspaceID: defaultIdentity.WorkspaceID,
		UserID:      defaultIdentity.UserID,
		Role:        domain.WorkspaceRoleOwner,
		CreatedAt:   now,
	})

	fileService = &service.FileService{
		Storage:       localstorage.New(config.Cfg.StorageRoot, ""),
		FileRepo:      fileRepo,
		WorkspaceRepo: workspaceRepo,
		TempDir:       config.Cfg.TempDir,
	}

	sessionManager = session.NewManager(config.Cfg.CacheRoot, defaultIdentity.WorkspaceID, defaultIdentity.UserID, fileService)
	sessionManager.SetSessionRepository(sessionRepo)
}

func AuthMiddleware(next http.Handler) http.Handler {
	return auth.Middleware(defaultIdentity)(next)
}
