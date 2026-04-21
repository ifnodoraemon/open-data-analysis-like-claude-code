package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/ifnodoraemon/openDataAnalysis/config"
	anthropic "github.com/liushuangls/go-anthropic/v2"
	openai "github.com/sashabaranov/go-openai"
)

// LLMClient 统一的 LLM 客户端，支持 OpenAI 和 Anthropic
type LLMClient struct {
	provider        string
	model           string
	openaiClient    *openai.Client
	anthropicClient *anthropic.Client
	httpClient      *http.Client
}

// NewLLMClient 创建 LLM 客户端
func NewLLMClient() *LLMClient {
	client := &LLMClient{
		provider: config.Cfg.LLMProvider,
		model:    config.Cfg.LLMModel,
		httpClient: &http.Client{
			Timeout: 90 * time.Second,
		},
	}

	switch client.provider {
	case "anthropic":
		client.initAnthropic()
	default:
		client.initOpenAI()
	}

	return client
}

func (l *LLMClient) initOpenAI() {
	cfg := openai.DefaultConfig(config.Cfg.LLMAPIKey)

	baseURL := config.Cfg.LLMBaseURL
	if baseURL != "" {
		cfg.BaseURL = baseURL
	}

	l.openaiClient = openai.NewClientWithConfig(cfg)
}

func (l *LLMClient) initAnthropic() {
	opts := []anthropic.ClientOption{}

	baseURL := config.Cfg.LLMBaseURL
	if baseURL != "" && baseURL != "https://api.anthropic.com" {
		opts = append(opts, anthropic.WithBaseURL(baseURL))
	}

	l.anthropicClient = anthropic.NewClient(config.Cfg.LLMAPIKey, opts...)
}

// SimpleChatFunc 返回一个简单的聊天函数，适配 data.LLMChatFunc 签名。
// 用于语义预分析等不需要 tool calling 的场景。
func (l *LLMClient) SimpleChatFunc() func(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	return func(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
		bundle := &PromptBundle{
			Policy: systemPrompt,
			Task:   userPrompt,
		}
		resp, err := l.ChatWithTools(ctx, bundle, nil)
		if err != nil {
			return "", err
		}
		if len(resp.Choices) == 0 {
			return "", fmt.Errorf("LLM returned empty response")
		}
		return resp.Choices[0].Message.Content, nil
	}
}

// isRetryableLLMError 判断 LLM 调用错误是否属于可重试的瞬态错误。
// 4xx（认证/权限/请求格式错误）不可重试；5xx 和网络层错误可重试。
func isRetryableLLMError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	// 非重试：客户端错误（4xx）
	for _, code := range []string{"status=400", "status=401", "status=403", "status=404", "status=422"} {
		if strings.Contains(msg, code) {
			return false
		}
	}
	// 可重试：常见瞬态错误
	if strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "tls handshake") ||
		strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "eof") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "status=429") || // rate limit
		strings.Contains(msg, "status=500") ||
		strings.Contains(msg, "status=502") ||
		strings.Contains(msg, "status=503") ||
		strings.Contains(msg, "status=504") {
		return true
	}
	// context 取消不重试
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	return false
}

// ChatWithTools 统一的调用接口，包含对底层网络不稳定的重试逻辑（指数退避，区分可重试错误）
func (l *LLMClient) ChatWithTools(ctx context.Context, bundle *PromptBundle, tools []openai.Tool) (*openai.ChatCompletionResponse, error) {
	retryCtx, retryCancel := context.WithTimeout(ctx, 120*time.Second)
	defer retryCancel()

	var resp *openai.ChatCompletionResponse
	var err error

	// 指数退避：1s, 3s, 8s（共 3 次重试，第 0 次无等待）
	retryDelays := []time.Duration{time.Second, 3 * time.Second, 8 * time.Second}

	for attempt := 0; attempt <= len(retryDelays); attempt++ {
		if retryCtx.Err() != nil {
			return nil, fmt.Errorf("LLM retry budget exceeded: %w", retryCtx.Err())
		}

		switch l.provider {
		case "anthropic":
			resp, err = l.chatAnthropic(ctx, bundle, tools)
		default:
			resp, err = l.chatOpenAI(ctx, bundle, tools)
		}

		if err == nil {
			return resp, nil
		}

		// 不可重试的错误直接返回
		if !isRetryableLLMError(err) {
			return nil, err
		}

		if attempt < len(retryDelays) {
			log.Printf("LLM transient error (attempt %d, retry in %.0fs): %v", attempt+1, retryDelays[attempt].Seconds(), err)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(retryDelays[attempt]):
			}
		}
	}

	return nil, fmt.Errorf("LLM API request failed after %d retries: %v", len(retryDelays), err)
}

