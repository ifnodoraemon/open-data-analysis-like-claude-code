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
}

func TestSubgoalManager(t *testing.T) {
	sm := NewSubgoalManager()

	// Test IsAllCompleted empty
	if !sm.IsAllCompleted() {
		t.Errorf("expected empty subgoals to be true")
	}

	// Test AddGoal
	id1 := sm.AddGoal("goal 1")
	id2 := sm.AddGoal("goal 2")

	if sm.IsAllCompleted() {
		t.Errorf("expected IsAllCompleted to be false when pending goals exist")
	}

	// Test UpdateGoalStatus
	err := sm.UpdateGoalStatus(id1, StatusComplete, "done")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if sm.IsAllCompleted() {
		t.Errorf("expected IsAllCompleted to be false, goal 2 is still pending")
	}

	err = sm.UpdateGoalStatus(id2, StatusRejected, "rejected")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !sm.IsAllCompleted() {
		t.Errorf("expected IsAllCompleted to be true as all goals are complete/rejected")
	}

	summary := sm.GetSummary()
	if !strings.Contains(summary, "goal 1") || !strings.Contains(summary, "goal 2") || !strings.Contains(summary, "rejected") {
		t.Errorf("expected summary to contain goals and results, got %s", summary)
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
