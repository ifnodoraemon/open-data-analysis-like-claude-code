package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ifnodoraemon/openDataAnalysis/domain"
	"github.com/ifnodoraemon/openDataAnalysis/tools"
	openai "github.com/sashabaranov/go-openai"
)

func init() {
	tools.RegisterGlobalTool(func(ctx tools.ToolContext) tools.Tool {
		if ctx.Subgoals == nil {
			return nil
		}
		var memory *WorkingMemory
		if ctx.Memory != nil {
			if typed, ok := ctx.Memory.(*WorkingMemory); ok {
				memory = typed
			}
		}
		return &ManageSubgoalsTool{
			Subgoals: ctx.Subgoals.(*SubgoalManager),
			EmitFunc: func(event WSEvent) {
				if ctx.EmitFunc != nil {
					ctx.EmitFunc(event)
				}
			},
			Memory: memory,
		}
	})
	tools.RegisterGlobalTool(func(ctx tools.ToolContext) tools.Tool {
		var memory *WorkingMemory
		if ctx.Memory != nil {
			if typed, ok := ctx.Memory.(*WorkingMemory); ok {
				memory = typed
			}
		}
		var subgoals *SubgoalManager
		if ctx.Subgoals != nil {
			subgoals = ctx.Subgoals.(*SubgoalManager)
		}
		return &DelegateTaskTool{
			BaseRegistry: ctx.DelegateRegistry,
			EmitFunc: func(event WSEvent) {
				if ctx.EmitFunc != nil {
					ctx.EmitFunc(event)
				}
			},
			Memory:   memory,
			Subgoals: subgoals,
		}
	})
}

type ManageSubgoalsTool struct {
	Subgoals *SubgoalManager
	EmitFunc func(WSEvent)
	Memory   *WorkingMemory
}

func (t *ManageSubgoalsTool) SetEventEmitter(emit func(WSEvent)) {
	t.EmitFunc = emit
}

func (t *ManageSubgoalsTool) Name() string {
	return "goal_manage"
}

func (t *ManageSubgoalsTool) Description() string {
	return "记录或更新目标树中的节点状态。只修改状态，不执行任务。"
}

func (t *ManageSubgoalsTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {
				"type": "string",
				"enum": ["add", "complete", "reject"],
				"description": "要执行的操作。add（在看板上记录一个目标）, complete（标记目标已充分解决）, reject（任务无法完成放弃）."
			},
			"description": {
				"type": "string",
				"description": "仅 action=add 时需要。清晰描述记录的目标。"
			},
			"parent_goal_id": {
				"type": "string",
				"description": "仅 action=add 时可选。父目标 ID。用于表达当前目标是某个更大目标下的子步骤。"
			},
			"goal_id": {
				"type": "string",
				"description": "仅 action=complete 或 reject 时需要。你要变更状态的目标 ID。"
			},
			"result": {
				"type": "string",
				"description": "仅 action=complete 或 reject 时需要。给目标追加的最终结论或者放弃原因。"
			}
		},
		"required": ["action"]
	}`)
}

func (t *ManageSubgoalsTool) Execute(args json.RawMessage) (string, error) {
	if t.Subgoals == nil {
		return "", fmt.Errorf("subgoal manager is not initialized")
	}

	var payload struct {
		Action       string `json:"action"`
		Description  string `json:"description"`
		ParentGoalID string `json:"parent_goal_id"`
		GoalID       string `json:"goal_id"`
		Result       string `json:"result"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return "", fmt.Errorf("invalid arguments: %v", err)
	}

	switch payload.Action {
	case "add":
		if payload.Description == "" {
			return "", fmt.Errorf("description is required for add action")
		}
		id := t.Subgoals.AddGoal(payload.Description, payload.ParentGoalID)
		t.emitUpdate()
		return fmt.Sprintf("已记录目标，ID: %s。该记录不会自动执行。", id), nil
	case "complete":
		if payload.GoalID == "" {
			return "", fmt.Errorf("goal_id is required for complete action")
		}
		if err := t.Subgoals.UpdateGoalStatus(payload.GoalID, StatusComplete, payload.Result); err != nil {
			return "", err
		}
		t.emitUpdate()
		return fmt.Sprintf("已将目标[%s]标记为完成。附加结论: %s", payload.GoalID, payload.Result), nil
	case "reject":
		if payload.GoalID == "" {
			return "", fmt.Errorf("goal_id is required for reject action")
		}
		if err := t.Subgoals.UpdateGoalStatus(payload.GoalID, StatusRejected, payload.Result); err != nil {
			return "", err
		}
		t.emitUpdate()
		return fmt.Sprintf("已将目标[%s]标记为放弃。原因: %s", payload.GoalID, payload.Result), nil
	default:
		return "", fmt.Errorf("unknown action: %s", payload.Action)
	}
}

