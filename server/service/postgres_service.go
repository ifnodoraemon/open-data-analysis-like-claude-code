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
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/ifnodoraemon/openDataAnalysis/data"
	"github.com/ifnodoraemon/openDataAnalysis/domain"
)

type PGImportResult struct {
	SnapshotID        string
	ProfileID         string
	TableName         string
	RowCount          int
	ColCount          int
	RowsImported      int
	ImportDurationMs  int
	ProfileDurationMs int
	SnapshotSizeBytes int64
	ProfileMode       domain.ProfileMode
	ProfErr           error
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

	resolvedSchema := schemaName
	if resolvedSchema == "" {
		resolvedSchema = conn.DefaultSchema
	}

	allowlist, allowErr := ParseAllowlist(conn.AllowlistJSON)
	if allowErr != nil {
		return nil, fmt.Errorf("failed to parse allowlist: %w", allowErr)
	}
	if !isInAllowlist(allowlist, resolvedSchema, objectName) {
		return nil, fmt.Errorf("object %s.%s is not in the data source allowlist", resolvedSchema, objectName)
	}

	pgDB, err := OpenPostgresConnection(ctx, conn, authSecret)
	if err != nil {
		return nil, err
	}
	defer pgDB.Close()

	schema := resolvedSchema

	tableName := sanitizePGTableName(objectName)

	preSnapshot := &domain.SourceSnapshot{
		ID:                "snap_" + uuid.New().String()[:12],
		SessionID:         sessionID,
		SourceID:          sourceID,
		UpstreamKind:      "postgres_connection",
		UpstreamSchema:    schema,
		UpstreamObject:    objectName,
		AnalysisTableName: tableName,
		Status:            domain.SnapshotStatusCreating,
		ImportedAt:        time.Now(),
	}
	if err := s.SnapshotRepo.Create(ctx, preSnapshot); err != nil {
		return nil, fmt.Errorf("failed to create pre-import snapshot: %w", err)
	}
	binding := &domain.SessionSourceBinding{
		SessionID:        sessionID,
		SourceID:         sourceID,
		ActiveSnapshotID: preSnapshot.ID,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	if err := s.SessionSourceBindingRepo.Upsert(ctx, binding); err != nil {
		bindErr := "failed to create session source binding"
		_ = s.SnapshotRepo.UpdateStatus(ctx, preSnapshot.ID, domain.SnapshotStatusFailed, &bindErr)
		return nil, fmt.Errorf("failed to upsert session source binding: %w", err)
	}

	importStart := time.Now()

	rowCount, colCount, err := s.streamImportToSQLite(ctx, pgDB, schema, objectName, sessIngester, tableName)
	importDuration := time.Since(importStart)

	if err != nil {
		importErrMsg := err.Error()
		_ = s.SnapshotRepo.UpdateStatus(ctx, preSnapshot.ID, domain.SnapshotStatusFailed, &importErrMsg)
		return nil, fmt.Errorf("import failed: %w", err)
	}

	// Determine profile mode from row count
	var rowCountForMode int
	rowCountForMode = rowCount
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
	if sessIngester.SemanticEnricher != nil {
		activeTables := sessIngester.GetActiveTables()
		semCtx, semCancel := context.WithTimeout(ctx, 30*time.Second)
		sp, semErr := data.AnalyzeTableSemantics(semCtx, sessIngester.SemanticEnricher, schemaInfo, activeTables)
		semCancel()
		if semErr != nil {
			log.Printf("pg import: LLM semantic analysis skipped table=%s err=%v", tableName, semErr)
		} else {
			semanticProfile = sp
		}
	}
	snapshotSizeBytes := int64(0)
	if dbPath := sessIngester.DBPath(); dbPath != "" {
		if fi, fiErr := os.Stat(dbPath); fiErr == nil {
			snapshotSizeBytes = fi.Size()
		}
	}

	facts := s.BuildProfileFacts(schemaInfo, semanticProfile, nil, string(profileMode), snapshotSizeBytes)
	profileDuration := time.Since(profileStart)

	if err := s.SnapshotRepo.UpdateStatus(ctx, preSnapshot.ID, domain.SnapshotStatusReady, nil); err != nil {
		log.Printf("pg import: failed to update snapshot status to ready snapshot_id=%s err=%v", preSnapshot.ID, err)
	}
	if err := s.SnapshotRepo.UpdateSnapshotCompletion(ctx, preSnapshot.ID, rowCount, colCount, schemaSig,
		rowCount, int(importDuration.Milliseconds()), int(profileDuration.Milliseconds()), snapshotSizeBytes, profileMode); err != nil {
		log.Printf("pg import: failed to update snapshot completion facts snapshot_id=%s err=%v", preSnapshot.ID, err)
	}

	ds, err := s.DataSourceRepo.GetByID(ctx, sourceID)
	if err != nil {
		return nil, err
	}

	profile, profErr := s.CreateSemanticProfile(
		ctx, sessionID, ds.WorkspaceID, sourceID, preSnapshot.ID,
		tableName, schemaSig, facts,
	)
	profileID := ""
	if profile != nil {
		profileID = profile.ID
	}
	if profErr != nil {
		log.Printf("pg import: create semantic profile failed source_id=%s err=%v", sourceID, profErr)
		errMsg := profErr.Error()
		if updateErr := s.SnapshotRepo.UpdateStatus(ctx, preSnapshot.ID, domain.SnapshotStatusFailed, &errMsg); updateErr != nil {
			log.Printf("pg import: failed to write profile error to snapshot snapshot_id=%s err=%v", preSnapshot.ID, updateErr)
		}
	}

	return &PGImportResult{
		SnapshotID:        preSnapshot.ID,
		ProfileID:         profileID,
		TableName:         tableName,
		RowCount:          rowCount,
		ColCount:          colCount,
		RowsImported:      rowCount,
		ImportDurationMs:  int(importDuration.Milliseconds()),
		ProfileDurationMs: int(profileDuration.Milliseconds()),
		SnapshotSizeBytes: snapshotSizeBytes,
		ProfileMode:       profileMode,
		ProfErr:           profErr,
	}, nil
}

var pgIdentifierPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

func sanitizePGIdentifier(name string) string {
	clean := strings.TrimSpace(name)
	if clean == "" || !pgIdentifierPattern.MatchString(clean) {
		return "_invalid"
	}
	return clean
}

func (s *SourceService) streamImportToSQLite(ctx context.Context, pgDB *sql.DB, schema, object string, ingester *data.Ingester, tableName string) (int, int, error) {
	schema = sanitizePGIdentifier(schema)
	object = sanitizePGIdentifier(object)
	if err := data.ValidateSQLIdent(tableName); err != nil {
		return 0, 0, fmt.Errorf("invalid SQLite table name after sanitization: %w", err)
	}
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
			log.Printf("pg import: scan error on row %d table=%s err=%v", rowCount, tableName, err)
			continue
		}
		row := make([]string, colCount)
		for i := range vals {
			if vals[i] == nil {
				row[i] = ""
			} else {
				row[i] = formatPGValue(vals[i])
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
	name = strings.ReplaceAll(name, " ", "_")
	if len(name) > 0 && name[0] >= '0' && name[0] <= '9' {
		name = "t_" + name
	}
	if name == "" || !pgIdentifierPattern.MatchString(name) {
		return "_table_invalid"
	}
	return name
}

func sanitizePGColumnName(name string) string {
	clean := strings.ToLower(strings.TrimSpace(name))
	clean = strings.ReplaceAll(clean, ".", "_")
	clean = strings.ReplaceAll(clean, "-", "_")
	clean = strings.ReplaceAll(clean, " ", "_")
	if clean == "" || !pgIdentifierPattern.MatchString(clean) {
		return "_col_invalid"
	}
	return clean
}

func isInAllowlist(entries []AllowlistEntry, schema, object string) bool {
	for _, e := range entries {
		if e.Schema == schema && e.Name == object {
			return true
		}
		if strings.EqualFold(e.Schema, schema) && strings.EqualFold(e.Name, object) {
			return true
		}
	}
	return false
}

func formatPGValue(v interface{}) string {
	switch tv := v.(type) {
	case time.Time:
		return tv.Format("2006-01-02 15:04:05")
	case *time.Time:
		if tv == nil {
			return ""
		}
		return tv.Format("2006-01-02 15:04:05")
	default:
		return fmt.Sprintf("%v", v)
	}
}
