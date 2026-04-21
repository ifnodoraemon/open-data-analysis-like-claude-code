package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/ifnodoraemon/openDataAnalysis/domain"
	"github.com/ifnodoraemon/openDataAnalysis/repository"
	"github.com/ifnodoraemon/openDataAnalysis/storage"
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
	ReportRepo    repository.ReportRepository
	WorkspaceRepo repository.WorkspaceRepository
	TempDir       string
}

func (s *FileService) Upload(ctx context.Context, in UploadFileInput) (*domain.File, error) {
	ok, err := s.WorkspaceRepo.IsMember(ctx, in.WorkspaceID, in.UserID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("user not authorized to access workspace")
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
		_ = s.FileRepo.Delete(ctx, file.ID)
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

func (s *FileService) MaterializeToTemp(ctx context.Context, sessionID, workspaceID, fileID string) (string, *domain.File, error) {
	file, err := s.FileRepo.GetByID(ctx, fileID)
	if err != nil {
		return "", nil, err
	}
	if file.WorkspaceID != workspaceID {
		return "", nil, fmt.Errorf("file does not belong to current workspace")
	}

	sessionFiles, err := s.GetSessionFiles(ctx, sessionID)
	if err != nil {
		return "", nil, fmt.Errorf("failed to read session file list: %w", err)
	}
	found := false
	for _, sf := range sessionFiles {
		if sf.ID == fileID {
			found = true
			break
		}
	}
	if !found {
		return "", nil, fmt.Errorf("security block: cannot access file not mounted in current session")
	}

	reader, err := s.Storage.Get(ctx, file.StorageKey)
	if err != nil {
		return "", nil, err
	}
	defer reader.Close()

	if err := os.MkdirAll(s.TempDir, 0o755); err != nil {
		return "", nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	tempPath := filepath.Join(s.TempDir, fmt.Sprintf("%s-%s", file.ID, sanitizeFilename(file.DisplayName)))
	dest, err := os.Create(tempPath)
	if err != nil {
		return "", nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer dest.Close()

	if _, err := io.Copy(dest, reader); err != nil {
		return "", nil, fmt.Errorf("failed to write temp file: %w", err)
	}

	return tempPath, file, nil
}

type SaveReportInput struct {
	UserID      string
	WorkspaceID string
	SessionID   string
	RunID       string
	Title       string
	Author      string
	HTML        string
	Snapshot    domain.ReportSnapshot
}

func (s *FileService) SaveReportHTML(ctx context.Context, in SaveReportInput) (*domain.File, error) {
	ok, err := s.WorkspaceRepo.IsMember(ctx, in.WorkspaceID, in.UserID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("user not authorized to access workspace")
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

	if s.ReportRepo != nil {
		snapshotJSON, err := json.Marshal(in.Snapshot)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize report snapshot: %w", err)
		}
		report := &domain.Report{
			ID:                  "report_" + in.RunID,
			RunID:               in.RunID,
			WorkspaceID:         in.WorkspaceID,
			Title:               strings.TrimSpace(in.Snapshot.Title),
			Author:              strings.TrimSpace(in.Snapshot.Author),
			HTMLStorageProvider: obj.Provider,
			HTMLBucket:          obj.Bucket,
			HTMLStorageKey:      obj.Key,
			SnapshotJSON:        string(snapshotJSON),
			CreatedAt:           now,
		}
		if report.Title == "" {
			report.Title = strings.TrimSpace(in.Title)
		}
		if report.Author == "" {
			report.Author = strings.TrimSpace(in.Author)
		}
		if err := s.ReportRepo.Create(ctx, report); err != nil {
			_ = s.Storage.Delete(ctx, obj.Key)
			_ = s.FileRepo.Delete(ctx, file.ID)
			return nil, fmt.Errorf("failed to save report metadata: %w", err)
		}
	}
	return file, nil
}

func (s *FileService) OpenForDownload(ctx context.Context, userID, workspaceID, fileID string) (io.ReadCloser, *domain.File, error) {
	ok, err := s.WorkspaceRepo.IsMember(ctx, workspaceID, userID)
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		return nil, nil, fmt.Errorf("user not authorized to access workspace")
	}

	file, err := s.FileRepo.GetByID(ctx, fileID)
	if err != nil {
		return nil, nil, err
	}
	if file.WorkspaceID != workspaceID {
		return nil, nil, fmt.Errorf("file does not belong to current workspace")
	}

	reader, err := s.Storage.Get(ctx, file.StorageKey)
	if err != nil {
		return nil, nil, err
	}
	return reader, file, nil
}

func (s *FileService) OpenStoredObject(ctx context.Context, userID, workspaceID, storageKey string) (io.ReadCloser, error) {
	ok, err := s.WorkspaceRepo.IsMember(ctx, workspaceID, userID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("user not authorized to access workspace")
	}
	return s.Storage.Get(ctx, storageKey)
}

var safeFilePattern = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

func sanitizeFilename(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "" || name == "." || name == ".." || !safeFilePattern.MatchString(name) {
		return "upload.bin"
	}
	return name
}
