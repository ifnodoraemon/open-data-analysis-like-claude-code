package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ifnodoraemon/openDataAnalysis/tools"
)

func init() {
	tools.RegisterGlobalTool(func(ctx tools.ToolContext) tools.Tool {
		if ctx.Memory == nil {
			return nil
		}
		memory, ok := ctx.Memory.(*WorkingMemory)
		if !ok {
			return nil
		}
		return &SaveMemoryTool{
			Memory: memory,
		}
	})
	tools.RegisterGlobalTool(func(ctx tools.ToolContext) tools.Tool {
		if ctx.Memory == nil {
			return nil
		}
		memory, ok := ctx.Memory.(*WorkingMemory)
		if !ok {
			return nil
		}
		return &InspectMemoryTool{Memory: memory}
	})
	tools.RegisterGlobalTool(func(ctx tools.ToolContext) tools.Tool {
		return &AskUserTool{}
	})
}

// SaveMemoryTool 允许 Agent 主动将关键结论写入 Working Memory，以便脱离上下文窗口长期保存
type SaveMemoryTool struct {
	Memory   *WorkingMemory
	EmitFunc func(WSEvent)
}

type InspectMemoryTool struct {
	Memory *WorkingMemory
}

func (t *SaveMemoryTool) Name() string {
	return "memory_save_fact"
}

func (t *SaveMemoryTool) Description() string {
	return "Write a working memory fact. Same key overwrites old value; this tool modifies working memory state but does not directly modify reports, goal tree, or data, and does not change report delivery_state."
}

func (t *SaveMemoryTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"key": {
				"type": "string",
				"description": "Memory key, e.g. 'roi_definition', 'user_table_pk'. Writing the same key overwrites the old value."
			},
			"fact": {
				"type": "string",
				"description": "Fact or conclusion to write."
			}
		},
		"required": ["key", "fact"]
	}`)
}

func (t *SaveMemoryTool) Execute(args json.RawMessage) (string, error) {
	if t.Memory == nil {
		return "", fmt.Errorf("working memory is not initialized")
	}

	var payload struct {
		Key  string `json:"key"`
		Fact string `json:"fact"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return "", fmt.Errorf("invalid arguments: %v", err)
	}

	if payload.Key == "" || payload.Fact == "" {
		return "", fmt.Errorf("both 'key' and 'fact' are required")
	}

	_, existed := t.Memory.Snapshot()[payload.Key]
	saved := t.Memory.SaveFact(payload.Key, payload.Fact)
	if !saved {
		return marshalToolPayload(map[string]interface{}{
			"ok":         false,
			"tool":       "memory_save_fact",
			"memory_key": payload.Key,
			"error":      fmt.Sprintf("working memory full (%d facts max)", maxFacts),
			"fact_count": len(t.Memory.Snapshot()),
			"ui_summary": fmt.Sprintf("Working memory full, cannot save [%s].", payload.Key),
		})
	}
	facts := t.Memory.Snapshot()
	if t.EmitFunc != nil {
		t.EmitFunc(WSEvent{
			Type: EventStateMemoryUpdated,
			Data: MemoryUpdatedData{Facts: facts},
		})
	}
	return marshalToolPayload(map[string]interface{}{
		"ok":                      true,
		"tool":                    "memory_save_fact",
		"memory_key":              payload.Key,
		"fact":                    payload.Fact,
		"fact_count":              len(facts),
		"overwrote_existing":      existed,
		"affects_report_delivery": false,
		"affects_goal_state":      false,
		"ui_summary":              fmt.Sprintf("Working memory written [%s].", payload.Key),
	})
}

func (t *SaveMemoryTool) SetEventEmitter(emit func(WSEvent)) {
	t.EmitFunc = emit
}

func (t *InspectMemoryTool) Name() string {
	return "state_memory_inspect"
}

func (t *InspectMemoryTool) Description() string {
	return "Read the fact state in working memory. Returns key/value pairs and count; does not modify any state."
}

