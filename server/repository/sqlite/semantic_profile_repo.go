package sqlite

import (
	"context"
	"database/sql"

	"github.com/ifnodoraemon/openDataAnalysis/domain"
)

type SemanticProfileRepository struct{ db *sql.DB }

func NewSemanticProfileRepository(db *sql.DB) *SemanticProfileRepository {
	return &SemanticProfileRepository{db: db}
}

func (r *SemanticProfileRepository) Create(ctx context.Context, profile *domain.SemanticProfile) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO semantic_profiles (id, session_id, source_id, snapshot_id, analysis_table_name, schema_signature, profile_status, profile_json, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		profile.ID, profile.SessionID, profile.SourceID, profile.SnapshotID, profile.AnalysisTableName, profile.SchemaSignature, string(profile.ProfileStatus), profile.ProfileJSON, profile.CreatedAt, profile.UpdatedAt)
	return err
}

func (r *SemanticProfileRepository) GetByID(ctx context.Context, id string) (*domain.SemanticProfile, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, session_id, source_id, snapshot_id, analysis_table_name, schema_signature, profile_status, profile_json, created_at, updated_at FROM semantic_profiles WHERE id = ?`, id)
	var p domain.SemanticProfile
	var profileStatus string
	if err := row.Scan(&p.ID, &p.SessionID, &p.SourceID, &p.SnapshotID, &p.AnalysisTableName, &p.SchemaSignature, &profileStatus, &p.ProfileJSON, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	p.ProfileStatus = domain.ProfileStatus(profileStatus)
	return &p, nil
}

func (r *SemanticProfileRepository) ListBySession(ctx context.Context, sessionID string) ([]domain.SemanticProfile, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, session_id, source_id, snapshot_id, analysis_table_name, schema_signature, profile_status, profile_json, created_at, updated_at FROM semantic_profiles WHERE session_id = ? ORDER BY created_at ASC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []domain.SemanticProfile
	for rows.Next() {
		var p domain.SemanticProfile
		var profileStatus string
		if err := rows.Scan(&p.ID, &p.SessionID, &p.SourceID, &p.SnapshotID, &p.AnalysisTableName, &p.SchemaSignature, &profileStatus, &p.ProfileJSON, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		p.ProfileStatus = domain.ProfileStatus(profileStatus)
		results = append(results, p)
	}
	return results, rows.Err()
}

func (r *SemanticProfileRepository) ListBySource(ctx context.Context, sourceID string) ([]domain.SemanticProfile, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, session_id, source_id, snapshot_id, analysis_table_name, schema_signature, profile_status, profile_json, created_at, updated_at FROM semantic_profiles WHERE source_id = ? ORDER BY created_at DESC`, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []domain.SemanticProfile
	for rows.Next() {
		var p domain.SemanticProfile
		var profileStatus string
		if err := rows.Scan(&p.ID, &p.SessionID, &p.SourceID, &p.SnapshotID, &p.AnalysisTableName, &p.SchemaSignature, &profileStatus, &p.ProfileJSON, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		p.ProfileStatus = domain.ProfileStatus(profileStatus)
		results = append(results, p)
	}
	return results, rows.Err()
}

func (r *SemanticProfileRepository) UpdateStatus(ctx context.Context, id string, status domain.ProfileStatus) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE semantic_profiles SET profile_status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		string(status), id)
	return err
}

func (r *SemanticProfileRepository) UpdateProfileJSON(ctx context.Context, id string, profileJSON string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE semantic_profiles SET profile_json = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		profileJSON, id)
	return err
}

func (r *SemanticProfileRepository) FindWorkspaceConfirmation(ctx context.Context, workspaceID, schemaSignature string) (*domain.SemanticConfirmation, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT sc.id, sc.profile_id, sc.workspace_id, sc.session_id, sc.confirmed_by, sc.scope, sc.overrides_json, sc.created_at
		 FROM semantic_confirmations sc
		 JOIN semantic_profiles sp ON sp.id = sc.profile_id
		 JOIN data_sources ds ON ds.id = sp.source_id
		 WHERE ds.workspace_id = ? AND sp.schema_signature = ? AND sc.scope = 'workspace'
		 ORDER BY sc.created_at DESC LIMIT 1`, workspaceID, schemaSignature)
	var c domain.SemanticConfirmation
	var scope string
	if err := row.Scan(&c.ID, &c.ProfileID, &c.WorkspaceID, &c.SessionID, &c.ConfirmedBy, &scope, &c.OverridesJSON, &c.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	c.Scope = domain.ConfirmationScope(scope)
	return &c, nil
}

func (r *SemanticProfileRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM semantic_profiles WHERE id = ?`, id)
	return err
}
