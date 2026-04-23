package agent

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/ifnodoraemon/openDataAnalysis/tools"
)

func init() {
	tools.RegisterGlobalTool(func(ctx tools.ToolContext) tools.Tool {
		if ctx.Subgoals == nil {
			return nil
		}
		manager, ok := ctx.Subgoals.(*SubgoalManager)
		if !ok {
			return nil
		}
		return &InspectGoalsTool{Subgoals: manager}
	})
	tools.RegisterGlobalTool(func(ctx tools.ToolContext) tools.Tool {
		if ctx.ReportState == nil {
			return nil
		}
		return &InspectReportStateTool{ReportState: ctx.ReportState}
	})
	tools.RegisterGlobalTool(func(ctx tools.ToolContext) tools.Tool {
		if ctx.EditState == nil {
			return nil
		}
		return &InspectReportEditStateTool{
			EditState:   ctx.EditState,
			ReportState: ctx.ReportState,
		}
	})
}

type InspectGoalsTool struct {
	Subgoals *SubgoalManager
}

func (t *InspectGoalsTool) Name() string {
	return "state_goal_inspect"
}

func (t *InspectGoalsTool) Description() string {
	return "Read the fact state of the goal tree. Returns goal count, status distribution, active root goals, active branches, and finalize readiness; does not modify any state."
}

func (t *InspectGoalsTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"required":[]}`)
}

func (t *InspectGoalsTool) Execute(args json.RawMessage) (string, error) {
	if t.Subgoals == nil {
		return "", fmt.Errorf("subgoals is not initialized")
	}

	payload := buildGoalStateFacts(t.Subgoals, true)
	payload["ok"] = true
	payload["tool"] = "state_goal_inspect"
	payload["ui_summary"] = fmt.Sprintf("Current goals: %d, active branches: %d.", payload["goal_count"], payload["active_branch_count"])

	return marshalToolPayload(payload)
}

type InspectReportStateTool struct {
	ReportState *tools.ReportState
}

type InspectReportEditStateTool struct {
	EditState   *tools.ReportEditState
	ReportState *tools.ReportState
}

func (t *InspectReportStateTool) Name() string {
	return "state_report_inspect"
}

func (t *InspectReportStateTool) Description() string {
	return "Read the fact view of the current report state. Returns blocks, charts, reference relationships, delivery_state, finalize completeness counts, and report_shape_facts such as observed opening synthesis, closing synthesis, action-plan, numbering, and cross-section language signals; does not modify any state."
}

func (t *InspectReportStateTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"required":[]}`)
}

