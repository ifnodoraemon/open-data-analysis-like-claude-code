package tools

import (
	"encoding/json"
	"errors"
	"fmt"
)

func init() {
	RegisterGlobalTool(func(ctx ToolContext) Tool {
		return &ManageReportBlocksTool{ReportState: ctx.ReportState, EditState: ctx.EditState}
	})
	RegisterGlobalTool(func(ctx ToolContext) Tool {
		return &ConfigureReportTool{ReportState: ctx.ReportState}
	})
	RegisterGlobalTool(func(ctx ToolContext) Tool {
		return &FinalizeReportTool{
			ReportState:      ctx.ReportState,
			Subgoals:         ctx.Subgoals,
			AmbiguityChecker: ctx.AmbiguityChecker,
		}
	})
}

type ConfigureReportTool struct {
	ReportState *ReportState
}

type ManageReportBlocksTool struct {
	ReportState *ReportState
	EditState   *ReportEditState
}

// FinalizeReportTool 校验并更新报告交付状态
type FinalizeReportTool struct {
	ReportState      *ReportState
	Subgoals         SubgoalChecker
	AmbiguityChecker AmbiguityChecker
}

type AmbiguityBlocker struct {
	Kind        string   `json:"kind"`
	Description string   `json:"description"`
	Candidates  []string `json:"candidates"`
}

type AmbiguityChecker interface {
	CheckAmbiguities() ([]AmbiguityBlocker, error)
}

func (t *ConfigureReportTool) Name() string { return "report_configure_layout" }
func (t *ConfigureReportTool) Description() string {
	return "Read and modify report layout configuration. Supports updating or resetting CSS and body class; modifies report layout state but does not directly modify blocks or charts. Returns updated layout facts and delivery_state."
}
func (t *ConfigureReportTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {"type": "string", "enum": ["merge", "reset"], "description": "merge (default) or reset."},
			"custom_css": {"type": "string", "description": "Custom CSS appended to the page."},
			"body_class": {"type": "string", "description": "Class appended to the body element."}
		},
		"required": []
	}`)
}

func (t *ConfigureReportTool) Execute(args json.RawMessage) (string, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(args, &raw); err != nil {
		return "", fmt.Errorf("failed to parse parameters: %w", err)
	}
	unsupported := make([]string, 0)
	for key := range raw {
		switch key {
		case "action", "custom_css", "body_class":
		default:
			unsupported = append(unsupported, key)
		}
	}
	if len(unsupported) > 0 {
		return toolFailure("report_configure_layout", "unsupported_layout_fields", "unsupported layout fields", map[string]interface{}{
			"unsupported_fields": unsupported,
			"ui_summary":         "unsupported layout fields.",
		}), nil
	}

	var params reportLayoutParams
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("failed to parse parameters: %w", err)
	}

	t.ReportState.Lock()
	result, err := applyReportLayoutMutation(t.ReportState, params)
	if err != nil {
		t.ReportState.Unlock()
		return "", err
	}

	success := reportDraftSuccess("report_configure_layout", t.ReportState, map[string]interface{}{
		"action":         result.Action,
		"has_custom_css": result.HasCustomCSS,
		"body_class":     result.BodyClass,
		"ui_summary":     result.UISummary,
	})
	t.ReportState.Unlock()
	return success, nil
}

func (t *ManageReportBlocksTool) Name() string { return "report_manage_blocks" }
func (t *ManageReportBlocksTool) Description() string {
	return "Modify report block structure. Supports append, upsert, remove, move for markdown, html, and chart blocks; markdown/html block content supports `{{chart:chart_id}}` placeholders for inline chart display, chart blocks are for standalone chart sections. Directly modifies report content structure and returns block_id, block_count, and delivery_state facts. When a partial edit scope is active, only authorized blocks can be modified."
}
func (t *ManageReportBlocksTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {"type": "string", "enum": ["append", "upsert", "remove", "move"], "description": "append (default), upsert, remove, or move"},
			"block_id": {"type": "string", "description": "Stable block ID. Required for upsert/remove/move; optional for append (auto-generated if omitted)."},
			"block_kind": {"type": "string", "enum": ["markdown", "html", "chart"], "description": "Block type."},
			"title": {"type": "string", "description": "Block title."},
			"content": {"type": "string", "description": "Block content. Markdown/HTML blocks support {{chart:chart_id}} for inline charts; chart blocks use this as caption below the chart."},
			"chart_id": {"type": "string", "description": "Chart ID referenced by a chart block."},
			"before_block_id": {"type": "string", "description": "Insert before this block ID."},
			"after_block_id": {"type": "string", "description": "Insert after this block ID."},
			"sources": {
				"type": "array",
				"description": "Source citations for the block's conclusions, recording which query/chart/table the conclusions are based on.",
				"items": {
					"type": "object",
					"properties": {
						"kind":       {"type": "string", "enum": ["sql", "chart", "table", "python", "tool_result"]},
						"tool_name":  {"type": "string"},
						"sql":        {"type": "string"},
						"table_name": {"type": "string"},
						"chart_id":   {"type": "string"},
						"summary":    {"type": "string"}
					},
					"required": ["kind"]
				}
			}
		},
		"required": []
	}`)
}