func countRuntimeContextChars(ctxs []RuntimeContextBlock) int {
	var total int
	for _, c := range ctxs {
		total += len([]rune(c.Content))
	}
	return total
}

func countHistoryChars(hist []ConversationItem) int {
	var total int
	for _, h := range hist {
		total += len([]rune(h.Content))
	}
	return total
}

// chatOpenAI OpenAI 格式调用
func (l *LLMClient) chatOpenAI(ctx context.Context, bundle *PromptBundle, tools []openai.Tool) (*openai.ChatCompletionResponse, error) {
	reqBody, err := l.buildResponsesRequest(bundle, tools)
	if err != nil {
		return nil, err
	}

	endpoint := strings.TrimSpace(config.Cfg.LLMAPIEndpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("LLM_API_ENDPOINT not configured")
	}
	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize Responses request: %w", err)
	}
	start := time.Now()
	span := llmDebugWriter.StartSpan(TraceMetadataFromContext(ctx), "llm", l.provider, "", "")
	requestPath := llmDebugWriter.WriteBlob(span, "request.json", reqBytes)
	l.debugLog(span, "llm.request", map[string]interface{}{
		"provider":              l.provider,
		"model":                 l.model,
		"endpoint":              endpoint,
		"message_count":         len(bundle.History),
		"tool_count":            len(tools),
		"tools":                 summarizeTools(tools),
		"user_preview":          clipText(lastUserMessage(bundle.History), 240),
		"instruction_chars":     len([]rune(reqBody.Instructions)),
		"policy_chars":          len([]rune(bundle.Policy)),
		"task_chars":            len([]rune(bundle.Task)),
		"runtime_context_chars": countRuntimeContextChars(bundle.RuntimeContext),
		"history_chars":         countHistoryChars(bundle.History),
		"request_bytes":         len(reqBytes),
		"request_sha256":        blobSHA256(reqBytes),
		"request_path":          requestPath,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create Responses request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+config.Cfg.LLMAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := l.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OpenAI Responses API call failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		responsePath := llmDebugWriter.WriteBlob(span, "response.error.txt", body)
		l.debugLog(span, "llm.error", map[string]interface{}{
			"status":          resp.StatusCode,
			"duration_ms":     time.Since(start).Milliseconds(),
			"error_preview":   clipText(string(body), 500),
			"response_bytes":  len(body),
			"response_sha256": blobSHA256(body),
			"response_path":   responsePath,
		})
		return nil, fmt.Errorf("OpenAI Responses API call failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("failed to read Responses body: %w", err)
	}
	responsePath := llmDebugWriter.WriteBlob(span, "response.json", respBytes)

	var apiResp responsesAPIResponse
	if err := json.Unmarshal(respBytes, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse Responses body: %w", err)
	}
	l.debugLog(span, "llm.response", map[string]interface{}{
		"duration_ms":         time.Since(start).Milliseconds(),
		"output_preview":      clipText(apiResp.OutputText, 300),
		"output_chars":        len([]rune(apiResp.OutputText)),
		"item_count":          len(apiResp.Output),
		"tool_call_count":     countResponsesToolCalls(apiResp.Output),
		"tool_calls":          responseToolNames(apiResp.Output),
		"usage_input_tokens":  apiResp.Usage.InputTokens,
		"usage_output_tokens": apiResp.Usage.OutputTokens,
		"usage_total_tokens":  apiResp.Usage.TotalTokens,
		"response_bytes":      len(respBytes),
		"response_sha256":     blobSHA256(respBytes),
		"response_path":       responsePath,
	})
	return l.convertResponsesResponse(&apiResp), nil
}

type responsesAPIRequest struct {
	Model        string           `json:"model"`
	Instructions string           `json:"instructions,omitempty"`
	Input        []responsesInput `json:"input,omitempty"`
	Tools        []responsesTool  `json:"tools,omitempty"`
	ToolChoice   string           `json:"tool_choice,omitempty"`
}

type responsesInput map[string]interface{}

type responsesTool struct {
	Type        string      `json:"type"`
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Parameters  interface{} `json:"parameters,omitempty"`
}

type responsesAPIResponse struct {
	OutputText string                `json:"output_text"`
	Output     []responsesOutputItem `json:"output"`
	Usage      responsesAPIUsage     `json:"usage"`
}

type responsesAPIUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

type responsesOutputItem struct {
	Type      string                   `json:"type"`
	ID        string                   `json:"id"`
	CallID    string                   `json:"call_id"`
	Name      string                   `json:"name"`
	Arguments string                   `json:"arguments"`
	Role      string                   `json:"role"`
	Content   []responsesOutputContent `json:"content"`
}

type responsesOutputContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (l *LLMClient) buildResponsesRequest(bundle *PromptBundle, tools []openai.Tool) (*responsesAPIRequest, error) {
	req := &responsesAPIRequest{
		Model: l.model,
	}

	if bundle.Policy != "" {
		req.Instructions = bundle.Policy
	}

	for _, block := range bundle.RuntimeContext {
		req.Input = append(req.Input, responsesInput{
			"role":    "user",
			"content": fmt.Sprintf("[%s]\n%s", block.Name, block.Content),
		})
	}

	for _, msg := range bundle.History {
		switch msg.Role {
		case openai.ChatMessageRoleUser:
			req.Input = append(req.Input, responsesInput{
				"role":    "user",
				"content": msg.Content,
			})
		case openai.ChatMessageRoleAssistant:
			if strings.TrimSpace(msg.Content) != "" {
				req.Input = append(req.Input, responsesInput{
					"role":    "assistant",
					"content": msg.Content,
				})
			}
			for _, tc := range msg.ToolCalls {
				req.Input = append(req.Input, responsesInput{
					"type":      "function_call",
					"call_id":   tc.ID,
					"name":      tc.Function.Name,
					"arguments": tc.Function.Arguments,
				})
			}
		case openai.ChatMessageRoleTool:
			req.Input = append(req.Input, responsesInput{
				"type":    "function_call_output",
				"call_id": msg.ToolCallID,
				"output":  msg.Content,
			})
		}
	}

	if bundle.Task != "" {
		req.Input = append(req.Input, responsesInput{
			"role":    "user",
			"content": bundle.Task,
		})
	}

	for _, tool := range tools {
		if tool.Function == nil {
			continue
		}
		req.Tools = append(req.Tools, responsesTool{
			Type:        "function",
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			Parameters:  tool.Function.Parameters,
		})
	}
	if len(req.Tools) > 0 {
		req.ToolChoice = "auto"
	}

	return req, nil
}

func (l *LLMClient) convertResponsesResponse(resp *responsesAPIResponse) *openai.ChatCompletionResponse {
	choice := openai.ChatCompletionChoice{
		Index: 0,
		Message: openai.ChatCompletionMessage{
			Role: openai.ChatMessageRoleAssistant,
		},
		FinishReason: openai.FinishReasonStop,
	}

	var textParts []string
	if strings.TrimSpace(resp.OutputText) != "" {
		// 如果有聚合好的文本，则直接使用，避免和 message 分块重复
		textParts = append(textParts, resp.OutputText)
	}

	for _, item := range resp.Output {
		switch item.Type {
		case "message":
			// 如果 OutputText 为空，才从零散的 message 块聚合文本
			if strings.TrimSpace(resp.OutputText) == "" {
				for _, content := range item.Content {
					if strings.TrimSpace(content.Text) != "" {
						textParts = append(textParts, content.Text)
					}
				}
			}
		case "function_call":
			choice.FinishReason = openai.FinishReasonToolCalls
			choice.Message.ToolCalls = append(choice.Message.ToolCalls, openai.ToolCall{
				ID:   item.CallID,
				Type: openai.ToolTypeFunction,
				Function: openai.FunctionCall{
					Name:      item.Name,
					Arguments: item.Arguments,
				},
			})
		default:
			l.debugLog(SpanInfo{}, "llm.output_item", map[string]interface{}{
				"type": item.Type,
				"name": item.Name,
				"id":   item.ID,
			})
		}
	}

	choice.Message.Content = strings.TrimSpace(strings.Join(textParts, "\n"))
	return &openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{choice},
		Usage: openai.Usage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
	}
}

