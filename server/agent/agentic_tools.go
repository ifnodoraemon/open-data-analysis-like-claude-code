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
	return "写入一条工作记忆事实。相同 key 会覆盖旧值；此工具会修改 working memory 状态，但不会直接修改报告、目标树或数据，也不会改变 report delivery_state。"
}

func (t *SaveMemoryTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"key": {
				"type": "string",
				"description": "记忆的标识键，例如 'roi_definition', 'user_table_pk'。重复写入同一 key 会覆盖旧值。"
			},
			"fact": {
				"type": "string",
				"description": "要写入的事实或结论。"
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
		"message":                 "工作记忆已更新。",
		"ui_summary":              fmt.Sprintf("已写入工作记忆 [%s]。", payload.Key),
	})
}

func (t *SaveMemoryTool) SetEventEmitter(emit func(WSEvent)) {
	t.EmitFunc = emit
}

func (t *InspectMemoryTool) Name() string {
	return "state_memory_inspect"
}

func (t *InspectMemoryTool) Description() string {
	return "读取工作记忆中的事实状态。返回 key/value 和数量；不修改任何状态。"
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
		"ui_summary": fmt.Sprintf("当前工作记忆共有 %d 条事实。", len(facts)),
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
	return "向用户发起一次输入请求，并将当前 run 挂起为 waiting_user_input。读取参数 `question` 与可选的 `options`；不会直接返回用户答案，后续用户回复会作为该工具调用结果写回对话。"
}

func (t *AskUserTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"question": {
				"type": "string",
				"description": "问题文本。"
			},
			"options": {
				"type": "array",
				"items": {"type": "string"},
				"description": "可选项列表。"
			}
		},
		"required": ["question"]
	}`)
}

func (t *AskUserTool) Execute(args json.RawMessage) (string, error) {
	var payload struct {
		Question string   `json:"question"`
		Options  []string `json:"options"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return "", fmt.Errorf("invalid arguments: %v", err)
	}
	return marshalToolPayload(map[string]interface{}{
		"ok":         true,
		"tool":       "user_request_input",
		"question":   payload.Question,
		"options":    payload.Options,
		"run_status": "waiting_user_input",
		"message":    "已发起用户输入请求，等待用户回复。",
		"ui_summary": "已向用户发起输入请求。",
	})
}
