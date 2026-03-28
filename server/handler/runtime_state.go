package handler

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/ifnodoraemon/openDataAnalysis/agent"
	"github.com/ifnodoraemon/openDataAnalysis/domain"
)

func serializeRuntimeState(memory map[string]string, subgoals []agent.Subgoal) map[string]interface{} {
	return map[string]interface{}{
		"memory":   memory,
		"subgoals": subgoals,
	}
}

func getSessionRuntimeState(ctx context.Context, workspaceID, userID, sessionID string) (map[string]string, []agent.Subgoal) {
	if sessionManager != nil {
		if sess, ok, err := sessionManager.Peek(sessionID, workspaceID, userID); err == nil && ok && sess != nil {
			return sess.RuntimeState()
		}
	}

	return loadSessionRuntimeStateFromPersistence(ctx, sessionID)
}

func loadSessionRuntimeStateFromPersistence(ctx context.Context, sessionID string) (map[string]string, []agent.Subgoal) {
	if strings.TrimSpace(sessionID) == "" {
		return map[string]string{}, []agent.Subgoal{}
	}

	runs, err := runRepo.ListBySession(ctx, sessionID, 1000)
	if err != nil || len(runs) == 0 {
		return map[string]string{}, []agent.Subgoal{}
	}

	memory := map[string]string{}
	var subgoals []agent.Subgoal

	// 历史状态由最新到最旧（DESC排），因此从最后一个（最古老）正向重置确保新状态覆盖老状态（Finding 2 Review）
	for i := len(runs) - 1; i >= 0; i-- {
		runMemory, runSubgoals := deriveRuntimeStateFromRun(ctx, runs[i].ID)
		for k, v := range runMemory {
			memory[k] = v
		}
		for _, sg := range runSubgoals {
			found := false
			for j := range subgoals {
				if subgoals[j].ID == sg.ID {
					found = true
					if sg.Status != agent.StatusPending {
						subgoals[j].Status = sg.Status
						subgoals[j].Result = sg.Result
					}
					break
				}
			}
			if !found {
				subgoals = append(subgoals, sg)
			}
		}
	}
	return memory, subgoals
}

func deriveRuntimeStateFromRun(ctx context.Context, runID string) (map[string]string, []agent.Subgoal) {
	if messageRepo == nil {
		return map[string]string{}, []agent.Subgoal{}
	}

	messages, err := messageRepo.ListByRun(ctx, runID)
	if err != nil {
		return map[string]string{}, []agent.Subgoal{}
	}

	// 聚合所有层级后代 run 的消息，并按时间排序，以防共享状态被旧值覆盖（Finding 1 Final）
	if runRepo != nil {
		queue := []string{runID}
		visited := map[string]bool{runID: true}

		for len(queue) > 0 {
			curr := queue[0]
			queue = queue[1:]

			if childRuns, err := runRepo.ListByParent(ctx, curr); err == nil {
				for _, childRun := range childRuns {
					if !visited[childRun.ID] {
						visited[childRun.ID] = true
						queue = append(queue, childRun.ID)
						if childMsgs, err := messageRepo.ListByRun(ctx, childRun.ID); err == nil {
							messages = append(messages, childMsgs...)
						}
					}
				}
			}
		}
	}

	sort.SliceStable(messages, func(i, j int) bool {
		return messages[i].CreatedAt.Before(messages[j].CreatedAt)
	})

	memory := map[string]string{}
	subgoals := []agent.Subgoal{}
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
					// 从 tool result 中提取真实 goal_id，而不是用 fallback derived_goal_N
					realID := extractGoalIDFromResult(msg.Content)
					if realID == "" {
						// 最后手段：无法获取真实 ID，说明 result 格式异常，跳过
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

	return memory, subgoals
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
	memory, subgoals := getSessionRuntimeState(ctx, workspaceID, userID, sessionID)
	resp["runtimeState"] = serializeRuntimeState(memory, subgoals)
}

func attachRunRuntimeState(ctx context.Context, resp map[string]interface{}, run domain.AnalysisRun) {
	memory, subgoals := deriveRuntimeStateFromRun(ctx, run.ID)
	resp["runtimeState"] = serializeRuntimeState(memory, subgoals)
}
