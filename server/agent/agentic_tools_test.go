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
	for _, want := range []string{"options", "selection_mode", "allow_custom", "input_hint"} {
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
		"selection_mode":"multiple"
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
	if payload["selection_mode"] != "multiple" || payload["allow_custom"] != true {
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

func TestAskUserToolDescriptionStatesSuspendBoundary(t *testing.T) {
	t.Parallel()

	description := (&AskUserTool{}).Description()
	for _, want := range []string{
		"suspend the current run as waiting_user_input",
		"normal assistant text response that asks a question is final output",
		"does not suspend the run",
	} {
		if !strings.Contains(description, want) {
			t.Fatalf("expected description to contain %q, got %q", want, description)
		}
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
		"selection_mode":"multiple",
		"options":[{"id":"gross","label":"Gross","hint":"含税"}]
	}`)
	if err != nil {
		t.Fatalf("parse current protocol: %v", err)
	}
	if payload.Question != "请选择口径" || payload.Scope != "metric" || payload.ContextRef != "orders.amount" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
	if !payload.Required || payload.SelectionMode != "multiple" || !payload.AllowCustom {
		t.Fatalf("unexpected flags: %#v", payload)
	}
	if len(payload.Options) != 1 || payload.Options[0].ID != "gross" || payload.Options[0].Label != "Gross" {
		t.Fatalf("unexpected options: %#v", payload.Options)
	}
}

func TestParseAskUserToolCallArgumentsSelectionModeSingle(t *testing.T) {
	t.Parallel()

	payload, err := parseAskUserToolCallArguments(`{
		"question":"您希望从哪个维度进行深度对比分析？",
		"selection_mode":"single",
		"options":[
			{"id":"channel_compare","label":"营销渠道全面对比"},
			{"id":"region_compare","label":"区域销售全面对比"},
			{"id":"inventory_compare","label":"库存品类/仓库对比"}
		]
	}`)
	if err != nil {
		t.Fatalf("parse single selection mode: %v", err)
	}
	if payload.SelectionMode != "single" {
		t.Fatalf("expected explicit selection_mode=single to render single-select: %#v", payload)
	}
}

func TestParseAskUserToolCallArgumentsSelectionModeMultipleEnablesMultiSelect(t *testing.T) {
	t.Parallel()

	payload, err := parseAskUserToolCallArguments(`{
		"question":"请选择一个或多个需要补充分析的维度",
		"selection_mode":"multiple",
		"options":[
			{"id":"channel_compare","label":"营销渠道"},
			{"id":"region_compare","label":"区域销售"}
		]
	}`)
	if err != nil {
		t.Fatalf("parse multi-select question: %v", err)
	}
	if payload.SelectionMode != "multiple" {
		t.Fatalf("expected selection_mode=multiple to render multi-select: %#v", payload)
	}
}

func TestParseAskUserToolCallArgumentsDefaultsToSingleSelectionMode(t *testing.T) {
	t.Parallel()

	payload, err := parseAskUserToolCallArguments(`{
		"question":"请选择需要分析的维度",
		"options":[
			{"id":"channel_compare","label":"营销渠道"},
			{"id":"region_compare","label":"区域销售"}
		]
	}`)
	if err != nil {
		t.Fatalf("parse default selection mode: %v", err)
	}
	if payload.SelectionMode != "single" {
		t.Fatalf("expected omitted selection_mode to default to single: %#v", payload)
	}
}

func TestParseAskUserToolCallArgumentsRejectsInvalidSelectionMode(t *testing.T) {
	t.Parallel()

	_, err := parseAskUserToolCallArguments(`{"question":"请选择口径","selection_mode":"combo"}`)
	if err == nil {
		t.Fatal("expected invalid selection_mode to be rejected")
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
