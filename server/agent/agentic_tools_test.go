package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAskUserToolSupportsOptionsAndCustomAnswer(t *testing.T) {
	t.Parallel()

	tool := &AskUserTool{}
	params := string(tool.Parameters())
	for _, want := range []string{"options", "allow_multiple", "allow_custom", "input_hint"} {
		if !strings.Contains(params, want) {
			t.Fatalf("expected %s in user input contract, got %s", want, params)
		}
	}
	result, err := tool.Execute(json.RawMessage(`{
		"question":"请选择口径",
		"reason":"指标存在歧义",
		"context_ref":"orders.amount",
		"input_hint":"直接输入要采用的口径",
		"required":true,
		"options":[{"id":"gross","label":"Gross"}],
		"allow_multiple":true
	}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("payload json: %v", err)
	}
	if payload["ok"] != true || payload["tool"] != "user_request_input" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
	if payload["allow_multiple"] != true || payload["allow_custom"] != true {
		t.Fatalf("unexpected selection flags: %#v", payload)
	}
	if payload["input_hint"] != "直接输入要采用的口径" {
		t.Fatalf("unexpected input_hint: %#v", payload["input_hint"])
	}
	options, ok := payload["options"].([]interface{})
	if !ok || len(options) != 1 {
		t.Fatalf("expected one selectable option, got %#v", payload["options"])
	}
	first, _ := options[0].(map[string]interface{})
	if first["id"] != "gross" || first["label"] != "Gross" {
		t.Fatalf("unexpected option payload: %#v", first)
	}
}

func TestProvideAskUserResultStoresStructuredToolResult(t *testing.T) {
	t.Parallel()

	engine := &Engine{
		history: []ConversationItem{
			{
				Role: LLMRoleAssistant,
				ToolCalls: []LLMToolCall{
					{
						ID: "call_1",
						Function: LLMFunctionCall{
							Name: "user_request_input",
						},
					},
				},
			},
		},
	}
	answer := `{"response_type":"selection","selected_option_ids":["gross"],"custom_response":""}`
	if err := engine.ProvideAskUserResult(answer); err != nil {
		t.Fatalf("ProvideAskUserResult: %v", err)
	}
	if len(engine.history) != 2 || engine.history[1].Role != LLMRoleTool || engine.history[1].ToolCallID != "call_1" {
		t.Fatalf("unexpected history after user input: %#v", engine.history)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(engine.history[1].Content), &payload); err != nil {
		t.Fatalf("expected structured tool result JSON, got %q: %v", engine.history[1].Content, err)
	}
	if payload["ok"] != true || payload["tool"] != "user_request_input" || payload["response_json"] != true {
		t.Fatalf("unexpected user input tool payload: %#v", payload)
	}
	response, ok := payload["response"].(map[string]interface{})
	if !ok || response["response_type"] != "selection" {
		t.Fatalf("expected parsed response object, got %#v", payload["response"])
	}
}

func TestParseAskUserToolCallArgumentsUsesCurrentProtocol(t *testing.T) {
	t.Parallel()

	payload, err := parseAskUserToolCallArguments(`{
		"question":"请选择口径",
		"reason":"指标存在歧义",
		"scope":"metric",
		"context_ref":"orders.amount",
		"input_hint":"补充口径说明",
		"required":true,
		"allow_multiple":true,
		"options":[{"id":"gross","label":"Gross","hint":"含税"}]
	}`)
	if err != nil {
		t.Fatalf("parse current protocol: %v", err)
	}
	if payload.Question != "请选择口径" || payload.Scope != "metric" || payload.ContextRef != "orders.amount" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
	if !payload.Required || !payload.AllowMultiple || !payload.AllowCustom {
		t.Fatalf("unexpected flags: %#v", payload)
	}
	if len(payload.Options) != 1 || payload.Options[0].ID != "gross" || payload.Options[0].Label != "Gross" {
		t.Fatalf("unexpected options: %#v", payload.Options)
	}
}

func TestParseAskUserToolCallArgumentsRejectsUnstructuredOptions(t *testing.T) {
	t.Parallel()

	_, err := parseAskUserToolCallArguments(`{"question":"请选择口径","options":["gross"]}`)
	if err == nil {
		t.Fatal("expected non-structured options to be rejected")
	}
}

func TestParseAskUserToolCallArgumentsRejectsIncompleteOptions(t *testing.T) {
	t.Parallel()

	_, err := parseAskUserToolCallArguments(`{"question":"请选择口径","options":[{"label":"Gross"}]}`)
	if err == nil {
		t.Fatal("expected option without id to be rejected")
	}
}

func TestParseAskUserToolCallArgumentsRejectsNoResponsePath(t *testing.T) {
	t.Parallel()

	_, err := parseAskUserToolCallArguments(`{"question":"请选择口径","allow_custom":false}`)
	if err == nil {
		t.Fatal("expected request without options or custom answer path to be rejected")
	}
}