func (t *ManageSubgoalsTool) emitUpdate() {
	if t.EmitFunc == nil {
		return
	}
	t.EmitFunc(WSEvent{
		Type: EventStateSubgoalsUpdated,
		Data: map[string]interface{}{"goals": t.Subgoals.ListAll()},
	})
}

type DelegateTaskTool struct {
	BaseRegistry  *tools.Registry
	EmitFunc      func(WSEvent)
	ParentContext context.Context
	Memory        *WorkingMemory
	Subgoals      *SubgoalManager
}

type delegateTraceItem struct {
	Kind    string `json:"kind"`
	Summary string `json:"summary"`
}

func (t *DelegateTaskTool) SetEventEmitter(emit func(WSEvent)) {
	t.EmitFunc = emit
}

func (t *DelegateTaskTool) SetExecutionContext(ctx context.Context) {
	t.ParentContext = ctx
}

func (t *DelegateTaskTool) Name() string {
	return "task_delegate"
}

func (t *DelegateTaskTool) Description() string {
	return "创建一个受限子代理来执行指定任务。需要提供任务说明和允许使用的工具列表。"
}

func (t *DelegateTaskTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"role_name": {
				"type": "string",
				"description": "子代理的标签，用于区分不同实例。"
			},
			"system_prompt": {
				"type": "string",
				"description": "附加给子代理的额外约束。"
			},
			"task_instruction": {
				"type": "string",
				"description": "子代理本次要完成的具体任务。"
			},
			"allowed_tools": {
				"type": "array",
				"items": {"type": "string"},
				"description": "子代理允许使用的工具列表。"
			},
			"goal_id": {
				"type": "string",
				"description": "可选。关联的目标 ID。"
			}
		},
		"required": ["role_name", "task_instruction", "allowed_tools"]
	}`)
}

func (t *DelegateTaskTool) Execute(args json.RawMessage) (string, error) {
	if t.BaseRegistry == nil {
		return "", fmt.Errorf("delegate base registry is not configured")
	}

	var payload struct {
		RoleName        string   `json:"role_name"`
		SystemPrompt    string   `json:"system_prompt"`
		TaskInstruction string   `json:"task_instruction"`
		AllowedTools    []string `json:"allowed_tools"`
		GoalID          string   `json:"goal_id"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return "", fmt.Errorf("invalid arguments: %v", err)
	}
	if strings.TrimSpace(payload.RoleName) == "" {
		return "", fmt.Errorf("role_name is required")
	}
	if strings.TrimSpace(payload.TaskInstruction) == "" {
		return "", fmt.Errorf("task_instruction is required")
	}
	if len(payload.AllowedTools) == 0 {
		return "", fmt.Errorf("allowed_tools is required")
	}

	subReg := t.BaseRegistry.CloneFiltered(payload.AllowedTools)
	if len(subReg.ListTools()) == 0 {
		return "", fmt.Errorf("no allowed tools resolved for delegate")
	}

	ctx := t.ParentContext
	if ctx == nil {
		ctx = context.Background()
	}

	emit := t.EmitFunc
	if emit == nil {
		emit = func(WSEvent) {}
	}
	persistence := DelegateRunPersistenceFromContext(ctx)
	childRunID := ""
	if persistence != nil {
		var err error
		childRunID, err = persistence.StartChildRun(ctx, ChildRunStart{
			ParentRunID:  TraceMetadataFromContext(ctx).RunID,
			RoleName:     payload.RoleName,
			InputMessage: payload.TaskInstruction,
			GoalID:       payload.GoalID,
			AllowedTools: payload.AllowedTools,
		})
		if err != nil {
			return "", fmt.Errorf("failed to start child run: %w", err)
		}
	}
	childCtx := ctx
	if childRunID != "" {
		meta := TraceMetadataFromContext(ctx)
		meta.RunID = childRunID
		meta.TraceID = childRunID
		childCtx = WithTraceMetadata(ctx, meta)
	}

	if t.Subgoals != nil && payload.GoalID != "" {
		if err := t.Subgoals.UpdateGoalStatus(payload.GoalID, StatusRunning, ""); err == nil {
			emit(WSEvent{
				Type: EventStateSubgoalsUpdated,
				Data: map[string]interface{}{"goals": t.Subgoals.ListAll()},
			})
		}
	}

	emit(WSEvent{Type: EventThinking, Data: ThinkingData{Content: fmt.Sprintf("[%s 启动] 已派生子 Agent，工具边界: %s", payload.RoleName, strings.Join(payload.AllowedTools, ", "))}})

	childPrompt := BuildPlannerPrompt(subReg)
	if extra := strings.TrimSpace(payload.SystemPrompt); extra != "" {
		childPrompt = childPrompt + "\n\n## 本次派生附加约束\n" + extra
	}

	llmClient := NewLLMClient()
	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: childPrompt,
		},
		{
			Role:    openai.ChatMessageRoleUser,
			Content: payload.TaskInstruction,
		},
	}
	oaiTools := subReg.GetOpenAITools()
	trace := make([]delegateTraceItem, 0, 12)

	const maxWorkerIterations = 25
	for i := 0; i < maxWorkerIterations; i++ {
		if childCtx.Err() != nil {
			if t.Subgoals != nil && payload.GoalID != "" {
				_ = t.Subgoals.UpdateGoalStatus(payload.GoalID, StatusPending, "任务被取消")
				emit(WSEvent{
					Type: EventStateSubgoalsUpdated,
					Data: map[string]interface{}{"goals": t.Subgoals.ListAll()},
				})
			}
			if persistence != nil && childRunID != "" {
				cancelMsg := "任务被取消"
				_ = persistence.UpdateChildRunStatus(childCtx, childRunID, string(domain.RunStatusCancelled), &cancelMsg)
			}
			return "", childCtx.Err()
		}

		resp, err := llmClient.ChatWithTools(childCtx, messages, oaiTools)
		if err != nil {
			if t.Subgoals != nil && payload.GoalID != "" {
				_ = t.Subgoals.UpdateGoalStatus(payload.GoalID, StatusPending, err.Error())
				emit(WSEvent{
					Type: EventStateSubgoalsUpdated,
					Data: map[string]interface{}{"goals": t.Subgoals.ListAll()},
				})
			}
			if persistence != nil && childRunID != "" {
				msg := err.Error()
				_ = persistence.UpdateChildRunStatus(childCtx, childRunID, string(domain.RunStatusFailed), &msg)
			}
			return "", fmt.Errorf("delegated agent failed: %w", err)
		}
		if len(resp.Choices) == 0 {
			if persistence != nil && childRunID != "" {
				msg := "delegated agent returned no response"
				_ = persistence.UpdateChildRunStatus(childCtx, childRunID, string(domain.RunStatusFailed), &msg)
			}
			return "", fmt.Errorf("delegated agent returned no response")
		}

		choice := resp.Choices[0]
		if choice.Message.Content != "" {
			content := strings.TrimSpace(choice.Message.Content)
			trace = append(trace, delegateTraceItem{Kind: "thinking", Summary: clipDelegateText(content, 160)})
			ev := WSEvent{Type: EventThinking, RunID: childRunID, Data: ThinkingData{Content: fmt.Sprintf("[%s 思考] %s", payload.RoleName, content)}}
			emit(ev)
			if persistence != nil && childRunID != "" {
				_ = persistence.AppendChildEvent(childCtx, childRunID, WSEvent{Type: EventThinking, Data: ThinkingData{Content: content}})
			}
		}

		if choice.FinishReason == openai.FinishReasonStop && len(choice.Message.ToolCalls) == 0 {
			result := strings.TrimSpace(choice.Message.Content)
			emit(WSEvent{Type: EventThinking, Data: ThinkingData{Content: fmt.Sprintf("[%s 完成] %s", payload.RoleName, result)}})
			if persistence != nil && childRunID != "" {
				_ = persistence.UpdateChildRunSummary(childCtx, childRunID, result)
				_ = persistence.AppendChildEvent(childCtx, childRunID, WSEvent{Type: EventRunCompleted, Data: CompleteData{Summary: result}})
				_ = persistence.UpdateChildRunStatus(childCtx, childRunID, string(domain.RunStatusCompleted), nil)
			}
			return delegateToolSuccess(childRunID, payload.RoleName, payload.TaskInstruction, payload.AllowedTools, payload.GoalID, result, trace), nil
		}

		if len(choice.Message.ToolCalls) == 0 {
			continue
		}

		messages = append(messages, compactAssistantMessage(choice.Message))
		for _, toolCall := range choice.Message.ToolCalls {
			trace = append(trace, delegateTraceItem{
				Kind:    "tool_call",
				Summary: fmt.Sprintf("%s(%s)", toolCall.Function.Name, clipDelegateText(toolCall.Function.Arguments, 120)),
			})
			callEv := WSEvent{
				Type:  EventToolCall,
				RunID: childRunID,
				Data: ToolCallData{
					ID:        toolCall.ID,
					Name:      toolCall.Function.Name,
					Arguments: json.RawMessage(toolCall.Function.Arguments),
				},
			}
			emit(callEv)
			if persistence != nil && childRunID != "" {
				_ = persistence.AppendChildEvent(childCtx, childRunID, callEv)
			}

			start := time.Now()
			result, execErr := subReg.Execute(toolCall.Function.Name, json.RawMessage(toolCall.Function.Arguments))
			duration := time.Since(start).Milliseconds()

			if execErr != nil {
				trace = append(trace, delegateTraceItem{
					Kind:    "tool_error",
					Summary: fmt.Sprintf("%s failed: %s", toolCall.Function.Name, clipDelegateText(execErr.Error(), 160)),
				})
				resultEv := WSEvent{Type: EventToolResult, RunID: childRunID, Data: ToolResultData{
					ID:       toolCall.ID,
					Name:     toolCall.Function.Name,
					Result:   execErr.Error(),
					Success:  false,
					Duration: duration,
				}}
				emit(resultEv)
				if persistence != nil && childRunID != "" {
					_ = persistence.AppendChildEvent(childCtx, childRunID, resultEv)
				}
				messages = append(messages, openai.ChatCompletionMessage{
					Role:       openai.ChatMessageRoleTool,
					Content:    fmt.Sprintf("Tool Failed: %v", execErr),
					ToolCallID: toolCall.ID,
				})
				continue
			}

			trace = append(trace, delegateTraceItem{
				Kind:    "tool_result",
				Summary: fmt.Sprintf("%s ok: %s", toolCall.Function.Name, clipDelegateToolResult(result, 160)),
			})
			resultEv := WSEvent{Type: EventToolResult, RunID: childRunID, Data: ToolResultData{
				ID:       toolCall.ID,
				Name:     toolCall.Function.Name,
				Result:   result,
				Success:  true,
				Duration: duration,
			}}
			emit(resultEv)
			if persistence != nil && childRunID != "" {
				_ = persistence.AppendChildEvent(childCtx, childRunID, resultEv)
			}
			messages = append(messages, openai.ChatCompletionMessage{
				Role:       openai.ChatMessageRoleTool,
				Content:    result,
				ToolCallID: toolCall.ID,
			})
		}
	}

	if persistence != nil && childRunID != "" {
		msg := fmt.Sprintf("delegated agent %s max iterations reached", payload.RoleName)
		_ = persistence.UpdateChildRunStatus(childCtx, childRunID, string(domain.RunStatusFailed), &msg)
	}
	return "", fmt.Errorf("delegated agent %s max iterations reached", payload.RoleName)
}

