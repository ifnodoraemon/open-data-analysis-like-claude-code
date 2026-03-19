package agent

import "strings"

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
		payload["can_finalize"] = true
		payload["active_branches"] = []string{}
		payload["active_branch_count"] = 0
		return payload
	}

	goals := subgoals.ListAll()
	payload["goal_count"] = len(goals)
	if includeGoals {
		payload["goals"] = goals
	}

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
			payload["active_roots"] = payload["active_roots"].(int) + 1
		}
		switch goal.Status {
		case StatusRunning:
			payload["running_goals"] = payload["running_goals"].(int) + 1
		case StatusPending:
			payload["pending_goals"] = payload["pending_goals"].(int) + 1
		case StatusComplete:
			payload["complete_goals"] = payload["complete_goals"].(int) + 1
		case StatusRejected:
			payload["rejected_goals"] = payload["rejected_goals"].(int) + 1
		}
	}
	payload["active_root_goal_ids"] = activeRootIDs
	payload["active_root_goals"] = activeRootGoals

	canFinalize, blockers := subgoals.CanFinalize()
	if blockers == nil {
		blockers = []string{}
	}
	payload["can_finalize"] = canFinalize
	payload["active_branches"] = blockers
	payload["active_branch_count"] = len(blockers)
	return payload
}
