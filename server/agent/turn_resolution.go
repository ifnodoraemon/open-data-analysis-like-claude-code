package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/ifnodoraemon/openDataAnalysis/tools"
)

const (
	TurnArtifactNone   = "none"
	TurnArtifactReport = "report"
)

const (
	TurnOperationAnswerAbout     = "answer_about"
	TurnOperationRevise          = "revise"
	TurnOperationAppend          = "append"
	TurnOperationRemove          = "remove"
	TurnOperationMove            = "move"
	TurnOperationFinalize        = "finalize"
	TurnOperationReviseChart     = "revise_chart"
	TurnOperationConfigureLayout = "configure_layout"
	TurnOperationClarify         = "clarify"
	TurnOperationUnknown         = "unknown"
)

const (
	TurnScopeNone        = "none"
	TurnScopeWholeReport = "whole_report"
	TurnScopeBlock       = "block"
	TurnScopeSelection   = "selection"
	TurnScopeChart       = "chart"
	TurnScopeLayout      = "layout"
	TurnScopeUnknown     = "unknown"
)

const autoWholeReportEditConfidence = 0.80

var groundingNoisePattern = regexp.MustCompile(`[\s\p{P}\p{S}]+`)

type TurnResolution struct {
	Artifact           string   `json:"artifact"`
	Operation          string   `json:"operation"`
	Scope              string   `json:"scope"`
	TargetRefs         []string `json:"target_refs,omitempty"`
	TargetRefHint      string   `json:"target_ref_hint,omitempty"`
	MutationRequested  bool     `json:"mutation_requested"`
	NeedsClarification bool     `json:"needs_clarification"`
	ClarificationNote  string   `json:"clarification_note,omitempty"`
	Confidence         float64  `json:"confidence,omitempty"`
}

type GroundedTurnResolution struct {
	Resolution       TurnResolution
	GroundedScope    string
	TargetBlockID    string
	TargetBlockTitle string
	TargetChartID    string
	MatchKind        string
	Ambiguous        bool
	CandidateCount   int
}

func BuildTurnResolutionPrompt() string {
	return strings.TrimSpace(`You are resolving the current user turn into a structured operation contract.

Return JSON only. Do not wrap in markdown. Do not explain.

Decide whether the current user turn is asking to mutate the current report artifact or merely discuss it.
Use the runtime context facts if a current report artifact exists.

Allowed values:
- artifact: "report" | "none"
- operation: "answer_about" | "revise" | "append" | "remove" | "move" | "finalize" | "revise_chart" | "configure_layout" | "clarify" | "unknown"
- scope: "whole_report" | "block" | "selection" | "chart" | "layout" | "none" | "unknown"

Semantics:
- "answer_about" means explain, summarize, evaluate, or discuss the report without modifying it.
- "revise" means rewrite, polish, reorganize, or otherwise modify existing report content.
- "append" means add a new section or missing content to the report.
- "remove" means delete existing report content.
- "move" means reorder existing report content.
- "revise_chart" means alter an existing chart or chart section.
- "configure_layout" means change appearance or layout, not content meaning.
- "finalize" means make the report ready for delivery or complete the draft.
- "clarify" means the user intent is too ambiguous and a clarification should be requested.

Rules:
- If runtime context contains an explicit current turn report target, treat it as the report artifact identity for this turn.
- Prefer artifact="report" only when the user is clearly operating on the current report artifact.
- Prefer scope="whole_report" only when the user means the report as a whole.
- Prefer scope="block" when the user names a section or part of the report.
- Prefer scope="selection" only when the user refers to a quoted/selected excerpt.
- Prefer scope="chart" only when the user refers to a chart or visualization.
- Prefer scope="layout" only when the user refers to style, appearance, layout, or formatting.
- If the user is just asking what the report says, use operation="answer_about" and mutation_requested=false.
- Set mutation_requested=true only when the user wants the report artifact changed.
- target_refs should contain explicit user-mentioned references when present, such as a section title, chart id, or block id.
- target_ref_hint may restate a single natural-language target like "结论部分" when no stable id is available.
- If ambiguous, set needs_clarification=true and operation="clarify" or "unknown".

Return an object with fields:
artifact, operation, scope, target_refs, target_ref_hint, mutation_requested, needs_clarification, clarification_note, confidence`)
}

