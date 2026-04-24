package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	GoalResolutionKeep      = "keep"
	GoalResolutionReconcile = "reconcile"
	GoalResolutionClarify   = "clarify"
	GoalResolutionUnknown   = "unknown"
)

const autoGoalResolutionConfidence = 0.70

type GoalResolution struct {
	Action             string   `json:"action"`
	CompleteGoalIDs    []string `json:"complete_goal_ids,omitempty"`
	RejectGoalIDs      []string `json:"reject_goal_ids,omitempty"`
	AddGoals           []string `json:"add_goals,omitempty"`
	NeedsClarification bool     `json:"needs_clarification"`
	ClarificationNote  string   `json:"clarification_note,omitempty"`
	Confidence         float64  `json:"confidence,omitempty"`
}

func BuildGoalResolutionPrompt() string {
	return strings.TrimSpace(`You are reconciling the current user turn against the current goal state.

Return JSON only. Do not wrap in markdown. Do not explain.

Use runtime context facts, especially current_goal_state, to decide whether the current user turn continues the same work or changes what the active goals should be.

Allowed values:
- action: "keep" | "reconcile" | "clarify" | "unknown"

Semantics:
- "keep": the current goal tree still fits this turn; do not change it.
- "reconcile": the user turn changes scope, supersedes old work, narrows the task, or marks some goals done; provide goal mutations.
- "clarify": the turn is too ambiguous to safely reconcile goals.

Rules:
- Only complete or reject existing goals when the user turn clearly makes them done, obsolete, or out of scope.
- Prefer reconciling root goals over micromanaging child goals unless a specific child goal is directly referenced.
- add_goals should contain concise descriptions of newly active goals that now better reflect the user turn.
- If this turn is simply continuing the same task, use action="keep".
- If unsure, use action="clarify" and explain what is ambiguous.

Return an object with fields:
action, complete_goal_ids, reject_goal_ids, add_goals, needs_clarification, clarification_note, confidence`)
}

func normalizeGoalResolution(res GoalResolution) GoalResolution {
	res.Action = normalizeEnum(strings.TrimSpace(res.Action), GoalResolutionUnknown, []string{
		GoalResolutionKeep,
		GoalResolutionReconcile,
		GoalResolutionClarify,
		GoalResolutionUnknown,
	})
	if res.Confidence < 0 {
		res.Confidence = 0
	}
	if res.Confidence > 1 {
		res.Confidence = 1
	}
	res.ClarificationNote = strings.TrimSpace(res.ClarificationNote)
	res.CompleteGoalIDs = normalizeGoalResolutionStrings(res.CompleteGoalIDs)
	res.RejectGoalIDs = normalizeGoalResolutionStrings(res.RejectGoalIDs)
	res.AddGoals = normalizeGoalResolutionStrings(res.AddGoals)
	return res
}

func normalizeGoalResolutionStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func (r GoalResolution) IsMeaningful() bool {
	res := normalizeGoalResolution(r)
	return res.Action != GoalResolutionUnknown || res.NeedsClarification || len(res.CompleteGoalIDs) > 0 || len(res.RejectGoalIDs) > 0 || len(res.AddGoals) > 0
}

func (r GoalResolution) ShouldApply() bool {
	res := normalizeGoalResolution(r)
	if res.NeedsClarification || res.Confidence < autoGoalResolutionConfidence {
		return false
	}
	return res.Action == GoalResolutionReconcile
}