func (t *InspectReportStateTool) Execute(args json.RawMessage) (string, error) {
	if t.ReportState == nil {
		return "", fmt.Errorf("report state is not initialized")
	}

	t.ReportState.RLock()

	type reportBlockSnapshot struct {
		ID                         string   `json:"id"`
		Kind                       string   `json:"kind"`
		Title                      string   `json:"title,omitempty"`
		ChartID                    string   `json:"chart_id,omitempty"`
		ChartRefs                  []string `json:"chart_refs,omitempty"`
		HasContent                 bool     `json:"has_content"`
		LeadingHeadingMatchesTitle bool     `json:"leading_heading_matches_title"`
		NeedsChartCaption          bool     `json:"needs_chart_caption"`
	}

	blocks := make([]reportBlockSnapshot, 0, len(t.ReportState.Blocks))
	referenceCounts := make(map[string]int)
	blocksWithDuplicateHeading := make([]string, 0)
	chartBlocksMissingCaption := make([]string, 0)
	for _, block := range t.ReportState.Blocks {
		refs := chartRefsInContent(block.Content)
		if strings.EqualFold(strings.TrimSpace(block.Kind), "chart") && strings.TrimSpace(block.ChartID) != "" {
			refs = append(refs, strings.TrimSpace(block.ChartID))
		}
		for _, ref := range refs {
			referenceCounts[ref]++
		}
		leadingHeadingMatchesTitle := tools.HasDuplicateLeadingHeadingForAgent(block)
		needsChartCaption := strings.EqualFold(strings.TrimSpace(block.Kind), "chart") &&
			strings.TrimSpace(block.ChartID) != "" &&
			strings.TrimSpace(block.Content) == ""
		if leadingHeadingMatchesTitle {
			blocksWithDuplicateHeading = append(blocksWithDuplicateHeading, block.ID)
		}
		if needsChartCaption {
			chartBlocksMissingCaption = append(chartBlocksMissingCaption, block.ID)
		}
		blocks = append(blocks, reportBlockSnapshot{
			ID:                         block.ID,
			Kind:                       block.Kind,
			Title:                      block.Title,
			ChartID:                    block.ChartID,
			ChartRefs:                  refs,
			HasContent:                 strings.TrimSpace(block.Content) != "",
			LeadingHeadingMatchesTitle: leadingHeadingMatchesTitle,
			NeedsChartCaption:          needsChartCaption,
		})
	}

	chartIDs := make([]string, 0, len(t.ReportState.Charts))
	chartSet := make(map[string]struct{}, len(t.ReportState.Charts))
	for _, chart := range t.ReportState.Charts {
		chartID := strings.TrimSpace(chart.ID)
		if chartID == "" {
			continue
		}
		chartIDs = append(chartIDs, chartID)
		chartSet[chartID] = struct{}{}
	}
	sort.Strings(chartIDs)

	var unreferencedCharts []string
	for _, chartID := range chartIDs {
		if referenceCounts[chartID] == 0 {
			unreferencedCharts = append(unreferencedCharts, chartID)
		}
	}

	var missingChartRefs []string
	for chartID := range referenceCounts {
		if _, ok := chartSet[chartID]; !ok {
			missingChartRefs = append(missingChartRefs, chartID)
		}
	}
	sort.Strings(missingChartRefs)

	duplicated := make(map[string]int)
	for chartID, count := range referenceCounts {
		if count > 1 {
			duplicated[chartID] = count
		}
	}

	textBlockCount := 0
	textBlocksWithoutCharts := 0
	for _, block := range t.ReportState.Blocks {
		kind := strings.ToLower(strings.TrimSpace(block.Kind))
		if kind != "markdown" && kind != "html" {
			continue
		}
		if strings.TrimSpace(block.Content) == "" {
			continue
		}
		textBlockCount++
		if len(chartRefsInContent(block.Content)) == 0 {
			textBlocksWithoutCharts++
		}
	}

	delivery := tools.DescribeReportDeliveryStateLocked(t.ReportState)
	finalizeIssues := tools.ReportFinalizeIssuesForAgentLocked(t.ReportState)
	renderableBlockCount := tools.RenderableReportBlockCountLocked(t.ReportState)
	blockCount := len(t.ReportState.Blocks)
	reportShapeFacts := buildReportShapeFacts(t.ReportState.Blocks)

	t.ReportState.RUnlock()

	payload := map[string]interface{}{
		"ok":                            true,
		"tool":                          "state_report_inspect",
		"block_count":                   blockCount,
		"renderable_block_count":        renderableBlockCount,
		"chart_count":                   len(chartIDs),
		"blocks":                        blocks,
		"chart_ids":                     chartIDs,
		"chart_reference_counts":        referenceCounts,
		"unreferenced_charts":           unreferencedCharts,
		"missing_chart_references":      missingChartRefs,
		"duplicated_chart_refs":         duplicated,
		"blocks_with_duplicate_heading": blocksWithDuplicateHeading,
		"chart_blocks_missing_caption":  chartBlocksMissingCaption,
		"text_block_count":              textBlockCount,
		"text_blocks_without_chart":     textBlocksWithoutCharts,
		"has_content":                   delivery.HasContent,
		"delivery_state":                delivery.DeliveryState,
		"is_finalized":                  delivery.IsFinalized,
		"needs_finalize":                delivery.NeedsFinalize,
		"final_title":                   delivery.FinalTitle,
		"final_author":                  delivery.FinalAuthor,
		"can_finalize_structurally":     len(finalizeIssues) == 0,
		"finalize_issue_count":          len(finalizeIssues),
		"finalize_issues":               finalizeIssues,
		"report_shape_facts":            reportShapeFacts,
		"ui_summary":                    fmt.Sprintf("Current report: %d renderable blocks, %d charts.", renderableBlockCount, len(chartIDs)),
	}
	return marshalToolPayload(payload)
}

