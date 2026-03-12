package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/domain"
)

type UserRepository struct{ db *sql.DB }
type WorkspaceRepository struct{ db *sql.DB }
type FileRepository struct{ db *sql.DB }
type SessionRepository struct{ db *sql.DB }
type RunRepository struct{ db *sql.DB }

func NewUserRepository(db *sql.DB) *UserRepository           { return &UserRepository{db: db} }
func NewWorkspaceRepository(db *sql.DB) *WorkspaceRepository { return &WorkspaceRepository{db: db} }
func NewFileRepository(db *sql.DB) *FileRepository           { return &FileRepository{db: db} }
func NewSessionRepository(db *sql.DB) *SessionRepository     { return &SessionRepository{db: db} }
func NewRunRepository(db *sql.DB) *RunRepository             { return &RunRepository{db: db} }

func (r *UserRepository) GetByID(ctx context.Context, userID string) (*domain.User, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, email, password_hash, name, avatar_url, status, created_at, updated_at, last_login_at FROM users WHERE id = ?`, userID)
	var user domain.User
	var avatarURL string
	var status string
	var lastLogin sql.NullTime
	if err := row.Scan(&user.ID, &user.Email, &user.PasswordHash, &user.Name, &avatarURL, &status, &user.CreatedAt, &user.UpdatedAt, &lastLogin); err != nil {
		return nil, err
	}
	user.AvatarURL = avatarURL
	user.Status = domain.UserStatus(status)
	if lastLogin.Valid {
		user.LastLoginAt = &lastLogin.Time
	}
	return &user, nil
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, email, password_hash, name, avatar_url, status, created_at, updated_at, last_login_at FROM users WHERE email = ?`, email)
	var user domain.User
	var avatarURL string
	var status string
	var lastLogin sql.NullTime
	if err := row.Scan(&user.ID, &user.Email, &user.PasswordHash, &user.Name, &avatarURL, &status, &user.CreatedAt, &user.UpdatedAt, &lastLogin); err != nil {
		return nil, err
	}
	user.AvatarURL = avatarURL
	user.Status = domain.UserStatus(status)
	if lastLogin.Valid {
		user.LastLoginAt = &lastLogin.Time
	}
	return &user, nil
}

func (r *UserRepository) Create(ctx context.Context, user *domain.User) error {
	_, err := r.db.ExecContext(ctx, `INSERT OR REPLACE INTO users (id, email, password_hash, name, avatar_url, status, created_at, updated_at, last_login_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		user.ID, user.Email, user.PasswordHash, user.Name, user.AvatarURL, string(user.Status), user.CreatedAt, user.UpdatedAt, user.LastLoginAt)
	return err
}

func (r *WorkspaceRepository) GetByID(ctx context.Context, workspaceID string) (*domain.Workspace, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, name, slug, owner_user_id, status, created_at, updated_at FROM workspaces WHERE id = ?`, workspaceID)
	var workspace domain.Workspace
	var status string
	if err := row.Scan(&workspace.ID, &workspace.Name, &workspace.Slug, &workspace.OwnerUserID, &status, &workspace.CreatedAt, &workspace.UpdatedAt); err != nil {
		return nil, err
	}
	workspace.Status = domain.WorkspaceStatus(status)
	return &workspace, nil
}

func (r *WorkspaceRepository) ListByUser(ctx context.Context, userID string) ([]domain.Workspace, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT w.id, w.name, w.slug, w.owner_user_id, w.status, w.created_at, w.updated_at
		FROM workspaces w
		INNER JOIN workspace_members wm ON wm.workspace_id = w.id
		WHERE wm.user_id = ?
		ORDER BY w.created_at ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var workspaces []domain.Workspace
	for rows.Next() {
		var workspace domain.Workspace
		var status string
		if err := rows.Scan(&workspace.ID, &workspace.Name, &workspace.Slug, &workspace.OwnerUserID, &status, &workspace.CreatedAt, &workspace.UpdatedAt); err != nil {
			return nil, err
		}
		workspace.Status = domain.WorkspaceStatus(status)
		workspaces = append(workspaces, workspace)
	}
	return workspaces, rows.Err()
}

