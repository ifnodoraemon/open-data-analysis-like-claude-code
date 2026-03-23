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
	llm           *LLMClient
	registry      *tools.Registry
	policy        string
	history       []ConversationItem
	contextDigest string
	mu            sync.Mutex
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

// isRetryableToolError 判断工具执行错误是否属于可重试的网络临时故障。
func isRetryableToolError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "tls handshake") ||
		strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "eof") ||
		strings.Contains(msg, "connection reset by peer")
}

// retryableToolExec 在工具执行层面对瞬态网络错误做最多 3 次指数退避重试。
// 注意：special handler（user_request_input / report_finalize）不经过此函数。
func retryableToolExec(ctx context.Context, registry *tools.Registry, toolName string, args json.RawMessage) (string, error) {
	delays := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}
	var result string
	var execErr error
	for attempt := 0; attempt <= len(delays); attempt++ {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		result, execErr = registry.Execute(toolName, args)
		if execErr == nil || !isRetryableToolError(execErr) {
			return result, execErr
		}
		if attempt < len(delays) {
			log.Printf("Tool %s 瞬态错误 (第 %d 次): %v — %s 后重试", toolName, attempt+1, execErr, delays[attempt])
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(delays[attempt]):
			}
		}
	}
	return result, execErr
}

// compactWorkerBundle 对子代理消息历史做上下文压缩。
func compactWorkerBundle(bundle *PromptBundle, promptTokens int) {
	if len(bundle.History) <= 1 {
		return
	}
	if promptTokens <= 0 || promptTokens <= contextCompactTriggerTokens {
		return
	}

	recentStart := len(bundle.History) - recentContextWindow
	if recentStart <= 0 {
		return
	}

	existingDigest := ""
	for _, ctx := range bundle.RuntimeContext {
		if ctx.Name == "digest" {
			existingDigest = ctx.Content
		}
	}

	digest := buildHistoryDigest(existingDigest, bundle.History[:recentStart])
	if digest == "" {
		return
	}

	found := false
	for i := range bundle.RuntimeContext {
		if bundle.RuntimeContext[i].Name == "digest" {
			bundle.RuntimeContext[i].Content = digest
			found = true
			break
		}
	}
	if !found {
		bundle.RuntimeContext = append(bundle.RuntimeContext, RuntimeContextBlock{Name: "digest", Content: digest})
	}

	bundle.History = bundle.History[recentStart:]
}

// NewEngine 创建 Agent 引擎（支持多轮对话）
func NewEngine(registry *tools.Registry, systemPrompt string) *Engine {
	if systemPrompt == "" {
		systemPrompt = BuildPolicyPrompt()
	}
	return &Engine{
		llm:      NewLLMClient(),
		registry: registry,
		policy:   systemPrompt,
	}
}

func (e *Engine) ResetMessages() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.history = nil
	e.contextDigest = ""
}

