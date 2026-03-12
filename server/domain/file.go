package domain

import "time"

type FileStatus string
type FileVisibility string

const (
	FileStatusUploaded   FileStatus = "uploaded"
	FileStatusProcessing FileStatus = "processing"
	FileStatusReady      FileStatus = "ready"
	FileStatusDeleted    FileStatus = "deleted"

	FileVisibilityPrivate   FileVisibility = "private"
	FileVisibilityWorkspace FileVisibility = "workspace"
)

type File struct {
	ID              string
	WorkspaceID     string
	UploadedBy      string
	DisplayName     string
	ContentType     string
	SizeBytes       int64
	StorageProvider string
	Bucket          string
	StorageKey      string
	Checksum        string
	Status          FileStatus
	Visibility      FileVisibility
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       *time.Time
}