func (r *WorkspaceRepository) IsMember(ctx context.Context, workspaceID, userID string) (bool, error) {
	row := r.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM workspace_members WHERE workspace_id = ? AND user_id = ?`, workspaceID, userID)
	var count int
	if err := row.Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *WorkspaceRepository) CreateWorkspace(ctx context.Context, workspace *domain.Workspace) error {
	_, err := r.db.ExecContext(ctx, `INSERT OR REPLACE INTO workspaces (id, name, slug, owner_user_id, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		workspace.ID, workspace.Name, workspace.Slug, workspace.OwnerUserID, string(workspace.Status), workspace.CreatedAt, workspace.UpdatedAt)
	return err
}

func (r *WorkspaceRepository) AddMember(ctx context.Context, member *domain.WorkspaceMember) error {
	_, err := r.db.ExecContext(ctx, `INSERT OR REPLACE INTO workspace_members (workspace_id, user_id, role, created_at) VALUES (?, ?, ?, ?)`,
		member.WorkspaceID, member.UserID, string(member.Role), member.CreatedAt)
	return err
}

func (r *FileRepository) Create(ctx context.Context, file *domain.File) error {
	_, err := r.db.ExecContext(ctx, `INSERT OR REPLACE INTO files (id, workspace_id, uploaded_by, display_name, content_type, size_bytes, storage_provider, bucket, storage_key, checksum, status, visibility, created_at, updated_at, deleted_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		file.ID, file.WorkspaceID, file.UploadedBy, file.DisplayName, file.ContentType, file.SizeBytes, file.StorageProvider, file.Bucket, file.StorageKey, file.Checksum, string(file.Status), string(file.Visibility), file.CreatedAt, file.UpdatedAt, file.DeletedAt)
	return err
}

func (r *FileRepository) GetByID(ctx context.Context, fileID string) (*domain.File, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, workspace_id, uploaded_by, display_name, content_type, size_bytes, storage_provider, bucket, storage_key, checksum, status, visibility, created_at, updated_at, deleted_at FROM files WHERE id = ?`, fileID)
	var file domain.File
	var status, visibility string
	var deletedAt sql.NullTime
	if err := row.Scan(&file.ID, &file.WorkspaceID, &file.UploadedBy, &file.DisplayName, &file.ContentType, &file.SizeBytes, &file.StorageProvider, &file.Bucket, &file.StorageKey, &file.Checksum, &status, &visibility, &file.CreatedAt, &file.UpdatedAt, &deletedAt); err != nil {
		return nil, err
	}
	file.Status = domain.FileStatus(status)
	file.Visibility = domain.FileVisibility(visibility)
	if deletedAt.Valid {
		file.DeletedAt = &deletedAt.Time
	}
	return &file, nil
}

func (r *FileRepository) ListBySession(ctx context.Context, sessionID string) ([]domain.File, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT f.id, f.workspace_id, f.uploaded_by, f.display_name, f.content_type, f.size_bytes,
		       f.storage_provider, f.bucket, f.storage_key, f.checksum, f.status, f.visibility,
		       f.created_at, f.updated_at, f.deleted_at
		FROM files f
		INNER JOIN session_files sf ON sf.file_id = f.id
		WHERE sf.session_id = ?
		ORDER BY sf.created_at ASC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []domain.File
	for rows.Next() {
		var file domain.File
		var status, visibility string
		var deletedAt sql.NullTime
		if err := rows.Scan(&file.ID, &file.WorkspaceID, &file.UploadedBy, &file.DisplayName, &file.ContentType, &file.SizeBytes,
			&file.StorageProvider, &file.Bucket, &file.StorageKey, &file.Checksum, &status, &visibility,
			&file.CreatedAt, &file.UpdatedAt, &deletedAt); err != nil {
			return nil, err
		}
		file.Status = domain.FileStatus(status)
		file.Visibility = domain.FileVisibility(visibility)
		if deletedAt.Valid {
			file.DeletedAt = &deletedAt.Time
		}
		files = append(files, file)
	}
	return files, rows.Err()
}

