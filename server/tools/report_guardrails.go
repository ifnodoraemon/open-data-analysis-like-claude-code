package tools

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/ifnodoraemon/openDataAnalysis/service"
)

var (
	sectionChapterPrefixPattern = regexp.MustCompile(`^(第[一二三四五六七八九十百千0-9]+(?:章|节|部分|篇)\s*)`)
	sectionOrdinalPrefixPattern = regexp.MustCompile(`^([一二三四五六七八九十百千0-9]+[.、)）]\s*)`)
	sectionWhitespacePattern    = regexp.MustCompile(`\s+`)
)

func (s *ReportEditState) RefreshFromReportState(state *ReportState) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.AllowedChartIDs = collectEditableChartIDs(state, s.TargetBlockID, s.TargetChartID)
}

func (s *ReportEditState) BlockMutationAllowed(action, blockID string) bool {
	if !s.Active() || !s.PreserveOtherBlocks {
		return true
	}
	if strings.TrimSpace(s.TargetChartID) != "" && strings.TrimSpace(s.TargetBlockID) == "" {
		return false
	}
	target := strings.TrimSpace(s.TargetBlockID)
	id := strings.TrimSpace(blockID)
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "upsert":
		return target != "" && id == target
	default:
		return false
	}
}

func (s *ReportEditState) ChartMutationAllowed(chartID string) bool {
	if !s.Active() || !s.PreserveOtherBlocks {
		return true
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.AllowedChartIDs[strings.TrimSpace(chartID)]
	return ok
}

func collectEditableChartIDs(state *ReportState, blockID, chartID string) map[string]struct{} {
	refs := make(map[string]struct{})
	if trimmedChartID := strings.TrimSpace(chartID); trimmedChartID != "" {
		refs[trimmedChartID] = struct{}{}
	}
	if state == nil || strings.TrimSpace(blockID) == "" {
		return refs
	}
	index := findReportBlockIndex(state.Blocks, strings.TrimSpace(blockID))
	if index < 0 {
		return refs
	}
	block := state.Blocks[index]
	if strings.TrimSpace(block.ChartID) != "" {
		refs[strings.TrimSpace(block.ChartID)] = struct{}{}
	}
	for _, ref := range chartRefsOutsideChartBlock(block.Content) {
		refs[ref] = struct{}{}
	}
	return refs
}

func referencedChartsOutsideChartBlocks(blocks []ReportBlock) map[string]struct{} {
	refs := make(map[string]struct{})
	for _, block := range blocks {
		if strings.EqualFold(strings.TrimSpace(block.Kind), "chart") {
			continue
		}
		for _, ref := range chartRefsOutsideChartBlock(block.Content) {
			refs[ref] = struct{}{}
		}
	}
	return refs
}

func renderableReportBlockCount(blocks []ReportBlock) int {
	count := 0
	for _, block := range blocks {
		switch strings.ToLower(strings.TrimSpace(block.Kind)) {
		case "markdown", "html", "chart":
			count++
		}
	}
	return count
}

func RenderableReportBlockCount(state *ReportState) int {
	if state == nil {
		return 0
	}
	return renderableReportBlockCount(state.Blocks)
}

func RenderableReportBlockCountLocked(state *ReportState) int {
	return RenderableReportBlockCount(state)
}

func reportFinalizeIssues(state *ReportState) []string {
	if state == nil {
		return []string{"report_state_missing"}
	}

	chartSet := make(map[string]struct{}, len(state.Charts))
	for _, chart := range state.Charts {
		chartID := strings.TrimSpace(chart.ID)
		if chartID != "" {
			chartSet[chartID] = struct{}{}
		}
	}

	refCounts := make(map[string]int)
	for _, block := range state.Blocks {
		if strings.EqualFold(strings.TrimSpace(block.Kind), "chart") && strings.TrimSpace(block.ChartID) != "" {
			refCounts[strings.TrimSpace(block.ChartID)]++
		}
		for chartID := range referencedChartsOutsideChartBlocks([]ReportBlock{block}) {
			refCounts[chartID]++
		}
	}

	var issues []string
	if renderableReportBlockCount(state.Blocks) == 0 {
		issues = append(issues, "report_has_no_blocks")
	}

	var missingCharts []string
	for chartID := range refCounts {
		if _, ok := chartSet[chartID]; !ok {
			missingCharts = append(missingCharts, chartID)
		}
	}
	sort.Strings(missingCharts)
	for _, chartID := range missingCharts {
		issues = append(issues, "missing_chart:"+chartID)
	}

	var duplicateCharts []string
	for chartID, count := range refCounts {
		if count > 1 {
			duplicateCharts = append(duplicateCharts, fmt.Sprintf("%s(x%d)", chartID, count))
		}
	}
	sort.Strings(duplicateCharts)
	for _, item := range duplicateCharts {
		issues = append(issues, "duplicate_chart:"+item)
	}

	return issues
}

func reportSemanticFinalizeIssues(state *ReportState, sources []service.SessionSourceSummary) []string {
	if state == nil || len(sources) == 0 {
		return nil
	}

	var unresolved []service.SessionSourceSummary
	for _, source := range sources {
		if source.AmbiguityCount <= 0 {
			continue
		}
		status := strings.ToLower(strings.TrimSpace(source.SemanticStatus))
		if status == "confirmed" || source.ConfirmedOverrideCount >= source.AmbiguityCount {
			continue
		}
		unresolved = append(unresolved, source)
	}
	if len(unresolved) == 0 {
		return nil
	}

	state.RLock()
	defer state.RUnlock()
	if reportContainsSemanticAssumptionLocked(state) {
		return nil
	}

	issues := make([]string, 0, len(unresolved))
	for _, source := range unresolved {
		ref := strings.TrimSpace(source.AnalysisTableName)
		if ref == "" {
			ref = strings.TrimSpace(source.ProfileID)
		}
		if ref == "" {
			ref = strings.TrimSpace(source.SourceID)
		}
		if ref == "" {
			ref = "unknown_source"
		}
		issues = append(issues, fmt.Sprintf("unresolved_semantic_ambiguity:%s(%d)", ref, source.AmbiguityCount))
	}
	sort.Strings(issues)
	return issues
}

func reportContainsSemanticAssumptionLocked(state *ReportState) bool {
	if state == nil {
		return false
	}
	markers := []string{
		"假设",
		"口径",
		"歧义",
		"未确认",
		"待确认",
		"assumption",
		"assumed",
		"ambiguous",
		"unconfirmed",
		"definition",
	}
	for _, block := range state.Blocks {
		text := strings.ToLower(strings.TrimSpace(block.Content + " " + block.Title))
		for _, source := range block.Sources {
			text += " " + strings.ToLower(source.Summary+" "+source.SQL+" "+source.TableName+" "+source.ToolName)
		}
		for _, marker := range markers {
			if strings.Contains(text, marker) {
				return true
			}
		}
	}
	return false
}

func hasDuplicateLeadingHeading(block ReportBlock) bool {
	kind := strings.ToLower(strings.TrimSpace(block.Kind))
	if kind != "markdown" && kind != "html" {
		return false
	}
	if strings.TrimSpace(block.Title) == "" || strings.TrimSpace(block.Content) == "" {
		return false
	}
	lines := strings.Split(block.Content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, "#") {
			return false
		}
		heading := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
		return normalizeSectionTitle(heading) == normalizeSectionTitle(block.Title)
	}
	return false
}

func HasDuplicateLeadingHeadingForAgent(block ReportBlock) bool {
	return hasDuplicateLeadingHeading(block)
}

func normalizeSectionTitle(value string) string {
	normalized := strings.TrimSpace(value)
	normalized = sectionChapterPrefixPattern.ReplaceAllString(normalized, "")
	normalized = sectionOrdinalPrefixPattern.ReplaceAllString(normalized, "")
	normalized = sectionWhitespacePattern.ReplaceAllString(normalized, "")
	return normalized
}

func ReportFinalizeIssuesForAgent(state *ReportState) []string {
	return reportFinalizeIssues(state)
}

func ReportFinalizeIssuesForAgentLocked(state *ReportState) []string {
	return reportFinalizeIssues(state)
}
