package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/tools"
	openai "github.com/sashabaranov/go-openai"
)

// Engine Agent 主循环引擎
type Engine struct {
	llm          *LLMClient
	registry     *tools.Registry
	messages     []openai.ChatCompletionMessage
	systemPrompt string
	memory       *WorkingMemory
	subgoals     *SubgoalManager
	mu           sync.Mutex
}

type eventEmitterAware interface {
	SetEventEmitter(func(WSEvent))
}

type executionContextAware interface {
	SetExecutionContext(context.Context)
}

type specialToolHandler func(context.Context, openai.ToolCall, func(WSEvent)) (string, error, bool)

// NewEngine 创建 Agent 引擎（支持多轮对话）
func NewEngine(registry *tools.Registry, systemPrompt string, memory *WorkingMemory, subgoals *SubgoalManager) *Engine {
	if systemPrompt == "" {
		systemPrompt = BuildPlannerPrompt(registry)
	}
	return &Engine{
		llm:          NewLLMClient(),
		registry:     registry,
		systemPrompt: systemPrompt,
		memory:       memory,
		subgoals:     subgoals,
		messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: systemPrompt,
			},
		},
	}
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

func (e *Engine) prepareRuntimeTools(ctx context.Context, emit func(WSEvent)) {
	if e.registry == nil {
		return
	}
	for _, tool := range e.registry.ListTools() {
		if next, ok := tool.(eventEmitterAware); ok {
			next.SetEventEmitter(emit)
		}
		if next, ok := tool.(executionContextAware); ok {
			next.SetExecutionContext(ctx)
		}
	}
}

func (e *Engine) specialToolHandlers() map[string]specialToolHandler {
	return map[string]specialToolHandler{
		"user_request_input": func(ctx context.Context, toolCall openai.ToolCall, emit func(WSEvent)) (string, error, bool) {
			var payload AskUserData
			if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &payload); err != nil {
				return "", fmt.Errorf("user_request_input 参数解析失败: %w", err), true
			}
			emit(WSEvent{Type: EventUserRequestInput, Data: payload})
			return "", nil, true
		},
		"report_finalize": func(ctx context.Context, toolCall openai.ToolCall, emit func(WSEvent)) (string, error, bool) {
			emit(WSEvent{Type: EventThinking, Data: ThinkingData{Content: "开始生成最终报告..."}})
			result, err := e.registry.Execute(toolCall.Function.Name, json.RawMessage(toolCall.Function.Arguments))
			return result, err, false
		},
	}
}

