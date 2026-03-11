package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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
}

// NewLLMClient 创建 LLM 客户端
func NewLLMClient() *LLMClient {
	client := &LLMClient{
		provider: config.Cfg.LLMProvider,
		model:    config.Cfg.LLMModel,
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
		if !strings.HasSuffix(baseURL, "/v1") && !strings.HasSuffix(baseURL, "/v1/") {
			if !strings.HasSuffix(baseURL, "/") {
				baseURL += "/"
			}
			baseURL += "v1"
		}
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
	req := openai.ChatCompletionRequest{
		Model:    l.model,
		Messages: messages,
	}
	if len(tools) > 0 {
		req.Tools = tools
	}

	resp, err := l.openaiClient.CreateChatCompletion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("OpenAI API 调用失败: %w", err)
	}
	return &resp, nil
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