func (l *LLMClient) debugLog(span SpanInfo, event string, payload map[string]interface{}) {
	llmDebugWriter.WriteEvent(span, event, payload)
}

func summarizeTools(tools []openai.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		if tool.Function != nil {
			names = append(names, tool.Function.Name)
		}
	}
	return names
}

// clipText 已迁移至 stringutil.go

func firstAnthropicText(content []anthropic.MessageContent) string {
	for _, block := range content {
		if block.Type == "text" && block.Text != nil {
			return *block.Text
		}
	}
	return ""
}

func lastUserMessage(messages []ConversationItem) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == openai.ChatMessageRoleUser {
			return messages[i].Content
		}
	}
	return ""
}

func countResponsesToolCalls(items []responsesOutputItem) int {
	count := 0
	for _, item := range items {
		if item.Type == "function_call" {
			count++
		}
	}
	return count
}

func responseToolNames(items []responsesOutputItem) []string {
	names := make([]string, 0, len(items))
	for _, item := range items {
		if item.Type == "function_call" && strings.TrimSpace(item.Name) != "" {
			names = append(names, item.Name)
		}
	}
	return names
}

func countAnthropicToolUses(content []anthropic.MessageContent) int {
	count := 0
	for _, block := range content {
		if block.Type == "tool_use" {
			count++
		}
	}
	return count
}

