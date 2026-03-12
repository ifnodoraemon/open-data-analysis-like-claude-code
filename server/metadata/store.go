package metadata

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

type Store struct {
	DB *sql.DB
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("创建 metadata 目录失败: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("打开 metadata 数据库失败: %w", err)
	}

	store := &Store{DB: db}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

func (s *Store) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			email TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL,
			avatar_url TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'active',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			last_login_at DATETIME
		)`,
		`CREATE TABLE IF NOT EXISTS workspaces (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			slug TEXT NOT NULL UNIQUE,
			owner_user_id TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'active',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS workspace_members (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			workspace_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			role TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			UNIQUE(workspace_id, user_id)
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			title TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'active',
			last_run_id TEXT,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			last_seen_at DATETIME NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS files (
			id TEXT PRIMARY KEY,
			workspace_id TEXT NOT NULL,
			uploaded_by TEXT NOT NULL,
			display_name TEXT NOT NULL,
			content_type TEXT NOT NULL DEFAULT '',
			size_bytes INTEGER NOT NULL,
			storage_provider TEXT NOT NULL,
			bucket TEXT NOT NULL DEFAULT '',
			storage_key TEXT NOT NULL,
			checksum TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'uploaded',
			visibility TEXT NOT NULL DEFAULT 'private',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			deleted_at DATETIME
		)`,
		`CREATE TABLE IF NOT EXISTS session_files (
			session_id TEXT NOT NULL,
			file_id TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			PRIMARY KEY (session_id, file_id)
		)`,
		`CREATE TABLE IF NOT EXISTS analysis_runs (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			workspace_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			status TEXT NOT NULL,
			input_message TEXT NOT NULL,
			error_message TEXT,
			report_file_id TEXT,
			started_at DATETIME,
			finished_at DATETIME,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`,
	}

	for _, stmt := range stmts {
		if _, err := s.DB.Exec(stmt); err != nil {
			return fmt.Errorf("执行 metadata migration 失败: %w", err)
		}
	}

	return nil
}
