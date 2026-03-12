package domain

import "time"

type FileStatus string
type FileVisibility string
type FilePurpose string

const (
	FileStatusUploaded   FileStatus = "uploaded"
	FileStatusProcessing FileStatus = "processing"
	FileStatusReady      FileStatus = "ready"
	FileStatusDeleted    FileStatus = "deleted"

	FileVisibilityPrivate   FileVisibility = "private"
	FileVisibilityWorkspace FileVisibility = "workspace"

	FilePurposeSource   FilePurpose = "source"
	FilePurposeReport   FilePurpose = "report"
	FilePurposeArtifact FilePurpose = "artifact"
)

type File struct {
	ID              string
	WorkspaceID     string
	UploadedBy      string
	DisplayName     string
	Purpose         FilePurpose
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