func (r *FileRepository) AttachFilesToSession(ctx context.Context, sessionID string, fileIDs []string) error {
	now := time.Now()
	for _, fileID := range fileIDs {
		if _, err := r.db.ExecContext(ctx, `INSERT OR REPLACE INTO session_files (session_id, file_id, created_at) VALUES (?, ?, ?)`, sessionID, fileID, now); err != nil {
			return err
		}
	}
	return nil
}

func (r *SessionRepository) Create(ctx context.Context, session *domain.Session) error {
	_, err := r.db.ExecContext(ctx, `INSERT OR REPLACE INTO sessions (id, workspace_id, user_id, title, status, last_run_id, created_at, updated_at, last_seen_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		session.ID, session.WorkspaceID, session.UserID, session.Title, string(session.Status), session.LastRunID, session.CreatedAt, session.UpdatedAt, session.LastSeenAt)
	return err
}

func (r *SessionRepository) GetByID(ctx context.Context, sessionID string) (*domain.Session, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, workspace_id, user_id, title, status, last_run_id, created_at, updated_at, last_seen_at FROM sessions WHERE id = ?`, sessionID)
	var session domain.Session
	var status string
	var lastRun sql.NullString
	if err := row.Scan(&session.ID, &session.WorkspaceID, &session.UserID, &session.Title, &status, &lastRun, &session.CreatedAt, &session.UpdatedAt, &session.LastSeenAt); err != nil {
		return nil, err
	}
	session.Status = domain.SessionStatus(status)
	if lastRun.Valid {
		session.LastRunID = &lastRun.String
	}
	return &session, nil
}

func (r *SessionRepository) ListByUserWorkspace(ctx context.Context, userID, workspaceID string, limit int) ([]domain.Session, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id, workspace_id, user_id, title, status, last_run_id, created_at, updated_at, last_seen_at FROM sessions WHERE user_id = ? AND workspace_id = ? ORDER BY last_seen_at DESC, created_at DESC LIMIT ?`, userID, workspaceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sessions := make([]domain.Session, 0, limit)
	for rows.Next() {
		var session domain.Session
		var status string
		var lastRun sql.NullString
		if err := rows.Scan(&session.ID, &session.WorkspaceID, &session.UserID, &session.Title, &status, &lastRun, &session.CreatedAt, &session.UpdatedAt, &session.LastSeenAt); err != nil {
			return nil, err
		}
		session.Status = domain.SessionStatus(status)
		if lastRun.Valid {
			session.LastRunID = &lastRun.String
		}
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}

func (r *SessionRepository) UpdateTitle(ctx context.Context, sessionID, title string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE sessions SET title = ?, updated_at = ? WHERE id = ?`, title, time.Now(), sessionID)
	return err
}

func (r *SessionRepository) UpdateLastSeen(ctx context.Context, sessionID string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE sessions SET last_seen_at = ?, updated_at = ? WHERE id = ?`, time.Now(), time.Now(), sessionID)
	return err
}

func (r *SessionRepository) UpdateLastRun(ctx context.Context, sessionID, runID string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE sessions SET last_run_id = ?, updated_at = ? WHERE id = ?`, runID, time.Now(), sessionID)
	return err
}