func (t *ManageReportBlocksTool) Execute(args json.RawMessage) (string, error) {
	normalizedArgs, err := normalizeStringifiedJSONFields(args, "sources")
	if err != nil {
		return "", fmt.Errorf("failed to parse parameters: %w", err)
	}

	var params reportBlockMutationParams
	if err := json.Unmarshal(normalizedArgs, &params); err != nil {
		return "", fmt.Errorf("failed to parse parameters: %w", err)
	}

	t.ReportState.Lock()
	result, err := applyReportBlockMutation(t.ReportState, t.EditState, params)
	if err != nil {
		t.ReportState.Unlock()
		var scopeErr reportBlockScopeError
		if errors.As(err, &scopeErr) {
			return reportEditScopeFailure("report_manage_blocks", "block_id", scopeErr.BlockID, " block", fmt.Sprintf("block %s is outside current partial edit scope", scopeErr.BlockID), map[string]interface{}{
				"action": scopeErr.Action,
			}), nil
		}
		return "", err
	}

	success := reportDraftSuccess("report_manage_blocks", t.ReportState, map[string]interface{}{
		"action":      result.Action,
		"block_id":    result.BlockID,
		"block_kind":  result.BlockKind,
		"block_count": result.BlockCount,
		"ui_summary":  result.UISummary,
	})
	t.ReportState.Unlock()
	return success, nil
}

func (t *FinalizeReportTool) Name() string { return "report_finalize" }
func (t *FinalizeReportTool) Description() string {
	return "Validate current report state and unclosed goals; if valid, write final title/author and set delivery_state to finalized. On failure returns blockers or structural issues; does not auto-complete missing content or silently rewrite existing blocks or charts."
}
func (t *FinalizeReportTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"report_title": {"type": "string", "description": "Report title"},
			"author": {"type": "string", "description": "Author/analyst name"}
		},
		"required": ["report_title"]
	}`)
}

func (t *FinalizeReportTool) Execute(args json.RawMessage) (string, error) {
	var params reportFinalizeParams
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("failed to parse parameters: %w", err)
	}

	t.ReportState.Lock()
	result, err := finalizeReportState(t.ReportState, t.Subgoals, params, t.AmbiguityChecker)
	if err != nil {
		var blockedErr reportFinalizeBlockedError
		if errors.As(err, &blockedErr) {
			failure := reportFinalizeBlockedFailure(t.ReportState, blockedErr.Blockers)
			t.ReportState.Unlock()
			return failure, nil
		}
		var issuesErr reportFinalizeIssuesError
		if errors.As(err, &issuesErr) {
			failure := reportFinalizeIssuesFailure(t.ReportState, issuesErr.Issues)
			t.ReportState.Unlock()
			return failure, nil
		}
		var alreadyFinalizedErr reportAlreadyFinalizedError
		if errors.As(err, &alreadyFinalizedErr) {
			failure := reportAlreadyFinalizedFailure(t.ReportState)
			t.ReportState.Unlock()
			return failure, nil
		}
		t.ReportState.Unlock()
		return "", err
	}

	success := reportFinalizeSuccess(map[string]interface{}{
		"report_title": result.ReportTitle,
		"author":       result.Author,
		"block_count":  result.BlockCount,
		"chart_count":  result.ChartCount,
		"ui_summary":   fmt.Sprintf("delivery_state=finalized; block_count=%d; chart_count=%d", result.BlockCount, result.ChartCount),
	})
	t.ReportState.Unlock()
	return success, nil
}
