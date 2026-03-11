package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/tools"
	openai "github.com/sashabaranov/go-openai"
)

const MaxIterations = 20

// Engine Agent 主循环引擎
type Engine struct {
	llm      *LLMClient
	registry *tools.Registry
	emitter  func(event WSEvent)
	messages []openai.ChatCompletionMessage
}

// NewEngine 创建 Agent 引擎
func NewEngine(registry *tools.Registry, emitter func(event WSEvent)) *Engine {
	return &Engine{
		llm:      NewLLMClient(),
		registry: registry,
		emitter:  emitter,
		messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: SystemPrompt,
			},
		},
	}
}

// Run 执行 Agent 主循环
func (e *Engine) Run(ctx context.Context, userInput string) {
	// 添加用户消息
	e.messages = append(e.messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: userInput,
	})

	oaiTools := e.registry.GetOpenAITools()

	for i := 0; i < MaxIterations; i++ {
		select {
		case <-ctx.Done():
			e.emitter(WSEvent{Type: EventError, Data: ErrorData{Message: "任务被取消"}})
			return
		default:
		}

		// 通知前端: 正在思考
		e.emitter(WSEvent{Type: EventThinking, Data: ThinkingData{Content: "正在分析..."}})

		// 调用 LLM
		resp, err := e.llm.ChatWithTools(ctx, e.messages, oaiTools)
		if err != nil {
			e.emitter(WSEvent{Type: EventError, Data: ErrorData{Message: err.Error()}})
			return
		}

		if len(resp.Choices) == 0 {
			e.emitter(WSEvent{Type: EventError, Data: ErrorData{Message: "LLM 返回空响应"}})
			return
		}

		choice := resp.Choices[0]

		// 如果有文本回复 (非工具调用)
		if choice.Message.Content != "" && len(choice.Message.ToolCalls) == 0 {
			e.emitter(WSEvent{Type: EventComplete, Data: CompleteData{Summary: choice.Message.Content}})
			return
		}

		// 如果 finish_reason 是 stop 且没有工具调用，结束
		if choice.FinishReason == openai.FinishReasonStop && len(choice.Message.ToolCalls) == 0 {
			e.emitter(WSEvent{Type: EventComplete, Data: CompleteData{Summary: choice.Message.Content}})
			return
		}

		// 处理工具调用
		if len(choice.Message.ToolCalls) > 0 {
			// 将 assistant 消息加入历史
			e.messages = append(e.messages, choice.Message)

			for _, toolCall := range choice.Message.ToolCalls {
				// 通知前端: 工具调用
				e.emitter(WSEvent{
					Type: EventToolCall,
					Data: ToolCallData{
						ID:        toolCall.ID,
						Name:      toolCall.Function.Name,
						Arguments: json.RawMessage(toolCall.Function.Arguments),
					},
				})

				// 执行工具
				start := time.Now()
				result, execErr := e.registry.Execute(toolCall.Function.Name, json.RawMessage(toolCall.Function.Arguments))
				duration := time.Since(start).Milliseconds()

				success := true
				if execErr != nil {
					result = fmt.Sprintf("工具执行错误: %s", execErr.Error())
					success = false
					log.Printf("Tool %s error: %v", toolCall.Function.Name, execErr)
				}

				// 通知前端: 工具结果
				e.emitter(WSEvent{
					Type: EventToolResult,
					Data: ToolResultData{
						ID:       toolCall.ID,
						Name:     toolCall.Function.Name,
						Result:   result,
						Duration: duration,
						Success:  success,
					},
				})

				// 将工具结果加入消息历史
				e.messages = append(e.messages, openai.ChatCompletionMessage{
					Role:       openai.ChatMessageRoleTool,
					Content:    result,
					ToolCallID: toolCall.ID,
				})
			}

			continue // 继续循环
		}

		// 默认结束
		e.emitter(WSEvent{Type: EventComplete, Data: CompleteData{Summary: "分析完成"}})
		return
	}

	e.emitter(WSEvent{Type: EventError, Data: ErrorData{Message: "达到最大迭代次数"}})
}
