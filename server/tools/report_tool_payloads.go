package tools

import "strconv"

const reportDraftMessage = "当前报告仍处于草稿态，尚未生成最终报告文件。"

func reportDraftSuccess(toolName string, state *ReportState, fields map[string]interface{}) string {
	payload := reportDraftPayload(state, fields)
	return toolSuccess(toolName, payload)
}

func reportDraftPayload(state *ReportState, fields map[string]interface{}) map[string]interface{} {
	payload := clonePayload(fields)
	delivery := DescribeReportDeliveryState(state)
	payload["delivery_state"] = delivery.DeliveryState
	payload["is_finalized"] = delivery.IsFinalized
	payload["needs_finalize"] = delivery.NeedsFinalize
	payload["requires_finalize_for_delivery"] = delivery.HasContent
	payload["message"] = reportDraftMessage
	return payload
}

func reportEditScopeFailure(toolName, targetKey, targetID, targetLabel, uiSummary string, fields map[string]interface{}) string {
	payload := clonePayload(fields)
	payload[targetKey] = targetID
	payload["ui_summary"] = uiSummary
	return toolFailure(toolName, "edit_scope_violation", "当前编辑范围不允许修改该"+targetLabel, payload)
}

func reportFinalizeBlockedFailure(state *ReportState, blockers []string) string {
	return toolFailure("report_finalize", "active_goals_block_finalize", "当前仍有未闭环的根目标 / active branch，暂不允许生成最终报告。", mergePayloads(
		reportDraftPayload(state, nil),
		map[string]interface{}{
			"active_branch_count": len(blockers),
			"active_branches":     blockers,
			"can_finalize":        false,
			"message":             "当前仍有未闭环的根目标 / active branch，暂不允许生成最终报告。",
			"ui_summary":          formatFinalizeBlockedSummary(len(blockers)),
		},
	))
}

func reportFinalizeIssuesFailure(state *ReportState, issues []string) string {
	return toolFailure("report_finalize", "report_state_invalid", "当前报告状态未通过最终收尾校验。", mergePayloads(
		reportDraftPayload(state, nil),
		map[string]interface{}{
			"can_finalize":         false,
			"finalize_issue_count": len(issues),
			"finalize_issues":      issues,
			"message":              "当前报告状态未通过最终收尾校验。",
			"ui_summary":           formatFinalizeIssuesSummary(len(issues)),
		},
	))
}

func reportFinalizeSuccess(fields map[string]interface{}) string {
	payload := clonePayload(fields)
	payload["delivery_state"] = "finalized"
	payload["is_finalized"] = true
	payload["needs_finalize"] = false
	payload["message"] = "当前报告已完成最终收尾，并可作为最终报告交付。"
	return toolSuccess("report_finalize", payload)
}

func clonePayload(fields map[string]interface{}) map[string]interface{} {
	if len(fields) == 0 {
		return map[string]interface{}{}
	}
	payload := make(map[string]interface{}, len(fields))
	for key, value := range fields {
		payload[key] = value
	}
	return payload
}

func mergePayloads(base map[string]interface{}, extra map[string]interface{}) map[string]interface{} {
	payload := clonePayload(base)
	for key, value := range extra {
		payload[key] = value
	}
	return payload
}

func formatFinalizeBlockedSummary(count int) string {
	return "报告暂不能 finalize：仍有 " + strconv.Itoa(count) + " 条活跃分支。"
}

func formatFinalizeIssuesSummary(count int) string {
	return "报告暂不能 finalize：还有 " + strconv.Itoa(count) + " 个结构问题。"
}
