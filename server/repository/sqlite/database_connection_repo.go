package sqlite

import (
	"context"
	"database/sql"

	"github.com/ifnodoraemon/openDataAnalysis/domain"
)

type DatabaseConnectionRepository struct{ db *sql.DB }

func NewDatabaseConnectionRepository(db *sql.DB) *DatabaseConnectionRepository {
	return &DatabaseConnectionRepository{db: db}
}

func (r *DatabaseConnectionRepository) Create(ctx context.Context, conn *domain.DatabaseConnection) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO database_connections (source_id, driver, host, port, database_name, default_schema, ssl_mode, username, secret_ciphertext, allowlist_json, last_tested_at, last_test_status, last_error_message) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		conn.SourceID, conn.Driver, conn.Host, conn.Port, conn.DatabaseName, conn.DefaultSchema, conn.SSLMode, conn.Username, conn.SecretCiphertext, conn.AllowlistJSON, conn.LastTestedAt, conn.LastTestStatus, conn.LastErrorMessage)
	return err
}

func (r *DatabaseConnectionRepository) GetBySourceID(ctx context.Context, sourceID string) (*domain.DatabaseConnection, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT source_id, driver, host, port, database_name, default_schema, ssl_mode, username, secret_ciphertext, allowlist_json, last_tested_at, last_test_status, last_error_message FROM database_connections WHERE source_id = ?`, sourceID)
	var conn domain.DatabaseConnection
	var lastTestedAt sql.NullTime
	var lastTestStatus, lastErrMsg sql.NullString
	if err := row.Scan(&conn.SourceID, &conn.Driver, &conn.Host, &conn.Port, &conn.DatabaseName, &conn.DefaultSchema, &conn.SSLMode, &conn.Username, &conn.SecretCiphertext, &conn.AllowlistJSON, &lastTestedAt, &lastTestStatus, &lastErrMsg); err != nil {
		return nil, err
	}
	if lastTestedAt.Valid {
		conn.LastTestedAt = &lastTestedAt.Time
	}
	if lastTestStatus.Valid {
		conn.LastTestStatus = lastTestStatus.String
	}
	if lastErrMsg.Valid {
		conn.LastErrorMessage = &lastErrMsg.String
	}
	return &conn, nil
}

func (r *DatabaseConnectionRepository) Update(ctx context.Context, conn *domain.DatabaseConnection) error {
	var lastTestedAt interface{}
	if conn.LastTestedAt != nil {
		lastTestedAt = *conn.LastTestedAt
	}
	var lastErrMsg interface{}
	if conn.LastErrorMessage != nil {
		lastErrMsg = *conn.LastErrorMessage
	}
	var lastTestStatus interface{}
	if conn.LastTestStatus != "" {
		lastTestStatus = conn.LastTestStatus
	}
	_, err := r.db.ExecContext(ctx,
		`UPDATE database_connections SET driver=?, host=?, port=?, database_name=?, default_schema=?, ssl_mode=?, username=?, secret_ciphertext=?, allowlist_json=?, last_tested_at=?, last_test_status=?, last_error_message=? WHERE source_id=?`,
		conn.Driver, conn.Host, conn.Port, conn.DatabaseName, conn.DefaultSchema, conn.SSLMode, conn.Username, conn.SecretCiphertext, conn.AllowlistJSON, lastTestedAt, lastTestStatus, lastErrMsg, conn.SourceID)
	return err
}
