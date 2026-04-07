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
			ReportState: ctx.ReportState,
			Subgoals:    ctx.Subgoals,
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
	ReportState *ReportState
	Subgoals    SubgoalChecker
}

func (t *ConfigureReportTool) Name() string { return "report_configure_layout" }
func (t *ConfigureReportTool) Description() string {
	return "读取并修改报告布局配置。支持更新或重置 CSS 与 body class；会修改 report layout 状态，但不会直接修改 block 或 chart。返回结果包含更新后的布局事实与 delivery_state。"
}
func (t *ConfigureReportTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {"type": "string", "enum": ["merge", "reset"], "description": "merge（默认）或 reset。"},
			"custom_css": {"type": "string", "description": "追加到页面中的自定义 CSS。"},
			"body_class": {"type": "string", "description": "附加到 body 的 class。"}
		},
		"required": []
	}`)
}

func (t *ConfigureReportTool) Execute(args json.RawMessage) (string, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(args, &raw); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
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
		return toolFailure("report_configure_layout", "unsupported_layout_fields", "存在不支持的布局字段", map[string]interface{}{
			"unsupported_fields": unsupported,
			"ui_summary":         "存在不支持的布局字段。",
		}), nil
	}

	var params reportLayoutParams
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}

	result, err := applyReportLayoutMutation(t.ReportState, params)
	if err != nil {
		return "", err
	}

	return reportDraftSuccess("report_configure_layout", t.ReportState, map[string]interface{}{
		"action":         result.Action,
		"has_custom_css": result.HasCustomCSS,
		"body_class":     result.BodyClass,
		"ui_summary":     result.UISummary,
	}), nil
}

func (t *ManageReportBlocksTool) Name() string { return "report_manage_blocks" }
func (t *ManageReportBlocksTool) Description() string {
	return "修改报告中的 block 结构。支持 append、upsert、remove、move，作用对象是 markdown、html、chart 三类 block；markdown/html block 的 content 支持使用 `{{chart:chart_id}}` 占位符在正文中内联展示图表，chart block 用于独立图表段落。会直接修改报告内容结构，并返回 block_id、block_count 与 delivery_state 等事实。在局部编辑范围存在时，此工具只允许修改被授权的 block。"
}
func (t *ManageReportBlocksTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {"type": "string", "enum": ["append", "upsert", "remove", "move"], "description": "append（默认）、upsert、remove、move"},
			"block_id": {"type": "string", "description": "block 稳定 ID。upsert/remove/move 必填；append 可选，不填则自动生成。"},
			"block_kind": {"type": "string", "enum": ["markdown", "html", "chart"], "description": "block 类型。"},
			"title": {"type": "string", "description": "标题。"},
			"content": {"type": "string", "description": "block 内容。markdown/html block 可使用 {{chart:chart_id}} 内联图表；chart block 时作为图下说明。"},
			"chart_id": {"type": "string", "description": "chart block 引用的图表 ID。"},
			"before_block_id": {"type": "string", "description": "插入到某个 block 之前。"},
			"after_block_id": {"type": "string", "description": "插入到某个 block 之后。"},
			"sources": {
				"type": "array",
				"description": "当前 block 结论的来源引用，记录基于哪次查询/哪张图/哪张表得出。",
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
	var params reportBlockMutationParams
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}

	result, err := applyReportBlockMutation(t.ReportState, t.EditState, params)
	if err != nil {
		var scopeErr reportBlockScopeError
		if errors.As(err, &scopeErr) {
			return reportEditScopeFailure("report_manage_blocks", "block_id", scopeErr.BlockID, " block", fmt.Sprintf("block %s 不在当前局部编辑范围内", scopeErr.BlockID), map[string]interface{}{
				"action": scopeErr.Action,
			}), nil
		}
		return "", err
	}

	return reportDraftSuccess("report_manage_blocks", t.ReportState, map[string]interface{}{
		"action":      result.Action,
		"block_id":    result.BlockID,
		"block_kind":  result.BlockKind,
		"block_count": result.BlockCount,
		"ui_summary":  result.UISummary,
	}), nil
}

func (t *FinalizeReportTool) Name() string { return "report_finalize" }
func (t *FinalizeReportTool) Description() string {
	return "校验当前 report state 与未闭环目标；如果状态合法，则写入最终标题/作者并将 delivery_state 置为 finalized。失败时返回 blockers 或结构问题；不会自动补全缺失内容，也不会静默改写已有 block 或 chart。"
}
func (t *FinalizeReportTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"report_title": {"type": "string", "description": "报告标题"},
			"author": {"type": "string", "description": "作者/分析师名称"}
		},
		"required": ["report_title"]
	}`)
}

func (t *FinalizeReportTool) Execute(args json.RawMessage) (string, error) {
	var params reportFinalizeParams
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}

	result, err := finalizeReportState(t.ReportState, t.Subgoals, params)
	if err != nil {
		var blockedErr reportFinalizeBlockedError
		if errors.As(err, &blockedErr) {
			return reportFinalizeBlockedFailure(t.ReportState, blockedErr.Blockers), nil
		}
		var issuesErr reportFinalizeIssuesError
		if errors.As(err, &issuesErr) {
			return reportFinalizeIssuesFailure(t.ReportState, issuesErr.Issues), nil
		}
		var alreadyFinalizedErr reportAlreadyFinalizedError
		if errors.As(err, &alreadyFinalizedErr) {
			return reportAlreadyFinalizedFailure(t.ReportState), nil
		}
		return "", err
	}

	return reportFinalizeSuccess(map[string]interface{}{
		"report_title": result.ReportTitle,
		"author":       result.Author,
		"block_count":  result.BlockCount,
		"chart_count":  result.ChartCount,
		"ui_summary":   fmt.Sprintf("delivery_state=finalized；block_count=%d；chart_count=%d", result.BlockCount, result.ChartCount),
	}), nil
}