// summarizeMessageForDigest 为历史摘要提取每条消息的最有价值片段。
// - tool result：优先使用 ui_summary/message 等语义字段（由 digestSummary 提取），不截断
// - assistant thinking：取末段结论（结论往往在末尾），而非首部截断
// - user / tool_call：取末段，400 字上限
func summarizeMessageForDigest(msg ConversationItem) string {
	switch msg.Role {
	case openai.ChatMessageRoleUser:
		if text := digestSummary(msg.Content, 400); text != "" {
			return "用户: " + text
		}
	case openai.ChatMessageRoleAssistant:
		parts := make([]string, 0, len(msg.ToolCalls)+1)
		// 取 thinking 末段结论，400 字
		if text := digestSummary(msg.Content, 400); text != "" {
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
		// digestSummary 会优先提取 ui_summary 等语义字段，不截断语义完整的摘要
		rawSummary := extractToolSummary(msg.Content)
		if summary := digestSummary(rawSummary, 400); summary != "" {
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
		if summary := buildStructuredToolSummary(payload); summary != "" {
			if tool, ok := payload["tool"].(string); ok && tool == "task_delegate" {
				if delegateSum, hasSum := payload["delegate_summary"].(string); hasSum && strings.TrimSpace(delegateSum) != "" {
					return summary + ", delegate_summary=" + strings.TrimSpace(delegateSum)
				}
			}
			return summary
		}
		if result, ok := payload["result"].(string); ok && strings.TrimSpace(result) != "" {
			return result
		}
		if message, ok := payload["message"].(string); ok && strings.TrimSpace(message) != "" {
			return message
		}
		if summary, ok := payload["ui_summary"].(string); ok && strings.TrimSpace(summary) != "" {
			return summary
		}
		if summary, ok := payload["summary_text"].(string); ok && strings.TrimSpace(summary) != "" {
			return summary
		}
		if tool, ok := payload["tool"].(string); ok {
			return fmt.Sprintf("tool=%s", tool)
		}
	}
	return trimmed
}

func buildStructuredToolSummary(payload map[string]interface{}) string {
	parts := make([]string, 0, 8)
	if tool, ok := payload["tool"].(string); ok && strings.TrimSpace(tool) != "" {
		parts = append(parts, "tool="+strings.TrimSpace(tool))
	}
	if ok, exists := payload["ok"].(bool); exists {
		parts = append(parts, fmt.Sprintf("ok=%t", ok))
	}
	for _, key := range []string{
		"error_code",
		"action",
		"status",
		"memory_key",
		"table_name",
		"row_count",
		"table_count",
		"file_count",
		"fact_count",
		"goal_count",
		"goal_id",
		"active_branch_count",
		"active_roots",
		"can_finalize",
		"affects_report_delivery",
		"run_status",
		"child_run_status",
		"block_count",
		"chart_count",
		"delivery_state",
		"finalize_issue_count",
		"target_block_id",
		"child_run_id",
		"delegate_role",
		"report_title",
	} {
		if value, exists := payload[key]; exists {
			if part := formatSummaryField(key, value); part != "" {
				parts = append(parts, part)
			}
		}
	}
	return strings.Join(parts, ", ")
}

func formatSummaryField(key string, value interface{}) string {
	switch typed := value.(type) {
	case string:
		typed = strings.TrimSpace(typed)
		if typed == "" {
			return ""
		}
		return fmt.Sprintf("%s=%s", key, typed)
	case bool:
		return fmt.Sprintf("%s=%t", key, typed)
	case float64:
		if typed == float64(int64(typed)) {
			return fmt.Sprintf("%s=%d", key, int64(typed))
		}
		return fmt.Sprintf("%s=%g", key, typed)
	default:
		return ""
	}
}

func buildHistoryDigest(existing string, messages []ConversationItem) string {
	// 收集本轮新增的 bullet 条目（不含已有 digest 文本）
	bullets := make([]string, 0, len(messages))
	for _, msg := range messages {
		if summary := summarizeMessageForDigest(msg); summary != "" {
			bullets = append(bullets, "- "+summary)
		}
	}

	// 对新增 bullets 超限时截断，existing digest 整段保留不参与 bullet 计数
	if len(bullets) > maxDigestBulletCount {
		bullets = append(bullets[:maxDigestBulletCount-1], "- 更早的执行细节已被压缩。")
	}

	// 拼接：existing digest（已有摘要）在前，新 bullets 在后
	parts := make([]string, 0, 2)
	if trimmed := strings.TrimSpace(existing); trimmed != "" {
		parts = append(parts, trimmed)
	}
	if len(bullets) > 0 {
		parts = append(parts, strings.Join(bullets, "\n"))
	}
	if len(parts) == 0 {
		return ""
	}
	return historyDigestPrefix + "\n" + strings.Join(parts, "\n")
}

func (e *Engine) compactMessagesLocked(promptTokens int) {
	if len(e.history) <= 1 {
		return
	}
	if promptTokens <= 0 || promptTokens <= contextCompactTriggerTokens {
		return
	}

	recentStart := len(e.history) - recentContextWindow
	if recentStart <= 0 {
		return
	}

	digest := buildHistoryDigest(e.contextDigest, e.history[:recentStart])
	if digest == "" {
		return
	}

	e.contextDigest = digest
	e.history = e.history[recentStart:]
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
				// LLM 可能发送了字符串数组作为 options，尝试降级兼容
				var raw map[string]interface{}
				if err2 := json.Unmarshal([]byte(toolCall.Function.Arguments), &raw); err2 == nil {
					if q, ok := raw["question"].(string); ok { payload.Question = q }
					if r, ok := raw["reason"].(string); ok { payload.Reason = r }
					if s, ok := raw["scope"].(string); ok { payload.Scope = s }
					if c, ok := raw["context_ref"].(string); ok { payload.ContextRef = c }
					if req, ok := raw["required"].(bool); ok { payload.Required = req }
					if am, ok := raw["allow_multiple"].(bool); ok { payload.AllowMultiple = am }
					if opts, ok := raw["options"].([]interface{}); ok {
						for _, o := range opts {
							if strOpt, ok2 := o.(string); ok2 {
								payload.Options = append(payload.Options, AskUserOption{ID: strOpt, Label: strOpt})
							}
						}
					}
				} else {
					return "", fmt.Errorf("user_request_input 参数解析失败: %w", err), true
				}
			}
			emit(WSEvent{Type: EventUserRequestInput, Data: payload})
			return "", nil, true
		},
		"report_finalize": func(ctx context.Context, toolCall openai.ToolCall, emit func(WSEvent)) (string, error, bool) {
			emit(WSEvent{Type: EventThinking, Data: ThinkingData{Content: "开始生成最终报告..."}})
			result, err := e.registry.Execute(toolCall.Function.Name, json.RawMessage(toolCall.Function.Arguments))
			if err == nil && result != "" {
				result = applyReportHTMLGuardrail(result)
			}
			return result, err, false
		},
	}
}

