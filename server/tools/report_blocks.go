package tools

import (
	"fmt"
	"regexp"
	"strings"
)

type reportBlockMutationParams struct {
	Action        string        `json:"action"`
	BlockID       string        `json:"block_id"`
	BlockKind     string        `json:"block_kind"`
	Title         string        `json:"title"`
	Content       string        `json:"content"`
	ChartID       string        `json:"chart_id"`
	BeforeBlockID string        `json:"before_block_id"`
	AfterBlockID  string        `json:"after_block_id"`
	Sources       []EvidenceRef `json:"sources"`
}

type reportBlockMutationResult struct {
	Action     string
	BlockID    string
	BlockKind  string
	BlockCount int
	UISummary  string
}

type reportBlockScopeError struct {
	Action  string
	BlockID string
}

func (e reportBlockScopeError) Error() string {
	return fmt.Sprintf("block %s is outside editable scope for %s", e.BlockID, e.Action)
}

func applyReportBlockMutation(state *ReportState, editState *ReportEditState, params reportBlockMutationParams) (reportBlockMutationResult, error) {
	if state == nil {
		return reportBlockMutationResult{}, fmt.Errorf("report state is not initialized")
	}

	action := strings.TrimSpace(params.Action)
	if action == "" {
		action = "append"
	}
	params.Action = action

	blockID := strings.TrimSpace(params.BlockID)
	if blockID == "" && action != "append" {
		return reportBlockMutationResult{}, fmt.Errorf("block_id is required for %s action", action)
	}
	params.BlockID = blockID

	switch action {
	case "append", "upsert":
		return upsertReportBlock(state, editState, params)
	case "remove":
		return removeReportBlock(state, editState, params)
	case "move":
		return moveReportBlock(state, editState, params)
	default:
		return reportBlockMutationResult{}, fmt.Errorf("unknown action: %s", action)
	}
}

func upsertReportBlock(state *ReportState, editState *ReportEditState, params reportBlockMutationParams) (reportBlockMutationResult, error) {
	kind := strings.TrimSpace(params.BlockKind)
	if kind == "" {
		kind = "markdown"
	}
	block, err := buildReportBlock(kind, params.BlockID, strings.TrimSpace(params.Title), params.Content, strings.TrimSpace(params.ChartID), params.Sources, len(state.Blocks)+1)
	if err != nil {
		return reportBlockMutationResult{}, err
	}
	if editState != nil && !editState.BlockMutationAllowed(params.Action, block.ID) {
		return reportBlockMutationResult{}, reportBlockScopeError{Action: params.Action, BlockID: block.ID}
	}

	existingIndex := findReportBlockIndex(state.Blocks, block.ID)
	insertHintIndex := -1
	summaryText := fmt.Sprintf("已将 block [%s] %s 写入 report state；delivery_state=draft", block.Kind, block.ID)
	if existingIndex >= 0 {
		if len(block.Sources) == 0 && len(state.Blocks[existingIndex].Sources) > 0 {
			block.Sources = state.Blocks[existingIndex].Sources
		}
		state.Blocks = append(state.Blocks[:existingIndex], state.Blocks[existingIndex+1:]...)
		insertHintIndex = existingIndex
		summaryText = fmt.Sprintf("已更新 report state 中的 block [%s] %s；delivery_state=draft", block.Kind, block.ID)
	}

	insertAt := len(state.Blocks)
	if strings.TrimSpace(params.BeforeBlockID) == "" && strings.TrimSpace(params.AfterBlockID) == "" && insertHintIndex >= 0 {
		insertAt = insertHintIndex
	} else {
		insertAt, err = resolveReportBlockInsertIndex(state.Blocks, strings.TrimSpace(params.BeforeBlockID), strings.TrimSpace(params.AfterBlockID))
		if err != nil {
			return reportBlockMutationResult{}, err
		}
	}

	state.Blocks = insertReportBlockAt(state.Blocks, block, insertAt)
	state.NeedsFinalize = true
	return reportBlockMutationResult{
		Action:     params.Action,
		BlockID:    block.ID,
		BlockKind:  block.Kind,
		BlockCount: len(state.Blocks),
		UISummary:  summaryText,
	}, nil
}

func removeReportBlock(state *ReportState, editState *ReportEditState, params reportBlockMutationParams) (reportBlockMutationResult, error) {
	if editState != nil && !editState.BlockMutationAllowed(params.Action, params.BlockID) {
		return reportBlockMutationResult{}, reportBlockScopeError{Action: params.Action, BlockID: params.BlockID}
	}

	index := findReportBlockIndex(state.Blocks, params.BlockID)
	if index < 0 {
		return reportBlockMutationResult{}, fmt.Errorf("block_id %s not found", params.BlockID)
	}

	removed := state.Blocks[index]
	state.Blocks = append(state.Blocks[:index], state.Blocks[index+1:]...)
	state.NeedsFinalize = true
	return reportBlockMutationResult{
		Action:     params.Action,
		BlockID:    params.BlockID,
		BlockKind:  removed.Kind,
		BlockCount: len(state.Blocks),
		UISummary:  fmt.Sprintf("已从 report state 删除 block [%s] %s；delivery_state=draft", removed.Kind, removed.ID),
	}, nil
}