func (r GoalResolution) RuntimeContextBlock() *RuntimeContextBlock {
	res := normalizeGoalResolution(r)
	if !res.IsMeaningful() {
		return nil
	}
	lines := []string{
		fmt.Sprintf("Action: %s", res.Action),
		fmt.Sprintf("NeedsClarification: %t", res.NeedsClarification),
	}
	if len(res.CompleteGoalIDs) > 0 {
		lines = append(lines, "CompleteGoalIDs: "+strings.Join(res.CompleteGoalIDs, ", "))
	}
	if len(res.RejectGoalIDs) > 0 {
		lines = append(lines, "RejectGoalIDs: "+strings.Join(res.RejectGoalIDs, ", "))
	}
	if len(res.AddGoals) > 0 {
		lines = append(lines, "AddGoals: "+strings.Join(res.AddGoals, " | "))
	}
	if res.ClarificationNote != "" {
		lines = append(lines, "ClarificationNote: "+res.ClarificationNote)
	}
	if res.Confidence > 0 {
		lines = append(lines, fmt.Sprintf("Confidence: %.2f", res.Confidence))
	}
	return &RuntimeContextBlock{
		Name:    "current_goal_resolution",
		Role:    "developer",
		Content: strings.Join(lines, "\n"),
	}
}

func parseGoalResolution(raw string) (GoalResolution, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return GoalResolution{}, fmt.Errorf("empty goal resolution")
	}
	var parsed GoalResolution
	candidates := []string{trimmed}
	if codeFence := extractCodeFenceJSON(trimmed); codeFence != "" {
		candidates = append([]string{codeFence}, candidates...)
	}
	if obj := extractJSONObject(trimmed); obj != "" && obj != trimmed {
		candidates = append([]string{obj}, candidates...)
	}
	for _, candidate := range candidates {
		if err := json.Unmarshal([]byte(candidate), &parsed); err == nil {
			return normalizeGoalResolution(parsed), nil
		}
	}
	return GoalResolution{}, fmt.Errorf("failed to parse goal resolution JSON")
}

func (e *Engine) ResolveGoals(ctx context.Context, userInput string, runtimeCtx []RuntimeContextBlock) (GoalResolution, error) {
	trimmed := strings.TrimSpace(userInput)
	if trimmed == "" {
		return GoalResolution{}, nil
	}
	bundle := &PromptBundle{
		Policy:         BuildGoalResolutionPrompt(),
		Task:           trimmed,
		RuntimeContext: append([]RuntimeContextBlock(nil), runtimeCtx...),
	}
	e.mu.Lock()
	if len(e.history) > 0 {
		hist := e.history
		if len(hist) > 6 {
			hist = hist[len(hist)-6:]
		}
		bundle.History = append([]ConversationItem(nil), hist...)
	}
	resolver := e.goalResolver
	e.mu.Unlock()

	if resolver != nil {
		return resolver(ctx, bundle)
	}

	resp, err := e.llm.ChatWithTools(ctx, bundle, nil)
	if err != nil {
		return GoalResolution{}, err
	}
	if len(resp.Choices) == 0 {
		return GoalResolution{}, fmt.Errorf("LLM returned empty goal resolution")
	}
	return parseGoalResolution(resp.Choices[0].Message.Content)
}

func ApplyGoalResolution(subgoals *SubgoalManager, resolution GoalResolution) bool {
	if subgoals == nil {
		return false
	}
	res := normalizeGoalResolution(resolution)
	if !res.ShouldApply() {
		return false
	}

	changed := false
	for _, goalID := range res.CompleteGoalIDs {
		if err := subgoals.UpdateGoalStatus(goalID, StatusComplete, "completed by current user turn"); err == nil {
			changed = true
		}
	}
	for _, goalID := range res.RejectGoalIDs {
		if err := subgoals.UpdateGoalStatus(goalID, StatusRejected, "superseded by current user turn"); err == nil {
			changed = true
		}
	}
	existingActive := make(map[string]struct{})
	for _, goal := range subgoals.ListAll() {
		if isTerminalSubgoalStatus(goal.Status) {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(goal.Description))
		if key != "" {
			existingActive[key] = struct{}{}
		}
	}
	for _, desc := range res.AddGoals {
		key := strings.ToLower(strings.TrimSpace(desc))
		if _, exists := existingActive[key]; exists {
			continue
		}
		if _, err := subgoals.AddGoal(desc, ""); err == nil {
			changed = true
			if key != "" {
				existingActive[key] = struct{}{}
			}
		}
	}
	return changed
}
