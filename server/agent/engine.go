package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/tools"
	openai "github.com/sashabaranov/go-openai"
)

const MaxIterations = 25

// Engine Agent 主循环引擎
type Engine struct {
	llm          *LLMClient
	registry     *tools.Registry
	emitter      func(event WSEvent)
	messages     []openai.ChatCompletionMessage
	systemPrompt string
	mu           sync.Mutex
}

// NewEngine 创建 Agent 引擎（支持多轮对话）
func NewEngine(registry *tools.Registry, systemPrompt string, emitter func(event WSEvent)) *Engine {
	if emitter == nil {
		emitter = func(WSEvent) {}
	}
	if systemPrompt == "" {
		systemPrompt = BuildSystemPrompt(true)
	}
	return &Engine{
		llm:          NewLLMClient(),
		registry:     registry,
		emitter:      emitter,
		systemPrompt: systemPrompt,
		messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: systemPrompt,
			},
		},
	}
}

func (e *Engine) SetEmitter(emitter func(event WSEvent)) {
	if emitter == nil {
		emitter = func(WSEvent) {}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.emitter = emitter
}

func (e *Engine) ResetMessages() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.messages = []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: e.systemPrompt,
		},
	}
}

func (e *Engine) emit(event WSEvent) {
	e.mu.Lock()
	emitter := e.emitter
	e.mu.Unlock()
	if emitter != nil {
		emitter(event)
	}
}

// Run 执行 Agent 主循环
func (e *Engine) Run(ctx context.Context, userInput string) {
	e.mu.Lock()
	// 添加用户消息
	e.messages = append(e.messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: userInput,
	})

	oaiTools := e.registry.GetOpenAITools()
	e.mu.Unlock()

	for i := 0; i < MaxIterations; i++ {
		select {
		case <-ctx.Done():
			e.emit(WSEvent{Type: EventRunCancelled, Data: ErrorData{Message: "任务被取消"}})
			return
		default:
		}

		// 通知前端: 正在思考
		e.emit(WSEvent{Type: EventThinking, Data: ThinkingData{Content: fmt.Sprintf("正在分析... (第 %d 轮)", i+1)}})

		// 调用 LLM
		e.mu.Lock()
		messages := append([]openai.ChatCompletionMessage(nil), e.messages...)
		e.mu.Unlock()
		resp, err := e.llm.ChatWithTools(ctx, messages, oaiTools)
		if err != nil {
			e.emit(WSEvent{Type: EventError, Data: ErrorData{Message: err.Error()}})
			return
		}

		if len(resp.Choices) == 0 {
			e.emit(WSEvent{Type: EventError, Data: ErrorData{Message: "LLM 返回空响应"}})
			return
		}

		choice := resp.Choices[0]

		// 有文本内容时，推送 LLM 的实际思考（而不是固定文字）
		if choice.Message.Content != "" {
			if len(choice.Message.ToolCalls) > 0 {
				// 有文本 + 有工具调用 → 推送思考内容
				e.emit(WSEvent{Type: EventThinking, Data: ThinkingData{Content: choice.Message.Content}})
			} else {
				// 有文本 + 无工具调用 → 最终回复
				e.mu.Lock()
				e.messages = append(e.messages, choice.Message)
				e.mu.Unlock()
				e.emit(WSEvent{Type: EventRunCompleted, Data: CompleteData{Summary: choice.Message.Content}})
				return
			}
		}

		// 如果 finish_reason 是 stop 且没有工具调用，结束
		if choice.FinishReason == openai.FinishReasonStop && len(choice.Message.ToolCalls) == 0 {
			e.mu.Lock()
			e.messages = append(e.messages, choice.Message)
			e.mu.Unlock()
			e.emit(WSEvent{Type: EventRunCompleted, Data: CompleteData{Summary: choice.Message.Content}})
			return
		}

		// 处理工具调用
		if len(choice.Message.ToolCalls) > 0 {
			// 将 assistant 消息加入历史
			e.mu.Lock()
			e.messages = append(e.messages, choice.Message)
			e.mu.Unlock()

			for _, toolCall := range choice.Message.ToolCalls {
				// 通知前端: 工具调用
				e.emit(WSEvent{
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
				e.emit(WSEvent{
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
				e.mu.Lock()
				e.messages = append(e.messages, openai.ChatCompletionMessage{
					Role:       openai.ChatMessageRoleTool,
					Content:    result,
					ToolCallID: toolCall.ID,
				})
				e.mu.Unlock()
			}

			continue // 继续循环
		}

		// 默认结束
		e.emit(WSEvent{Type: EventRunCompleted, Data: CompleteData{Summary: "分析完成"}})
		return
	}

	e.emit(WSEvent{Type: EventError, Data: ErrorData{Message: "达到最大迭代次数"}})
}
