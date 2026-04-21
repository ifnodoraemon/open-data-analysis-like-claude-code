package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/url"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/ifnodoraemon/openDataAnalysis/data"
	"github.com/ifnodoraemon/openDataAnalysis/domain"
)

type PGImportResult struct {
	SnapshotID        string
	TableName         string
	RowCount          int
	ColCount          int
	ImportDurationMs  int
	ProfileDurationMs int
	ProfileMode       domain.ProfileMode
}

func EncryptPassword(password, authSecret string) ([]byte, error) {
	key := sha256.Sum256([]byte(authSecret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, aesGCM.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	payload := map[string]string{"password": password}
	payloadJSON, _ := json.Marshal(payload)

	ciphertext := aesGCM.Seal(nonce, nonce, payloadJSON, nil)
	return ciphertext, nil
}

func DecryptPassword(ciphertext []byte, authSecret string) (string, error) {
	key := sha256.Sum256([]byte(authSecret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", fmt.Errorf("failed to create AES cipher: %w", err)
	}

	aesGCM, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := aesGCM.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertextBody := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := aesGCM.Open(nil, nonce, ciphertextBody, nil)
	if err != nil {
		return "", fmt.Errorf("decryption failed: %w", err)
	}

	var payload map[string]string
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		return "", fmt.Errorf("failed to parse password payload: %w", err)
	}
	return payload["password"], nil
}

func OpenPostgresConnection(ctx context.Context, conn *domain.DatabaseConnection, authSecret string) (*sql.DB, error) {
	password, err := DecryptPassword(conn.SecretCiphertext, authSecret)
	if err != nil {
		return nil, fmt.Errorf("credential decryption failed: %w", err)
	}

	sslMode := conn.SSLMode
	if sslMode == "" {
		sslMode = "disable"
	}

	dsn := fmt.Sprintf("host=%s port=%d dbname=%s user=%s password=%s sslmode=%s",
		url.QueryEscape(conn.Host), conn.Port, url.QueryEscape(conn.DatabaseName),
		url.QueryEscape(conn.Username), url.QueryEscape(password), url.QueryEscape(sslMode))

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open PostgreSQL connection: %w", err)
	}

	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(1)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("connection test failed: %w", err)
	}

	_, _ = db.ExecContext(ctx, "SET default_transaction_read_only = on")

	return db, nil
}

type AllowlistEntry struct {
	Schema string `json:"schema"`
	Name   string `json:"name"`
	Kind   string `json:"kind"`
}

func ParseAllowlist(allowlistJSON string) ([]AllowlistEntry, error) {
	var entries []AllowlistEntry
	if err := json.Unmarshal([]byte(allowlistJSON), &entries); err != nil {
		return nil, fmt.Errorf("failed to parse allowlist: %w", err)
	}
	return entries, nil
}

func (s *SourceService) TestPostgresConnection(ctx context.Context, sourceID, authSecret string) map[string]interface{} {
	conn, err := s.findDBConnection(ctx, sourceID)
	if err != nil {
		return map[string]interface{}{"success": false, "message": err.Error()}
	}

	pgDB, err := OpenPostgresConnection(ctx, conn, authSecret)
	if err != nil {
		return map[string]interface{}{"success": false, "message": err.Error()}
	}
	defer pgDB.Close()

	allowlist, _ := ParseAllowlist(conn.AllowlistJSON)
	var validated []map[string]interface{}
	for _, entry := range allowlist {
		exists := s.checkObjectExists(ctx, pgDB, entry)
		validated = append(validated, map[string]interface{}{
			"schema": entry.Schema, "name": entry.Name, "kind": entry.Kind, "exists": exists,
		})
	}

	return map[string]interface{}{
		"success":  true,
		"message":  "connection successful",
		"allowlist": validated,
	}
}

func (s *SourceService) ImportPostgresSnapshot(ctx context.Context, sourceID, sessionID, schemaName, objectName string, sessIngester *data.Ingester, authSecret string) (*PGImportResult, error) {
	conn, err := s.findDBConnection(ctx, sourceID)
	if err != nil {
		return nil, err
	}

	pgDB, err := OpenPostgresConnection(ctx, conn, authSecret)
	if err != nil {
		return nil, err
	}
	defer pgDB.Close()

	schema := schemaName
	if schema == "" {
		schema = conn.DefaultSchema
	}

	tableName := sanitizePGTableName(objectName)

	importStart := time.Now()

	rowCount, colCount, err := s.streamImportToSQLite(ctx, pgDB, schema, objectName, sessIngester, tableName)
	importDuration := time.Since(importStart)

	if err != nil {
		return nil, fmt.Errorf("import failed: %w", err)
	}

	// Determine profile mode from row count
	var rowCountForMode int
	sessIngester.GetDB().QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM \"%s\"", tableName)).Scan(&rowCountForMode)
	var profileMode domain.ProfileMode = domain.ProfileModeSampled
	if rowCountForMode < 10000 {
		profileMode = domain.ProfileModeExact
	} else if rowCountForMode < 100000 {
		profileMode = domain.ProfileModeMixed
	}

	var schemaInfo *data.SchemaInfo
	var schemaErr error
	if profileMode == domain.ProfileModeExact {
		schemaInfo, schemaErr = data.ExtractSchema(sessIngester.GetDB(), tableName)
	} else {
		schemaInfo, schemaErr = data.ExtractSchemaSampled(sessIngester.GetDB(), tableName)
	}
	if schemaErr != nil {
		return nil, fmt.Errorf("schema extraction failed: %w", schemaErr)
	}
	schemaSig := ComputeSchemaSignature(schemaInfo)

	profileStart := time.Now()
	var semanticProfile *data.SemanticProfile
	facts := s.BuildProfileFacts(schemaInfo, semanticProfile, nil)
	profileDuration := time.Since(profileStart)

	snapshot, err := s.CreateSnapshot(
		ctx, sessionID, sourceID,
		"postgres_connection", schema, objectName,
		tableName, rowCount, colCount, schemaSig,
		rowCount, int(importDuration.Milliseconds()), int(profileDuration.Milliseconds()), 0, profileMode,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create snapshot: %w", err)
	}

	ds, err := s.DataSourceRepo.GetByID(ctx, sourceID)
	if err != nil {
		return nil, err
	}

	_, profErr := s.CreateSemanticProfile(
		ctx, sessionID, ds.WorkspaceID, sourceID, snapshot.ID,
		tableName, schemaSig, facts,
	)
	if profErr != nil {
		log.Printf("pg import: create semantic profile failed source_id=%s err=%v", sourceID, profErr)
	}

	return &PGImportResult{
		SnapshotID:        snapshot.ID,
		TableName:         tableName,
		RowCount:          rowCount,
		ColCount:          colCount,
		ImportDurationMs:  int(importDuration.Milliseconds()),
		ProfileDurationMs: int(profileDuration.Milliseconds()),
		ProfileMode:       profileMode,
	}, nil
}