func normalizeTurnResolution(res TurnResolution) TurnResolution {
	res.Artifact = normalizeEnum(strings.TrimSpace(res.Artifact), TurnArtifactNone, []string{
		TurnArtifactNone,
		TurnArtifactReport,
	})
	res.Operation = normalizeEnum(strings.TrimSpace(res.Operation), TurnOperationUnknown, []string{
		TurnOperationAnswerAbout,
		TurnOperationRevise,
		TurnOperationAppend,
		TurnOperationRemove,
		TurnOperationMove,
		TurnOperationFinalize,
		TurnOperationReviseChart,
		TurnOperationConfigureLayout,
		TurnOperationClarify,
		TurnOperationUnknown,
	})
	res.Scope = normalizeEnum(strings.TrimSpace(res.Scope), TurnScopeUnknown, []string{
		TurnScopeNone,
		TurnScopeWholeReport,
		TurnScopeBlock,
		TurnScopeSelection,
		TurnScopeChart,
		TurnScopeLayout,
		TurnScopeUnknown,
	})
	if res.Confidence < 0 {
		res.Confidence = 0
	}
	if res.Confidence > 1 {
		res.Confidence = 1
	}
	res.TargetRefHint = strings.TrimSpace(res.TargetRefHint)
	res.ClarificationNote = strings.TrimSpace(res.ClarificationNote)
	filtered := make([]string, 0, len(res.TargetRefs))
	for _, ref := range res.TargetRefs {
		if trimmed := strings.TrimSpace(ref); trimmed != "" {
			filtered = append(filtered, trimmed)
		}
	}
	res.TargetRefs = filtered
	return res
}

func normalizeEnum(value, fallback string, allowed []string) string {
	for _, item := range allowed {
		if value == item {
			return value
		}
	}
	return fallback
}

func (r TurnResolution) IsMeaningful() bool {
	return r.Artifact == TurnArtifactReport || r.MutationRequested || r.NeedsClarification
}

func (r TurnResolution) RuntimeContextBlock() *RuntimeContextBlock {
	res := normalizeTurnResolution(r)
	if !res.IsMeaningful() {
		return nil
	}
	lines := []string{
		fmt.Sprintf("Artifact: %s", res.Artifact),
		fmt.Sprintf("Operation: %s", res.Operation),
		fmt.Sprintf("Scope: %s", res.Scope),
		fmt.Sprintf("MutationRequested: %t", res.MutationRequested),
		fmt.Sprintf("NeedsClarification: %t", res.NeedsClarification),
	}
	if len(res.TargetRefs) > 0 {
		lines = append(lines, "TargetRefs: "+strings.Join(res.TargetRefs, ", "))
	}
	if res.TargetRefHint != "" {
		lines = append(lines, "TargetRefHint: "+res.TargetRefHint)
	}
	if res.ClarificationNote != "" {
		lines = append(lines, "ClarificationNote: "+res.ClarificationNote)
	}
	if res.Confidence > 0 {
		lines = append(lines, fmt.Sprintf("Confidence: %.2f", res.Confidence))
	}
	return &RuntimeContextBlock{
		Name:    "current_turn_resolution",
		Role:    "developer",
		Content: strings.Join(lines, "\n"),
	}
}

func (r TurnResolution) MaterializeEditContext() *ReportEditContext {
	res := normalizeTurnResolution(r)
	if res.Artifact != TurnArtifactReport || !res.MutationRequested {
		return nil
	}
	if res.NeedsClarification {
		return nil
	}
	if res.Scope == TurnScopeWholeReport && res.Confidence >= autoWholeReportEditConfidence {
		return &ReportEditContext{
			Mode:                "revise_report",
			PreserveOtherBlocks: false,
		}
	}
	return nil
}

func normalizeGroundingKey(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.TrimSuffix(normalized, "部分")
	normalized = strings.TrimSuffix(normalized, "章节")
	normalized = strings.TrimSuffix(normalized, "章")
	normalized = strings.TrimSuffix(normalized, "节")
	normalized = strings.TrimSuffix(normalized, "section")
	normalized = strings.TrimSuffix(normalized, "part")
	normalized = groundingNoisePattern.ReplaceAllString(normalized, "")
	return strings.TrimSpace(normalized)
}

func gatherGroundingRefs(res TurnResolution) []string {
	refs := make([]string, 0, len(res.TargetRefs)+1)
	for _, ref := range res.TargetRefs {
		if trimmed := strings.TrimSpace(ref); trimmed != "" {
			refs = append(refs, trimmed)
		}
	}
	if hint := strings.TrimSpace(res.TargetRefHint); hint != "" {
		refs = append(refs, hint)
	}
	return refs
}

type groundingCandidate struct {
	id     string
	label  string
	score  int
	reason string
}

