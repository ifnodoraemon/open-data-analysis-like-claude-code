package handler

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/ifnodoraemon/openDataAnalysis/agent"
	"github.com/ifnodoraemon/openDataAnalysis/domain"
	"github.com/ifnodoraemon/openDataAnalysis/session"
)

func serializeRuntimeState(memory map[string]string, subgoals []agent.Subgoal, reportHTML string) map[string]interface{} {
	return serializeRuntimeStateWithSnapshot(memory, subgoals, nil, reportHTML, nil)
}

func serializeRuntimeStateWithSnapshot(memory map[string]string, subgoals []agent.Subgoal, reportSnapshot *domain.ReportSnapshot, reportHTML string, editState *agent.EditStateUpdatedData) map[string]interface{} {
	resp := map[string]interface{}{
		"memory":      memory,
		"subgoals":    subgoals,
		"report_html": strings.TrimSpace(reportHTML),
	}
	if reportSnapshot != nil {
		resp["report_snapshot"] = reportSnapshot
	}
	if editState != nil {
		resp["edit_state"] = editState
	}
	return resp
}

func getSessionRuntimeState(ctx context.Context, workspaceID, userID, sessionID string) (map[string]string, []agent.Subgoal, *domain.ReportSnapshot, string, *agent.EditStateUpdatedData) {
	if sessionManager != nil {
		if sess, ok, err := sessionManager.Peek(sessionID, workspaceID, userID); err == nil && ok && sess != nil {
			memory, subgoals := sess.RuntimeState()
			reportSnapshot, reportHTML := renderLiveSessionRuntimeReport(sess)
			return memory, subgoals, reportSnapshot, reportHTML, sess.CurrentEditStateData()
		}
	}

	return loadSessionRuntimeStateFromPersistence(ctx, sessionID)
}

func loadSessionRuntimeStateFromPersistence(ctx context.Context, sessionID string) (map[string]string, []agent.Subgoal, *domain.ReportSnapshot, string, *agent.EditStateUpdatedData) {
	messages := collectSessionMessages(ctx, sessionID)
	if len(messages) == 0 {
		return map[string]string{}, []agent.Subgoal{}, nil, "", nil
	}
	return deriveRuntimeStateFromMessages(messages)
}

func deriveRuntimeStateFromRun(ctx context.Context, runID string) (map[string]string, []agent.Subgoal, *domain.ReportSnapshot, string, *agent.EditStateUpdatedData) {
	messages := collectRunTreeMessages(ctx, runID)
	if len(messages) == 0 {
		return map[string]string{}, []agent.Subgoal{}, nil, "", nil
	}
	return deriveRuntimeStateFromMessages(messages)
}

func collectSessionMessages(ctx context.Context, sessionID string) []domain.RunMessage {
	if runRepo == nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	runs, err := runRepo.ListBySession(ctx, sessionID, -1)
	if err != nil || len(runs) == 0 {
		return nil
	}
	var messages []domain.RunMessage
	for _, run := range runs {
		messages = append(messages, collectRunTreeMessages(ctx, run.ID)...)
	}
	sortRunMessages(messages)
	return messages
}

func collectRunTreeMessages(ctx context.Context, runID string) []domain.RunMessage {
	if messageRepo == nil || strings.TrimSpace(runID) == "" {
		return nil
	}
	messages, err := messageRepo.ListByRun(ctx, runID)
	if err != nil {
		return nil
	}
	if runRepo == nil {
		sortRunMessages(messages)
		return messages
	}

	queue := []string{runID}
	visited := map[string]bool{runID: true}
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		childRuns, err := runRepo.ListByParent(ctx, curr)
		if err != nil {
			continue
		}
		for _, childRun := range childRuns {
			if visited[childRun.ID] {
				continue
			}
			visited[childRun.ID] = true
			queue = append(queue, childRun.ID)
			if childMsgs, err := messageRepo.ListByRun(ctx, childRun.ID); err == nil {
				messages = append(messages, childMsgs...)
			}
		}
	}
	sortRunMessages(messages)
	return messages
}

func sortRunMessages(messages []domain.RunMessage) {
	sort.SliceStable(messages, func(i, j int) bool {
		return messages[i].CreatedAt.Before(messages[j].CreatedAt)
	})
}

