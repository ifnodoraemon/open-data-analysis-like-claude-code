package agent

import (
	"encoding/json"
	"fmt"

	"github.com/ifnodoraemon/openDataAnalysis/tools"
)

func init() {
	tools.RegisterGlobalTool(func(ctx tools.ToolContext) tools.Tool {
		if ctx.Memory == nil {
			return nil
		}
		return &SaveMemoryTool{
			Memory: ctx.Memory.(*WorkingMemory),
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
	t.Memory.SaveFact(payload.Key, payload.Fact)
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
		"message":                 "Working memory updated.",
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
	return "Send an input request to the user and suspend the current run as waiting_user_input. Reads the `question` parameter and optional `options`; does not directly return the user answer. The subsequent user reply will be written back as the tool call result."
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
			"required": {
				"type": "boolean",
				"description": "Whether confirmation is required; if true, user cannot skip.",
				"default": false
			},
			"allow_multiple": {
				"type": "boolean",
				"description": "Whether multiple selections are allowed; if true, returns option IDs as a JSON array.",
				"default": false
			},
			"options": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {
						"id":    {"type": "string", "description": "Stable option ID"},
						"label": {"type": "string", "description": "Option display text"},
						"hint":  {"type": "string", "description": "Option description"}
					},
					"required": ["id", "label"]
				},
				"description": "List of selectable options, each with a stable ID."
			}
		},
		"required": ["question"]
	}`)
}

func (t *AskUserTool) Execute(args json.RawMessage) (string, error) {
	var payload struct {
		Question      string          `json:"question"`
		Reason        string          `json:"reason"`
		Scope         string          `json:"scope"`
		ContextRef    string          `json:"context_ref"`
		Required      bool            `json:"required"`
		AllowMultiple bool            `json:"allow_multiple"`
		Options       []AskUserOption `json:"options"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return "", fmt.Errorf("invalid arguments: %v", err)
	}

	return marshalToolPayload(map[string]interface{}{
		"ok":                  true,
		"tool":                "user_request_input",
		"question":            payload.Question,
		"reason":              payload.Reason,
		"scope":               payload.Scope,
		"context_ref":         payload.ContextRef,
		"required":            payload.Required,
		"allow_multiple":      payload.AllowMultiple,
		"options":             payload.Options,
		"run_status":          "waiting_user_input",
		"message":             "User input request sent, waiting for user reply.",
		"ui_summary":          "User input request sent.",
	})
}