func (t *InspectReportEditStateTool) Name() string {
	return "state_report_edit_inspect"
}

func (t *InspectReportEditStateTool) Description() string {
	return "Read the fact state of the current report partial edit scope. Returns the target block, allowed modification scope, and associated charts. Does not modify any state."
}

func (t *InspectReportEditStateTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"required":[]}`)
}

func (t *InspectReportEditStateTool) Execute(args json.RawMessage) (string, error) {
	if t.EditState == nil {
		return "", fmt.Errorf("report edit state is not initialized")
	}
	payload := t.EditState.Snapshot()
	if t.ReportState != nil && t.EditState.Active() {
		t.ReportState.RLock()
		if block, ok := findEditTargetBlock(t.ReportState, t.EditState.TargetBlockID); ok {
			payload["target_block"] = map[string]interface{}{
				"id":       block.ID,
				"kind":     block.Kind,
				"title":    block.Title,
				"chart_id": block.ChartID,
				"content":  block.Content,
			}
		}
		t.ReportState.RUnlock()
	}
	payload["ok"] = true
	payload["tool"] = "state_report_edit_inspect"
	if active, _ := payload["active"].(bool); active {
		payload["ui_summary"] = fmt.Sprintf("Active partial edit scope, target block: %s.", t.EditState.TargetBlockID)
	} else {
		payload["ui_summary"] = "No active partial edit scope."
	}
	return marshalToolPayload(payload)
}

func findEditTargetBlock(state *tools.ReportState, blockID string) (tools.ReportBlock, bool) {
	if state == nil {
		return tools.ReportBlock{}, false
	}
	target := strings.TrimSpace(blockID)
	for _, block := range state.Blocks {
		if strings.TrimSpace(block.ID) == target {
			return block, true
		}
	}
	return tools.ReportBlock{}, false
}

func chartRefsInContent(content string) []string {
	re := regexp.MustCompile(`\{\{chart:(\w+)\}\}`)
	matches := re.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}
	refs := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 && strings.TrimSpace(match[1]) != "" {
			refs = append(refs, strings.TrimSpace(match[1]))
		}
	}
	return refs
}

var reportNumberedSectionPattern = regexp.MustCompile(`^\s*(第[一二三四五六七八九十百千0-9]+(?:章|节|部分|篇)|[一二三四五六七八九十百千0-9]+[.、)）])\s*`)

func buildReportShapeFacts(blocks []tools.ReportBlock) map[string]interface{} {
	renderableBlocks := make([]tools.ReportBlock, 0, len(blocks))
	blockTitleSequence := make([]string, 0, len(blocks))
	openingIDs := make([]string, 0)
	closingIDs := make([]string, 0)
	actionPlanIDs := make([]string, 0)
	numberedIDs := make([]string, 0)
	crossSectionIDs := make([]string, 0)
	repeatedLocalHeadingCounts := map[string]int{
		"核心发现":   0,
		"关键发现":   0,
		"分析结论":   0,
		"趋势分析":   0,
		"核心建议":   0,
		"数据质量说明": 0,
	}

	for _, block := range blocks {
		kind := strings.ToLower(strings.TrimSpace(block.Kind))
		if kind != "markdown" && kind != "html" && kind != "chart" {
			continue
		}
		renderableBlocks = append(renderableBlocks, block)
		title := strings.TrimSpace(block.Title)
		if title == "" {
			title = strings.TrimSpace(block.ID)
		}
		blockTitleSequence = append(blockTitleSequence, title)

		text := title + "\n" + strings.TrimSpace(block.Content)
		normalizedText := strings.ToLower(text)
		if hasAnyTextSignal(normalizedText, []string{
			"执行摘要", "管理摘要", "核心发现", "关键发现", "全局概览", "业务概览", "总体概览",
			"executive summary", "management summary", "key findings", "overall view",
		}) {
			openingIDs = append(openingIDs, block.ID)
		}
		if hasAnyTextSignal(normalizedText, []string{
			"综合总结", "综合对比总结", "综合建议", "整体结论", "最终结论", "行动计划", "监控建议",
			"conclusion", "recommendations", "action plan", "next steps",
		}) {
			closingIDs = append(closingIDs, block.ID)
		}
		if hasAnyTextSignal(normalizedText, []string{
			"行动计划", "核心建议", "综合建议", "优先级", "kpi", "监控建议", "下一步",
			"action plan", "recommendations", "next steps", "owner",
		}) {
			actionPlanIDs = append(actionPlanIDs, block.ID)
		}
		if hasNumberedSectionSignal(block) {
			numberedIDs = append(numberedIDs, block.ID)
		}
		if hasAnyTextSignal(normalizedText, []string{
			"综合来看", "整体来看", "结合", "同时", "因此", "由此", "相比", "相较",
			"关联", "共同", "一方面", "另一方面", "这意味着", "overall", "combined",
			"therefore", "in contrast", "compared with", "driven by",
		}) {
			crossSectionIDs = append(crossSectionIDs, block.ID)
		}
		for heading := range repeatedLocalHeadingCounts {
			if strings.Contains(text, heading) {
				repeatedLocalHeadingCounts[heading]++
			}
		}
	}

	for heading, count := range repeatedLocalHeadingCounts {
		if count < 2 {
			delete(repeatedLocalHeadingCounts, heading)
		}
	}

	numberingStyle := "none"
	if len(renderableBlocks) > 0 && len(numberedIDs) == len(renderableBlocks) {
		numberingStyle = "consistent"
	} else if len(numberedIDs) > 0 {
		numberingStyle = "partial"
	}

	return map[string]interface{}{
		"block_title_sequence":             blockTitleSequence,
		"has_opening_synthesis":            len(openingIDs) > 0,
		"opening_synthesis_block_ids":      openingIDs,
		"has_closing_synthesis":            len(closingIDs) > 0,
		"closing_synthesis_block_ids":      closingIDs,
		"has_action_plan_language":         len(actionPlanIDs) > 0,
		"action_plan_language_block_ids":   actionPlanIDs,
		"section_numbering_style":          numberingStyle,
		"numbered_section_block_ids":       numberedIDs,
		"cross_section_language_block_ids": crossSectionIDs,
		"cross_section_language_count":     len(crossSectionIDs),
		"repeated_local_heading_counts":    repeatedLocalHeadingCounts,
	}
}

func hasAnyTextSignal(text string, signals []string) bool {
	for _, signal := range signals {
		if strings.Contains(text, strings.ToLower(signal)) {
			return true
		}
	}
	return false
}

func hasNumberedSectionSignal(block tools.ReportBlock) bool {
	candidates := []string{block.Title, block.ID}
	for _, line := range strings.Split(block.Content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			candidates = append(candidates, strings.TrimSpace(strings.TrimLeft(trimmed, "#")))
		}
		break
	}
	for _, candidate := range candidates {
		if reportNumberedSectionPattern.MatchString(strings.TrimSpace(candidate)) {
			return true
		}
	}
	return false
}

func marshalToolPayload(payload map[string]interface{}) (string, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}
