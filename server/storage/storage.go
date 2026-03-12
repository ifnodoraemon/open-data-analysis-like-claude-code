package storage

import (
	"context"
	"io"
	"time"
)

type PutObjectRequest struct {
	Key         string
	Body        io.Reader
	Size        int64
	ContentType string
	Metadata    map[string]string
}

type StoredObject struct {
	Provider    string
	Bucket      string
	Key         string
	ETag        string
	VersionID   string
	Size        int64
	ContentType string
}

type ObjectStorage interface {
	Put(ctx context.Context, req PutObjectRequest) (*StoredObject, error)
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
}
