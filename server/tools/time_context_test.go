package tools

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ifnodoraemon/openDataAnalysis/service"
)

func TestInspectTimeContextToolReturnsCurrentDateAndDataPeriods(t *testing.T) {
	t.Parallel()

	loc := time.FixedZone("CST", 8*60*60)
	tool := &InspectTimeContextTool{
		Now: func() time.Time {
			return time.Date(2026, 4, 23, 9, 30, 0, 0, loc)
		},
		SessionSourcesProvider: func() ([]service.SessionSourceSummary, error) {
			return []service.SessionSourceSummary{
				{
					SourceID:          "src_1",
					DisplayName:       "销售数据",
					SourceType:        "file",
					AnalysisTableName: "sales",
					ProfileID:         "profile_1",
					LastImportedAt:    time.Date(2026, 4, 22, 18, 0, 0, 0, time.UTC),
				},
			}, nil
		},
		ProfileDetailProvider: func(profileID string) (string, string, error) {
			return `{
				"time_candidates": [
					{"column_name":"month","grain":"month","coverage_start":"2025-01","coverage_end":"2025-06","estimated":false}
				]
			}`, `[]`, nil
		},
	}

	result, err := tool.Execute(nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	var payload struct {
		OK                    bool   `json:"ok"`
		CurrentDate           string `json:"current_date"`
		DataPeriodSourceCount int    `json:"data_period_source_count"`
		DataPeriods           []struct {
			AnalysisTableName  string `json:"analysis_table_name"`
			CoverageStart      string `json:"coverage_start"`
			CoverageEnd        string `json:"coverage_end"`
			TimeCandidateCount int    `json:"time_candidate_count"`
			TimeCandidates     []struct {
				ColumnName    string `json:"column_name"`
				Grain         string `json:"grain"`
				CoverageStart string `json:"coverage_start"`
				CoverageEnd   string `json:"coverage_end"`
			} `json:"time_candidates"`
		} `json:"data_periods"`
	}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !payload.OK || payload.CurrentDate != "2026-04-23" {
		t.Fatalf("unexpected current date payload: %#v", payload)
	}
	if payload.DataPeriodSourceCount != 1 || len(payload.DataPeriods) != 1 {
		t.Fatalf("unexpected data periods: %#v", payload.DataPeriods)
	}
	period := payload.DataPeriods[0]
	if period.AnalysisTableName != "sales" || period.CoverageStart != "2025-01" || period.CoverageEnd != "2025-06" {
		t.Fatalf("unexpected aggregate period: %#v", period)
	}
	if period.TimeCandidateCount != 1 || len(period.TimeCandidates) != 1 || period.TimeCandidates[0].ColumnName != "month" {
		t.Fatalf("unexpected time candidates: %#v", period.TimeCandidates)
	}
}

func TestInspectTimeContextToolKeepsInjectedTimezoneDate(t *testing.T) {
	t.Parallel()

	loc := time.FixedZone("CST", 8*60*60)
	tool := &InspectTimeContextTool{
		Now: func() time.Time {
			return time.Date(2026, 4, 23, 0, 30, 0, 0, loc)
		},
	}

	result, err := tool.Execute(nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload["current_date"] != "2026-04-23" || payload["timezone_offset_seconds"] != float64(8*60*60) {
		t.Fatalf("unexpected timezone payload: %#v", payload)
	}
}

func TestInspectTimeContextToolWorksWithoutSources(t *testing.T) {
	t.Parallel()

	tool := &InspectTimeContextTool{
		Now: func() time.Time {
			return time.Date(2026, 4, 23, 1, 2, 3, 0, time.UTC)
		},
	}

	result, err := tool.Execute(nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload["current_date"] != "2026-04-23" || payload["source_count"] != float64(0) {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}
