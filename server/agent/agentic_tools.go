package agent

import (
	"encoding/json"
	"fmt"
	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/tools"
)

func init() {
	tools.RegisterGlobalTool(func(ctx tools.ToolContext) tools.Tool {
		if ctx.Memory == nil {
			return nil
		}
		return &SaveMemoryTool{Memory: ctx.Memory.(*WorkingMemory)}
	})
	tools.RegisterGlobalTool(func(ctx tools.ToolContext) tools.Tool {
		return &AskUserTool{}
	})
}

// SaveMemoryTool 允许 Agent 主动将关键结论写入 Working Memory，以便脱离上下文窗口长期保存
type SaveMemoryTool struct {
	Memory *WorkingMemory
}

func (t *SaveMemoryTool) Name() string {
	return "save_to_memory"
}

func (t *SaveMemoryTool) Description() string {
	return `将一个重要的发现、确认的业务口径、或者排除了的假设记录到工作记忆 (Working Memory) 中。
一旦写入，它将被长久保留并在每次你思考前作为前置上下文注入，直到它被明确移除或覆盖。
使用场景：
1. 你花了很多步骤查清了某个重要维度（比如"大客户"的定义是 order_value > 10000）。
2. 你和用户确认了某个表关联的具体字段并取得成功。
3. 把这些确凿的“经验”写入记忆，你就可以放心地让之前的冗长查询结果从当前对话中淘汰，而不会忘记关键结论。`
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
	return fmt.Sprintf("已成功将工作记忆保存: [%s] = %s", payload.Key, payload.Fact), nil
}

// AskUserTool 允许 Agent 主动中断当前执行流，向用户发起提问或索要确切指令
type AskUserTool struct{}

func (t *AskUserTool) Name() string {
	return "ask_user"
}

func (t *AskUserTool) Description() string {
	return `当你遇到歧义边界（例如：业务口径定义有多种可能、缺乏必须的关键指引）时，调用此工具向人类最终确认。
调用此工具后，你的分析执行将被立即冻结并推送到前端，直到用户回复。
如果你只是想汇报结论，请不要使用此工具，只有在你遭遇 blocker 无法前进时才使用。`
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
