package agent

import (
	"strings"
	"testing"
)

func TestWorkingMemory(t *testing.T) {
	mem := NewWorkingMemory()

	// Test GetSummary empty
	summary := mem.GetSummary()
	if !strings.Contains(summary, "<empty>") {
		t.Errorf("expected <empty> summary, got %s", summary)
	}

	// Test SaveFact
	mem.SaveFact("key1", "fact1")
	mem.SaveFact("key2", "fact2")

	summary = mem.GetSummary()
	if !strings.Contains(summary, "key1") || !strings.Contains(summary, "fact2") {
		t.Errorf("expected summary to contain facts, got %s", summary)
	}

	// Test RemoveFact
	mem.RemoveFact("key1")
	summary = mem.GetSummary()
	if strings.Contains(summary, "key1") {
		t.Errorf("expected summary to not contain key1, got %s", summary)
	}

	mem.Reset()
	if len(mem.Snapshot()) != 0 {
		t.Errorf("expected snapshot to be empty after reset")
	}
}

func TestSubgoalManager(t *testing.T) {
	sm := NewSubgoalManager()

	canFinalize, blockers := sm.CanFinalize()
	if !canFinalize || len(blockers) != 0 {
		t.Errorf("expected empty subgoals to allow finalize")
	}

	// Test AddGoal
	id1 := sm.AddGoal("goal 1", "")
	id2 := sm.AddGoal("goal 2", id1)

	canFinalize, blockers = sm.CanFinalize()
	if canFinalize || len(blockers) != 1 {
		t.Errorf("expected pending root goal to block finalize, blockers=%v", blockers)
	}

	// Test UpdateGoalStatus
	err := sm.UpdateGoalStatus(id1, StatusComplete, "done")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	canFinalize, blockers = sm.CanFinalize()
	if !canFinalize || len(blockers) != 0 {
		t.Errorf("expected completed root to allow finalize, blockers=%v", blockers)
	}

	err = sm.UpdateGoalStatus(id2, StatusRejected, "rejected")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	canFinalize, blockers = sm.CanFinalize()
	if !canFinalize || len(blockers) != 0 {
		t.Errorf("expected finalize to be allowed as root/branch are terminal, blockers=%v", blockers)
	}

	summary := sm.GetSummary()
	if !strings.Contains(summary, "goal 1") || !strings.Contains(summary, "goal 2") || !strings.Contains(summary, "rejected") {
		t.Errorf("expected summary to contain goals and results, got %s", summary)
	}
	if sm.Goals[1].ParentGoalID != id1 {
		t.Errorf("expected nested goal parent id to be preserved")
	}
}

func TestSubgoalManagerAllowsFinalizeWhenClosedRootHasStaleChildren(t *testing.T) {
	sm := NewSubgoalManager()
	rootID := sm.AddGoal("root goal", "")
	childID := sm.AddGoal("stale child", rootID)

	if err := sm.UpdateGoalStatus(rootID, StatusComplete, "done"); err != nil {
		t.Fatalf("update root status: %v", err)
	}

	canFinalize, blockers := sm.CanFinalize()
	if !canFinalize {
		t.Fatalf("expected completed root to allow finalize even with stale child, blockers=%v", blockers)
	}

	summary := sm.GetSummary()
	if !strings.Contains(summary, "stale child") {
		t.Fatalf("expected summary to still show stale child")
	}
	if strings.Contains(summary, "stale child[pending]") {
		t.Fatalf("expected stale child not to appear as active branch blocker")
	}

	if err := sm.UpdateGoalStatus(childID, StatusRejected, "obsolete"); err != nil {
		t.Fatalf("update child status: %v", err)
	}
}

func TestAutoCompleteReportGoalsOnlyClosesReportRoots(t *testing.T) {
	sm := NewSubgoalManager()
	reportGoalID := sm.AddGoal("生成图表并整理成完整研究报告", "")
	otherGoalID := sm.AddGoal("继续核对退款异常原因", "")

	completed := sm.AutoCompleteReportGoals("报告已完成")
	if completed != 1 {
		t.Fatalf("expected 1 report goal to be auto-completed, got %d", completed)
	}

	var reportStatus, otherStatus SubgoalStatus
	for _, goal := range sm.Goals {
		switch goal.ID {
		case reportGoalID:
			reportStatus = goal.Status
		case otherGoalID:
			otherStatus = goal.Status
		}
	}
	if reportStatus != StatusComplete {
		t.Fatalf("expected report goal complete, got %s", reportStatus)
	}
	if otherStatus != StatusPending {
		t.Fatalf("expected non-report goal to stay pending, got %s", otherStatus)
	}
}

func TestSaveMemoryTool(t *testing.T) {
	mem := NewWorkingMemory()
	tool := &SaveMemoryTool{Memory: mem}

	args := []byte(`{"key": "test_key", "fact": "test_fact"}`)
	res, err := tool.Execute(args)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !strings.Contains(res, "test_key") {
		t.Errorf("expected result to contain test_key, got %s", res)
	}
	if mem.Facts["test_key"] != "test_fact" {
		t.Errorf("expected fact to be saved in memory")
	}
}

func TestSaveMemoryToolEmitsUpdate(t *testing.T) {
	mem := NewWorkingMemory()
	var emitted bool
	tool := &SaveMemoryTool{
		Memory: mem,
		EmitFunc: func(event WSEvent) {
			if event.Type != EventStateMemoryUpdated {
				t.Fatalf("unexpected event type: %s", event.Type)
			}
			payload, ok := event.Data.(MemoryUpdatedData)
			if !ok {
				t.Fatalf("unexpected payload type: %T", event.Data)
			}
			if payload.Facts["alpha"] != "beta" {
				t.Fatalf("expected emitted facts to contain saved value")
			}
			emitted = true
		},
	}

	if _, err := tool.Execute([]byte(`{"key":"alpha","fact":"beta"}`)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !emitted {
		t.Fatalf("expected memory update event to be emitted")
	}
}

func TestManageSubgoalsTool(t *testing.T) {
	sm := NewSubgoalManager()
	tool := &ManageSubgoalsTool{Subgoals: sm}

	// Add
	args := []byte(`{"action": "add", "description": "new goal"}`)
	res, err := tool.Execute(args)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(sm.Goals) != 1 {
		t.Errorf("expected 1 goal, got %d", len(sm.Goals))
	}

	id := sm.Goals[0].ID

	argsChild := []byte(`{"action": "add", "description": "child goal", "parent_goal_id": "` + id + `"}`)
	if _, err := tool.Execute(argsChild); err != nil {
		t.Errorf("unexpected error adding child goal: %v", err)
	}
	if len(sm.Goals) != 2 || sm.Goals[1].ParentGoalID != id {
		t.Errorf("expected child goal to be nested under parent")
	}

	// Complete
	argsComplete := []byte(`{"action": "complete", "goal_id": "` + id + `", "result": "done"}`)
	res, err = tool.Execute(argsComplete)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if sm.Goals[0].Status != StatusComplete {
		t.Errorf("expected status complete, got %s", sm.Goals[0].Status)
	}
	if !strings.Contains(res, "结论: done") {
		t.Errorf("expected result string in response")
	}

	// Unknown Action
	argsUnknown := []byte(`{"action": "foo"}`)
	_, err = tool.Execute(argsUnknown)
	if err == nil {
		t.Errorf("expected error for unknown action")
	}
}
