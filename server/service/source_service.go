package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/ifnodoraemon/openDataAnalysis/data"
	"github.com/ifnodoraemon/openDataAnalysis/domain"
	"github.com/ifnodoraemon/openDataAnalysis/repository"
)

type SourceService struct {
	DataSourceRepo           repository.DataSourceRepository
	DBConnectionRepo         repository.DatabaseConnectionRepository
	SnapshotRepo             repository.SourceSnapshotRepository
	SessionSourceBindingRepo repository.SessionSourceBindingRepository
	SemanticProfileRepo      repository.SemanticProfileRepository
	SemanticConfirmationRepo repository.SemanticConfirmationRepository
}

func NewSourceService(
	dsRepo repository.DataSourceRepository,
	dbConnRepo repository.DatabaseConnectionRepository,
	snapRepo repository.SourceSnapshotRepository,
	bindingRepo repository.SessionSourceBindingRepository,
	profileRepo repository.SemanticProfileRepository,
	confirmRepo repository.SemanticConfirmationRepository,
) *SourceService {
	return &SourceService{
		DataSourceRepo:           dsRepo,
		DBConnectionRepo:         dbConnRepo,
		SnapshotRepo:             snapRepo,
		SessionSourceBindingRepo: bindingRepo,
		SemanticProfileRepo:      profileRepo,
		SemanticConfirmationRepo: confirmRepo,
	}
}

type FileMaterializeResult struct {
	SourceID   string
	SnapshotID string
	TableName  string
	RowCount   int
	ColCount   int
}

