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
			Subgoals: func() *SubgoalManager {
				if sm, ok := ctx.Subgoals.(*SubgoalManager); ok {
					return sm
				}
				return nil
			}(),
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
			if sm, ok := ctx.Subgoals.(*SubgoalManager); ok {
				subgoals = sm
			}
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
	return "记录或更新目标树中的节点状态。支持 add、complete、reject；只修改目标状态，不执行任务。返回变更后的 goal_id 与当前目标树事实。"
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
		result := buildGoalStateFacts(t.Subgoals, false)
		result["ok"] = true
		result["tool"] = "goal_manage"
		result["action"] = "add"
		result["goal_id"] = id
		result["status"] = StatusPending
		result["description"] = payload.Description
		if strings.TrimSpace(payload.ParentGoalID) != "" {
			result["parent_goal_id"] = payload.ParentGoalID
		}
		result["message"] = "已记录目标状态。该记录不会自动执行。"
		result["ui_summary"] = fmt.Sprintf("已记录目标 %s。", id)
		return marshalToolPayload(result)
	case "complete":
		if payload.GoalID == "" {
			return "", fmt.Errorf("goal_id is required for complete action")
		}
		if err := t.Subgoals.UpdateGoalStatus(payload.GoalID, StatusComplete, payload.Result); err != nil {
			return "", err
		}
		t.emitUpdate()
		result := buildGoalStateFacts(t.Subgoals, false)
		result["ok"] = true
		result["tool"] = "goal_manage"
		result["action"] = "complete"
		result["goal_id"] = payload.GoalID
		result["status"] = StatusComplete
		result["result"] = payload.Result
		result["message"] = "已更新目标状态。"
		result["ui_summary"] = fmt.Sprintf("已将目标 %s 标记为完成。", payload.GoalID)
		return marshalToolPayload(result)
	case "reject":
		if payload.GoalID == "" {
			return "", fmt.Errorf("goal_id is required for reject action")
		}
		if err := t.Subgoals.UpdateGoalStatus(payload.GoalID, StatusRejected, payload.Result); err != nil {
			return "", err
		}
		t.emitUpdate()
		result := buildGoalStateFacts(t.Subgoals, false)
		result["ok"] = true
		result["tool"] = "goal_manage"
		result["action"] = "reject"
		result["goal_id"] = payload.GoalID
		result["status"] = StatusRejected
		result["result"] = payload.Result
		result["message"] = "已更新目标状态。"
		result["ui_summary"] = fmt.Sprintf("已将目标 %s 标记为放弃。", payload.GoalID)
		return marshalToolPayload(result)
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
	BaseRegistry    *tools.Registry
	RegistryFactory func([]string) *tools.Registry
	EmitFunc        func(WSEvent)
	ParentContext   context.Context
	Memory          *WorkingMemory
	Subgoals        *SubgoalManager
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
	return "创建一个受限子代理并执行指定任务。读取 role_name、task_instruction、allowed_tools 与可选 goal_id/policy_appendix；allowed_tools 只描述子代理可见的工具边界，`user_request_input` 与 `report_finalize` 不能下放。成功时返回 child_run_id、delegate_summary、trace，失败时返回结构化错误、child_run_status 与可选 disallowed_tools。"
}

