package agent

import "testing"

func TestParseTurnPlanFromCodeFence(t *testing.T) {
	t.Parallel()

	raw := "```json\n{\"report\":{\"artifact\":\"report\",\"operation\":\"revise\",\"scope\":\"whole_report\",\"mutation_requested\":true,\"needs_clarification\":false,\"confidence\":0.91},\"goals\":{\"action\":\"reconcile\",\"reject_goal_ids\":[\"goal_old\"],\"add_goals\":[\"整理当前报告结构\"],\"needs_clarification\":false,\"confidence\":0.86}}\n```"
	plan, err := parseTurnPlan(raw)
	if err != nil {
		t.Fatalf("parse turn plan: %v", err)
	}
	if plan.Report.Artifact != TurnArtifactReport || plan.Report.Scope != TurnScopeWholeReport {
		t.Fatalf("unexpected report plan: %#v", plan.Report)
	}
	if plan.Goals.Action != GoalResolutionReconcile || len(plan.Goals.AddGoals) != 1 {
		t.Fatalf("unexpected goal plan: %#v", plan.Goals)
	}
}
