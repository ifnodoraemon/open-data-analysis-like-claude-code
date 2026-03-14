package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/ifnodoraemon/openDataAnalysis/tools"
	openai "github.com/sashabaranov/go-openai"
)

// Engine Agent 主循环引擎
type Engine struct {
	llm          *LLMClient
	registry     *tools.Registry
	messages     []openai.ChatCompletionMessage
	systemPrompt string
	mu           sync.Mutex
}

const (
	contextBudgetTokens         = 128000
	contextCompactTriggerTokens = contextBudgetTokens * 9 / 10
	recentContextWindow         = 12
	historyDigestPrefix         = "=== 历史执行摘要 ==="
	maxDigestBulletCount        = 24
)

type eventEmitterAware interface {
	SetEventEmitter(func(WSEvent))
}

type executionContextAware interface {
	SetExecutionContext(context.Context)
}

type specialToolHandler func(context.Context, openai.ToolCall, func(WSEvent)) (string, error, bool)

// NewEngine 创建 Agent 引擎（支持多轮对话）
func NewEngine(registry *tools.Registry, systemPrompt string) *Engine {
	if systemPrompt == "" {
		systemPrompt = BuildPlannerPrompt(registry)
	}
	return &Engine{
		llm:          NewLLMClient(),
		registry:     registry,
		systemPrompt: systemPrompt,
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

func isHistoryDigestMessage(msg openai.ChatCompletionMessage) bool {
	return msg.Role == openai.ChatMessageRoleSystem && strings.HasPrefix(strings.TrimSpace(msg.Content), historyDigestPrefix)
}

func compactTextForDigest(input string, limit int) string {
	text := strings.Join(strings.Fields(strings.TrimSpace(input)), " ")
	if text == "" || limit <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "...(truncated)"
}

func summarizeMessageForDigest(msg openai.ChatCompletionMessage) string {
	switch msg.Role {
	case openai.ChatMessageRoleUser:
		if text := compactTextForDigest(msg.Content, 180); text != "" {
			return "用户: " + text
		}
	case openai.ChatMessageRoleAssistant:
		parts := make([]string, 0, len(msg.ToolCalls)+1)
		if text := compactTextForDigest(msg.Content, 180); text != "" {
			parts = append(parts, "助手: "+text)
		}
		if len(msg.ToolCalls) > 0 {
			names := make([]string, 0, len(msg.ToolCalls))
			for _, tc := range msg.ToolCalls {
				names = append(names, tc.Function.Name)
			}
			parts = append(parts, "助手调用工具: "+strings.Join(names, ", "))
		}
		return strings.Join(parts, " | ")
	case openai.ChatMessageRoleTool:
		if summary := compactTextForDigest(extractToolSummary(msg.Content), 180); summary != "" {
			return "工具结果: " + summary
		}
	}
	return ""
}

func extractToolSummary(content string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return ""
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(trimmed), &payload); err == nil {
		if summary, ok := payload["summary_text"].(string); ok && strings.TrimSpace(summary) != "" {
			return summary
		}
		if result, ok := payload["result"].(string); ok && strings.TrimSpace(result) != "" {
			return result
		}
		if tool, ok := payload["tool"].(string); ok {
			return fmt.Sprintf("%s completed", tool)
		}
	}
	return trimmed
}

func buildHistoryDigest(existing string, messages []openai.ChatCompletionMessage) string {
	lines := make([]string, 0, len(messages)+1)
	if trimmed := strings.TrimSpace(existing); trimmed != "" {
		lines = append(lines, trimmed)
	}
	for _, msg := range messages {
		if summary := summarizeMessageForDigest(msg); summary != "" {
			lines = append(lines, "- "+summary)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	if len(lines) > maxDigestBulletCount {
		lines = append(lines[:maxDigestBulletCount-1], "- 更早的执行细节已被压缩，请优先依赖工作记忆、目标状态和最近几轮工具结果继续推进。")
	}
	return historyDigestPrefix + "\n" + strings.Join(lines, "\n")
}

func (e *Engine) compactMessagesLocked(promptTokens int) {
	if len(e.messages) <= 2 {
		return
	}
	if promptTokens <= 0 || promptTokens <= contextCompactTriggerTokens {
		return
	}

	base := e.messages[0]
	start := 1
	existingDigest := ""
	if len(e.messages) > 1 && isHistoryDigestMessage(e.messages[1]) {
		existingDigest = strings.TrimSpace(strings.TrimPrefix(e.messages[1].Content, historyDigestPrefix))
		start = 2
	}
	if len(e.messages)-start <= recentContextWindow {
		return
	}

	recentStart := len(e.messages) - recentContextWindow
	if recentStart < start {
		recentStart = start
	}
	digest := buildHistoryDigest(existingDigest, e.messages[start:recentStart])
	if digest == "" {
		return
	}

	trimmed := []openai.ChatCompletionMessage{base, {
		Role:    openai.ChatMessageRoleSystem,
		Content: digest,
	}}
	trimmed = append(trimmed, e.messages[recentStart:]...)
	e.messages = trimmed
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

		resp, err := e.llm.ChatWithTools(ctx, messages, oaiTools)
		if err != nil {
			emit(WSEvent{Type: EventError, Data: ErrorData{Message: err.Error()}})
			return
		}
		e.mu.Lock()
		e.compactMessagesLocked(resp.Usage.PromptTokens)
		e.mu.Unlock()

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