func scoreGroundingCandidate(ref, id, label string) groundingCandidate {
	trimmedRef := strings.TrimSpace(ref)
	trimmedID := strings.TrimSpace(id)
	trimmedLabel := strings.TrimSpace(label)
	if trimmedRef == "" {
		return groundingCandidate{}
	}
	if strings.EqualFold(trimmedRef, trimmedID) && trimmedID != "" {
		return groundingCandidate{id: trimmedID, label: trimmedLabel, score: 100, reason: "exact_id"}
	}
	refKey := normalizeGroundingKey(trimmedRef)
	idKey := normalizeGroundingKey(trimmedID)
	labelKey := normalizeGroundingKey(trimmedLabel)
	if refKey == "" {
		return groundingCandidate{}
	}
	if idKey != "" && refKey == idKey {
		return groundingCandidate{id: trimmedID, label: trimmedLabel, score: 95, reason: "normalized_id"}
	}
	if labelKey != "" && refKey == labelKey {
		return groundingCandidate{id: trimmedID, label: trimmedLabel, score: 90, reason: "normalized_title"}
	}
	if labelKey != "" && len(refKey) >= 2 && (strings.Contains(refKey, labelKey) || strings.Contains(labelKey, refKey)) {
		return groundingCandidate{id: trimmedID, label: trimmedLabel, score: 70, reason: "title_overlap"}
	}
	return groundingCandidate{}
}

func chooseGroundingCandidate(candidates []groundingCandidate) (groundingCandidate, bool, int) {
	if len(candidates) == 0 {
		return groundingCandidate{}, false, 0
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score == candidates[j].score {
			return candidates[i].id < candidates[j].id
		}
		return candidates[i].score > candidates[j].score
	})
	best := candidates[0]
	tied := 1
	for i := 1; i < len(candidates); i++ {
		if candidates[i].score != best.score {
			break
		}
		tied++
		if candidates[i].id != best.id {
			return best, false, tied
		}
	}
	return best, true, tied
}

func GroundTurnResolution(res TurnResolution, state *tools.ReportState) GroundedTurnResolution {
	base := normalizeTurnResolution(res)
	grounded := GroundedTurnResolution{
		Resolution:    base,
		GroundedScope: base.Scope,
	}
	if state == nil || base.Artifact != TurnArtifactReport || !base.MutationRequested || base.NeedsClarification {
		return grounded
	}
	if base.Scope == TurnScopeWholeReport || base.Scope == TurnScopeLayout {
		return grounded
	}

	refs := gatherGroundingRefs(base)
	if len(refs) == 0 {
		return grounded
	}

	state.RLock()
	defer state.RUnlock()

	switch base.Scope {
	case TurnScopeBlock:
		candidates := make([]groundingCandidate, 0)
		for _, ref := range refs {
			for _, block := range state.Blocks {
				candidate := scoreGroundingCandidate(ref, block.ID, block.Title)
				if candidate.score > 0 {
					candidates = append(candidates, candidate)
				}
			}
		}
		best, ok, candidateCount := chooseGroundingCandidate(candidates)
		grounded.CandidateCount = candidateCount
		if ok {
			grounded.TargetBlockID = best.id
			grounded.TargetBlockTitle = best.label
			grounded.MatchKind = best.reason
		} else if candidateCount > 0 {
			grounded.Ambiguous = true
		}
	case TurnScopeChart:
		chartCandidates := make([]groundingCandidate, 0)
		for _, ref := range refs {
			for _, chart := range state.Charts {
				candidate := scoreGroundingCandidate(ref, chart.ID, "")
				if candidate.score > 0 {
					chartCandidates = append(chartCandidates, candidate)
				}
			}
			for _, block := range state.Blocks {
				if strings.TrimSpace(block.ChartID) == "" {
					continue
				}
				candidate := scoreGroundingCandidate(ref, block.ChartID, block.Title)
				if candidate.score > 0 {
					chartCandidates = append(chartCandidates, candidate)
				}
			}
		}
		best, ok, candidateCount := chooseGroundingCandidate(chartCandidates)
		grounded.CandidateCount = candidateCount
		if ok {
			grounded.TargetChartID = best.id
			grounded.MatchKind = best.reason
			for _, block := range state.Blocks {
				if strings.TrimSpace(block.ChartID) == best.id {
					grounded.TargetBlockID = strings.TrimSpace(block.ID)
					grounded.TargetBlockTitle = strings.TrimSpace(block.Title)
					break
				}
			}
		} else if candidateCount > 0 {
			grounded.Ambiguous = true
		}
	}
	return grounded
}

