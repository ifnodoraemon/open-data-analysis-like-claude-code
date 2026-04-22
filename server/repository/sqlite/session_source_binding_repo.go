package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/ifnodoraemon/openDataAnalysis/domain"
)

type SessionSourceBindingRepository struct{ db *sql.DB }

func NewSessionSourceBindingRepository(db *sql.DB) *SessionSourceBindingRepository {
	return &SessionSourceBindingRepository{db: db}
}

func (r *SessionSourceBindingRepository) Upsert(ctx context.Context, binding *domain.SessionSourceBinding) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO session_source_bindings (session_id, source_id, active_snapshot_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(session_id, source_id) DO UPDATE SET active_snapshot_id = excluded.active_snapshot_id, updated_at = excluded.updated_at`,
		binding.SessionID, binding.SourceID, binding.ActiveSnapshotID, binding.CreatedAt, binding.UpdatedAt)
	return err
}

func (r *SessionSourceBindingRepository) GetBySession(ctx context.Context, sessionID string) ([]domain.SessionSourceBinding, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT session_id, source_id, active_snapshot_id, created_at, updated_at FROM session_source_bindings WHERE session_id = ?`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []domain.SessionSourceBinding
	for rows.Next() {
		var b domain.SessionSourceBinding
		if err := rows.Scan(&b.SessionID, &b.SourceID, &b.ActiveSnapshotID, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, err
		}
		results = append(results, b)
	}
	return results, rows.Err()
}

func (r *SessionSourceBindingRepository) GetBySessionAndSource(ctx context.Context, sessionID, sourceID string) (*domain.SessionSourceBinding, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT session_id, source_id, active_snapshot_id, created_at, updated_at FROM session_source_bindings WHERE session_id = ? AND source_id = ?`, sessionID, sourceID)
	var b domain.SessionSourceBinding
	if err := row.Scan(&b.SessionID, &b.SourceID, &b.ActiveSnapshotID, &b.CreatedAt, &b.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &b, nil
}

func (r *SessionSourceBindingRepository) Delete(ctx context.Context, sessionID, sourceID string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM session_source_bindings WHERE session_id = ? AND source_id = ?`, sessionID, sourceID)
	return err
}

func BackfillSessionSourceBindings(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx,
		`SELECT sf.session_id, ds.id, ss.id
		 FROM session_files sf
		 JOIN files f ON f.id = sf.file_id
		 JOIN data_sources ds ON ds.file_id = f.id
		 JOIN source_snapshots ss ON ss.source_id = ds.id AND ss.session_id = sf.session_id
		 WHERE NOT EXISTS (
		   SELECT 1 FROM session_source_bindings b WHERE b.session_id = sf.session_id AND b.source_id = ds.id
		 )`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var sessionID, sourceID, snapshotID string
		if err := rows.Scan(&sessionID, &sourceID, &snapshotID); err != nil {
			continue
		}
		now := time.Now()
		_, _ = db.ExecContext(ctx,
			`INSERT OR IGNORE INTO session_source_bindings (session_id, source_id, active_snapshot_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			sessionID, sourceID, snapshotID, now, now)
	}
	return rows.Err()
}
