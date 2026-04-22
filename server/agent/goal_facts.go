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

	canFinalize, blockers := subgoals.CanFinalize()
	if blockers == nil {
		blockers = []string{}
	}
	payload["can_finalize"] = canFinalize
	payload["active_branches"] = blockers
	payload["active_branch_count"] = len(blockers)
	return payload
}
