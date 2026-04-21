package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ifnodoraemon/openDataAnalysis/auth"
	"github.com/ifnodoraemon/openDataAnalysis/config"
	"github.com/ifnodoraemon/openDataAnalysis/domain"
	"github.com/ifnodoraemon/openDataAnalysis/service"
)

func SessionSourcesHandler(w http.ResponseWriter, r *http.Request) {
	identity, ok := auth.FromContext(r.Context())
	if !ok || identity.UserID == "" {
		http.Error(w, "not authenticated", http.StatusUnauthorized)
		return
	}
	sessionID := chi.URLParam(r, "sessionID")

	sess, err := sessionRepo.GetByID(r.Context(), sessionID)
	if writeRepoLookupError(w, err, "session does not exist") {
		return
	}
	if sess.UserID != identity.UserID || sess.WorkspaceID != identity.WorkspaceID {
		http.Error(w, "not authorized to access this session", http.StatusForbidden)
		return
	}

	sources, err := sourceService.GetSessionSources(r.Context(), sessionID)
	if err != nil {
		http.Error(w, "failed to get data sources", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"sources": sources,
	})
}

func SemanticProfileDetailHandler(w http.ResponseWriter, r *http.Request) {
	identity, ok := auth.FromContext(r.Context())
	if !ok || identity.UserID == "" {
		http.Error(w, "not authenticated", http.StatusUnauthorized)
		return
	}
	profileID := chi.URLParam(r, "profileID")

	profile, confirmations, err := sourceService.GetProfileDetail(r.Context(), profileID)
	if err != nil {
		http.Error(w, "failed to get profile", http.StatusNotFound)
		return
	}

	ds, err := dataSourceRepo.GetByID(r.Context(), profile.SourceID)
	if err != nil || ds.WorkspaceID != identity.WorkspaceID {
		http.Error(w, "not authorized", http.StatusForbidden)
		return
	}

	confJSON, _ := json.Marshal(confirmations)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"profile_id":       profile.ID,
		"session_id":       profile.SessionID,
		"source_id":        profile.SourceID,
		"snapshot_id":      profile.SnapshotID,
		"table_name":       profile.AnalysisTableName,
		"schema_signature": profile.SchemaSignature,
		"profile_status":   string(profile.ProfileStatus),
		"profile_json":     json.RawMessage(profile.ProfileJSON),
		"confirmations":    json.RawMessage(string(confJSON)),
		"created_at":       profile.CreatedAt,
		"updated_at":       profile.UpdatedAt,
	})
}

type ConfirmProfileRequest struct {
	SessionID string                 `json:"session_id"`
	Scope     string                 `json:"scope"`
	Overrides map[string]interface{} `json:"overrides"`
}

