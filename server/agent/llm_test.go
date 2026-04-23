package agent

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ifnodoraemon/openDataAnalysis/tools"
	anthropic "github.com/liushuangls/go-anthropic/v2"
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

	if resp.Choices[0].FinishReason != LLMFinishReasonStop {
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

func TestBuildResponsesRequestIncludesStrictToolSpecs(t *testing.T) {
	t.Parallel()

	client := &LLMClient{model: "gpt-4o"}
	req, err := client.buildResponsesRequest(&PromptBundle{}, []tools.ToolSpec{
		{
			Type: "function",
			Function: tools.FunctionSpec{
				Name:        "report_create_chart",
				Description: "Create a chart",
				Parameters:  json.RawMessage(`{"type":"object"}`),
				Strict:      true,
			},
		},
	})
	if err != nil {
		t.Fatalf("buildResponsesRequest: %v", err)
	}
	if len(req.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(req.Tools))
	}
	if !req.Tools[0].Strict {
		t.Fatalf("expected strict tool flag to be preserved: %#v", req.Tools[0])
	}
	if req.Tools[0].Name != "report_create_chart" {
		t.Fatalf("unexpected tool name: %#v", req.Tools[0])
	}
}

func TestBuildAnthropicSystemPromptIncludesOnlyDeveloperRuntimeContext(t *testing.T) {
	t.Parallel()

	bundle := &PromptBundle{
		Policy: "policy",
		RuntimeContext: []RuntimeContextBlock{
			{Name: "active_edit_scope", Role: "developer", Content: "TargetBlockID: block-1"},
			{Name: "digest", Role: "user", Content: "- user: asked for update"},
		},
	}

	systemPrompt := buildAnthropicSystemPrompt(bundle)
	if !strings.Contains(systemPrompt, "policy") {
		t.Fatalf("expected policy in system prompt, got %q", systemPrompt)
	}
	if strings.Contains(systemPrompt, "TargetBlockID: block-1") || strings.Contains(systemPrompt, "[digest]") {
		t.Fatalf("did not expect runtime facts in system prompt, got %q", systemPrompt)
	}
}

func TestBuildAnthropicMessagesKeepsUserRuntimeContextInUserStream(t *testing.T) {
	t.Parallel()

	bundle := &PromptBundle{
		RuntimeContext: []RuntimeContextBlock{
			{Name: "active_edit_scope", Role: "developer", Content: "TargetBlockID: block-1"},
			{Name: "digest", Role: "user", Content: "- user: asked for update"},
		},
		Task: "finish analysis",
	}

	msgs := buildAnthropicMessages(bundle)
	if len(msgs) != 1 {
		t.Fatalf("expected one user message, got %d", len(msgs))
	}
	if msgs[0].Role != anthropic.RoleUser {
		t.Fatalf("expected user role, got %q", msgs[0].Role)
	}
	if len(msgs[0].Content) != 3 {
		t.Fatalf("expected developer runtime, user runtime, and task content, got %#v", msgs[0].Content)
	}
	if msgs[0].Content[0].Text == nil || *msgs[0].Content[0].Text != "[runtime_context role=developer name=active_edit_scope]\nTargetBlockID: block-1" {
		t.Fatalf("unexpected first content block: %#v", msgs[0].Content[0])
	}
	if msgs[0].Content[1].Text == nil || *msgs[0].Content[1].Text != "[runtime_context role=user name=digest]\n- user: asked for update" {
		t.Fatalf("unexpected second content block: %#v", msgs[0].Content[1])
	}
	if msgs[0].Content[2].Text == nil || *msgs[0].Content[2].Text != "finish analysis" {
		t.Fatalf("unexpected task content block: %#v", msgs[0].Content[2])
	}
}
