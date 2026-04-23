package tools

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func init() {
	RegisterGlobalTool(func(ctx ToolContext) Tool {
		return &InspectTimeContextTool{
			SessionSourcesProvider: ctx.SessionSourcesProvider,
			ProfileDetailProvider:  ctx.ProfileDetailProvider,
			Now:                    ctx.Now,
		}
	})
}

type InspectTimeContextTool struct {
	SessionSourcesProvider SessionSourcesProvider
	ProfileDetailProvider  ProfileDetailProvider
	Now                    func() time.Time
}

func (t *InspectTimeContextTool) Name() string { return "state_time_context_inspect" }

func (t *InspectTimeContextTool) Description() string {
	return "Read current wall-clock date facts and uploaded data time coverage facts. Returns current_date/current_datetime/timezone plus each current session source's imported timestamp and time column coverage candidates from semantic profiles. Does not modify any state."
}

func (t *InspectTimeContextTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"required":[]}`)
}

func (t *InspectTimeContextTool) Execute(args json.RawMessage) (string, error) {
	now := time.Now()
	if t.Now != nil {
		now = t.Now()
	}
	current := now
	zoneName, zoneOffset := current.Zone()

	dataPeriods := []map[string]interface{}{}
	sourceCount := 0
	dataPeriodSourceCount := 0
	var profileErrors []string
	if t.SessionSourcesProvider != nil {
		sources, err := t.SessionSourcesProvider()
		if err != nil {
			return "", err
		}
		sourceCount = len(sources)
		for _, source := range sources {
			item := map[string]interface{}{
				"source_id":           source.SourceID,
				"display_name":        source.DisplayName,
				"source_type":         source.SourceType,
				"analysis_table_name": source.AnalysisTableName,
			}
			if !source.LastImportedAt.IsZero() {
				item["last_imported_at"] = source.LastImportedAt.Format(time.RFC3339)
			}
			candidates := []map[string]interface{}{}
			if strings.TrimSpace(source.ProfileID) != "" && t.ProfileDetailProvider != nil {
				profileJSON, _, err := t.ProfileDetailProvider(source.ProfileID)
				if err != nil {
					profileErrors = append(profileErrors, fmt.Sprintf("%s: %v", source.ProfileID, err))
				} else {
					candidates = extractTimeCandidates(profileJSON)
				}
			}
			item["time_candidates"] = candidates
			item["time_candidate_count"] = len(candidates)
			if start, end, ok := aggregateCoverage(candidates); ok {
				item["coverage_start"] = start
				item["coverage_end"] = end
				dataPeriodSourceCount++
			}
			dataPeriods = append(dataPeriods, item)
		}
	}

	payload := map[string]interface{}{
		"current_date":             current.Format("2006-01-02"),
		"current_datetime":         current.Format(time.RFC3339),
		"timezone":                 current.Location().String(),
		"timezone_abbreviation":    zoneName,
		"timezone_offset_seconds":  zoneOffset,
		"source_count":             sourceCount,
		"data_period_source_count": dataPeriodSourceCount,
		"data_periods":             dataPeriods,
		"data_period_basis":        "uploaded data time column coverage candidates; last_imported_at is an import timestamp, not the analysis data period",
		"ui_summary":               fmt.Sprintf("current_date=%s; source_count=%d; data_period_sources=%d", current.Format("2006-01-02"), sourceCount, dataPeriodSourceCount),
	}
	if len(profileErrors) > 0 {
		payload["profile_errors"] = profileErrors
	}
	return toolSuccess("state_time_context_inspect", payload), nil
}

func extractTimeCandidates(profileJSON string) []map[string]interface{} {
	var profile map[string]interface{}
	if err := json.Unmarshal([]byte(profileJSON), &profile); err != nil {
		return []map[string]interface{}{}
	}
	raw, ok := profile["time_candidates"]
	if !ok {
		return []map[string]interface{}{}
	}
	items, ok := raw.([]interface{})
	if !ok {
		return []map[string]interface{}{}
	}
	candidates := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		candidate, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		normalized := map[string]interface{}{}
		copyStringField(candidate, normalized, "column_name")
		copyStringField(candidate, normalized, "grain")
		copyStringField(candidate, normalized, "coverage_start")
		copyStringField(candidate, normalized, "coverage_end")
		copyBoolField(candidate, normalized, "estimated")
		copyBoolField(candidate, normalized, "confirmed")
		candidates = append(candidates, normalized)
	}
	return candidates
}

func copyStringField(from, to map[string]interface{}, key string) {
	value, ok := from[key].(string)
	if ok && strings.TrimSpace(value) != "" {
		to[key] = strings.TrimSpace(value)
	}
}

func copyBoolField(from, to map[string]interface{}, key string) {
	if value, ok := from[key].(bool); ok {
		to[key] = value
	}
}

func aggregateCoverage(candidates []map[string]interface{}) (string, string, bool) {
	start := ""
	end := ""
	for _, candidate := range candidates {
		if value, ok := candidate["coverage_start"].(string); ok && strings.TrimSpace(value) != "" {
			value = strings.TrimSpace(value)
			if start == "" || value < start {
				start = value
			}
		}
		if value, ok := candidate["coverage_end"].(string); ok && strings.TrimSpace(value) != "" {
			value = strings.TrimSpace(value)
			if end == "" || value > end {
				end = value
			}
		}
	}
	return start, end, start != "" && end != ""
}
