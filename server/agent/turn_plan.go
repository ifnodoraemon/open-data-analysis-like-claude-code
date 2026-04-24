package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type TurnPlan struct {
	Report TurnResolution `json:"report"`
	Goals  GoalResolution `json:"goals"`
}

func BuildTurnPlanPrompt() string {
	return strings.TrimSpace(`You are resolving the current user turn into a structured turn plan.

Return JSON only. Do not wrap in markdown. Do not explain.

You must resolve two things at once:
1. Whether this turn is operating on the current report artifact and how.
2. Whether this turn should keep or reconcile the current goal tree.

Use runtime context facts such as current_report_artifact, current_turn_target, and current_goal_state when present.

Return an object with exactly two top-level fields:
- report
- goals

report object:
- artifact: "report" | "none"
- operation: "answer_about" | "revise" | "append" | "remove" | "move" | "finalize" | "revise_chart" | "configure_layout" | "clarify" | "unknown"
- scope: "whole_report" | "block" | "selection" | "chart" | "layout" | "none" | "unknown"
- target_refs: array of explicit references such as block ids, titles, chart ids
- target_ref_hint: optional natural-language target such as "结论部分"
- mutation_requested: boolean
- needs_clarification: boolean
- clarification_note: string
- confidence: number in [0,1]

report rules:
- Use artifact="report" only when the user is clearly operating on the current report artifact.
- Use answer_about when the user is only asking to explain, summarize, or discuss the report.
- Set mutation_requested=true only when the user wants the report changed.
- Prefer whole_report only when the whole report is the target.
- Prefer block, selection, chart, or layout when the user is clearly targeting that narrower scope.
- If runtime context contains an active partial_selection edit scope and the user gives a follow-up refinement without naming a new target, prefer scope="selection" so the current anchored selection can be reused.

goals object:
- action: "keep" | "reconcile" | "clarify" | "unknown"
- complete_goal_ids: array
- reject_goal_ids: array
- add_goals: array of concise new active goal descriptions
- needs_clarification: boolean
- clarification_note: string
- confidence: number in [0,1]

goal rules:
- Use keep when the current goal tree still matches this turn.
- Use reconcile when the user turn changes scope, supersedes prior work, narrows the task, or clearly makes some existing goals done or obsolete.
- Prefer reconciling root goals over micromanaging child goals unless a specific child goal is directly referenced.
- Do not complete or reject goals unless the user turn makes that change clear.
- If there is no current goal state, prefer keep with empty mutations.

If either side is ambiguous, mark that side with needs_clarification=true and explain briefly in clarification_note.`)
}

func normalizeTurnPlan(plan TurnPlan) TurnPlan {
	plan.Report = normalizeTurnResolution(plan.Report)
	plan.Goals = normalizeGoalResolution(plan.Goals)
	return plan
}

func parseTurnPlan(raw string) (TurnPlan, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return TurnPlan{}, fmt.Errorf("empty turn plan")
	}
	var parsed TurnPlan
	candidates := []string{trimmed}
	if codeFence := extractCodeFenceJSON(trimmed); codeFence != "" {
		candidates = append([]string{codeFence}, candidates...)
	}
	if obj := extractJSONObject(trimmed); obj != "" && obj != trimmed {
		candidates = append([]string{obj}, candidates...)
	}
	for _, candidate := range candidates {
		if err := json.Unmarshal([]byte(candidate), &parsed); err == nil {
			return normalizeTurnPlan(parsed), nil
		}
	}
	return TurnPlan{}, fmt.Errorf("failed to parse turn plan JSON")
}

func (e *Engine) ResolveTurnPlan(ctx context.Context, userInput string, runtimeCtx []RuntimeContextBlock) (TurnPlan, error) {
	trimmed := strings.TrimSpace(userInput)
	if trimmed == "" {
		return TurnPlan{}, nil
	}
	bundle := &PromptBundle{
		Policy:         BuildTurnPlanPrompt(),
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
	planResolver := e.turnPlanResolver
	turnResolver := e.turnResolver
	goalResolver := e.goalResolver
	e.mu.Unlock()

	if planResolver != nil {
		return planResolver(ctx, bundle)
	}
	if turnResolver != nil || goalResolver != nil {
		var plan TurnPlan
		var err error
		if turnResolver != nil {
			plan.Report, err = turnResolver(ctx, bundle)
			if err != nil {
				return TurnPlan{}, err
			}
		}
		if goalResolver != nil {
			plan.Goals, err = goalResolver(ctx, bundle)
			if err != nil {
				return TurnPlan{}, err
			}
		}
		return normalizeTurnPlan(plan), nil
	}

	resp, err := e.llm.ChatWithTools(ctx, bundle, nil)
	if err != nil {
		return TurnPlan{}, err
	}
	if len(resp.Choices) == 0 {
		return TurnPlan{}, fmt.Errorf("LLM returned empty turn plan")
	}
	return parseTurnPlan(resp.Choices[0].Message.Content)
}
