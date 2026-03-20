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

// FinalizeReportTool 生成最终报告
type FinalizeReportTool struct {
	ReportState *ReportState
	Subgoals    SubgoalChecker
}

func (t *ConfigureReportTool) Name() string { return "report_configure_layout" }
func (t *ConfigureReportTool) Description() string {
	return "读取并修改报告布局配置。可用于更新或重置 CSS、body class 和封面/目录显示选项；会修改 report layout 状态，但不会直接修改 block 或 chart。执行后若当前报告已有内容，delivery_state 仍会保持 draft，只有 report_finalize 才会把当前报告变成最终可交付状态。出于安全原因，不支持注入自定义 HTML 壳或自定义 JS。"
}
func (t *ConfigureReportTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {"type": "string", "enum": ["merge", "reset"], "description": "merge（默认）或 reset。"},
			"custom_css": {"type": "string", "description": "追加到页面中的自定义 CSS。"},
			"body_class": {"type": "string", "description": "附加到 body 的 class。"},
			"hide_cover": {"type": "boolean", "description": "是否隐藏默认封面。"},
			"hide_toc": {"type": "boolean", "description": "是否隐藏默认目录。"}
		},
		"required": []
	}`)
}

func (t *ConfigureReportTool) Execute(args json.RawMessage) (string, error) {
	var params reportLayoutParams
	if err := json.Unmarshal(args, &params); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}

	result, err := applyReportLayoutMutation(t.ReportState, params)
	if err != nil {
		var unsafeErr reportLayoutUnsafeError
		if errors.As(err, &unsafeErr) {
			return toolFailure("report_configure_layout", "unsafe_layout_option", "出于安全原因，当前版本不支持 custom_html_shell 或 custom_js", map[string]interface{}{
				"action":     unsafeErr.Action,
				"ui_summary": "当前版本已禁用自定义 HTML 壳和自定义 JS",
			}), nil
		}
		return "", err
	}

	return reportDraftSuccess("report_configure_layout", t.ReportState, map[string]interface{}{
		"action":         result.Action,
		"has_custom_css": result.HasCustomCSS,
		"body_class":     result.BodyClass,
		"hide_cover":     result.HideCover,
		"hide_toc":       result.HideTOC,
		"ui_summary":     result.UISummary,
	}), nil
}

func (t *ManageReportBlocksTool) Name() string { return "report_manage_blocks" }
func (t *ManageReportBlocksTool) Description() string {
	return "修改报告中的 block 结构。支持 append、upsert、remove、move，作用对象是 title、markdown、html、chart 四类 block；会直接修改报告内容结构，但执行后 report delivery_state 仍会保持 draft，只有 report_finalize 才会把当前报告变成最终可交付状态。在局部编辑范围存在时，此工具只允许修改被授权的 block。"
}
func (t *ManageReportBlocksTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {"type": "string", "enum": ["append", "upsert", "remove", "move"], "description": "append（默认）、upsert、remove、move"},
			"block_id": {"type": "string", "description": "block 稳定 ID。upsert/remove/move 必填；append 可选，不填则自动生成。"},
			"block_kind": {"type": "string", "enum": ["title", "markdown", "html", "chart"], "description": "block 类型。"},
			"title": {"type": "string", "description": "标题。"},
			"content": {"type": "string", "description": "block 内容。chart block 时作为图下说明。"},
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
	return "将当前 report state 从 draft 收尾为 finalized，并写入最终标题/作者。调用时会校验报告结构和未闭环目标；如果状态不合法会拒绝执行。该工具不负责补全缺失内容，只负责在当前状态可落地时完成收尾；未调用时，当前 block/chart 只停留在中间状态，不会落地为最终报告文件。"
}
func (t *FinalizeReportTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"report_title": {"type": "string", "description": "报告标题"},
			"author": {"type": "string", "description": "作者/分析师名称", "default": "AI 数据分析师"}
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
		return "", err
	}

	return reportFinalizeSuccess(map[string]interface{}{
		"report_title": result.ReportTitle,
		"author":       result.Author,
		"block_count":  result.BlockCount,
		"chart_count":  result.ChartCount,
		"ui_summary":   fmt.Sprintf("研究报告已生成完成（%d 个内容块，%d 个交互式图表）", result.BlockCount, result.ChartCount),
	}), nil
}