// Run 执行 Agent 主循环
func (e *Engine) Run(ctx context.Context, userInput string, emit func(WSEvent)) {
	if emit == nil {
		emit = func(WSEvent) {}
	}
	e.prepareRuntimeTools(ctx, emit)
	specialHandlers := e.specialToolHandlers()

	e.mu.Lock()
	// 添加用户消息
	e.messages = append(e.messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: userInput,
	})

	oaiTools := e.registry.GetOpenAITools()
	e.mu.Unlock()

	for i := 1; ; i++ {
		select {
		case <-ctx.Done():
			emit(WSEvent{Type: EventRunCancelled, Data: ErrorData{Message: "任务被取消"}})
			return
		default:
		}

		// 通知前端: 正在思考
		emit(WSEvent{Type: EventThinking, Data: ThinkingData{Content: fmt.Sprintf("正在分析... (第 %d 轮)", i)}})

		// 调用 LLM
		e.mu.Lock()
		messages := append([]openai.ChatCompletionMessage(nil), e.messages...)
		e.mu.Unlock()

		// 动态注入 Memory & Subgoal 上下文作为最新的 System Prompt
		var stateContext string
		if e.memory != nil {
			stateContext += e.memory.GetSummary() + "\n\n"
		}
		if e.subgoals != nil {
			stateContext += e.subgoals.GetSummary()
		}
		if stateContext != "" {
			messages = append(messages, openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleSystem,
				Content: strings.TrimSpace(stateContext),
			})
		}

		resp, err := e.llm.ChatWithTools(ctx, messages, oaiTools)
		if err != nil {
			emit(WSEvent{Type: EventError, Data: ErrorData{Message: err.Error()}})
			return
		}

		if len(resp.Choices) == 0 {
			emit(WSEvent{Type: EventError, Data: ErrorData{Message: "LLM 返回空响应"}})
			return
		}

		choice := resp.Choices[0]

		// 有文本内容时，推送 LLM 的实际思考（而不是固定文字）
		if choice.Message.Content != "" {
			if len(choice.Message.ToolCalls) > 0 {
				// 有文本 + 有工具调用 → 推送思考内容
				emit(WSEvent{Type: EventThinking, Data: ThinkingData{Content: choice.Message.Content}})
			} else {
				// 有文本 + 无工具调用 → 最终回复
				e.mu.Lock()
				e.messages = append(e.messages, choice.Message)
				e.mu.Unlock()
				emit(WSEvent{Type: EventRunCompleted, Data: CompleteData{Summary: choice.Message.Content}})
				return
			}
		}

		// 如果 finish_reason 是 stop 且没有工具调用，结束
		if choice.FinishReason == openai.FinishReasonStop && len(choice.Message.ToolCalls) == 0 {
			e.mu.Lock()
			e.messages = append(e.messages, choice.Message)
			e.mu.Unlock()
			emit(WSEvent{Type: EventRunCompleted, Data: CompleteData{Summary: choice.Message.Content}})
			return
		}

		// 处理工具调用
		if len(choice.Message.ToolCalls) > 0 {
			// 将 assistant 消息加入历史
			e.mu.Lock()
			e.messages = append(e.messages, compactAssistantMessage(choice.Message))
			e.mu.Unlock()

			for _, toolCall := range choice.Message.ToolCalls {
				toolSpan := llmDebugWriter.StartSpan(
					TraceMetadataFromContext(ctx),
					"tool",
					toolCall.Function.Name,
					"",
					toolCall.ID,
				)

				// 通知前端: 工具调用
				emit(WSEvent{
					Type: EventToolCall,
					Data: ToolCallData{
						ID:        toolCall.ID,
						Name:      toolCall.Function.Name,
						Arguments: json.RawMessage(toolCall.Function.Arguments),
					},
				})
				argPath := llmDebugWriter.WriteBlob(toolSpan, "arguments.json", []byte(toolCall.Function.Arguments))
				llmDebugWriter.WriteEvent(toolSpan, "tool.call", map[string]interface{}{
					"tool_name":        toolCall.Function.Name,
					"tool_call_id":     toolCall.ID,
					"arguments_path":   argPath,
					"arguments_bytes":  len([]byte(toolCall.Function.Arguments)),
					"arguments_sha256": blobSHA256([]byte(toolCall.Function.Arguments)),
				})

				// 执行工具
				start := time.Now()

				var result string
				var execErr error

				if handler, ok := specialHandlers[toolCall.Function.Name]; ok {
					var stop bool
					result, execErr, stop = handler(ctx, toolCall, emit)
					if execErr != nil && toolCall.Function.Name == "user_request_input" {
						emit(WSEvent{Type: EventError, Data: ErrorData{Message: execErr.Error()}})
						return
					}
					if stop {
						return
					}
				} else {
					result, execErr = e.registry.Execute(toolCall.Function.Name, json.RawMessage(toolCall.Function.Arguments))
				}

				duration := time.Since(start).Milliseconds()

				// If we got canceled during execution (or context ended), drop the result, abort tool loop, allow ctx.Done to catch in next loop
				if ctx.Err() != nil {
					return
				}

				success := toolCallSucceeded(result, execErr)
				if execErr != nil {
					result = fmt.Sprintf("工具执行错误: %s", execErr.Error())
					log.Printf("Tool %s error: %v", toolCall.Function.Name, execErr)
				}
				resultBytes := []byte(result)
				resultPath := llmDebugWriter.WriteBlob(toolSpan, "result.txt", resultBytes)
				llmDebugWriter.WriteEvent(toolSpan, "tool.result", map[string]interface{}{
					"tool_name":       toolCall.Function.Name,
					"tool_call_id":    toolCall.ID,
					"duration_ms":     duration,
					"success":         success,
					"result_preview":  clipText(result, 300),
					"result_bytes":    len(resultBytes),
					"result_sha256":   blobSHA256(resultBytes),
					"result_path":     resultPath,
					"execution_error": errorString(execErr),
				})

				// 通知前端: 工具结果
				emit(WSEvent{
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
					Content:    compactToolResult(toolCall.Function.Name, result),
					ToolCallID: toolCall.ID,
				})
				e.mu.Unlock()
			}

			continue // 继续循环
		}

		// 默认结束
		emit(WSEvent{Type: EventRunCompleted, Data: CompleteData{Summary: "分析完成"}})
		return
	}

}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func compactAssistantMessage(message openai.ChatCompletionMessage) openai.ChatCompletionMessage {
	if len(message.ToolCalls) == 0 {
		return message
	}

	compacted := message
	compacted.ToolCalls = make([]openai.ToolCall, 0, len(message.ToolCalls))
	for _, toolCall := range message.ToolCalls {
		next := toolCall
		next.Function.Arguments = compactToolArguments(toolCall.Function.Name, toolCall.Function.Arguments)
		compacted.ToolCalls = append(compacted.ToolCalls, next)
	}
	return compacted
}