func (r *RunRepository) Create(ctx context.Context, run *domain.AnalysisRun) error {
	_, err := r.db.ExecContext(ctx, `INSERT OR REPLACE INTO analysis_runs (id, session_id, workspace_id, user_id, status, input_message, summary, error_message, report_file_id, started_at, finished_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.ID, run.SessionID, run.WorkspaceID, run.UserID, string(run.Status), run.InputMessage, run.Summary, run.ErrorMessage, run.ReportFileID, run.StartedAt, run.FinishedAt, run.CreatedAt, run.UpdatedAt)
	return err
}

func (r *RunRepository) GetByID(ctx context.Context, runID string) (*domain.AnalysisRun, error) {
	row := r.db.QueryRowContext(ctx, `SELECT id, session_id, workspace_id, user_id, status, input_message, summary, error_message, report_file_id, started_at, finished_at, created_at, updated_at FROM analysis_runs WHERE id = ?`, runID)
	var run domain.AnalysisRun
	var status string
	var errMsg, reportID sql.NullString
	var startedAt, finishedAt sql.NullTime
	if err := row.Scan(&run.ID, &run.SessionID, &run.WorkspaceID, &run.UserID, &status, &run.InputMessage, &run.Summary, &errMsg, &reportID, &startedAt, &finishedAt, &run.CreatedAt, &run.UpdatedAt); err != nil {
		return nil, err
	}
	run.Status = domain.RunStatus(status)
	if errMsg.Valid {
		run.ErrorMessage = &errMsg.String
	}
	if reportID.Valid {
		run.ReportFileID = &reportID.String
	}
	if startedAt.Valid {
		run.StartedAt = &startedAt.Time
	}
	if finishedAt.Valid {
		run.FinishedAt = &finishedAt.Time
	}
	return &run, nil
}

func (r *RunRepository) ListBySession(ctx context.Context, sessionID string, limit int) ([]domain.AnalysisRun, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id, session_id, workspace_id, user_id, status, input_message, summary, error_message, report_file_id, started_at, finished_at, created_at, updated_at FROM analysis_runs WHERE session_id = ? ORDER BY created_at DESC LIMIT ?`, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	runs := make([]domain.AnalysisRun, 0, limit)
	for rows.Next() {
		var run domain.AnalysisRun
		var status string
		var errMsg, reportID sql.NullString
		var startedAt, finishedAt sql.NullTime
		if err := rows.Scan(&run.ID, &run.SessionID, &run.WorkspaceID, &run.UserID, &status, &run.InputMessage, &run.Summary, &errMsg, &reportID, &startedAt, &finishedAt, &run.CreatedAt, &run.UpdatedAt); err != nil {
			return nil, err
		}
		run.Status = domain.RunStatus(status)
		if errMsg.Valid {
			run.ErrorMessage = &errMsg.String
		}
		if reportID.Valid {
			run.ReportFileID = &reportID.String
		}
		if startedAt.Valid {
			run.StartedAt = &startedAt.Time
		}
		if finishedAt.Valid {
			run.FinishedAt = &finishedAt.Time
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func (r *RunRepository) UpdateStatus(ctx context.Context, runID string, status domain.RunStatus, errMsg *string) error {
	now := time.Now()
	var finishedAt interface{}
	switch status {
	case domain.RunStatusCompleted, domain.RunStatusCancelled, domain.RunStatusFailed:
		finishedAt = now
	default:
		finishedAt = nil
	}
	_, err := r.db.ExecContext(ctx, `UPDATE analysis_runs SET status = ?, error_message = ?, finished_at = COALESCE(?, finished_at), updated_at = ? WHERE id = ?`, string(status), errMsg, finishedAt, now, runID)
	return err
}

func (r *RunRepository) UpdateSummary(ctx context.Context, runID, summary string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE analysis_runs SET summary = ?, updated_at = ? WHERE id = ?`, summary, time.Now(), runID)
	return err
}

func (r *RunRepository) BindReportFile(ctx context.Context, runID, reportFileID string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE analysis_runs SET report_file_id = ?, updated_at = ? WHERE id = ?`, reportFileID, time.Now(), runID)
	return err
}
