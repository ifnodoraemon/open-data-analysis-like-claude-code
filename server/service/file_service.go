package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/domain"
	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/repository"
	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/storage"
)

type UploadFileInput struct {
	UserID      string
	WorkspaceID string
	SessionID   string
	FileName    string
	ContentType string
	Size        int64
	Body        io.Reader
}

type FileService struct {
	Storage       storage.ObjectStorage
	FileRepo      repository.FileRepository
	WorkspaceRepo repository.WorkspaceRepository
	TempDir       string
}

func (s *FileService) Upload(ctx context.Context, in UploadFileInput) (*domain.File, error) {
	ok, err := s.WorkspaceRepo.IsMember(ctx, in.WorkspaceID, in.UserID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("用户无权访问工作区")
	}

	fileID := "f_" + uuid.New().String()[:8]
	key := SourceFileKey(in.WorkspaceID, fileID, sanitizeFilename(in.FileName))
	obj, err := s.Storage.Put(ctx, storage.PutObjectRequest{
		Key:         key,
		Body:        in.Body,
		Size:        in.Size,
		ContentType: in.ContentType,
		Metadata: map[string]string{
			"workspace_id": in.WorkspaceID,
			"session_id":   in.SessionID,
			"uploaded_by":  in.UserID,
			"file_id":      fileID,
		},
	})
	if err != nil {
		return nil, err
	}

	now := time.Now()
	file := &domain.File{
		ID:              fileID,
		WorkspaceID:     in.WorkspaceID,
		UploadedBy:      in.UserID,
		DisplayName:     in.FileName,
		Purpose:         domain.FilePurposeSource,
		ContentType:     in.ContentType,
		SizeBytes:       obj.Size,
		StorageProvider: obj.Provider,
		Bucket:          obj.Bucket,
		StorageKey:      obj.Key,
		Checksum:        obj.ETag,
		Status:          domain.FileStatusUploaded,
		Visibility:      domain.FileVisibilityPrivate,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.FileRepo.Create(ctx, file); err != nil {
		_ = s.Storage.Delete(ctx, obj.Key)
		return nil, err
	}
	if err := s.FileRepo.AttachFilesToSession(ctx, in.SessionID, []string{file.ID}); err != nil {
		_ = s.Storage.Delete(ctx, obj.Key)
		return nil, err
	}
	return file, nil
}

func (s *FileService) GetSessionFiles(ctx context.Context, sessionID string) ([]domain.File, error) {
	return s.FileRepo.ListBySession(ctx, sessionID)
}

func (s *FileService) GetFile(ctx context.Context, fileID string) (*domain.File, error) {
	return s.FileRepo.GetByID(ctx, fileID)
}

func (s *FileService) MaterializeToTemp(ctx context.Context, fileID string) (string, *domain.File, error) {
	file, err := s.FileRepo.GetByID(ctx, fileID)
	if err != nil {
		return "", nil, err
	}

	reader, err := s.Storage.Get(ctx, file.StorageKey)
	if err != nil {
		return "", nil, err
	}
	defer reader.Close()

	if err := os.MkdirAll(s.TempDir, 0o755); err != nil {
		return "", nil, fmt.Errorf("创建临时目录失败: %w", err)
	}

	tempPath := filepath.Join(s.TempDir, fmt.Sprintf("%s-%s", file.ID, sanitizeFilename(file.DisplayName)))
	dest, err := os.Create(tempPath)
	if err != nil {
		return "", nil, fmt.Errorf("创建临时文件失败: %w", err)
	}
	defer dest.Close()

	if _, err := io.Copy(dest, reader); err != nil {
		return "", nil, fmt.Errorf("写入临时文件失败: %w", err)
	}

	return tempPath, file, nil
}

type SaveReportInput struct {
	UserID      string
	WorkspaceID string
	SessionID   string
	RunID       string
	Title       string
	HTML        string
}

func (s *FileService) SaveReportHTML(ctx context.Context, in SaveReportInput) (*domain.File, error) {
	ok, err := s.WorkspaceRepo.IsMember(ctx, in.WorkspaceID, in.UserID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("用户无权访问工作区")
	}

	fileID := "rep_" + in.RunID
	displayName := sanitizeFilename(strings.TrimSpace(in.Title))
	if displayName == "" || displayName == "upload.bin" {
		displayName = "report-" + in.RunID
	}
	if filepath.Ext(displayName) != ".html" {
		displayName += ".html"
	}

	body := []byte(in.HTML)
	key := ReportHTMLKey(in.WorkspaceID, in.RunID)
	obj, err := s.Storage.Put(ctx, storage.PutObjectRequest{
		Key:         key,
		Body:        bytes.NewReader(body),
		Size:        int64(len(body)),
		ContentType: "text/html; charset=utf-8",
		Metadata: map[string]string{
			"workspace_id": in.WorkspaceID,
			"session_id":   in.SessionID,
			"uploaded_by":  in.UserID,
			"run_id":       in.RunID,
			"file_id":      fileID,
		},
	})
	if err != nil {
		return nil, err
	}

	now := time.Now()
	file := &domain.File{
		ID:              fileID,
		WorkspaceID:     in.WorkspaceID,
		UploadedBy:      in.UserID,
		DisplayName:     displayName,
		Purpose:         domain.FilePurposeReport,
		ContentType:     obj.ContentType,
		SizeBytes:       obj.Size,
		StorageProvider: obj.Provider,
		Bucket:          obj.Bucket,
		StorageKey:      obj.Key,
		Checksum:        obj.ETag,
		Status:          domain.FileStatusReady,
		Visibility:      domain.FileVisibilityPrivate,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.FileRepo.Create(ctx, file); err != nil {
		return nil, err
	}
	return file, nil
}

func (s *FileService) OpenForDownload(ctx context.Context, userID, workspaceID, fileID string) (io.ReadCloser, *domain.File, error) {
	ok, err := s.WorkspaceRepo.IsMember(ctx, workspaceID, userID)
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		return nil, nil, fmt.Errorf("用户无权访问工作区")
	}

	file, err := s.FileRepo.GetByID(ctx, fileID)
	if err != nil {
		return nil, nil, err
	}
	if file.WorkspaceID != workspaceID {
		return nil, nil, fmt.Errorf("文件不属于当前工作区")
	}

	reader, err := s.Storage.Get(ctx, file.StorageKey)
	if err != nil {
		return nil, nil, err
	}
	return reader, file, nil
}

func sanitizeFilename(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "" {
		return "upload.bin"
	}
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	return name
}