func (t *InspectMemoryTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{},"required":[]}`)
}

func (t *InspectMemoryTool) Execute(args json.RawMessage) (string, error) {
	if t.Memory == nil {
		return "", fmt.Errorf("working memory is not initialized")
	}
	facts := t.Memory.Snapshot()
	payload := map[string]interface{}{
		"ok":         true,
		"tool":       "state_memory_inspect",
		"fact_count": len(facts),
		"facts":      facts,
		"ui_summary": fmt.Sprintf("Working memory has %d facts.", len(facts)),
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// AskUserTool 允许 Agent 主动中断当前执行流，向用户发起提问或索要确切指令
type AskUserTool struct{}

func (t *AskUserTool) Name() string {
	return "user_request_input"
}

func (t *AskUserTool) Description() string {
	return "Send an input request to the user and suspend the current run as waiting_user_input. Applies when the current run needs a user decision or clarification before continuing; a normal assistant text response that asks a question is final output and does not suspend the run. Supports optional selectable options, explicit selection_mode (single or multiple), and optional custom text. The model decides selection_mode; the runtime does not infer it from wording. Reads question, reason, scope, context_ref, input_hint, required, selection_mode, allow_custom, and options. Does not directly return the user answer; the subsequent user reply is written back as the tool call result."
}

func (t *AskUserTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"question": {
				"type": "string",
				"description": "Question text."
			},
			"reason": {
				"type": "string",
				"description": "Confirmation reason: why user confirmation is needed, e.g. 'multiple possible join keys'."
			},
			"scope": {
				"type": "string",
				"enum": ["join_key", "metric", "time_grain", "filter", "general"],
				"description": "Confirmation scope type."
			},
			"context_ref": {
				"type": "string",
				"description": "Associated context, e.g. table name, column name, metric name."
			},
			"input_hint": {
				"type": "string",
				"description": "Optional concise hint for the user's custom answer, e.g. 'Type the join key to use, or explain another rule'."
			},
			"required": {
				"type": "boolean",
				"description": "Whether confirmation is required; if true, user cannot skip.",
				"default": false
			},
			"selection_mode": {
				"type": "string",
				"enum": ["single", "multiple"],
				"description": "Whether options should be presented as a single-choice or multiple-choice control. Choose explicitly based on the user's decision surface.",
				"default": "single"
			},
			"allow_custom": {
				"type": "boolean",
				"description": "Whether the user may provide a custom text answer instead of, or in addition to, selecting options.",
				"default": true
			},
			"options": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {
						"id": {"type": "string", "description": "Stable option ID."},
						"label": {"type": "string", "description": "Display label."},
						"hint": {"type": "string", "description": "Optional short explanation."}
					},
					"required": ["id", "label"]
				},
				"description": "Optional selectable options. If omitted, the UI presents a custom text answer only."
			}
		},
		"required": ["question"]
	}`)
}

type askUserToolCallArguments struct {
	Question      string          `json:"question"`
	Reason        string          `json:"reason"`
	Scope         string          `json:"scope"`
	ContextRef    string          `json:"context_ref"`
	InputHint     string          `json:"input_hint"`
	Required      bool            `json:"required"`
	SelectionMode string          `json:"selection_mode"`
	AllowCustom   *bool           `json:"allow_custom"`
	Options       []AskUserOption `json:"options"`
}

func parseAskUserToolCallArguments(rawArgs string) (AskUserData, error) {
	var args askUserToolCallArguments
	if err := json.Unmarshal([]byte(rawArgs), &args); err != nil {
		return AskUserData{}, fmt.Errorf("user_request_input parameter parse failed: %w", err)
	}
	question := strings.TrimSpace(args.Question)
	if question == "" {
		return AskUserData{}, fmt.Errorf("user_request_input question is required")
	}
	allowCustom := true
	if args.AllowCustom != nil {
		allowCustom = *args.AllowCustom
	}
	options, err := validateAskUserOptions(args.Options)
	if err != nil {
		return AskUserData{}, err
	}
	if len(options) == 0 && !allowCustom {
		return AskUserData{}, fmt.Errorf("user_request_input requires options when allow_custom is false")
	}
	selectionMode, err := normalizeAskUserSelectionMode(args.SelectionMode)
	if err != nil {
		return AskUserData{}, err
	}
	return AskUserData{
		Question:      question,
		Reason:        strings.TrimSpace(args.Reason),
		Scope:         strings.TrimSpace(args.Scope),
		ContextRef:    strings.TrimSpace(args.ContextRef),
		InputHint:     strings.TrimSpace(args.InputHint),
		Required:      args.Required,
		SelectionMode: selectionMode,
		AllowCustom:   allowCustom,
		Options:       options,
	}, nil
}

func normalizeAskUserSelectionMode(selectionMode string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(selectionMode)) {
	case "":
		return "single", nil
	case "single":
		return "single", nil
	case "multiple":
		return "multiple", nil
	default:
		return "", fmt.Errorf("user_request_input selection_mode must be single or multiple")
	}
}

func (t *AskUserTool) Execute(args json.RawMessage) (string, error) {
	payload, err := parseAskUserToolCallArguments(string(args))
	if err != nil {
		return "", err
	}

	return marshalToolPayload(map[string]interface{}{
		"ok":             true,
		"tool":           "user_request_input",
		"question":       payload.Question,
		"reason":         payload.Reason,
		"scope":          payload.Scope,
		"context_ref":    payload.ContextRef,
		"input_hint":     payload.InputHint,
		"required":       payload.Required,
		"selection_mode": payload.SelectionMode,
		"allow_custom":   payload.AllowCustom,
		"options":        payload.Options,
		"run_status":     "waiting_user_input",
		"ui_summary":     "User input request sent.",
	})
}

func validateAskUserOptions(options []AskUserOption) ([]AskUserOption, error) {
	out := make([]AskUserOption, 0, len(options))
	seen := make(map[string]struct{}, len(options))
	for index, option := range options {
		id := strings.TrimSpace(option.ID)
		label := strings.TrimSpace(option.Label)
		if id == "" || label == "" {
			return nil, fmt.Errorf("user_request_input options[%d] requires non-empty id and label", index)
		}
		if _, exists := seen[id]; exists {
			return nil, fmt.Errorf("user_request_input option id %q is duplicated", id)
		}
		seen[id] = struct{}{}
		out = append(out, AskUserOption{
			ID:    id,
			Label: label,
			Hint:  strings.TrimSpace(option.Hint),
		})
	}
	return out, nil
}