func sanitizePGIdentifier(name string) string {
	clean := strings.ReplaceAll(name, `"`, "")
	clean = strings.TrimSpace(clean)
	return clean
}

func (s *SourceService) streamImportToSQLite(ctx context.Context, pgDB *sql.DB, schema, object string, ingester *data.Ingester, tableName string) (int, int, error) {
	schema = sanitizePGIdentifier(schema)
	object = sanitizePGIdentifier(object)
	qualifiedName := fmt.Sprintf("\"%s\".\"%s\"", schema, object)

	query := fmt.Sprintf("SELECT * FROM %s", qualifiedName)
	rows, err := pgDB.QueryContext(ctx, query)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to query upstream data: %w", err)
	}
	defer rows.Close()

	colTypes, err := rows.ColumnTypes()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read column types: %w", err)
	}
	colCount := len(colTypes)
	colNames := make([]string, colCount)
	for i, ct := range colTypes {
		colNames[i] = sanitizePGColumnName(ct.Name())
	}

	sqliteColTypes := make([]data.ColumnType, colCount)
	for i, ct := range colTypes {
		dbType := strings.ToUpper(ct.DatabaseTypeName())
		switch {
		case strings.Contains(dbType, "INT"), strings.Contains(dbType, "SERIAL"):
			sqliteColTypes[i] = data.TypeInteger
		case strings.Contains(dbType, "FLOAT"), strings.Contains(dbType, "DOUBLE"), strings.Contains(dbType, "REAL"), strings.Contains(dbType, "NUMERIC"), strings.Contains(dbType, "DECIMAL"):
			sqliteColTypes[i] = data.TypeReal
		default:
			sqliteColTypes[i] = data.TypeText
		}
	}

	if err := ingester.CreateTypedTable(tableName, colNames, sqliteColTypes); err != nil {
		return 0, 0, fmt.Errorf("failed to create SQLite table: %w", err)
	}

	batchSize := 5000
	batch := make([][]string, 0, batchSize)
	rowCount := 0

	vals := make([]interface{}, colCount)
	valPtrs := make([]interface{}, colCount)
	for i := range vals {
		valPtrs[i] = &vals[i]
	}

	for rows.Next() {
		if err := rows.Scan(valPtrs...); err != nil {
			continue
		}
		row := make([]string, colCount)
		for i := range vals {
			if vals[i] == nil {
				row[i] = ""
			} else {
				row[i] = fmt.Sprintf("%v", vals[i])
			}
		}
		batch = append(batch, row)
		if len(batch) >= batchSize {
			if err := ingester.InsertBatchTyped(tableName, colNames, sqliteColTypes, batch); err != nil {
				return rowCount, colCount, err
			}
			rowCount += len(batch)
			batch = batch[:0]
		}
	}
	if len(batch) > 0 {
		if err := ingester.InsertBatchTyped(tableName, colNames, sqliteColTypes, batch); err != nil {
			return rowCount, colCount, err
		}
		rowCount += len(batch)
	}

	_ = ingester.GenerateSchemaMetadata(tableName)

	return rowCount, colCount, nil
}

func (s *SourceService) findDBConnection(ctx context.Context, sourceID string) (*domain.DatabaseConnection, error) {
	return s.DBConnectionRepo.GetBySourceID(ctx, sourceID)
}

func (s *SourceService) checkObjectExists(ctx context.Context, pgDB *sql.DB, entry AllowlistEntry) bool {
	kindTable, ok := map[string]string{"table": "tables", "view": "views"}[entry.Kind]
	if !ok {
		return false
	}
	var count int
	query := fmt.Sprintf(
		"SELECT COUNT(1) FROM information_schema.%s WHERE table_schema = $1 AND table_name = $2",
		kindTable,
	)
	err := pgDB.QueryRowContext(ctx, query, entry.Schema, entry.Name).Scan(&count)
	return err == nil && count > 0
}

func sanitizePGTableName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, ".", "_")
	name = strings.ReplaceAll(name, "-", "_")
	if len(name) > 0 && name[0] >= '0' && name[0] <= '9' {
		name = "t_" + name
	}
	return name
}

func sanitizePGColumnName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, ".", "_")
	name = strings.ReplaceAll(name, "-", "_")
	name = strings.ReplaceAll(name, " ", "_")
	return name
}