func delegateToolSuccess(childRunID, roleName, taskInstruction string, allowedTools []string, goalID, summary string, trace []delegateTraceItem) string {
	payload := map[string]interface{}{
		"ok":               true,
		"tool":             "task_delegate",
		"child_run_id":     childRunID,
		"delegate_role":    roleName,
		"task_instruction": taskInstruction,
		"allowed_tools":    allowedTools,
		"delegate_summary": summary,
		"summary_text":     fmt.Sprintf("子 Agent %s 已完成: %s", roleName, summary),
		"trace":            trace,
	}
	if strings.TrimSpace(goalID) != "" {
		payload["goal_id"] = goalID
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return `{"ok":false,"tool":"task_delegate","message":"delegate response marshal failed"}`
	}
	return string(encoded)
}

func clipDelegateToolResult(raw string, max int) string {
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &payload); err == nil {
		if summary, ok := payload["summary_text"].(string); ok && strings.TrimSpace(summary) != "" {
			return clipDelegateText(summary, max)
		}
		if message, ok := payload["message"].(string); ok && strings.TrimSpace(message) != "" {
			return clipDelegateText(message, max)
		}
		encoded, _ := json.Marshal(payload)
		return clipDelegateText(string(encoded), max)
	}
	return clipDelegateText(raw, max)
}

func clipDelegateText(input string, max int) string {
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