// Run 执行 Agent 主循环
// userInput 为空字符串时表示从 user_request_input 挂起点恢复执行，
// 用户答案已通过 ProvideAskUserResult 注入历史，此时不再追加额外的 user 消息。
func (e *Engine) Run(ctx context.Context, userInput string, getRuntimeVars func() []RuntimeContextBlock, emit func(WSEvent)) {
	if emit == nil {
		emit = func(WSEvent) {}
	}
	e.prepareRuntimeTools(ctx, emit)
	specialHandlers := e.specialToolHandlers()

	e.mu.Lock()
	oaiTools := e.registry.GetOpenAITools()
	e.mu.Unlock()

	userTask := userInput

	for i := 1; ; i++ {
		select {
		case <-ctx.Done():
			emit(WSEvent{Type: EventRunCancelled, Data: ErrorData{Message: "任务被取消"}})
			return
		default:
		}

		// 通知前端: 正在思考
		emit(WSEvent{Type: EventThinking, Data: ThinkingData{Content: fmt.Sprintf("正在分析... (第 %d 轮)", i)}})

		e.mu.Lock()
		bundle := &PromptBundle{
			Policy: e.policy,
			Task:   userTask,
		}
		if getRuntimeVars != nil {
			bundle.RuntimeContext = append(bundle.RuntimeContext, getRuntimeVars()...)
		}
		if e.contextDigest != "" {
			bundle.RuntimeContext = append(bundle.RuntimeContext, RuntimeContextBlock{
				Name:    "digest",
				Content: historyDigestPrefix + "\n" + e.contextDigest,
			})
		}
		bundle.History = append([]ConversationItem(nil), e.history...)
		e.mu.Unlock()

		resp, err := e.llm.ChatWithTools(ctx, bundle, oaiTools)
		if err != nil {
			emit(WSEvent{Type: EventError, Data: ErrorData{Message: err.Error()}})
			return
		}

		e.mu.Lock()
		if userTask != "" {
			e.history = append(e.history, ConversationItem{
				Role:    openai.ChatMessageRoleUser,
				Content: userTask,
			})
			userTask = ""
		}
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
				e.history = append(e.history, ConversationItem{
					Role:    openai.ChatMessageRoleAssistant,
					Content: choice.Message.Content,
				})
				e.mu.Unlock()
				emit(WSEvent{Type: EventRunCompleted, Data: CompleteData{Summary: choice.Message.Content}})
				return
			}
		}

		// 如果 finish_reason 是 stop 且没有工具调用，结束
		if choice.FinishReason == openai.FinishReasonStop && len(choice.Message.ToolCalls) == 0 {
			e.mu.Lock()
			e.history = append(e.history, ConversationItem{
				Role:    openai.ChatMessageRoleAssistant,
				Content: choice.Message.Content,
			})
			e.mu.Unlock()
			emit(WSEvent{Type: EventRunCompleted, Data: CompleteData{Summary: choice.Message.Content}})
			return
		}

		// 处理工具调用
		if len(choice.Message.ToolCalls) > 0 {
			// 将 assistant 消息加入历史
			e.mu.Lock()
			e.history = append(e.history, compactAssistantMessage(choice.Message))
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
					result, execErr = retryableToolExec(ctx, e.registry, toolCall.Function.Name, json.RawMessage(toolCall.Function.Arguments))
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
				e.history = append(e.history, ConversationItem{
					Role:       openai.ChatMessageRoleTool,
					Content:    compactToolResult(toolCall.Function.Name, result),
					ToolCallID: toolCall.ID,
				})
				e.mu.Unlock()
			}

			continue // 继续循环
		}

		// 保护性兜底：正常流程不会到达此处（有文本或 stop 的分支均已提前 return），
		// 仅作为防御性路径保留，防止极端情况下 LLM 返回既无文本、无工具调用、也非 stop 的响应。
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

