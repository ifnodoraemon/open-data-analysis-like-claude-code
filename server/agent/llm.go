package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/config"
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

// ChatWithTools 统一的调用接口
func (l *LLMClient) ChatWithTools(ctx context.Context, messages []openai.ChatCompletionMessage, tools []openai.Tool) (*openai.ChatCompletionResponse, error) {
	switch l.provider {
	case "anthropic":
		return l.chatAnthropic(ctx, messages, tools)
	default:
		return l.chatOpenAI(ctx, messages, tools)
	}
}

// chatOpenAI OpenAI 格式调用
func (l *LLMClient) chatOpenAI(ctx context.Context, messages []openai.ChatCompletionMessage, tools []openai.Tool) (*openai.ChatCompletionResponse, error) {
	reqBody, err := l.buildResponsesRequest(messages, tools)
	if err != nil {
		return nil, err
	}

	endpoint := strings.TrimSpace(config.Cfg.LLMAPIEndpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("LLM_API_ENDPOINT 未配置")
	}
	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("序列化 Responses 请求失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("创建 Responses 请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+config.Cfg.LLMAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := l.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OpenAI Responses API 调用失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("OpenAI Responses API 调用失败: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var apiResp responsesAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("解析 Responses 响应失败: %w", err)
	}
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

func (l *LLMClient) buildResponsesRequest(messages []openai.ChatCompletionMessage, tools []openai.Tool) (*responsesAPIRequest, error) {
	req := &responsesAPIRequest{
		Model: l.model,
	}

	for _, msg := range messages {
		switch msg.Role {
		case openai.ChatMessageRoleSystem:
			if req.Instructions == "" {
				req.Instructions = msg.Content
			} else if strings.TrimSpace(msg.Content) != "" {
				req.Instructions += "\n\n" + msg.Content
			}
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
		textParts = append(textParts, resp.OutputText)
	}

	for _, item := range resp.Output {
		switch item.Type {
		case "message":
			for _, content := range item.Content {
				if strings.TrimSpace(content.Text) != "" {
					textParts = append(textParts, content.Text)
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
		}
	}

	choice.Message.Content = strings.TrimSpace(strings.Join(textParts, "\n"))
	return &openai.ChatCompletionResponse{
		Choices: []openai.ChatCompletionChoice{choice},
	}
}

// chatAnthropic Anthropic 格式调用，转换为统一的 OpenAI 格式返回
func (l *LLMClient) chatAnthropic(ctx context.Context, messages []openai.ChatCompletionMessage, tools []openai.Tool) (*openai.ChatCompletionResponse, error) {
	// 转换 messages: OpenAI → Anthropic 格式
	var systemPrompt string
	var anthropicMsgs []anthropic.Message

	for _, msg := range messages {
		switch msg.Role {
		case openai.ChatMessageRoleSystem:
			systemPrompt = msg.Content

		case openai.ChatMessageRoleUser:
			anthropicMsgs = append(anthropicMsgs, anthropic.Message{
				Role: anthropic.RoleUser,
				Content: []anthropic.MessageContent{
					anthropic.NewTextMessageContent(msg.Content),
				},
			})

		case openai.ChatMessageRoleAssistant:
			var content []anthropic.MessageContent
			if msg.Content != "" {
				content = append(content, anthropic.NewTextMessageContent(msg.Content))
			}
			// 转换 tool_calls
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
			anthropicMsgs = append(anthropicMsgs, anthropic.Message{
				Role: anthropic.RoleUser,
				Content: []anthropic.MessageContent{
					anthropic.NewToolResultMessageContent(msg.ToolCallID, msg.Content, false),
				},
			})
		}
	}

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
		MaxTokens: 4096,
		Messages:  anthropicMsgs,
	}
	if systemPrompt != "" {
		req.System = systemPrompt
	}
	if len(anthropicTools) > 0 {
		req.Tools = anthropicTools
	}

	resp, err := l.anthropicClient.CreateMessages(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("Anthropic API 调用失败: %w", err)
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
	}
}
