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
}

type InspectGoalsTool struct {
	Subgoals *SubgoalManager
}

func (t *InspectGoalsTool) Name() string {
	return "state_goal_inspect"
}

func (t *InspectGoalsTool) Description() string {
	return "返回目标树的原始状态，包括目标数量、状态分布、活跃分支和目标清单。"
}

func (t *InspectGoalsTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"required":[]}`)
}

func (t *InspectGoalsTool) Execute(args json.RawMessage) (string, error) {
	if t.Subgoals == nil {
		return "", fmt.Errorf("subgoals is not initialized")
	}

	goals := t.Subgoals.ListAll()
	payload := map[string]interface{}{
		"ok":             true,
		"tool":           "state_goal_inspect",
		"goal_count":     len(goals),
		"goals":          goals,
		"active_roots":   0,
		"running_goals":  0,
		"pending_goals":  0,
		"complete_goals": 0,
		"rejected_goals": 0,
	}

	for _, goal := range goals {
		if strings.TrimSpace(goal.ParentGoalID) == "" && !isTerminalSubgoalStatus(goal.Status) {
			payload["active_roots"] = payload["active_roots"].(int) + 1
		}
		switch goal.Status {
		case StatusRunning:
			payload["running_goals"] = payload["running_goals"].(int) + 1
		case StatusPending:
			payload["pending_goals"] = payload["pending_goals"].(int) + 1
		case StatusComplete:
			payload["complete_goals"] = payload["complete_goals"].(int) + 1
		case StatusRejected:
			payload["rejected_goals"] = payload["rejected_goals"].(int) + 1
		}
	}

	canFinalize, blockers := t.Subgoals.CanFinalize()
	payload["can_finalize"] = canFinalize
	payload["active_branches"] = blockers
	payload["active_branch_count"] = len(blockers)
	payload["summary_text"] = fmt.Sprintf("当前共有 %d 个目标，%d 条活跃分支。", len(goals), len(blockers))

	return marshalToolPayload(payload)
}

type InspectReportStateTool struct {
	ReportState *tools.ReportState
}

func (t *InspectReportStateTool) Name() string {
	return "state_report_inspect"
}

func (t *InspectReportStateTool) Description() string {
	return "返回报告状态的原始事实，包括 block、chart、引用关系和完整性计数。"
}

func (t *InspectReportStateTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"required":[]}`)
}

func (t *InspectReportStateTool) Execute(args json.RawMessage) (string, error) {
	if t.ReportState == nil {
		return "", fmt.Errorf("report state is not initialized")
	}

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

	payload := map[string]interface{}{
		"ok":                            true,
		"tool":                          "state_report_inspect",
		"block_count":                   len(t.ReportState.Blocks),
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
		"summary_text":                  fmt.Sprintf("当前报告共有 %d 个 block、%d 张图表。", len(t.ReportState.Blocks), len(chartIDs)),
	}
	return marshalToolPayload(payload)
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

func marshalToolPayload(payload map[string]interface{}) (string, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}