func compactAssistantMessage(message openai.ChatCompletionMessage) ConversationItem {
	item := ConversationItem{
		Role:    message.Role,
		Content: message.Content,
	}
	if len(message.ToolCalls) > 0 {
		for _, toolCall := range message.ToolCalls {
			next := toolCall
			next.Function.Arguments = compactToolArguments(toolCall.Function.Name, toolCall.Function.Arguments)
			item.ToolCalls = append(item.ToolCalls, next)
		}
	}
	return item
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
				"content_head":  clipText(payload.Content, 120),
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

// clipHistoryText 已迁移至 stringutil.go clipText

func compactToolResult(toolName, result string) string {
	trimmed := strings.TrimSpace(result)
	if trimmed == "" {
		return result
	}

	switch toolName {
	case "data_query_sql":
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(trimmed), &payload); err == nil {
			return compactQueryResult(payload)
		}
	case "data_describe_table":
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(trimmed), &payload); err == nil {
			minified, _ := json.Marshal(stripHistorySummaryFields(payload))
			return string(minified)
		}
	case "code_run_python":
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(trimmed), &payload); err == nil {
			minified, _ := json.Marshal(stripHistorySummaryFields(payload))
			return string(minified)
		}
	case "data_list_tables":
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(trimmed), &payload); err == nil {
			minified, _ := json.Marshal(stripHistorySummaryFields(payload))
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
				"trace_count":      traceCount(payload["trace"]),
			})
			return string(minified)
		}
	}

	return result
}

func stripHistorySummaryFields(payload map[string]interface{}) map[string]interface{} {
	if payload == nil {
		return nil
	}
	cloned := make(map[string]interface{}, len(payload))
	for key, value := range payload {
		if key == "summary_text" || key == "ui_summary" {
			continue
		}
		cloned[key] = value
	}
	return cloned
}

const queryCompactRowThreshold = 20
const queryCompactKeepRows = 10

// compactQueryResult 为 data_query_sql 的大结果添加列统计摘要，不截断行数据。
// 满足两个目标：保留完整数据供下游推理使用，同时提供统计摘要加速理解。
func compactQueryResult(payload map[string]interface{}) string {
	cloned := stripHistorySummaryFields(payload)

	rows, ok := cloned["rows"].([]interface{})
	if !ok || len(rows) <= queryCompactRowThreshold {
		minified, _ := json.Marshal(cloned)
		return string(minified)
	}

	// 为数值列生成统计摘要（不截断行）
	cloned["_row_count"] = len(rows)
	if columns, ok := cloned["columns"].([]interface{}); ok && len(rows) > 0 {
		stats := buildColumnStats(columns, rows)
		if len(stats) > 0 {
			cloned["column_stats"] = stats
		}
	}

	minified, _ := json.Marshal(cloned)
	return string(minified)
}

// buildColumnStats 为每个数值列生成 min/max/count 摘要
func buildColumnStats(columns []interface{}, rows []interface{}) map[string]interface{} {
	stats := make(map[string]interface{})
	for _, colRaw := range columns {
		col, ok := colRaw.(string)
		if !ok {
			continue
		}

		var min, max float64
		numericCount := 0
		for _, rowRaw := range rows {
			row, ok := rowRaw.(map[string]interface{})
			if !ok {
				continue
			}
			val, exists := row[col]
			if !exists || val == nil {
				continue
			}
			num, ok := val.(float64)
			if !ok {
				continue
			}
			if numericCount == 0 {
				min, max = num, num
			} else {
				if num < min {
					min = num
				}
				if num > max {
					max = num
				}
			}
			numericCount++
		}
		// 只为数值列生成统计
		if numericCount > 0 {
			stats[col] = map[string]interface{}{
				"min":   min,
				"max":   max,
				"count": numericCount,
			}
		}
	}
	return stats
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
	for i := len(e.history) - 1; i >= 0; i-- {
		msg := e.history[i]
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

	e.history = append(e.history, ConversationItem{
		Role:       openai.ChatMessageRoleTool,
		Content:    userResponse,
		ToolCallID: toolCallID,
	})

	return nil
}
