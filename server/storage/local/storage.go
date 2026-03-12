package local

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/storage"
)

type Storage struct {
	rootDir string
	baseURL string
}

func New(rootDir, baseURL string) *Storage {
	return &Storage{
		rootDir: rootDir,
		baseURL: baseURL,
	}
}

func (s *Storage) Put(ctx context.Context, req storage.PutObjectRequest) (*storage.StoredObject, error) {
	fullPath := filepath.Join(s.rootDir, filepath.FromSlash(req.Key))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return nil, fmt.Errorf("创建对象目录失败: %w", err)
	}

	f, err := os.Create(fullPath)
	if err != nil {
		return nil, fmt.Errorf("创建对象失败: %w", err)
	}
	defer f.Close()

	hasher := sha256.New()
	written, err := io.Copy(io.MultiWriter(f, hasher), req.Body)
	if err != nil {
		return nil, fmt.Errorf("写入对象失败: %w", err)
	}

	return &storage.StoredObject{
		Provider:    "local",
		Key:         req.Key,
		ETag:        hex.EncodeToString(hasher.Sum(nil)),
		Size:        written,
		ContentType: req.ContentType,
	}, nil
}

func (s *Storage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	fullPath := filepath.Join(s.rootDir, filepath.FromSlash(key))
	file, err := os.Open(fullPath)
	if err != nil {
		return nil, fmt.Errorf("打开对象失败: %w", err)
	}
	return file, nil
}

func (s *Storage) Delete(ctx context.Context, key string) error {
	fullPath := filepath.Join(s.rootDir, filepath.FromSlash(key))
	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除对象失败: %w", err)
	}
	return nil
}

func (s *Storage) Exists(ctx context.Context, key string) (bool, error) {
	fullPath := filepath.Join(s.rootDir, filepath.FromSlash(key))
	_, err := os.Stat(fullPath)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (s *Storage) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	if s.baseURL == "" {
		return "", fmt.Errorf("baseURL 未配置，无法生成下载地址")
	}
	return fmt.Sprintf("%s/objects/%s?ttl=%d", s.baseURL, url.PathEscape(key), int(ttl.Seconds())), nil
}
