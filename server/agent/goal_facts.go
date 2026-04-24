package agent

import (
	"fmt"
	"strings"
)

func buildGoalStateFacts(subgoals *SubgoalManager, includeGoals bool) map[string]interface{} {
	payload := map[string]interface{}{
		"goal_count":           0,
		"active_roots":         0,
		"running_goals":        0,
		"pending_goals":        0,
		"complete_goals":       0,
		"rejected_goals":       0,
		"active_root_goal_ids": []string{},
		"active_root_goals":    []map[string]interface{}{},
	}
	if subgoals == nil {
		if includeGoals {
			payload["goals"] = []Subgoal{}
		}
		payload["active_branches"] = []string{}
		payload["active_branch_count"] = 0
		return payload
	}

	goals := subgoals.ListAll()
	payload["goal_count"] = len(goals)
	if includeGoals {
		payload["goals"] = goals
	}

	activeRoots := 0
	runningGoals := 0
	pendingGoals := 0
	completeGoals := 0
	rejectedGoals := 0

	activeRootIDs := make([]string, 0)
	activeRootGoals := make([]map[string]interface{}, 0)
	for _, goal := range goals {
		if strings.TrimSpace(goal.ParentGoalID) == "" && !isTerminalSubgoalStatus(goal.Status) {
			activeRootIDs = append(activeRootIDs, goal.ID)
			activeRootGoals = append(activeRootGoals, map[string]interface{}{
				"id":          goal.ID,
				"description": goal.Description,
				"status":      goal.Status,
			})
			activeRoots++
		}
		switch goal.Status {
		case StatusRunning:
			runningGoals++
		case StatusPending:
			pendingGoals++
		case StatusComplete:
			completeGoals++
		case StatusRejected:
			rejectedGoals++
		}
	}
	payload["active_roots"] = activeRoots
	payload["running_goals"] = runningGoals
	payload["pending_goals"] = pendingGoals
	payload["complete_goals"] = completeGoals
	payload["rejected_goals"] = rejectedGoals
	payload["active_root_goal_ids"] = activeRootIDs
	payload["active_root_goals"] = activeRootGoals

	_, blockers := subgoals.CanFinalize()
	if blockers == nil {
		blockers = []string{}
	}
	payload["active_branches"] = blockers
	payload["active_branch_count"] = len(blockers)
	return payload
}

func BuildGoalRuntimeContextBlock(subgoals *SubgoalManager) *RuntimeContextBlock {
	payload := buildGoalStateFacts(subgoals, false)
	goalCount, _ := payload["goal_count"].(int)
	if goalCount == 0 {
		return nil
	}

	lines := []string{
		fmt.Sprintf("GoalCount: %d", goalCount),
		fmt.Sprintf("ActiveRoots: %d", payload["active_roots"]),
		fmt.Sprintf("RunningGoals: %d", payload["running_goals"]),
		fmt.Sprintf("PendingGoals: %d", payload["pending_goals"]),
		fmt.Sprintf("CompleteGoals: %d", payload["complete_goals"]),
		fmt.Sprintf("RejectedGoals: %d", payload["rejected_goals"]),
		fmt.Sprintf("ActiveBranchCount: %d", payload["active_branch_count"]),
	}

	if roots, ok := payload["active_root_goals"].([]map[string]interface{}); ok && len(roots) > 0 {
		rootParts := make([]string, 0, len(roots))
		for _, root := range roots {
			desc, _ := root["description"].(string)
			id, _ := root["id"].(string)
			status, _ := root["status"].(SubgoalStatus)
			rootParts = append(rootParts, fmt.Sprintf("%s:%s[%s]", id, desc, status))
		}
		lines = append(lines, "ActiveRootGoals: "+strings.Join(rootParts, " | "))
	}

	if branches, ok := payload["active_branches"].([]string); ok && len(branches) > 0 {
		lines = append(lines, "ActiveBranches:")
		for _, branch := range branches {
			lines = append(lines, "- "+branch)
		}
	}

	return &RuntimeContextBlock{
		Name:    "current_goal_state",
		Role:    "developer",
		Content: strings.Join(lines, "\n"),
	}
}