func ConfirmProfileHandler(w http.ResponseWriter, r *http.Request) {
	identity, ok := auth.FromContext(r.Context())
	if !ok || identity.UserID == "" {
		http.Error(w, "not authenticated", http.StatusUnauthorized)
		return
	}
	profileID := chi.URLParam(r, "profileID")

	var req ConfirmProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	overridesJSON, _ := json.Marshal(req.Overrides)

	updated, err := sourceService.ConfirmProfile(
		r.Context(), profileID,
		identity.WorkspaceID, req.SessionID,
		identity.UserID, req.Scope, string(overridesJSON),
	)
	if err != nil {
		http.Error(w, fmt.Sprintf("confirmation failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"profile_id":     updated.ID,
		"profile_status": string(updated.ProfileStatus),
	})
}

func ListDataSourcesHandler(w http.ResponseWriter, r *http.Request) {
	identity, ok := auth.FromContext(r.Context())
	if !ok || identity.UserID == "" {
		http.Error(w, "not authenticated", http.StatusUnauthorized)
		return
	}

	sources, err := dataSourceRepo.ListByWorkspace(r.Context(), identity.WorkspaceID)
	if err != nil {
		http.Error(w, "failed to get data source list", http.StatusInternalServerError)
		return
	}

	var result []map[string]interface{}
	for _, ds := range sources {
		result = append(result, map[string]interface{}{
			"id":          ds.ID,
			"name":        ds.Name,
			"source_type": string(ds.SourceType),
			"status":      string(ds.Status),
			"created_at":  ds.CreatedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"data_sources": result,
	})
}

type CreateDataSourceRequest struct {
	Name       string              `json:"name"`
	SourceType string              `json:"source_type"`
	Postgres   *PostgresConnection `json:"postgres,omitempty"`
}

type PostgresConnection struct {
	Host          string           `json:"host"`
	Port          int              `json:"port"`
	DatabaseName  string           `json:"database_name"`
	DefaultSchema string           `json:"default_schema"`
	SSLMode       string           `json:"ssl_mode"`
	Username      string           `json:"username"`
	Password      string           `json:"password"`
	Allowlist     []AllowlistEntry `json:"allowlist"`
}

type AllowlistEntry struct {
	Schema string `json:"schema"`
	Name   string `json:"name"`
	Kind   string `json:"kind"`
}

func CreateDataSourceHandler(w http.ResponseWriter, r *http.Request) {
	identity, ok := auth.FromContext(r.Context())
	if !ok || identity.UserID == "" {
		http.Error(w, "not authenticated", http.StatusUnauthorized)
		return
	}

	var req CreateDataSourceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.SourceType != "postgres_connection" {
		http.Error(w, "only postgres_connection type is supported", http.StatusBadRequest)
		return
	}
	if req.Postgres == nil {
		http.Error(w, "missing postgres connection config", http.StatusBadRequest)
		return
	}

	if len(config.Cfg.AuthSecret) < 32 {
		http.Error(w, "AUTH_SECRET too short, cannot create SQL data source", http.StatusForbidden)
		return
	}

	conn := &domain.DatabaseConnection{
		Driver:        "postgres",
		Host:          req.Postgres.Host,
		Port:          req.Postgres.Port,
		DatabaseName:  req.Postgres.DatabaseName,
		DefaultSchema: req.Postgres.DefaultSchema,
		SSLMode:       req.Postgres.SSLMode,
		Username:      req.Postgres.Username,
	}

	allowlistJSON, _ := json.Marshal(req.Postgres.Allowlist)
	conn.AllowlistJSON = string(allowlistJSON)

	ciphertext, encErr := service.EncryptPassword(req.Postgres.Password, config.Cfg.AuthSecret)
	if encErr != nil {
		http.Error(w, fmt.Sprintf("credential encryption failed: %v", encErr), http.StatusInternalServerError)
		return
	}
	conn.SecretCiphertext = ciphertext

	ds, err := sourceService.CreatePostgresSource(r.Context(), identity.WorkspaceID, req.Name, identity.UserID, conn)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to create data source: %v", err), http.StatusInternalServerError)
		return
	}

	if err := dbConnectionRepo.Create(r.Context(), conn); err != nil {
		_ = dataSourceRepo.Delete(r.Context(), ds.ID)
		http.Error(w, fmt.Sprintf("failed to save connection config: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"id":          ds.ID,
		"name":        ds.Name,
		"source_type": string(ds.SourceType),
		"status":      string(ds.Status),
	})
}

func TestDataSourceHandler(w http.ResponseWriter, r *http.Request) {
	sourceID := chi.URLParam(r, "sourceID")
	identity, ok := auth.FromContext(r.Context())
	if !ok || identity.UserID == "" {
		http.Error(w, "not authenticated", http.StatusUnauthorized)
		return
	}
	ds, err := dataSourceRepo.GetByID(r.Context(), sourceID)
	if err != nil || ds.WorkspaceID != identity.WorkspaceID {
		http.Error(w, "data source does not exist", http.StatusNotFound)
		return
	}

	result := sourceService.TestPostgresConnection(r.Context(), sourceID, config.Cfg.AuthSecret)

	conn, _ := dbConnectionRepo.GetBySourceID(r.Context(), sourceID)
	if conn != nil {
		now := time.Now()
		conn.LastTestedAt = &now
		success, _ := result["success"].(bool)
		if success {
			conn.LastTestStatus = "success"
			conn.LastErrorMessage = nil
		} else {
			conn.LastTestStatus = "failed"
			if msg, ok := result["message"].(string); ok {
				conn.LastErrorMessage = &msg
			}
		}
		_ = dbConnectionRepo.Update(r.Context(), conn)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func CatalogDataSourceHandler(w http.ResponseWriter, r *http.Request) {
	sourceID := chi.URLParam(r, "sourceID")
	identity, ok := auth.FromContext(r.Context())
	if !ok || identity.UserID == "" {
		http.Error(w, "not authenticated", http.StatusUnauthorized)
		return
	}
	ds, err := dataSourceRepo.GetByID(r.Context(), sourceID)
	if err != nil || ds.WorkspaceID != identity.WorkspaceID {
		http.Error(w, "data source does not exist", http.StatusNotFound)
		return
	}

	conn, err := dbConnectionRepo.GetBySourceID(r.Context(), sourceID)
	if err != nil {
		http.Error(w, "connection config does not exist", http.StatusNotFound)
		return
	}

	allowlist, _ := service.ParseAllowlist(conn.AllowlistJSON)
	result := make([]map[string]interface{}, len(allowlist))
	for i, e := range allowlist {
		result[i] = map[string]interface{}{
			"schema": e.Schema,
			"name":   e.Name,
			"kind":   e.Kind,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"objects": result,
	})
}

type ImportRequest struct {
	SessionID  string `json:"session_id"`
	SchemaName string `json:"schema_name"`
	ObjectName string `json:"object_name"`
}

func ImportDataSourceHandler(w http.ResponseWriter, r *http.Request) {
	sourceID := chi.URLParam(r, "sourceID")
	identity, ok := auth.FromContext(r.Context())
	if !ok || identity.UserID == "" {
		http.Error(w, "not authenticated", http.StatusUnauthorized)
		return
	}
	ds, err := dataSourceRepo.GetByID(r.Context(), sourceID)
	if err != nil || ds.WorkspaceID != identity.WorkspaceID {
		http.Error(w, "data source does not exist", http.StatusNotFound)
		return
	}

	var req ImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.SessionID == "" || req.ObjectName == "" {
		http.Error(w, "missing session_id or object_name", http.StatusBadRequest)
		return
	}

	sess, _, sessErr := sessionManager.GetOrCreate(r.Context(), req.SessionID, identity.WorkspaceID, identity.UserID)
	if sessErr != nil {
		http.Error(w, "failed to get session", http.StatusInternalServerError)
		return
	}

	result, err := sourceService.ImportPostgresSnapshot(
		r.Context(), sourceID, req.SessionID, req.SchemaName, req.ObjectName,
		sess.Ingester, config.Cfg.AuthSecret,
	)
	if err != nil {
		http.Error(w, fmt.Sprintf("import failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"source_id":           sourceID,
		"snapshot_id":         result.SnapshotID,
		"analysis_table_name": result.TableName,
		"row_count":           result.RowCount,
		"column_count":        result.ColCount,
		"import_duration_ms":  result.ImportDurationMs,
		"profile_duration_ms": result.ProfileDurationMs,
		"profile_mode":        string(result.ProfileMode),
	})
}

func encryptPassword(password, authSecret string) ([]byte, error) {
	return service.EncryptPassword(password, authSecret)
}