func compactToolArguments(toolName, raw string) string {
	if strings.TrimSpace(raw) == "" {
		return raw
	}

	switch toolName {
	case "report_create_chart":
		// report_create_chart 的参数结构会直接影响后续轮次的工具调用，
		// 这里保留原始参数，避免把摘要字段误导回模型。
		return raw
	case "report_manage_blocks":
		var payload struct {
			Action    string `json:"action"`
			BlockID   string `json:"block_id"`
			BlockKind string `json:"block_kind"`
			Title     string `json:"title"`
			Content   string `json:"content"`
			ChartID   string `json:"chart_id"`
		}
		if err := json.Unmarshal([]byte(raw), &payload); err == nil {
			summary, _ := json.Marshal(map[string]interface{}{
				"action":        payload.Action,
				"block_id":      payload.BlockID,
				"block_kind":    payload.BlockKind,
				"title":         payload.Title,
				"chart_id":      payload.ChartID,
				"content_note":  "compacted_for_history",
				"content_chars": len([]rune(payload.Content)),
				"content_head":  clipHistoryText(payload.Content, 120),
			})
			return string(summary)
		}
	case "report_finalize":
		var payload struct {
			ReportTitle string `json:"report_title"`
			Author      string `json:"author"`
		}
		if err := json.Unmarshal([]byte(raw), &payload); err == nil {
			summary, _ := json.Marshal(map[string]interface{}{
				"report_title": payload.ReportTitle,
				"author":       payload.Author,
			})
			return string(summary)
		}
	}

	return raw
}

func clipHistoryText(input string, max int) string {
	input = strings.Join(strings.Fields(strings.TrimSpace(input)), " ")
	if max <= 0 {
		return ""
	}
	runes := []rune(input)
	if len(runes) <= max {
		return input
	}
	return string(runes[:max]) + "...(truncated)"
}

func compactToolResult(toolName, result string) string {
	trimmed := strings.TrimSpace(result)
	if trimmed == "" {
		return result
	}

	switch toolName {
	case "data_query_sql":
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(trimmed), &payload); err == nil {
			minified, _ := json.Marshal(payload)
			return string(minified)
		}
	case "data_describe_table":
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(trimmed), &payload); err == nil {
			minified, _ := json.Marshal(payload)
			return string(minified)
		}
	case "code_run_python":
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(trimmed), &payload); err == nil {
			minified, _ := json.Marshal(payload)
			return string(minified)
		}
	case "data_list_tables":
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(trimmed), &payload); err == nil {
			minified, _ := json.Marshal(payload)
			return string(minified)
		}
		return strings.Join(strings.Fields(trimmed), " ")
	case "task_delegate":
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(trimmed), &payload); err == nil {
			minified, _ := json.Marshal(map[string]interface{}{
				"ok":               payload["ok"],
				"tool":             payload["tool"],
				"child_run_id":     payload["child_run_id"],
				"delegate_role":    payload["delegate_role"],
				"goal_id":          payload["goal_id"],
				"allowed_tools":    payload["allowed_tools"],
				"delegate_summary": payload["delegate_summary"],
				"summary_text":     payload["summary_text"],
				"trace_count":      traceCount(payload["trace"]),
			})
			return string(minified)
		}
	}

	return result
}

func traceCount(value interface{}) int {
	items, ok := value.([]interface{})
	if !ok {
		return 0
	}
	return len(items)
}

func toolCallSucceeded(result string, execErr error) bool {
	if execErr != nil {
		return false
	}

	var payload struct {
		OK *bool `json:"ok"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(result)), &payload); err == nil && payload.OK != nil {
		return *payload.OK
	}

	return true
}

// ProvideAskUserResult 将用户的直接回复作为 user_request_input 工具的执行结果注入 LLM 对话上下文
func (e *Engine) ProvideAskUserResult(userResponse string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	var toolCallID string
	for i := len(e.messages) - 1; i >= 0; i-- {
		msg := e.messages[i]
		if msg.Role == openai.ChatMessageRoleAssistant && len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				if tc.Function.Name == "user_request_input" {
					toolCallID = tc.ID
					break
				}
			}
		}
		if toolCallID != "" {
			break
		}
	}

	if toolCallID == "" {
		return fmt.Errorf("没有找到正在等待的用户确认 (user_request_input) 工具调用")
	}

	e.messages = append(e.messages, openai.ChatCompletionMessage{
		Role:       openai.ChatMessageRoleTool,
		Content:    userResponse,
		ToolCallID: toolCallID,
		Name:       "user_request_input",
	})

	return nil
}
