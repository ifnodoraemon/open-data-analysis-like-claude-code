package tools

import "strconv"

const reportDraftMessage = "delivery_state=draft；尚无 finalized report file。"

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
	return toolFailure("report_finalize", "active_goals_block_finalize", "检测到未闭环的根目标 / active branch；delivery_state 保持 draft。", mergePayloads(
		reportDraftPayload(state, nil),
		map[string]interface{}{
			"active_branch_count": len(blockers),
			"active_branches":     blockers,
			"can_finalize":        false,
			"message":             "检测到未闭环的根目标 / active branch；delivery_state 保持 draft。",
			"ui_summary":          formatFinalizeBlockedSummary(len(blockers)),
		},
	))
}

func reportFinalizeIssuesFailure(state *ReportState, issues []string) string {
	return toolFailure("report_finalize", "report_state_invalid", "报告结构校验未通过；delivery_state 保持 draft。", mergePayloads(
		reportDraftPayload(state, nil),
		map[string]interface{}{
			"can_finalize":         false,
			"finalize_issue_count": len(issues),
			"finalize_issues":      issues,
			"message":              "报告结构校验未通过；delivery_state 保持 draft。",
			"ui_summary":           formatFinalizeIssuesSummary(len(issues)),
		},
	))
}

func reportAlreadyFinalizedFailure(state *ReportState) string {
	return toolFailure("report_finalize", "report_already_finalized", "delivery_state 已为 finalized；未检测到新的 draft 变更。", mergePayloads(
		reportDraftPayload(state, nil),
		map[string]interface{}{
			"can_finalize": false,
			"message":      "delivery_state 已为 finalized；未检测到新的 draft 变更。",
			"ui_summary":   "delivery_state=finalized；未检测到新的草稿改动。",
		},
	))
}

func reportFinalizeSuccess(fields map[string]interface{}) string {
	payload := clonePayload(fields)
	payload["delivery_state"] = "finalized"
	payload["is_finalized"] = true
	payload["needs_finalize"] = false
	payload["message"] = "delivery_state 已更新为 finalized。"
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
	return "finalize blocked：active_branch_count=" + strconv.Itoa(count)
}

func formatFinalizeIssuesSummary(count int) string {
	return "finalize blocked：finalize_issue_count=" + strconv.Itoa(count)
}