func (t *DelegateTaskTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"role_name": {
				"type": "string",
				"description": "子代理的标签，用于区分不同实例。"
			},
			"policy_appendix": {
				"type": "string",
				"description": "附加给子代理的额外约束规则。仅限于行为准则，禁止在此放入背景事实、数据样例或历史记录。"
			},
			"task_instruction": {
				"type": "string",
				"description": "子代理本次要完成的具体任务。"
			},
			"allowed_tools": {
				"type": "array",
				"items": {"type": "string"},
				"description": "子代理允许使用的工具列表。该列表不能包含 user_request_input 或 report_finalize。"
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
	if t.BaseRegistry == nil && t.RegistryFactory == nil {
		return delegateToolFailure("", "", "", nil, "", "delegate_registry_missing", "delegate base registry is not configured", nil), nil
	}

	var payload struct {
		RoleName        string   `json:"role_name"`
		PolicyAppendix  string   `json:"policy_appendix"`
		TaskInstruction string   `json:"task_instruction"`
		AllowedTools    []string `json:"allowed_tools"`
		GoalID          string   `json:"goal_id"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return delegateToolFailure("", "", "", nil, "", "invalid_arguments", fmt.Sprintf("invalid arguments: %v", err), nil), nil
	}
	if strings.TrimSpace(payload.RoleName) == "" {
		return delegateToolFailure("", "", payload.TaskInstruction, payload.AllowedTools, payload.GoalID, "missing_role_name", "role_name is required", nil), nil
	}
	if strings.TrimSpace(payload.TaskInstruction) == "" {
		return delegateToolFailure("", payload.RoleName, "", payload.AllowedTools, payload.GoalID, "missing_task_instruction", "task_instruction is required", nil), nil
	}
	if len(payload.AllowedTools) == 0 {
		return delegateToolFailure("", payload.RoleName, payload.TaskInstruction, nil, payload.GoalID, "missing_allowed_tools", "allowed_tools is required", nil), nil
	}
	if forbidden := disallowedDelegateTools(payload.AllowedTools); len(forbidden) > 0 {
		return delegateToolFailure("", payload.RoleName, payload.TaskInstruction, payload.AllowedTools, payload.GoalID, "disallowed_delegate_tools", "delegate 不能使用这些工具: "+strings.Join(forbidden, ", "), map[string]interface{}{
			"disallowed_tools": forbidden,
		}), nil
	}
	if err := validatePolicyAppendix(payload.PolicyAppendix); err != nil {
		return delegateToolFailure("", payload.RoleName, payload.TaskInstruction, payload.AllowedTools, payload.GoalID, "policy_appendix_invalid", err.Error(), nil), nil
	}

	subReg := t.buildDelegateRegistry(payload.AllowedTools)
	if len(subReg.ListTools()) == 0 {
		return delegateToolFailure("", payload.RoleName, payload.TaskInstruction, payload.AllowedTools, payload.GoalID, "no_allowed_tools_resolved", "no allowed tools resolved for delegate", nil), nil
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
			return delegateToolFailure("", payload.RoleName, payload.TaskInstruction, payload.AllowedTools, payload.GoalID, "child_run_start_failed", fmt.Sprintf("failed to start child run: %v", err), nil), nil
		}
	}
	childCtx := ctx
	if childRunID != "" {
		meta := TraceMetadataFromContext(ctx)
		meta.RunID = childRunID
		meta.TraceID = childRunID
		childCtx = WithTraceMetadata(ctx, meta)
	}
	childEmit := func(ev WSEvent) {
		if strings.TrimSpace(childRunID) != "" && strings.TrimSpace(ev.RunID) == "" {
			ev.RunID = childRunID
		}
		emit(ev)
	}
	prepareRegistryRuntimeTools(subReg, childCtx, childEmit)

	if t.Subgoals != nil && payload.GoalID != "" {
		if err := t.Subgoals.UpdateGoalStatus(payload.GoalID, StatusRunning, ""); err == nil {
			childEmit(WSEvent{
				Type: EventStateSubgoalsUpdated,
				Data: map[string]interface{}{"goals": t.Subgoals.ListAll()},
			})
		}
	}

	childEmit(WSEvent{Type: EventThinking, Data: ThinkingData{Content: fmt.Sprintf("[%s 启动] 已派生子 Agent，工具边界: %s", payload.RoleName, strings.Join(payload.AllowedTools, ", "))}})

	childPrompt := BuildPolicyPrompt()
	if extra := strings.TrimSpace(payload.PolicyAppendix); extra != "" {
		childPrompt = childPrompt + "\n\n## 本次派生附加约束\n" + extra
	}

	llmClient := NewLLMClient()
	bundle := &PromptBundle{
		Policy: childPrompt,
		Task:   payload.TaskInstruction,
	}
	oaiTools := subReg.GetOpenAITools()
	trace := make([]delegateTraceItem, 0, 12)

	const maxWorkerIterations = 25
	totalPromptTokens := 0
	totalCompletionTokens := 0
	for i := 0; i < maxWorkerIterations; i++ {
		if childCtx.Err() != nil {
			if t.Subgoals != nil && payload.GoalID != "" {
				_ = t.Subgoals.UpdateGoalStatus(payload.GoalID, StatusPending, "任务被取消")
				childEmit(WSEvent{
					Type: EventStateSubgoalsUpdated,
					Data: map[string]interface{}{"goals": t.Subgoals.ListAll()},
				})
			}
			if persistence != nil && childRunID != "" {
				cancelMsg := "任务被取消"
				// 使用独立的 Background context 确保取消后仍能写入 DB
				bgCtx := context.Background()
				_ = persistence.UpdateChildRunStatus(bgCtx, childRunID, string(domain.RunStatusCancelled), &cancelMsg)
				// 补充 ISSUE5 修复：取消时也记录已累计的 token 消耗
				_ = persistence.UpdateChildRunTokens(bgCtx, childRunID, totalPromptTokens, totalCompletionTokens)
			}
			return "", childCtx.Err()
		}

		resp, err := llmClient.ChatWithTools(childCtx, bundle, oaiTools)
		if err == nil {
			// 累计 token 消耗，并触发上下文压缩
			totalPromptTokens += resp.Usage.PromptTokens
			totalCompletionTokens += resp.Usage.CompletionTokens

			if bundle.Task != "" {
				bundle.History = append(bundle.History, ConversationItem{
					Role:    openai.ChatMessageRoleUser,
					Content: bundle.Task,
				})
				bundle.Task = ""
			}
			// ISSUE6 注释：
			// 注意，这里的 compactWorkerBundle 逻辑与 engine.go 中的 compactMessagesLocked 不同。
			// 主代理由于需要维护长期存在的 Session，会执行较激进的压缩（提炼 Digest、保留少数最近的对话）；
			// 而子代理 (worker) 的生命周期较短，关注点更内聚。为了避免中间过程的细节事实在频繁提炼中丢失，
			// 我们通常采取较温和的滑动窗口策略（例如仅截断旧轮次，而不强制使用大范围摘要压缩）。
			compactWorkerBundle(bundle, resp.Usage.PromptTokens)
		}
		if err != nil {
			if t.Subgoals != nil && payload.GoalID != "" {
				_ = t.Subgoals.UpdateGoalStatus(payload.GoalID, StatusPending, err.Error())
				childEmit(WSEvent{
					Type: EventStateSubgoalsUpdated,
					Data: map[string]interface{}{"goals": t.Subgoals.ListAll()},
				})
			}
			if persistence != nil && childRunID != "" {
				msg := err.Error()
				_ = persistence.UpdateChildRunStatus(childCtx, childRunID, string(domain.RunStatusFailed), &msg)
				_ = persistence.UpdateChildRunTokens(childCtx, childRunID, totalPromptTokens, totalCompletionTokens)
			}
			return delegateToolFailure(childRunID, payload.RoleName, payload.TaskInstruction, payload.AllowedTools, payload.GoalID, "delegate_execution_failed", fmt.Sprintf("delegated agent failed: %v", err), map[string]interface{}{
				"child_run_status": string(domain.RunStatusFailed),
			}), nil
		}
		if len(resp.Choices) == 0 {
			if persistence != nil && childRunID != "" {
				msg := "delegated agent returned no response"
				_ = persistence.UpdateChildRunStatus(childCtx, childRunID, string(domain.RunStatusFailed), &msg)
			}
			return delegateToolFailure(childRunID, payload.RoleName, payload.TaskInstruction, payload.AllowedTools, payload.GoalID, "delegate_no_response", "delegated agent returned no response", map[string]interface{}{
				"child_run_status": string(domain.RunStatusFailed),
			}), nil
		}

		choice := resp.Choices[0]
		if choice.Message.Content != "" {
			content := strings.TrimSpace(choice.Message.Content)
			trace = append(trace, delegateTraceItem{Kind: "thinking", Summary: clipText(content, 160)})
			ev := WSEvent{Type: EventThinking, RunID: childRunID, Data: ThinkingData{Content: fmt.Sprintf("[%s 思考] %s", payload.RoleName, content)}}
			childEmit(ev)
		}

		if choice.FinishReason == openai.FinishReasonStop && len(choice.Message.ToolCalls) == 0 {
			result := strings.TrimSpace(choice.Message.Content)
			childEmit(WSEvent{Type: EventThinking, Data: ThinkingData{Content: fmt.Sprintf("[%s 完成] %s", payload.RoleName, result)}})
			childEmit(WSEvent{Type: EventRunCompleted, RunID: childRunID, Data: CompleteData{Summary: result}})
			if persistence != nil && childRunID != "" {
				_ = persistence.UpdateChildRunSummary(childCtx, childRunID, result)
				_ = persistence.UpdateChildRunStatus(childCtx, childRunID, string(domain.RunStatusCompleted), nil)
				_ = persistence.UpdateChildRunTokens(childCtx, childRunID, totalPromptTokens, totalCompletionTokens)
			}
			return delegateToolSuccess(childRunID, payload.RoleName, payload.TaskInstruction, payload.AllowedTools, payload.GoalID, result, trace), nil
		}

		if len(choice.Message.ToolCalls) == 0 {
			continue
		}

		bundle.History = append(bundle.History, compactAssistantMessage(choice.Message))
		for _, toolCall := range choice.Message.ToolCalls {
			trace = append(trace, delegateTraceItem{
				Kind:    "tool_call",
				Summary: fmt.Sprintf("%s(%s)", toolCall.Function.Name, clipText(toolCall.Function.Arguments, 120)),
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
			childEmit(callEv)

			start := time.Now()
			// 修复 #13：子代理工具调用与主代理保持一致，走 retryableToolExec 指数退避重试
			result, execErr := retryableToolExec(childCtx, subReg, toolCall.Function.Name, json.RawMessage(toolCall.Function.Arguments))
			duration := time.Since(start).Milliseconds()

			if execErr != nil {
				trace = append(trace, delegateTraceItem{
					Kind:    "tool_error",
					Summary: fmt.Sprintf("%s failed: %s", toolCall.Function.Name, clipText(execErr.Error(), 160)),
				})
				resultEv := WSEvent{Type: EventToolResult, RunID: childRunID, Data: ToolResultData{
					ID:       toolCall.ID,
					Name:     toolCall.Function.Name,
					Result:   delegateChildToolFailure(toolCall.Function.Name, execErr.Error()),
					Success:  false,
					Duration: duration,
				}}
				childEmit(resultEv)
				bundle.History = append(bundle.History, ConversationItem{
					Role:       openai.ChatMessageRoleTool,
					Content:    delegateChildToolFailure(toolCall.Function.Name, execErr.Error()),
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
			childEmit(resultEv)
			bundle.History = append(bundle.History, ConversationItem{
				Role:       openai.ChatMessageRoleTool,
				Content:    result,
				ToolCallID: toolCall.ID,
			})
		}
	}

	if persistence != nil && childRunID != "" {
		msg := fmt.Sprintf("delegated agent %s max iterations reached", payload.RoleName)
		_ = persistence.UpdateChildRunStatus(childCtx, childRunID, string(domain.RunStatusFailed), &msg)
		_ = persistence.UpdateChildRunTokens(childCtx, childRunID, totalPromptTokens, totalCompletionTokens)
	}
	return delegateToolFailure(childRunID, payload.RoleName, payload.TaskInstruction, payload.AllowedTools, payload.GoalID, "delegate_max_iterations_reached", fmt.Sprintf("delegated agent %s max iterations reached", payload.RoleName), map[string]interface{}{
		"child_run_status": string(domain.RunStatusFailed),
	}), nil
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
		"child_run_status": string(domain.RunStatusCompleted),
		"ui_summary":       fmt.Sprintf("子 Agent %s 已完成: %s", roleName, summary),
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

func delegateToolFailure(childRunID, roleName, taskInstruction string, allowedTools []string, goalID, code, message string, extra map[string]interface{}) string {
	payload := map[string]interface{}{
		"ok":               false,
		"tool":             "task_delegate",
		"error_code":       code,
		"message":          message,
		"delegate_role":    roleName,
		"task_instruction": taskInstruction,
		"allowed_tools":    allowedTools,
		"ui_summary":       fmt.Sprintf("子 Agent %s 执行失败。", roleName),
	}
	if strings.TrimSpace(childRunID) != "" {
		payload["child_run_id"] = childRunID
	}
	if strings.TrimSpace(goalID) != "" {
		payload["goal_id"] = goalID
	}
	if strings.TrimSpace(roleName) == "" {
		payload["ui_summary"] = "子 Agent 执行失败。"
	}
	for key, value := range extra {
		payload[key] = value
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return `{"ok":false,"tool":"task_delegate","error_code":"delegate_response_marshal_failed","message":"delegate response marshal failed"}`
	}
	return string(encoded)
}

func (t *DelegateTaskTool) buildDelegateRegistry(allowed []string) *tools.Registry {
	if t.RegistryFactory != nil {
		return t.RegistryFactory(allowed)
	}
	if t.BaseRegistry == nil {
		return tools.NewRegistry()
	}
	return t.BaseRegistry.CloneFiltered(allowed)
}

func prepareRegistryRuntimeTools(reg *tools.Registry, ctx context.Context, emit func(WSEvent)) {
	if reg == nil {
		return
	}
	for _, tool := range reg.ListTools() {
		if next, ok := tool.(eventEmitterAware); ok {
			next.SetEventEmitter(emit)
		}
		if next, ok := tool.(executionContextAware); ok {
			next.SetExecutionContext(ctx)
		}
	}
}

func validatePolicyAppendix(raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	if len([]rune(trimmed)) > 280 {
		return fmt.Errorf("policy_appendix 过长；它只能包含简短约束，不能作为上下文转储")
	}
	lines := strings.Split(trimmed, "\n")
	nonEmpty := 0
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			nonEmpty++
		}
	}
	if nonEmpty > 6 {
		return fmt.Errorf("policy_appendix 行数过多；它只能包含少量约束规则")
	}
	lower := strings.ToLower(trimmed)
	suspiciousTokens := []string{
		"```", "{\"", "\"tool\"", "|---", "context:", "history:", "runtime:", "schema:",
		"背景：", "背景:", "上下文：", "上下文:", "历史：", "历史:", "已知事实", "用户原话", "表结构", "列如下",
	}
	for _, token := range suspiciousTokens {
		if strings.Contains(lower, strings.ToLower(token)) {
			return fmt.Errorf("policy_appendix 只能写约束，不能包含背景事实、历史记录或结构化上下文转储")
		}
	}
	return nil
}

func disallowedDelegateTools(allowed []string) []string {
	disallowedSet := map[string]struct{}{
		"user_request_input": {},
		"report_finalize":    {},
		"task_delegate":      {},
	}
	var forbidden []string
	for _, name := range allowed {
		trimmed := strings.TrimSpace(name)
		if _, ok := disallowedSet[trimmed]; ok {
			forbidden = append(forbidden, trimmed)
		}
	}
	return forbidden
}

func delegateChildToolFailure(toolName, message string) string {
	payload := map[string]interface{}{
		"ok":         false,
		"tool":       toolName,
		"error_code": "execution_error",
		"message":    message,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Sprintf(`{"ok":false,"tool":"%s","error_code":"execution_error","message":"%s"}`, toolName, clipText(message, 120))
	}
	return string(encoded)
}

func clipDelegateToolResult(raw string, max int) string {
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &payload); err == nil {
		if summary, ok := payload["ui_summary"].(string); ok && strings.TrimSpace(summary) != "" {
			return clipText(summary, max)
		}
		if message, ok := payload["message"].(string); ok && strings.TrimSpace(message) != "" {
			return clipText(message, max)
		}
		encoded, _ := json.Marshal(payload)
		return clipText(string(encoded), max)
	}
	return clipText(raw, max)
}

// clipText 已迁移至 stringutil.go clipText
