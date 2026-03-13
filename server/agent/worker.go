package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/tools"
	openai "github.com/sashabaranov/go-openai"
)

func init() {
	tools.RegisterGlobalTool(func(ctx tools.ToolContext) tools.Tool {
		if ctx.Subgoals == nil {
			return nil
		}
		return &ManageSubgoalsTool{Subgoals: ctx.Subgoals.(*SubgoalManager)}
	})
}

type ManageSubgoalsTool struct {
	Subgoals *SubgoalManager
}

func (t *ManageSubgoalsTool) Name() string {
	return "manage_subgoals"
}

func (t *ManageSubgoalsTool) Description() string {
	return `管理你当前正在解决的目标拆解清单。
这是你获取具体数据和图表的唯一途径。当你需要执行 SQL 查数、或者画图表时，请使用 action="add" 下发一个明确的子任务，系统会自动派遣专业的 DataWorker 去执行并带回确凿的数据结论。
当你获得足够证据后，使用 action="complete" 结束任务。如果无法推进，使用 action="reject"。`
}

func (t *ManageSubgoalsTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {
				"type": "string",
				"enum": ["add", "complete", "reject"],
				"description": "要执行的操作。add（下发数据探查子任务以获取图表和SQL查询结论）, complete（标记已拿到充分证据结论）, reject（任务无法完成放弃）."
			},
			"description": {
				"type": "string",
				"description": "仅 action=add 时需要。清晰描述你希望 DataWorker 去执行的具体查数任务。例如：'查询最新的销售额按周分布数据并绘制图表'。"
			},
			"goal_id": {
				"type": "string",
				"description": "仅 action=complete 或 reject 时需要。你要变更状态的目标 ID。"
			},
			"result": {
				"type": "string",
				"description": "仅 action=complete 或 reject 时需要。给目标追加的最终结论或者放弃原因。"
			}
		},
		"required": ["action"]
	}`)
}

func (t *ManageSubgoalsTool) Execute(args json.RawMessage) (string, error) {
	if t.Subgoals == nil {
		return "", fmt.Errorf("subgoal manager is not initialized")
	}

	var payload struct {
		Action      string `json:"action"`
		Description string `json:"description"`
		GoalID      string `json:"goal_id"`
		Result      string `json:"result"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return "", fmt.Errorf("invalid arguments: %v", err)
	}

	switch payload.Action {
	case "add":
		if payload.Description == "" {
			return "", fmt.Errorf("description is required for add action")
		}
		id := t.Subgoals.AddGoal(payload.Description)
		return fmt.Sprintf("已成功创建新的查数子任务，ID: %s。系统即将自动派发由 DataWorker 执行该任务，拿到 SQL/图表 结论后会马上返回...", id), nil
	case "complete":
		if payload.GoalID == "" {
			return "", fmt.Errorf("goal_id is required for complete action")
		}
		if err := t.Subgoals.UpdateGoalStatus(payload.GoalID, StatusComplete, payload.Result); err != nil {
			return "", err
		}
		return fmt.Sprintf("已将目标[%s]标记为完成。附加结论: %s", payload.GoalID, payload.Result), nil

	case "reject":
		if payload.GoalID == "" {
			return "", fmt.Errorf("goal_id is required for reject action")
		}
		if err := t.Subgoals.UpdateGoalStatus(payload.GoalID, StatusRejected, payload.Result); err != nil {
			return "", err
		}
		return fmt.Sprintf("已将目标[%s]标记为放弃。原因: %s", payload.GoalID, payload.Result), nil

	default:
		return "", fmt.Errorf("unknown action: %s", payload.Action)
	}
}

type DataWorkerAgent struct {
	llm      *LLMClient
	registry *tools.Registry
}

func NewDataWorkerAgent(registry *tools.Registry) *DataWorkerAgent {
	return &DataWorkerAgent{
		llm:      NewLLMClient(),
		registry: registry,
	}
}

func (w *DataWorkerAgent) RunWorker(ctx context.Context, task string, emit func(WSEvent)) (string, error) {
	if emit == nil {
		emit = func(WSEvent) {}
	}
	emit(WSEvent{Type: EventThinking, Data: ThinkingData{Content: fmt.Sprintf("[DataWorker 启动] 接受目标: %s", task)}})

	// Registry exposes HasTool
	pythonEnabled := w.registry.HasTool("run_python")

	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: BuildWorkerPrompt(pythonEnabled),
		},
		{
			Role:    openai.ChatMessageRoleUser,
			Content: fmt.Sprintf("请解决以下数据查证探寻任务，并在确凿得出证据后，以纯文本作为你的 final reply 提交给我:\n\n%s", task),
		},
	}

	oaiTools := w.registry.GetOpenAITools()

	for i := 0; i < 15; i++ {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}

		resp, err := w.llm.ChatWithTools(ctx, messages, oaiTools)
		if err != nil {
			return "", fmt.Errorf("worker LLM failed: %v", err)
		}
		if len(resp.Choices) == 0 {
			return "", fmt.Errorf("worker LLM returned no response")
		}

		choice := resp.Choices[0]

		if choice.FinishReason == openai.FinishReasonStop && len(choice.Message.ToolCalls) == 0 {
			emit(WSEvent{Type: EventThinking, Data: ThinkingData{Content: "[DataWorker 结论] " + choice.Message.Content}})
			return choice.Message.Content, nil
		}
		
		if len(choice.Message.ToolCalls) > 0 {
			messages = append(messages, compactAssistantMessage(choice.Message))
			for _, toolCall := range choice.Message.ToolCalls {
				emit(WSEvent{
					Type: EventToolCall,
					Data: ToolCallData{
						ID:        toolCall.ID,
						Name:      toolCall.Function.Name,
						Arguments: json.RawMessage(toolCall.Function.Arguments),
					},
				})

				start := time.Now()
				result, execErr := w.registry.Execute(toolCall.Function.Name, json.RawMessage(toolCall.Function.Arguments))
				duration := time.Since(start).Milliseconds()

				if ctx.Err() != nil {
					return "", ctx.Err()
				}

				if execErr != nil {
					emit(WSEvent{Type: EventToolResult, Data: ToolResultData{
						ID:      toolCall.ID,
						Result:  execErr.Error(),
						Success: false,
						Duration: duration,
					}})
					messages = append(messages, openai.ChatCompletionMessage{
						Role:       openai.ChatMessageRoleTool,
						Content:    fmt.Sprintf("Tool Failed: %v", execErr),
						ToolCallID: toolCall.ID,
					})
				} else {
					emit(WSEvent{Type: EventToolResult, Data: ToolResultData{
						ID:      toolCall.ID,
						Result:  result,
						Success: true,
						Duration: duration,
					}})
					messages = append(messages, openai.ChatCompletionMessage{
						Role:       openai.ChatMessageRoleTool,
						Content:    result,
						ToolCallID: toolCall.ID,
					})
				}
			}
		}
	}
	return "", fmt.Errorf("worker max iterations reached")
}
