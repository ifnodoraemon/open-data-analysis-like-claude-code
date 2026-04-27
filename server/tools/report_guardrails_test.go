package tools

import (
	"sync"
	"testing"
)

func TestReportEditStateRefreshFromReportStateCollectsEditableCharts(t *testing.T) {
	t.Parallel()

	state := &ReportState{
		Blocks: []ReportBlock{
			{
				ID:      "analysis",
				Kind:    "markdown",
				Title:   "分析",
				Content: "结论如下：{{chart:chart_inline}}",
				ChartID: "chart_primary",
			},
			{
				ID:      "other",
				Kind:    "chart",
				ChartID: "chart_other",
				Content: "说明",
			},
		},
	}
	editState := &ReportEditState{
		Mode:                "regenerate_block",
		TargetBlockID:       "analysis",
		PreserveOtherBlocks: true,
	}

	editState.RefreshFromReportState(state)

	if len(editState.AllowedChartIDs) != 2 {
		t.Fatalf("expected 2 editable charts, got %#v", editState.AllowedChartIDs)
	}
	if !editState.ChartMutationAllowed("chart_primary") || !editState.ChartMutationAllowed("chart_inline") {
		t.Fatalf("expected referenced charts to remain editable, got %#v", editState.AllowedChartIDs)
	}
	if editState.ChartMutationAllowed("chart_other") {
		t.Fatalf("expected unrelated chart to be blocked, got %#v", editState.AllowedChartIDs)
	}
	if !editState.BlockMutationAllowed("upsert", "analysis") {
		t.Fatal("expected target block upsert to be allowed")
	}
	if editState.BlockMutationAllowed("remove", "analysis") {
		t.Fatal("expected remove to be blocked when preserving other blocks")
	}
	if editState.BlockMutationAllowed("upsert", "other") {
		t.Fatal("expected non-target block to be blocked")
	}
}

func TestReportEditStateChartScopeRestrictsToTargetChart(t *testing.T) {
	t.Parallel()

	state := &ReportState{
		Blocks: []ReportBlock{
			{
				ID:      "analysis",
				Kind:    "markdown",
				Title:   "分析",
				Content: "结论如下：{{chart:chart_inline}}",
				ChartID: "chart_primary",
			},
		},
	}
	editState := &ReportEditState{
		Mode:                "revise_chart",
		TargetChartID:       "chart_inline",
		PreserveOtherBlocks: true,
	}

	editState.RefreshFromReportState(state)

	if editState.ScopeKind() != "partial_chart" {
		t.Fatalf("expected partial_chart scope, got %q", editState.ScopeKind())
	}
	if !editState.ChartMutationAllowed("chart_inline") {
		t.Fatalf("expected target chart to remain editable, got %#v", editState.AllowedChartIDs)
	}
	if editState.ChartMutationAllowed("chart_primary") {
		t.Fatalf("expected unrelated chart to be blocked, got %#v", editState.AllowedChartIDs)
	}
	if editState.BlockMutationAllowed("upsert", "analysis") {
		t.Fatal("expected block mutations to be blocked for chart-only scope")
	}
}

func TestNormalizeSectionTitleStripsCommonOrdinalPrefixes(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"二、各维度分布":      "各维度分布",
		"2. 各维度分布":     "各维度分布",
		"第3章 各维度分布":    "各维度分布",
		"  第十部分 经营分析 ": "经营分析",
	}

	for input, want := range cases {
		if got := normalizeSectionTitle(input); got != want {
			t.Fatalf("normalizeSectionTitle(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestReportEditStateConcurrentRefreshAndSnapshot(t *testing.T) {
	state := &ReportState{
		Blocks: []ReportBlock{
			{ID: "b1", Kind: "markdown", ChartID: "c1", Content: "{{chart:c2}}"},
		},
	}
	edit := &ReportEditState{
		Mode:                "regenerate_block",
		TargetBlockID:       "b1",
		PreserveOtherBlocks: true,
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			edit.RefreshFromReportState(state)
		}()
		go func() {
			defer wg.Done()
			_ = edit.Snapshot()
		}()
	}
	wg.Wait()
}

func TestReportEditStateConcurrentChartMutationReadAndRefresh(t *testing.T) {
	state := &ReportState{
		Blocks: []ReportBlock{
			{ID: "b1", Kind: "markdown", ChartID: "c1", Content: "{{chart:c2}}"},
		},
	}
	edit := &ReportEditState{
		Mode:                "regenerate_block",
		TargetBlockID:       "b1",
		PreserveOtherBlocks: true,
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			edit.RefreshFromReportState(state)
		}()
		go func() {
			defer wg.Done()
			_ = edit.ChartMutationAllowed("c1")
		}()
	}
	wg.Wait()
}

func TestReportEditStateConcurrentResetAndSnapshot(t *testing.T) {
	edit := &ReportEditState{
		Mode:          "revise_block",
		TargetBlockID: "b1",
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			edit.Reset()
		}()
		go func() {
			defer wg.Done()
			_ = edit.Snapshot()
		}()
	}
	wg.Wait()
}

func TestReportEditStateConcurrentScopeKindAndReset(t *testing.T) {
	edit := &ReportEditState{
		Mode:                "regenerate_block",
		TargetBlockID:       "b1",
		TargetChartID:       "c1",
		PreserveOtherBlocks: true,
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = edit.ScopeKind()
		}()
		go func() {
			defer wg.Done()
			edit.Reset()
		}()
	}
	wg.Wait()
}

func TestReportEditStateConcurrentBlockMutationAndReset(t *testing.T) {
	edit := &ReportEditState{
		Mode:                "regenerate_block",
		TargetBlockID:       "b1",
		PreserveOtherBlocks: true,
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = edit.BlockMutationAllowed("upsert", "b1")
		}()
		go func() {
			defer wg.Done()
			edit.Reset()
		}()
	}
	wg.Wait()
}