func (s *SourceService) EnsureFileSource(ctx context.Context, workspaceID, fileID, displayName, uploadedBy string) (*domain.DataSource, error) {
	existing, err := s.DataSourceRepo.GetByFileID(ctx, fileID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}
	ds := &domain.DataSource{
		ID:          "ds_" + uuid.New().String()[:12],
		WorkspaceID: workspaceID,
		Name:        displayName,
		SourceType:  domain.SourceTypeFileUpload,
		Status:      domain.SourceStatusActive,
		FileID:      &fileID,
		CreatedBy:   uploadedBy,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := s.DataSourceRepo.Create(ctx, ds); err != nil {
		return nil, fmt.Errorf("failed to create file-backed data source: %w", err)
	}
	return ds, nil
}

func (s *SourceService) CreateSnapshot(ctx context.Context, sessionID, sourceID, upstreamKind, upstreamSchema, upstreamObject, analysisTableName string, rowCount, colCount int, schemaSignature string, rowsImported, importDurationMs, profileDurationMs int, snapshotSizeBytes int64, profileMode domain.ProfileMode) (*domain.SourceSnapshot, error) {
	snapshot := &domain.SourceSnapshot{
		ID:                "snap_" + uuid.New().String()[:12],
		SessionID:         sessionID,
		SourceID:          sourceID,
		UpstreamKind:      upstreamKind,
		UpstreamSchema:    upstreamSchema,
		UpstreamObject:    upstreamObject,
		AnalysisTableName: analysisTableName,
		RowCount:          rowCount,
		ColumnCount:       colCount,
		Status:            domain.SnapshotStatusReady,
		SchemaSignature:   schemaSignature,
		ImportedAt:        time.Now(),
		RowsImported:      rowsImported,
		ImportDurationMs:  importDurationMs,
		ProfileDurationMs: profileDurationMs,
		SnapshotSizeBytes: snapshotSizeBytes,
		ProfileMode:       profileMode,
	}
	if err := s.SnapshotRepo.Create(ctx, snapshot); err != nil {
		return nil, fmt.Errorf("failed to create source snapshot: %w", err)
	}
	binding := &domain.SessionSourceBinding{
		SessionID:        sessionID,
		SourceID:         sourceID,
		ActiveSnapshotID: snapshot.ID,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	if err := s.SessionSourceBindingRepo.Upsert(ctx, binding); err != nil {
		return nil, fmt.Errorf("failed to create session source binding: %w", err)
	}
	return snapshot, nil
}

func (s *SourceService) GetActiveSnapshotForFile(ctx context.Context, sessionID, fileID string) (*domain.SourceSnapshot, error) {
	ds, err := s.DataSourceRepo.GetByFileID(ctx, fileID)
	if err != nil {
		return nil, err
	}
	if ds == nil {
		return nil, nil
	}
	binding, err := s.SessionSourceBindingRepo.GetBySessionAndSource(ctx, sessionID, ds.ID)
	if err != nil {
		return nil, err
	}
	if binding == nil {
		return nil, nil
	}
	return s.SnapshotRepo.GetByID(ctx, binding.ActiveSnapshotID)
}

func (s *SourceService) GetSessionSources(ctx context.Context, sessionID string) ([]SessionSourceSummary, error) {
	bindings, err := s.SessionSourceBindingRepo.GetBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	var summaries []SessionSourceSummary
	for _, b := range bindings {
		ds, err := s.DataSourceRepo.GetByID(ctx, b.SourceID)
		if err != nil {
			log.Printf("GetSessionSources: source lookup failed source_id=%s err=%v", b.SourceID, err)
			continue
		}
		snapshot, err := s.SnapshotRepo.GetByID(ctx, b.ActiveSnapshotID)
		if err != nil {
			log.Printf("GetSessionSources: snapshot lookup failed snapshot_id=%s err=%v", b.ActiveSnapshotID, err)
			continue
		}
		profiles, profErr := s.SemanticProfileRepo.ListBySource(ctx, b.SourceID)
		if profErr != nil {
			log.Printf("GetSessionSources: profile list failed source_id=%s err=%v", b.SourceID, profErr)
		}
		var semanticStatus string
		var profileID string
		if len(profiles) > 0 {
			semanticStatus = string(profiles[0].ProfileStatus)
			profileID = profiles[0].ID
		} else {
			semanticStatus = "pending"
		}
		summaries = append(summaries, SessionSourceSummary{
			SourceID:          ds.ID,
			DisplayName:       ds.Name,
			SourceType:        string(ds.SourceType),
			ActiveSnapshotID:  b.ActiveSnapshotID,
			AnalysisTableName: snapshot.AnalysisTableName,
			SnapshotStatus:    string(snapshot.Status),
			SemanticStatus:    semanticStatus,
			ProfileID:         profileID,
			RowCount:          snapshot.RowCount,
			ColCount:          snapshot.ColumnCount,
			LastImportedAt:    snapshot.ImportedAt,
			LargeDataset:      snapshot.RowCount >= 1000000,
			RowsImported:      snapshot.RowsImported,
			ImportDurationMs:  snapshot.ImportDurationMs,
			ProfileDurationMs: snapshot.ProfileDurationMs,
			SnapshotSizeBytes: snapshot.SnapshotSizeBytes,
			ProfileMode:       string(snapshot.ProfileMode),
			ErrorMessage:      func() string { if snapshot.ErrorMessage != nil { return *snapshot.ErrorMessage }; return "" }(),
		})
	}
	return summaries, nil
}

func (s *SourceService) GetSessionProfiles(ctx context.Context, sessionID string) ([]SemanticProfileSummary, error) {
	profiles, err := s.SemanticProfileRepo.ListBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	var summaries []SemanticProfileSummary
	for _, p := range profiles {
		summaries = append(summaries, SemanticProfileSummary{
			ProfileID:         p.ID,
			SourceID:          p.SourceID,
			AnalysisTableName: p.AnalysisTableName,
			ProfileStatus:     string(p.ProfileStatus),
			SchemaSignature:   p.SchemaSignature,
		})
	}
	return summaries, nil
}

func (s *SourceService) GetProfileDetail(ctx context.Context, profileID string) (*domain.SemanticProfile, []domain.SemanticConfirmation, error) {
	profile, err := s.SemanticProfileRepo.GetByID(ctx, profileID)
	if err != nil {
		return nil, nil, err
	}
	confirmations, err := s.SemanticConfirmationRepo.ListByProfile(ctx, profileID)
	if err != nil {
		return profile, nil, err
	}
	return profile, confirmations, nil
}

type ProfiledFacts struct {
	Schema            *data.SchemaInfo     `json:"schema"`
	SemanticCandidates []SemanticCandidate `json:"semantic_candidates"`
	JoinCandidates    []JoinCandidate      `json:"join_candidates"`
	MetricCandidates  []MetricCandidate    `json:"metric_candidates"`
	TimeCandidates    []TimeCandidate      `json:"time_candidates"`
	UnitCandidates    []UnitCandidate      `json:"unit_candidates"`
	Ambiguities       []Ambiguity          `json:"ambiguities"`
	Warnings          []string             `json:"warnings"`
}

type SemanticCandidate struct {
	ColumnName    string `json:"column_name"`
	BusinessAlias string `json:"business_alias"`
	Role          string `json:"role"`
}

type JoinCandidate struct {
	LeftTable  string `json:"left_table"`
	LeftColumn string `json:"left_column"`
	RightTable string `json:"right_table"`
	RightColumn string `json:"right_column"`
	Reason     string `json:"reason"`
}

type MetricCandidate struct {
	ColumnName string `json:"column_name"`
	SemanticKey string `json:"semantic_key"`
}

type TimeCandidate struct {
	ColumnName string `json:"column_name"`
	Grain      string `json:"grain"`
	CoverageStart string `json:"coverage_start,omitempty"`
	CoverageEnd   string `json:"coverage_end,omitempty"`
}

type UnitCandidate struct {
	ColumnName      string  `json:"column_name"`
	DetectedUnit    string  `json:"detected_unit"`
	Scale           float64 `json:"scale,omitempty"`
	ConflictWith    *string `json:"conflict_with,omitempty"`
}

type Ambiguity struct {
	Kind        string   `json:"kind"`
	Description string   `json:"description"`
	Candidates  []string `json:"candidates"`
}

func (s *SourceService) BuildProfileFacts(schema *data.SchemaInfo, semanticProfile *data.SemanticProfile, activeTables []string) ProfiledFacts {
	facts := ProfiledFacts{
		Schema: schema,
	}

	for _, col := range schema.Columns {
		facts.SemanticCandidates = append(facts.SemanticCandidates, SemanticCandidate{
			ColumnName:    col.Name,
			BusinessAlias: "",
			Role:          inferColumnRole(col),
		})
	}

	if semanticProfile != nil {
		for i := range semanticProfile.Columns {
			col := &semanticProfile.Columns[i]
			for j := range facts.SemanticCandidates {
				if facts.SemanticCandidates[j].ColumnName == col.Name {
					facts.SemanticCandidates[j].BusinessAlias = col.BusinessAlias
					if col.Role != "" {
						facts.SemanticCandidates[j].Role = col.Role
					}
				}
			}
		}
	}

	for _, tc := range schema.TimeColumns {
		facts.TimeCandidates = append(facts.TimeCandidates, TimeCandidate{
			ColumnName:    tc.Name,
			Grain:         tc.Grain,
			CoverageStart: tc.CoverageStart,
			CoverageEnd:   tc.CoverageEnd,
		})
	}

	if len(schema.TimeColumns) > 1 {
		candidates := make([]string, len(schema.TimeColumns))
		for i, tc := range schema.TimeColumns {
			candidates[i] = tc.Name
		}
		facts.Ambiguities = append(facts.Ambiguities, Ambiguity{
			Kind:        "multiple_time_columns",
			Description: "multiple time column candidates",
			Candidates:  candidates,
		})
	}

	ambiguousMetricGroups := inferAmbiguousMetricGroups(schema)
	for key, names := range ambiguousMetricGroups {
		facts.Ambiguities = append(facts.Ambiguities, Ambiguity{
			Kind:        "ambiguous_metrics",
			Description: fmt.Sprintf("same-semantics numeric column candidates: %s", key),
			Candidates:  names,
		})
		for _, name := range names {
			facts.MetricCandidates = append(facts.MetricCandidates, MetricCandidate{
				ColumnName:  name,
				SemanticKey: key,
			})
		}
	}

	if semanticProfile != nil {
		for _, rel := range semanticProfile.Relations {
			facts.JoinCandidates = append(facts.JoinCandidates, JoinCandidate{
				LeftTable:   schema.TableName,
				LeftColumn:  rel.SourceColumn,
				RightTable:  rel.TargetTable,
				RightColumn: rel.TargetColumn,
				Reason:      rel.Reason,
			})
		}
	}

	return facts
}

func (s *SourceService) CreateSemanticProfile(ctx context.Context, sessionID, workspaceID, sourceID, snapshotID, analysisTableName, schemaSignature string, facts ProfiledFacts) (*domain.SemanticProfile, error) {
	profileJSON, err := json.Marshal(facts)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize profile: %w", err)
	}

	profile := &domain.SemanticProfile{
		ID:               "sp_" + uuid.New().String()[:12],
		SessionID:        sessionID,
		SourceID:         sourceID,
		SnapshotID:       snapshotID,
		AnalysisTableName: analysisTableName,
		SchemaSignature:  schemaSignature,
		ProfileStatus:    domain.ProfileStatusProfiled,
		ProfileJSON:      string(profileJSON),
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	if err := s.SemanticProfileRepo.Create(ctx, profile); err != nil {
		return nil, fmt.Errorf("failed to create semantic profile: %w", err)
	}

	wsConfirmation, _ := s.SemanticProfileRepo.FindWorkspaceConfirmation(ctx, workspaceID, schemaSignature)
	if wsConfirmation != nil {
		_ = s.SemanticProfileRepo.UpdateStatus(ctx, profile.ID, domain.ProfileStatusConfirmed)
		profile.ProfileStatus = domain.ProfileStatusConfirmed
		log.Printf("workspace confirmation auto-applied for profile %s (signature=%s)", profile.ID, schemaSignature)
	}

	return profile, nil
}

func (s *SourceService) ConfirmProfile(ctx context.Context, profileID, workspaceID, sessionID, confirmedBy, scope, overridesJSON string) (*domain.SemanticProfile, error) {
	profile, err := s.SemanticProfileRepo.GetByID(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("failed to get profile: %w", err)
	}

	confirmation := &domain.SemanticConfirmation{
		ID:            "sc_" + uuid.New().String()[:12],
		ProfileID:     profileID,
		WorkspaceID:   workspaceID,
		SessionID:     sessionID,
		ConfirmedBy:   confirmedBy,
		Scope:         domain.ConfirmationScope(scope),
		OverridesJSON: overridesJSON,
		CreatedAt:     time.Now(),
	}
	if err := s.SemanticConfirmationRepo.Create(ctx, confirmation); err != nil {
		return nil, fmt.Errorf("failed to create semantic confirmation: %w", err)
	}

	merged := s.applyConfirmations(ctx, profile.ProfileJSON, workspaceID, profile.SchemaSignature, sessionID)
	if err := s.SemanticProfileRepo.UpdateProfileJSON(ctx, profileID, merged); err != nil {
		log.Printf("ConfirmProfile: UpdateProfileJSON failed profile_id=%s err=%v", profileID, err)
	}
	if err := s.SemanticProfileRepo.UpdateStatus(ctx, profileID, domain.ProfileStatusConfirmed); err != nil {
		log.Printf("ConfirmProfile: UpdateStatus failed profile_id=%s err=%v", profileID, err)
	}
	profile.ProfileJSON = merged
	profile.ProfileStatus = domain.ProfileStatusConfirmed

	return profile, nil
}

func (s *SourceService) applyConfirmations(ctx context.Context, profileJSON, workspaceID, schemaSignature, sessionID string) string {
	var profile map[string]interface{}
	if err := json.Unmarshal([]byte(profileJSON), &profile); err != nil {
		return profileJSON
	}

	if workspaceID != "" && schemaSignature != "" {
		wsConf, _ := s.SemanticProfileRepo.FindWorkspaceConfirmation(ctx, workspaceID, schemaSignature)
		if wsConf != nil {
			profile = deepMergeOverrideIntoProfile(profile, wsConf.OverridesJSON)
		}
	}

	if sessionID != "" {
		sessionConfs, _ := s.SemanticConfirmationRepo.ListBySession(ctx, sessionID)
		for _, sc := range sessionConfs {
			profile = deepMergeOverrideIntoProfile(profile, sc.OverridesJSON)
		}
	}

	return marshalProfile(profile)
}

func deepMergeOverrideIntoProfile(profile map[string]interface{}, overridesJSON string) map[string]interface{} {
	var overrides map[string]interface{}
	if err := json.Unmarshal([]byte(overridesJSON), &overrides); err != nil {
		return profile
	}
	for k, v := range overrides {
		if existing, ok := profile[k]; ok {
			profile[k] = deepMergeValues(existing, v)
		} else {
			profile[k] = v
		}
	}
	return profile
}

func deepMergeValues(base, override interface{}) interface{} {
	baseMap, baseIsMap := base.(map[string]interface{})
	overMap, overIsMap := override.(map[string]interface{})
	if baseIsMap && overIsMap {
		for k, v := range overMap {
			if existing, ok := baseMap[k]; ok {
				baseMap[k] = deepMergeValues(existing, v)
			} else {
				baseMap[k] = v
			}
		}
		return baseMap
	}
	return override
}

// Deprecated: use deepMergeOverrideIntoProfile instead
func mergeOverrideIntoProfile(profile map[string]interface{}, overridesJSON string) map[string]interface{} {
	var overrides map[string]interface{}
	if err := json.Unmarshal([]byte(overridesJSON), &overrides); err != nil {
		return profile
	}
	for k, v := range overrides {
		profile[k] = v
	}
	return profile
}

func marshalProfile(profile map[string]interface{}) string {
	result, err := json.Marshal(profile)
	if err != nil {
		return "{}"
	}
	return string(result)
}

func (s *SourceService) CreatePostgresSource(ctx context.Context, workspaceID, name, createdBy string, conn *domain.DatabaseConnection) (*domain.DataSource, error) {
	ds := &domain.DataSource{
		ID:          "ds_" + uuid.New().String()[:12],
		WorkspaceID: workspaceID,
		Name:        name,
		SourceType:  domain.SourceTypePostgresConnection,
		Status:      domain.SourceStatusActive,
		CreatedBy:   createdBy,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	if err := s.DataSourceRepo.Create(ctx, ds); err != nil {
		return nil, err
	}
	conn.SourceID = ds.ID
	log.Printf("Created postgres data source id=%s name=%s", ds.ID, name)
	return ds, nil
}

func ComputeSchemaSignature(schema *data.SchemaInfo) string {
	h := sha256.New()
	for _, col := range schema.Columns {
		h.Write([]byte(col.Name + ":" + col.Type + ":"))
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}

func inferColumnRole(col data.ColumnInfo) string {
	if col.TimeProfile != nil {
		return "time"
	}
	name := strings.ToLower(col.Name)
	if strings.HasSuffix(name, "_id") || name == "id" {
		return "id"
	}
	if col.Type == "NUMERIC" || col.Type == "INTEGER" || col.Type == "REAL" {
		return "metric"
	}
	return "dimension"
}

func inferAmbiguousMetricGroups(schema *data.SchemaInfo) map[string][]string {
	grouped := make(map[string][]string)
	for _, column := range schema.Columns {
		if !strings.EqualFold(strings.TrimSpace(column.Type), "NUMERIC") {
			continue
		}
		tokens := tokenizeColumnName(column.Name)
		if len(tokens) < 2 {
			continue
		}
		core := make([]string, 0, len(tokens))
		qualifierCount := 0
		for _, token := range tokens {
			if _, ok := metricQualifierTokens[token]; ok {
				qualifierCount++
				continue
			}
			core = append(core, token)
		}
		if qualifierCount == 0 || len(core) == 0 {
			continue
		}
		key := strings.Join(core, "_")
		grouped[key] = append(grouped[key], column.Name)
	}
	result := make(map[string][]string)
	for key, names := range grouped {
		if len(names) >= 2 {
			result[key] = names
		}
	}
	return result
}

var metricQualifierTokens = map[string]struct{}{
	"actual": {}, "adjusted": {}, "booked": {}, "confirmed": {},
	"estimated": {}, "est": {}, "final": {}, "forecast": {},
	"gross": {}, "net": {}, "planned": {}, "plan": {},
	"projected": {}, "raw": {}, "recognized": {}, "target": {},
	"tentative": {}, "unconfirmed": {},
}

func tokenizeColumnName(name string) []string {
	return strings.FieldsFunc(strings.ToLower(strings.TrimSpace(name)), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	})
}

type SessionSourceSummary struct {
	SourceID          string    `json:"source_id"`
	DisplayName       string    `json:"display_name"`
	SourceType        string    `json:"source_type"`
	ActiveSnapshotID  string    `json:"active_snapshot_id"`
	AnalysisTableName string    `json:"analysis_table_name"`
	SnapshotStatus    string    `json:"snapshot_status"`
	SemanticStatus    string    `json:"semantic_status"`
	ProfileID         string    `json:"profile_id,omitempty"`
	RowCount          int       `json:"row_count"`
	ColCount          int       `json:"column_count"`
	LastImportedAt    time.Time `json:"last_imported_at"`
	LargeDataset      bool      `json:"large_dataset"`
	RowsImported      int       `json:"rows_imported"`
	ImportDurationMs  int       `json:"import_duration_ms"`
	ProfileDurationMs int       `json:"profile_duration_ms"`
	SnapshotSizeBytes int64     `json:"snapshot_size_bytes"`
	ProfileMode       string    `json:"profile_mode"`
	ErrorMessage      string    `json:"error_message,omitempty"`
}

type SemanticProfileSummary struct {
	ProfileID         string `json:"profile_id"`
	SourceID          string `json:"source_id"`
	AnalysisTableName string `json:"analysis_table_name"`
	ProfileStatus     string `json:"profile_status"`
	SchemaSignature   string `json:"schema_signature"`
}