func moveReportBlock(state *ReportState, editState *ReportEditState, params reportBlockMutationParams) (reportBlockMutationResult, error) {
	if editState != nil && !editState.BlockMutationAllowed(params.Action, params.BlockID) {
		return reportBlockMutationResult{}, reportBlockScopeError{Action: params.Action, BlockID: params.BlockID}
	}

	index := findReportBlockIndex(state.Blocks, params.BlockID)
	if index < 0 {
		return reportBlockMutationResult{}, fmt.Errorf("block_id %s not found", params.BlockID)
	}

	block := state.Blocks[index]
	blocks := append([]ReportBlock{}, state.Blocks[:index]...)
	blocks = append(blocks, state.Blocks[index+1:]...)
	insertAt, err := resolveReportBlockInsertIndex(blocks, strings.TrimSpace(params.BeforeBlockID), strings.TrimSpace(params.AfterBlockID))
	if err != nil {
		return reportBlockMutationResult{}, err
	}

	state.Blocks = insertReportBlockAt(blocks, block, insertAt)
	state.NeedsFinalize = true
	return reportBlockMutationResult{
		Action:     params.Action,
		BlockID:    params.BlockID,
		BlockKind:  block.Kind,
		BlockCount: len(state.Blocks),
		UISummary:  fmt.Sprintf("已重排 report state 中的 block [%s] %s；delivery_state=draft", block.Kind, block.ID),
	}, nil
}

func buildBlockID(title string, fallbackIndex int) string {
	base := strings.ToLower(strings.TrimSpace(title))
	base = strings.ReplaceAll(base, " ", "-")
	base = strings.ReplaceAll(base, "_", "-")
	base = strings.ReplaceAll(base, "/", "-")
	base = strings.ReplaceAll(base, "\\", "-")
	base = strings.Trim(base, "-")
	if base == "" {
		base = fmt.Sprintf("section-%d", fallbackIndex)
	}
	return base
}

func buildReportBlock(kind, blockID, title, content, chartID string, sources []EvidenceRef, fallbackIndex int) (ReportBlock, error) {
	if blockID == "" {
		switch {
		case title != "":
			blockID = buildBlockID(title, fallbackIndex)
		case chartID != "":
			blockID = buildBlockID(chartID, fallbackIndex)
		default:
			blockID = fmt.Sprintf("block-%d", fallbackIndex)
		}
	}
	block := ReportBlock{
		ID:      blockID,
		Kind:    kind,
		Title:   title,
		Content: content,
		ChartID: chartID,
		Sources: sources,
	}
	switch kind {
	case "markdown", "html":
		if strings.TrimSpace(content) == "" {
			return ReportBlock{}, fmt.Errorf("content is required for %s block", kind)
		}
	case "chart":
		if strings.TrimSpace(chartID) == "" {
			return ReportBlock{}, fmt.Errorf("chart_id is required for chart block")
		}
	default:
		return ReportBlock{}, fmt.Errorf("unsupported block_kind: %s", kind)
	}
	return block, nil
}

func findReportBlockIndex(blocks []ReportBlock, blockID string) int {
	for i, block := range blocks {
		if block.ID == blockID {
			return i
		}
	}
	return -1
}

func resolveReportBlockInsertIndex(blocks []ReportBlock, beforeBlockID, afterBlockID string) (int, error) {
	if beforeBlockID != "" && afterBlockID != "" {
		return 0, fmt.Errorf("before_block_id and after_block_id cannot both be set")
	}
	if beforeBlockID != "" {
		index := findReportBlockIndex(blocks, beforeBlockID)
		if index < 0 {
			return 0, fmt.Errorf("before_block_id %s not found", beforeBlockID)
		}
		return index, nil
	}
	if afterBlockID != "" {
		index := findReportBlockIndex(blocks, afterBlockID)
		if index < 0 {
			return 0, fmt.Errorf("after_block_id %s not found", afterBlockID)
		}
		return index + 1, nil
	}
	return len(blocks), nil
}

func insertReportBlockAt(blocks []ReportBlock, block ReportBlock, index int) []ReportBlock {
	if index < 0 {
		index = 0
	}
	if index > len(blocks) {
		index = len(blocks)
	}
	blocks = append(blocks, ReportBlock{})
	copy(blocks[index+1:], blocks[index:])
	blocks[index] = block
	return blocks
}

func chartRefsOutsideChartBlock(content string) []string {
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