func deriveRuntimeStateFromMessages(messages []domain.RunMessage) (map[string]string, []agent.Subgoal, *domain.ReportSnapshot, string, *agent.EditStateUpdatedData) {
	memory := map[string]string{}
	subgoals := []agent.Subgoal{}
	var reportSnapshot *domain.ReportSnapshot
	reportHTML := ""
	var editState *agent.EditStateUpdatedData
	pendingCalls := map[string]json.RawMessage{} // toolCallID -> arguments

	for _, msg := range messages {
		switch msg.Type {
		case string(agent.EventStateMemoryUpdated):
			var payload agent.MemoryUpdatedData
			if err := json.Unmarshal([]byte(msg.Content), &payload); err == nil {
				memory = payload.Facts
			}
		case string(agent.EventStateSubgoalsUpdated):
			var payload struct {
				Goals []agent.Subgoal `json:"goals"`
			}
			if err := json.Unmarshal([]byte(msg.Content), &payload); err == nil {
				subgoals = payload.Goals
			}
		case string(agent.EventReportUpdate), string(agent.EventReportFinal):
			var payload agent.ReportUpdateData
			if err := json.Unmarshal([]byte(msg.Content), &payload); err == nil {
				if payload.ReportSnapshot != nil {
					reportSnapshot = payload.ReportSnapshot
				}
				if strings.TrimSpace(payload.HTML) != "" {
					reportHTML = payload.HTML
				}
			}
		case string(agent.EventStateReportEditUpdated):
			var payload agent.EditStateUpdatedData
			if err := json.Unmarshal([]byte(msg.Content), &payload); err == nil {
				if payload.Active && payload.EditContext != nil {
					editState = &payload
				} else {
					editState = nil
				}
			}
		case string(agent.EventToolCall):
			if msg.ToolCallID != nil && *msg.ToolCallID != "" {
				pendingCalls[*msg.ToolCallID] = json.RawMessage(msg.Content)
			}

		case string(agent.EventToolResult):
			if msg.ToolCallID == nil || *msg.ToolCallID == "" {
				continue
			}
			args, ok := pendingCalls[*msg.ToolCallID]
			if !ok {
				continue
			}
			if msg.Success == nil || !*msg.Success {
				delete(pendingCalls, *msg.ToolCallID)
				continue
			}
			delete(pendingCalls, *msg.ToolCallID)

			switch msg.Name {
			case "memory_save_fact":
				var payload struct {
					Key  string `json:"key"`
					Fact string `json:"fact"`
				}
				if err := json.Unmarshal(args, &payload); err == nil && payload.Key != "" {
					memory[payload.Key] = payload.Fact
				}
			case "goal_manage":
				var callPayload struct {
					Action       string `json:"action"`
					Description  string `json:"description"`
					ParentGoalID string `json:"parent_goal_id"`
					GoalID       string `json:"goal_id"`
					Result       string `json:"result"`
				}
				if err := json.Unmarshal(args, &callPayload); err != nil {
					continue
				}
				switch callPayload.Action {
				case "add":
					// 从 tool result 中提取真实 goal_id；格式异常时不构造派生占位 ID。
					realID := extractGoalIDFromResult(msg.Content)
					if realID == "" {
						continue
					}
					subgoals = append(subgoals, agent.Subgoal{
						ID:           realID,
						ParentGoalID: callPayload.ParentGoalID,
						Description:  callPayload.Description,
						Status:       agent.StatusPending,
					})
				case "complete", "reject":
					status := agent.StatusComplete
					if callPayload.Action == "reject" {
						status = agent.StatusRejected
					}
					for i := range subgoals {
						if subgoals[i].ID == callPayload.GoalID {
							subgoals[i].Status = status
							subgoals[i].Result = callPayload.Result
						}
					}
				}
			}
		}
	}

	if strings.TrimSpace(reportHTML) == "" && reportSnapshot != nil {
		if html, ok := renderReportHTMLFromSnapshotData(reportSnapshot); ok {
			reportHTML = html
		}
	}

	return memory, subgoals, reportSnapshot, reportHTML, editState
}

// extractGoalIDFromResult 从 goal_manage tool result 的 JSON 中提取真实的 goal_id。
func extractGoalIDFromResult(resultContent string) string {
	var result struct {
		GoalID string `json:"goal_id"`
	}
	if err := json.Unmarshal([]byte(resultContent), &result); err == nil && result.GoalID != "" {
		return result.GoalID
	}
	return ""
}

func attachRuntimeState(ctx context.Context, resp map[string]interface{}, workspaceID, userID, sessionID string) {
	memory, subgoals, reportSnapshot, reportHTML, editState := getSessionRuntimeState(ctx, workspaceID, userID, sessionID)
	resp["runtimeState"] = serializeRuntimeStateWithSnapshot(memory, subgoals, reportSnapshot, reportHTML, editState)
}

func attachRunRuntimeState(ctx context.Context, resp map[string]interface{}, run domain.AnalysisRun) {
	memory, subgoals, reportSnapshot, reportHTML, editState := getSessionRuntimeState(ctx, run.WorkspaceID, run.UserID, run.SessionID)
	resp["runtimeState"] = serializeRuntimeStateWithSnapshot(memory, subgoals, reportSnapshot, reportHTML, editState)
}

func renderLiveSessionRuntimeReport(sess *session.Session) (*domain.ReportSnapshot, string) {
	if sess == nil || sess.ReportState == nil {
		return nil, ""
	}
	sess.ReportState.RLock()
	hasContent := len(sess.ReportState.Blocks) > 0 || len(sess.ReportState.Charts) > 0
	sess.ReportState.RUnlock()
	if !hasContent {
		return nil, ""
	}
	snapshot := buildReportSnapshot(sess.ReportState)
	html, _ := renderReportHTMLFromSnapshotData(&snapshot)
	return &snapshot, html
}
