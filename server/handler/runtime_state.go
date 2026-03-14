package handler

import (
	"context"
	"encoding/json"
	"fmt"

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

	runs, err := runRepo.ListBySession(ctx, sessionID, 1)
	if err != nil || len(runs) == 0 {
		return map[string]string{}, []agent.Subgoal{}
	}
	return deriveRuntimeStateFromRun(ctx, runs[0].ID)
}

func deriveRuntimeStateFromRun(ctx context.Context, runID string) (map[string]string, []agent.Subgoal) {
	if messageRepo == nil {
		return map[string]string{}, []agent.Subgoal{}
	}

	messages, err := messageRepo.ListByRun(ctx, runID)
	if err != nil {
		return map[string]string{}, []agent.Subgoal{}
	}

	memory := map[string]string{}
	subgoals := []agent.Subgoal{}
	pendingCalls := map[string][]json.RawMessage{}
	fallbackGoalCounter := 0

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
			pendingCalls[msg.Name] = append(pendingCalls[msg.Name], json.RawMessage(msg.Content))
		case string(agent.EventToolResult):
			if msg.Success == nil || !*msg.Success {
				if len(pendingCalls[msg.Name]) > 0 {
					pendingCalls[msg.Name] = pendingCalls[msg.Name][1:]
				}
				continue
			}
			if len(pendingCalls[msg.Name]) == 0 {
				continue
			}
			args := pendingCalls[msg.Name][0]
			pendingCalls[msg.Name] = pendingCalls[msg.Name][1:]
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
				var payload struct {
					Action       string `json:"action"`
					Description  string `json:"description"`
					ParentGoalID string `json:"parent_goal_id"`
					GoalID       string `json:"goal_id"`
					Result       string `json:"result"`
				}
				if err := json.Unmarshal(args, &payload); err != nil {
					continue
				}
				switch payload.Action {
				case "add":
					fallbackGoalCounter++
					fallbackID := fmt.Sprintf("derived_goal_%d", fallbackGoalCounter)
					subgoals = append(subgoals, agent.Subgoal{
						ID:           fallbackID,
						ParentGoalID: payload.ParentGoalID,
						Description:  payload.Description,
						Status:       agent.StatusPending,
					})
				case "complete", "reject":
					status := agent.StatusComplete
					if payload.Action == "reject" {
						status = agent.StatusRejected
					}
					for i := range subgoals {
						if subgoals[i].ID == payload.GoalID {
							subgoals[i].Status = status
							subgoals[i].Result = payload.Result
						}
					}
				}
			}
		}
	}

	return memory, subgoals
}

func attachRuntimeState(ctx context.Context, resp map[string]interface{}, workspaceID, userID, sessionID string) {
	memory, subgoals := getSessionRuntimeState(ctx, workspaceID, userID, sessionID)
	resp["runtimeState"] = serializeRuntimeState(memory, subgoals)
}

func attachRunRuntimeState(ctx context.Context, resp map[string]interface{}, run domain.AnalysisRun) {
	memory, subgoals := deriveRuntimeStateFromRun(ctx, run.ID)
	resp["runtimeState"] = serializeRuntimeState(memory, subgoals)
}
