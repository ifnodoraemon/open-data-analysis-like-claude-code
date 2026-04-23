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

func TestParseResponsesBodySupportsSSECompletedEvent(t *testing.T) {
	t.Parallel()

	body := []byte(`data: {"type":"response.created","response":{"id":"resp_1","status":"in_progress"}}

data: {"type":"response.completed","response":{"id":"resp_1","status":"completed","output_text":"done","usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5}}}

data: [DONE]
`)
	resp, err := parseResponsesBody(body)
	if err != nil {
		t.Fatalf("expected SSE body to parse: %v", err)
	}
	if resp.OutputText != "done" {
		t.Fatalf("expected output_text done, got %q", resp.OutputText)
	}
	if resp.Usage.TotalTokens != 5 {
		t.Fatalf("expected total tokens 5, got %d", resp.Usage.TotalTokens)
	}
}

func TestConvertResponsesResponseSupportsChatCompletionsShape(t *testing.T) {
	t.Parallel()

	client := &LLMClient{}
	resp := client.convertResponsesResponse(&responsesAPIResponse{
		Choices: []responsesChoice{
			{Message: responsesMessage{Content: "done"}},
		},
		Usage: responsesAPIUsage{
			PromptTokens:     10,
			CompletionTokens: 3,
		},
	})

	if resp.Choices[0].Message.Content != "done" {
		t.Fatalf("expected chat-compatible content, got %q", resp.Choices[0].Message.Content)
	}
	if resp.Usage.TotalTokens != 13 {
		t.Fatalf("expected normalized total tokens 13, got %d", resp.Usage.TotalTokens)
	}
}

func TestParseResponsesBodyAllowsTextConfigObject(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"id":"resp_bad_route",
		"status":"completed",
		"instructions":"codex cli prompt",
		"output_text":null,
		"output":[],
		"text":{"format":{"type":"text"},"verbosity":"medium"},
		"usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12}
	}`)

	resp, err := parseResponsesBody(body)
	if err != nil {
		t.Fatalf("expected text config object to parse: %v", err)
	}
	if resp.OutputText != "" {
		t.Fatalf("expected text config object not to become output text, got %q", resp.OutputText)
	}
	if !resp.isEmptyOutput() {
		t.Fatalf("expected response with only text config metadata to be empty")
	}
	if !hasPromptMismatch("data-analysis prompt", resp.Instructions) {
		t.Fatalf("expected prompt mismatch to remain detectable")
	}
}

func TestPromptMismatchOnlyFlagsNonEmptyDifference(t *testing.T) {
	t.Parallel()

	if hasPromptMismatch("data prompt", "data prompt") {
		t.Fatalf("expected identical prompts to match")
	}
	if hasPromptMismatch("", "other") {
		t.Fatalf("expected empty request prompt to skip mismatch detection")
	}
	if !hasPromptMismatch("data prompt", "codex prompt") {
		t.Fatalf("expected different non-empty prompts to mismatch")
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

func TestOpenAIBuildResponsesRequestFormatsRuntimeContext(t *testing.T) {
	t.Parallel()

	client := &LLMClient{model: "gpt-4o"}
	bundle := &PromptBundle{
		RuntimeContext: []RuntimeContextBlock{
			{Name: "active_subgoals", Content: "[g1] test_goal (pending)"},
		},
	}

	req, err := client.buildResponsesRequest(bundle, nil)
	if err != nil {
		t.Fatalf("expected no error building request: %v", err)
	}

	if len(req.Input) != 1 {
		t.Fatalf("expected 1 input, got %d", len(req.Input))
	}
	if req.Input[0]["role"] != "developer" {
		t.Fatalf("expected runtime context to use developer role, got %#v", req.Input[0]["role"])
	}
	expected := "[active_subgoals]\n[g1] test_goal (pending)"
	contentStr := req.Input[0]["content"].(string)
	if contentStr != expected {
		t.Fatalf("expected explicitly prefixed runtime core, got %q", contentStr)
	}
}
