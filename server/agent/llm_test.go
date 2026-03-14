package agent

import (
	"testing"

	anthropic "github.com/liushuangls/go-anthropic/v2"
	openai "github.com/sashabaranov/go-openai"
)

func TestConvertResponsesResponseMapsUsage(t *testing.T) {
	t.Parallel()

	client := &LLMClient{}
	resp := client.convertResponsesResponse(&responsesAPIResponse{
		OutputText: "done",
		Usage: responsesAPIUsage{
			InputTokens:  321,
			OutputTokens: 45,
			TotalTokens:  366,
		},
	})

	if resp.Usage.PromptTokens != 321 {
		t.Fatalf("expected prompt tokens 321, got %d", resp.Usage.PromptTokens)
	}
	if resp.Usage.CompletionTokens != 45 {
		t.Fatalf("expected completion tokens 45, got %d", resp.Usage.CompletionTokens)
	}
	if resp.Usage.TotalTokens != 366 {
		t.Fatalf("expected total tokens 366, got %d", resp.Usage.TotalTokens)
	}
}

func TestConvertAnthropicResponseMapsUsage(t *testing.T) {
	t.Parallel()

	client := &LLMClient{}
	text := "done"
	resp := client.convertAnthropicResponse(&anthropic.MessagesResponse{
		Content: []anthropic.MessageContent{
			anthropic.NewTextMessageContent(text),
		},
		StopReason: "end_turn",
		Usage: anthropic.MessagesUsage{
			InputTokens:  111,
			OutputTokens: 22,
		},
	})

	if resp.Choices[0].FinishReason != openai.FinishReasonStop {
		t.Fatalf("expected stop finish reason, got %s", resp.Choices[0].FinishReason)
	}
	if resp.Usage.PromptTokens != 111 {
		t.Fatalf("expected prompt tokens 111, got %d", resp.Usage.PromptTokens)
	}
	if resp.Usage.CompletionTokens != 22 {
		t.Fatalf("expected completion tokens 22, got %d", resp.Usage.CompletionTokens)
	}
	if resp.Usage.TotalTokens != 133 {
		t.Fatalf("expected total tokens 133, got %d", resp.Usage.TotalTokens)
	}
}
