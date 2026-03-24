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
	"strings"
	"time"

	"github.com/ifnodoraemon/openDataAnalysis/storage"
)

type Storage struct {
	rootDir string
	baseURL string
}

func New(rootDir, baseURL string) *Storage {
	absRoot, err := filepath.Abs(rootDir)
	if err != nil {
		absRoot = rootDir // fallback
	}
	return &Storage{
		rootDir: absRoot,
		baseURL: baseURL,
	}
}

func (s *Storage) resolvePath(key string) (string, error) {
	cleanKey := filepath.Clean(filepath.FromSlash(key))
	if strings.Contains(cleanKey, "..") {
		return "", fmt.Errorf("非法的存储路径 (包含 ..): %s", key)
	}
	fullPath := filepath.Join(s.rootDir, cleanKey)
	if !strings.HasPrefix(fullPath, s.rootDir) {
		return "", fmt.Errorf("路径越界访问: %s", key)
	}
	return fullPath, nil
}

func (s *Storage) Put(ctx context.Context, req storage.PutObjectRequest) (*storage.StoredObject, error) {
	fullPath, err := s.resolvePath(req.Key)
	if err != nil {
		return nil, err
	}
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
	fullPath, err := s.resolvePath(key)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(fullPath)
	if err != nil {
		return nil, fmt.Errorf("打开对象失败: %w", err)
	}
	return file, nil
}

func (s *Storage) Delete(ctx context.Context, key string) error {
	fullPath, err := s.resolvePath(key)
	if err != nil {
		return err // 对于非法路径，直接返回错误或视为不出错
	}
	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除对象失败: %w", err)
	}
	return nil
}

func (s *Storage) Exists(ctx context.Context, key string) (bool, error) {
	fullPath, err := s.resolvePath(key)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(fullPath)
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
