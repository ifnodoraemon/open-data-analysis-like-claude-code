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
	return "保存一个事实到工作记忆中。相同 key 会覆盖旧值。"
}

func (t *SaveMemoryTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"key": {
				"type": "string",
				"description": "记忆的简短标识键，例如 'roi_definition', 'user_table_pk'。如果使用相同的 key 再次写入，将会覆盖原来的值。"
			},
			"fact": {
				"type": "string",
				"description": "你需要记住的具体事实或结论。请尽可能详细、准确且结论性地描述。"
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

	t.Memory.SaveFact(payload.Key, payload.Fact)
	if t.EmitFunc != nil {
		t.EmitFunc(WSEvent{
			Type: EventStateMemoryUpdated,
			Data: MemoryUpdatedData{Facts: t.Memory.Snapshot()},
		})
	}
	return fmt.Sprintf("已成功将工作记忆保存: [%s] = %s", payload.Key, payload.Fact), nil
}

func (t *SaveMemoryTool) SetEventEmitter(emit func(WSEvent)) {
	t.EmitFunc = emit
}

func (t *InspectMemoryTool) Name() string {
	return "state_memory_inspect"
}

func (t *InspectMemoryTool) Description() string {
	return "返回工作记忆中的原始事实，包括 key/value 和数量。"
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
		"ok":           true,
		"tool":         "state_memory_inspect",
		"fact_count":   len(facts),
		"facts":        facts,
		"summary_text": fmt.Sprintf("当前工作记忆共有 %d 条事实。", len(facts)),
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
	return "向用户请求补充信息或确认。调用后当前执行会暂停。"
}

func (t *AskUserTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"question": {
				"type": "string",
				"description": "具体的问题。例如：'请问本次分析的统计范围是否包含已退款订单？'"
			},
			"options": {
				"type": "array",
				"items": {"type": "string"},
				"description": "可选方案。如果问题是单选性质，可提供候选菜单供用户快速点击，例如 ['包含已退款', '只计算成功交易']。"
			}
		},
		"required": ["question"]
	}`)
}

func (t *AskUserTool) Execute(args json.RawMessage) (string, error) {
	// Execute 本身是一个站位。真正的中断与挂起逻辑将在 Engine.Run 的 interceptor 里处理
	return "User requested to answer question, pending suspension...", nil
}
