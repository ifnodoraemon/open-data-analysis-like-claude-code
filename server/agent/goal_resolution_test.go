package agent

import "testing"

func TestParseGoalResolutionFromCodeFence(t *testing.T) {
	t.Parallel()

	raw := "```json\n{\"action\":\"reconcile\",\"reject_goal_ids\":[\"goal_old\"],\"add_goals\":[\"整理结论结构\"],\"needs_clarification\":false,\"confidence\":0.88}\n```"
	res, err := parseGoalResolution(raw)
	if err != nil {
		t.Fatalf("parse goal resolution: %v", err)
	}
	if res.Action != GoalResolutionReconcile {
		t.Fatalf("unexpected action: %#v", res)
	}
	if len(res.RejectGoalIDs) != 1 || res.RejectGoalIDs[0] != "goal_old" {
		t.Fatalf("unexpected reject ids: %#v", res)
	}
	if len(res.AddGoals) != 1 || res.AddGoals[0] != "整理结论结构" {
		t.Fatalf("unexpected add goals: %#v", res)
	}
}

func TestApplyGoalResolution(t *testing.T) {
	t.Parallel()

	subgoals := NewSubgoalManager()
	oldID, err := subgoals.AddGoal("旧目标", "")
	if err != nil {
		t.Fatalf("add goal: %v", err)
	}

	changed := ApplyGoalResolution(subgoals, GoalResolution{
		Action:        GoalResolutionReconcile,
		RejectGoalIDs: []string{oldID},
		AddGoals:      []string{"新目标"},
		Confidence:    0.91,
	})
	if !changed {
		t.Fatal("expected goal resolution to mutate goal tree")
	}

	goals := subgoals.ListAll()
	if len(goals) != 2 {
		t.Fatalf("expected 2 goals after reconciliation, got %#v", goals)
	}
	if goals[0].Status != StatusRejected {
		t.Fatalf("expected old goal rejected, got %#v", goals[0])
	}
	if goals[1].Description != "新目标" || goals[1].Status != StatusPending {
		t.Fatalf("expected new pending goal, got %#v", goals[1])
	}
}

func TestApplyGoalResolutionSkipsDuplicateActiveGoal(t *testing.T) {
	t.Parallel()

	subgoals := NewSubgoalManager()
	if _, err := subgoals.AddGoal("新目标", ""); err != nil {
		t.Fatalf("add goal: %v", err)
	}

	changed := ApplyGoalResolution(subgoals, GoalResolution{
		Action:     GoalResolutionReconcile,
		AddGoals:   []string{"新目标"},
		Confidence: 0.91,
	})
	if changed {
		t.Fatal("did not expect duplicate active goal to be added")
	}
	if len(subgoals.ListAll()) != 1 {
		t.Fatalf("expected duplicate goal to be skipped, got %#v", subgoals.ListAll())
	}
}