func anthropicToolNames(content []anthropic.MessageContent) []string {
	names := make([]string, 0, len(content))
	for _, block := range content {
		if block.Type == "tool_use" && strings.TrimSpace(block.Name) != "" {
			names = append(names, block.Name)
		}
	}
	return names
}

// chatAnthropic Anthropic 格式调用，转换为统一的 OpenAI 格式返回
func (l *LLMClient) chatAnthropic(ctx context.Context, bundle *PromptBundle, tools []openai.Tool) (*openai.ChatCompletionResponse, error) {
	span := llmDebugWriter.StartSpan(TraceMetadataFromContext(ctx), "llm", l.provider, "", "")

	systemPrompt := bundle.Policy

	var anthropicMsgs []anthropic.Message
	var currentUserContent []anthropic.MessageContent

	flushUserContent := func() {
		if len(currentUserContent) > 0 {
			anthropicMsgs = append(anthropicMsgs, anthropic.Message{
				Role:    anthropic.RoleUser,
				Content: currentUserContent,
			})
			currentUserContent = nil
		}
	}

	for _, block := range bundle.RuntimeContext {
		currentUserContent = append(currentUserContent, anthropic.NewTextMessageContent(fmt.Sprintf("[%s]\n%s", block.Name, block.Content)))
	}

	for _, msg := range bundle.History {
		switch msg.Role {
		case openai.ChatMessageRoleUser:
			currentUserContent = append(currentUserContent, anthropic.NewTextMessageContent(msg.Content))
		case openai.ChatMessageRoleAssistant:
			flushUserContent()
			var content []anthropic.MessageContent
			if msg.Content != "" {
				content = append(content, anthropic.NewTextMessageContent(msg.Content))
			}
			for _, tc := range msg.ToolCalls {
				inputRaw := json.RawMessage(tc.Function.Arguments)
				content = append(content, anthropic.NewToolUseMessageContent(tc.ID, tc.Function.Name, inputRaw))
			}
			if len(content) > 0 {
				anthropicMsgs = append(anthropicMsgs, anthropic.Message{
					Role:    anthropic.RoleAssistant,
					Content: content,
				})
			}
		case openai.ChatMessageRoleTool:
			currentUserContent = append(currentUserContent, anthropic.NewToolResultMessageContent(msg.ToolCallID, msg.Content, false))
		}
	}

	if bundle.Task != "" {
		currentUserContent = append(currentUserContent, anthropic.NewTextMessageContent(bundle.Task))
	}
	flushUserContent()

	// 转换 tools: OpenAI → Anthropic 格式
	var anthropicTools []anthropic.ToolDefinition
	for _, t := range tools {
		if t.Function != nil {
			// InputSchema 类型是 any，直接传 json.RawMessage
			var inputSchema json.RawMessage
			if params, ok := t.Function.Parameters.(json.RawMessage); ok {
				inputSchema = params
			} else {
				inputSchema, _ = json.Marshal(t.Function.Parameters)
			}

			anthropicTools = append(anthropicTools, anthropic.ToolDefinition{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				InputSchema: inputSchema,
			})
		}
	}

	// 调用 Anthropic API
	req := anthropic.MessagesRequest{
		Model:     anthropic.Model(l.model),
		MaxTokens: 8192,
		Messages:  anthropicMsgs,
	}
	if systemPrompt != "" {
		req.System = systemPrompt
	}
	if len(anthropicTools) > 0 {
		req.Tools = anthropicTools
	}
	reqBytes, err := json.Marshal(req)
	if err == nil {
		requestPath := llmDebugWriter.WriteBlob(span, "request.json", reqBytes)
		l.debugLog(span, "llm.request", map[string]interface{}{
			"provider":              l.provider,
			"model":                 l.model,
			"endpoint":              config.Cfg.LLMAPIEndpoint,
			"message_count":         len(bundle.History),
			"tool_count":            len(tools),
			"tools":                 summarizeTools(tools),
			"user_preview":          clipText(lastUserMessage(bundle.History), 240),
			"instruction_chars":     len([]rune(systemPrompt)),
			"policy_chars":          len([]rune(bundle.Policy)),
			"task_chars":            len([]rune(bundle.Task)),
			"runtime_context_chars": countRuntimeContextChars(bundle.RuntimeContext),
			"history_chars":         countHistoryChars(bundle.History),
			"request_bytes":         len(reqBytes),
			"request_sha256":        blobSHA256(reqBytes),
			"request_path":          requestPath,
		})
	}
	start := time.Now()

	resp, err := l.anthropicClient.CreateMessages(ctx, req)
	if err != nil {
		l.debugLog(span, "llm.error", map[string]interface{}{
			"duration_ms":   time.Since(start).Milliseconds(),
			"error_preview": clipText(err.Error(), 500),
		})
		return nil, fmt.Errorf("Anthropic API call failed: %w", err)
	}
	if respBytes, marshalErr := json.Marshal(resp); marshalErr == nil {
		responsePath := llmDebugWriter.WriteBlob(span, "response.json", respBytes)
		l.debugLog(span, "llm.response", map[string]interface{}{
			"duration_ms":         time.Since(start).Milliseconds(),
			"output_preview":      clipText(firstAnthropicText(resp.Content), 300),
			"output_chars":        len([]rune(firstAnthropicText(resp.Content))),
			"item_count":          len(resp.Content),
			"tool_call_count":     countAnthropicToolUses(resp.Content),
			"tool_calls":          anthropicToolNames(resp.Content),
			"usage_input_tokens":  resp.Usage.InputTokens,
			"usage_output_tokens": resp.Usage.OutputTokens,
			"usage_total_tokens":  resp.Usage.InputTokens + resp.Usage.OutputTokens,
			"response_bytes":      len(respBytes),
			"response_sha256":     blobSHA256(respBytes),
			"response_path":       responsePath,
		})
	}

	// 转换响应: Anthropic → OpenAI 格式
	return l.convertAnthropicResponse(&resp), nil
}