func (g GroundedTurnResolution) RuntimeContextBlock() *RuntimeContextBlock {
	if strings.TrimSpace(g.TargetBlockID) == "" && strings.TrimSpace(g.TargetChartID) == "" && !g.Ambiguous {
		return nil
	}
	lines := []string{
		fmt.Sprintf("GroundedScope: %s", strings.TrimSpace(g.GroundedScope)),
	}
	if g.TargetBlockID != "" {
		lines = append(lines, "TargetBlockID: "+g.TargetBlockID)
	}
	if g.TargetBlockTitle != "" {
		lines = append(lines, "TargetBlockTitle: "+g.TargetBlockTitle)
	}
	if g.TargetChartID != "" {
		lines = append(lines, "TargetChartID: "+g.TargetChartID)
	}
	if g.MatchKind != "" {
		lines = append(lines, "MatchKind: "+g.MatchKind)
	}
	if g.CandidateCount > 0 {
		lines = append(lines, fmt.Sprintf("CandidateCount: %d", g.CandidateCount))
	}
	if g.Ambiguous {
		lines = append(lines, "Ambiguous: true")
	}
	return &RuntimeContextBlock{
		Name:    "current_turn_grounding",
		Role:    "developer",
		Content: strings.Join(lines, "\n"),
	}
}

func (g GroundedTurnResolution) MaterializeEditContext(turnCtx *TurnContext) *ReportEditContext {
	base := g.Resolution
	if base.Artifact != TurnArtifactReport || !base.MutationRequested || base.NeedsClarification {
		return nil
	}
	switch g.GroundedScope {
	case TurnScopeBlock:
		if g.TargetBlockID == "" || g.Ambiguous {
			return nil
		}
		edit := &ReportEditContext{
			Mode:                "regenerate_block",
			BlockID:             g.TargetBlockID,
			BlockLabel:          g.TargetBlockTitle,
			PreserveOtherBlocks: true,
		}
		if turnCtx != nil {
			edit.TargetRunID = strings.TrimSpace(turnCtx.ReportTargetRunID)
		}
		return edit
	case TurnScopeChart:
		if g.TargetBlockID == "" || g.Ambiguous {
			return nil
		}
		edit := &ReportEditContext{
			Mode:                "regenerate_block",
			BlockID:             g.TargetBlockID,
			BlockLabel:          g.TargetBlockTitle,
			PreserveOtherBlocks: true,
		}
		if turnCtx != nil {
			edit.TargetRunID = strings.TrimSpace(turnCtx.ReportTargetRunID)
		}
		return edit
	default:
		return base.MaterializeEditContext()
	}
}

func parseTurnResolution(raw string) (TurnResolution, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return TurnResolution{}, fmt.Errorf("empty turn resolution")
	}
	var parsed TurnResolution
	candidates := []string{trimmed}
	if codeFence := extractCodeFenceJSON(trimmed); codeFence != "" {
		candidates = append([]string{codeFence}, candidates...)
	}
	if obj := extractJSONObject(trimmed); obj != "" && obj != trimmed {
		candidates = append([]string{obj}, candidates...)
	}
	for _, candidate := range candidates {
		if err := json.Unmarshal([]byte(candidate), &parsed); err == nil {
			return normalizeTurnResolution(parsed), nil
		}
	}
	return TurnResolution{}, fmt.Errorf("failed to parse turn resolution JSON")
}

func extractCodeFenceJSON(input string) string {
	start := strings.Index(input, "```")
	if start < 0 {
		return ""
	}
	rest := input[start+3:]
	if strings.HasPrefix(strings.ToLower(rest), "json") {
		rest = rest[4:]
	}
	end := strings.Index(rest, "```")
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(rest[:end])
}

func extractJSONObject(input string) string {
	start := strings.Index(input, "{")
	end := strings.LastIndex(input, "}")
	if start < 0 || end <= start {
		return ""
	}
	return strings.TrimSpace(input[start : end+1])
}

func (e *Engine) ResolveTurn(ctx context.Context, userInput string, runtimeCtx []RuntimeContextBlock) (TurnResolution, error) {
	trimmed := strings.TrimSpace(userInput)
	if trimmed == "" {
		return TurnResolution{}, nil
	}
	bundle := &PromptBundle{
		Policy:         BuildTurnResolutionPrompt(),
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
	resolver := e.turnResolver
	e.mu.Unlock()

	if resolver != nil {
		return resolver(ctx, bundle)
	}

	resp, err := e.llm.ChatWithTools(ctx, bundle, nil)
	if err != nil {
		return TurnResolution{}, err
	}
	if len(resp.Choices) == 0 {
		return TurnResolution{}, fmt.Errorf("LLM returned empty turn resolution")
	}
	return parseTurnResolution(resp.Choices[0].Message.Content)
}