// convertAnthropicResponse 将 Anthropic 响应转换为 OpenAI 格式
func (l *LLMClient) convertAnthropicResponse(resp *anthropic.MessagesResponse) *openai.ChatCompletionResponse {
	choice := openai.ChatCompletionChoice{
		Index: 0,
		Message: openai.ChatCompletionMessage{
			Role: openai.ChatMessageRoleAssistant,
		},
	}

	var textParts []string

	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			if block.Text != nil {
				textParts = append(textParts, *block.Text)
			}
		case "tool_use":
			if block.ID != "" {
				argsBytes, _ := json.Marshal(block.Input)
				choice.Message.ToolCalls = append(choice.Message.ToolCalls, openai.ToolCall{
					ID:   block.ID,
					Type: openai.ToolTypeFunction,
					Function: openai.FunctionCall{
						Name:      block.Name,
						Arguments: string(argsBytes),
					},
				})
			}
		}
	}

	choice.Message.Content = strings.Join(textParts, "\n")

	switch resp.StopReason {
	case "end_turn":
		choice.FinishReason = openai.FinishReasonStop
	case "tool_use":
		choice.FinishReason = openai.FinishReasonToolCalls
	default:
		choice.FinishReason = openai.FinishReasonStop
	}

	return &openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{choice},
		Usage: openai.Usage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		},
	}
}
