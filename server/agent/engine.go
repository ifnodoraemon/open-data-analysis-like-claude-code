package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/ifnodoraemon/open-data-analysis-like-claude-code/tools"
	openai "github.com/sashabaranov/go-openai"
)

const MaxIterations = 25

// Engine Agent 主循环引擎
type Engine struct {
	llm          *LLMClient
	registry     *tools.Registry
	workerReg    *tools.Registry
	messages     []openai.ChatCompletionMessage
	systemPrompt string
	memory       *WorkingMemory
	subgoals     *SubgoalManager
	mu           sync.Mutex
}

// NewEngine 创建 Agent 引擎（支持多轮对话）
func NewEngine(registry, workerReg *tools.Registry, systemPrompt string, memory *WorkingMemory, subgoals *SubgoalManager) *Engine {
	if systemPrompt == "" {
		systemPrompt = BuildPlannerPrompt()
	}
	return &Engine{
		llm:          NewLLMClient(),
		registry:     registry,
		workerReg:    workerReg,
		systemPrompt: systemPrompt,
		memory:       memory,
		subgoals:     subgoals,
		messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: systemPrompt,
			},
		},
	}
}

func (e *Engine) ResetMessages() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.messages = []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: e.systemPrompt,
		},
	}
}

// Run 执行 Agent 主循环
func (e *Engine) Run(ctx context.Context, userInput string, emit func(WSEvent)) {
	if emit == nil {
		emit = func(WSEvent) {}
	}

	e.mu.Lock()
	// 添加用户消息
	e.messages = append(e.messages, openai.ChatCompletionMessage{
		Role:    openai.ChatMessageRoleUser,
		Content: userInput,
	})

	oaiTools := e.registry.GetOpenAITools()
	e.mu.Unlock()

	for i := 0; i < MaxIterations; i++ {
		select {
		case <-ctx.Done():
			emit(WSEvent{Type: EventRunCancelled, Data: ErrorData{Message: "任务被取消"}})
			return
		default:
		}

		// 通知前端: 正在思考
		emit(WSEvent{Type: EventThinking, Data: ThinkingData{Content: fmt.Sprintf("正在分析... (第 %d 轮)", i+1)}})

		// 调用 LLM
		e.mu.Lock()
		messages := append([]openai.ChatCompletionMessage(nil), e.messages...)
		e.mu.Unlock()
		resp, err := e.llm.ChatWithTools(ctx, messages, oaiTools)
		if err != nil {
			emit(WSEvent{Type: EventError, Data: ErrorData{Message: err.Error()}})
			return
		}

		if len(resp.Choices) == 0 {
			emit(WSEvent{Type: EventError, Data: ErrorData{Message: "LLM 返回空响应"}})
			return
		}

		choice := resp.Choices[0]

		// 有文本内容时，推送 LLM 的实际思考（而不是固定文字）
		if choice.Message.Content != "" {
			if len(choice.Message.ToolCalls) > 0 {
				// 有文本 + 有工具调用 → 推送思考内容
				emit(WSEvent{Type: EventThinking, Data: ThinkingData{Content: choice.Message.Content}})
			} else {
				// 有文本 + 无工具调用 → 最终回复
				e.mu.Lock()
				e.messages = append(e.messages, choice.Message)
				e.mu.Unlock()
				emit(WSEvent{Type: EventRunCompleted, Data: CompleteData{Summary: choice.Message.Content}})
				return
			}
		}

		// 如果 finish_reason 是 stop 且没有工具调用，结束
		if choice.FinishReason == openai.FinishReasonStop && len(choice.Message.ToolCalls) == 0 {
			e.mu.Lock()
			e.messages = append(e.messages, choice.Message)
			e.mu.Unlock()
			emit(WSEvent{Type: EventRunCompleted, Data: CompleteData{Summary: choice.Message.Content}})
			return
		}

		// 处理工具调用
		if len(choice.Message.ToolCalls) > 0 {
			// 将 assistant 消息加入历史
			e.mu.Lock()
			e.messages = append(e.messages, compactAssistantMessage(choice.Message))
			e.mu.Unlock()

			for _, toolCall := range choice.Message.ToolCalls {
				toolSpan := llmDebugWriter.StartSpan(
					TraceMetadataFromContext(ctx),
					"tool",
					toolCall.Function.Name,
					"",
					toolCall.ID,
				)

				// 通知前端: 工具调用
				emit(WSEvent{
					Type: EventToolCall,
					Data: ToolCallData{
						ID:        toolCall.ID,
						Name:      toolCall.Function.Name,
						Arguments: json.RawMessage(toolCall.Function.Arguments),
					},
				})
				argPath := llmDebugWriter.WriteBlob(toolSpan, "arguments.json", []byte(toolCall.Function.Arguments))
				llmDebugWriter.WriteEvent(toolSpan, "tool.call", map[string]interface{}{
					"tool_name":        toolCall.Function.Name,
					"tool_call_id":     toolCall.ID,
					"arguments_path":   argPath,
					"arguments_bytes":  len([]byte(toolCall.Function.Arguments)),
					"arguments_sha256": blobSHA256([]byte(toolCall.Function.Arguments)),
				})

				// 执行工具
				start := time.Now()
				
				var result string
				var execErr error
				
				if toolCall.Function.Name == "manage_subgoals" {
					var payload struct {
						Action      string `json:"action"`
						Description string `json:"description"`
					}
					_ = json.Unmarshal([]byte(toolCall.Function.Arguments), &payload)
					
					// Planner 下发了新任务
					if payload.Action == "add" && payload.Description != "" {
						emit(WSEvent{Type: EventThinking, Data: ThinkingData{Content: "Planner 正在下发查数任务给 DataWorker..."}})
						
						goalID := e.subgoals.AddGoal(payload.Description)
						
						worker := NewDataWorkerAgent(e.workerReg)
						workerRes, workerErr := worker.RunWorker(ctx, payload.Description, emit)
						
						if workerErr != nil {
							execErr = fmt.Errorf("Data Worker Failed: %v", workerErr)
							_ = e.subgoals.UpdateGoalStatus(goalID, StatusRejected, execErr.Error())
							result = fmt.Sprintf("子目标执行失败被退回: %v", workerErr)
						} else {
							_ = e.subgoals.UpdateGoalStatus(goalID, StatusComplete, workerRes)
							result = fmt.Sprintf("Data Worker 已成功完成目标[%s]，返回结论: %s", goalID, workerRes)
							emit(WSEvent{Type: EventThinking, Data: ThinkingData{Content: "DataWorker 成功提交结论返回给 Planner！"}})
						}
					} else {
						// 正常执行完成/放弃状态流转
						result, execErr = e.registry.Execute(toolCall.Function.Name, json.RawMessage(toolCall.Function.Arguments))
					}
				} else if toolCall.Function.Name == "finalize_report" {
					// 省略原版的 Review 环节以简化流程，直接结案
					emit(WSEvent{Type: EventThinking, Data: ThinkingData{Content: "开始生成最终报告..."}})
					result, execErr = e.registry.Execute(toolCall.Function.Name, json.RawMessage(toolCall.Function.Arguments))
				} else {
					result, execErr = e.registry.Execute(toolCall.Function.Name, json.RawMessage(toolCall.Function.Arguments))
				}

				duration := time.Since(start).Milliseconds()

				// If we got canceled during execution (or context ended), drop the result, abort tool loop, allow ctx.Done to catch in next loop
				if ctx.Err() != nil {
					return
				}

				success := toolCallSucceeded(result, execErr)
				if execErr != nil {
					result = fmt.Sprintf("工具执行错误: %s", execErr.Error())
					log.Printf("Tool %s error: %v", toolCall.Function.Name, execErr)
				}
				resultBytes := []byte(result)
				resultPath := llmDebugWriter.WriteBlob(toolSpan, "result.txt", resultBytes)
				llmDebugWriter.WriteEvent(toolSpan, "tool.result", map[string]interface{}{
					"tool_name":       toolCall.Function.Name,
					"tool_call_id":    toolCall.ID,
					"duration_ms":     duration,
					"success":         success,
					"result_preview":  clipText(result, 300),
					"result_bytes":    len(resultBytes),
					"result_sha256":   blobSHA256(resultBytes),
					"result_path":     resultPath,
					"execution_error": errorString(execErr),
				})

				// 通知前端: 工具结果
				emit(WSEvent{
					Type: EventToolResult,
					Data: ToolResultData{
						ID:       toolCall.ID,
						Name:     toolCall.Function.Name,
						Result:   result,
						Duration: duration,
						Success:  success,
					},
				})

				// 将工具结果加入消息历史
				e.mu.Lock()
				e.messages = append(e.messages, openai.ChatCompletionMessage{
					Role:       openai.ChatMessageRoleTool,
					Content:    compactToolResult(toolCall.Function.Name, result),
					ToolCallID: toolCall.ID,
				})
				e.mu.Unlock()
			}

			continue // 继续循环
		}

		// 默认结束
		emit(WSEvent{Type: EventRunCompleted, Data: CompleteData{Summary: "分析完成"}})
		return
	}

	emit(WSEvent{Type: EventError, Data: ErrorData{Message: "达到最大迭代次数"}})
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func compactAssistantMessage(message openai.ChatCompletionMessage) openai.ChatCompletionMessage {
	if len(message.ToolCalls) == 0 {
		return message
	}

	compacted := message
	compacted.ToolCalls = make([]openai.ToolCall, 0, len(message.ToolCalls))
	for _, toolCall := range message.ToolCalls {
		next := toolCall
		next.Function.Arguments = compactToolArguments(toolCall.Function.Name, toolCall.Function.Arguments)
		compacted.ToolCalls = append(compacted.ToolCalls, next)
	}
	return compacted
}

func compactToolArguments(toolName, raw string) string {
	if strings.TrimSpace(raw) == "" {
		return raw
	}

	switch toolName {
	case "create_chart":
		// create_chart 的参数结构会直接影响后续轮次的工具调用，
		// 这里保留原始参数，避免把摘要字段误导回模型。
		return raw
	case "write_section":
		var payload struct {
			SectionType string `json:"section_type"`
			Title       string `json:"title"`
			Content     string `json:"content"`
		}
		if err := json.Unmarshal([]byte(raw), &payload); err == nil {
			summary, _ := json.Marshal(map[string]interface{}{
				"section_type":  payload.SectionType,
				"title":         payload.Title,
				"content_note":  "compacted_for_history",
				"content_chars": len([]rune(payload.Content)),
				"content_head":  clipHistoryText(payload.Content, 120),
			})
			return string(summary)
		}
	case "finalize_report":
		var payload struct {
			ReportTitle string `json:"report_title"`
			Author      string `json:"author"`
		}
		if err := json.Unmarshal([]byte(raw), &payload); err == nil {
			summary, _ := json.Marshal(map[string]interface{}{
				"report_title": payload.ReportTitle,
				"author":       payload.Author,
			})
			return string(summary)
		}
	}

	return raw
}

func clipHistoryText(input string, max int) string {
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

func compactToolResult(toolName, result string) string {
	trimmed := strings.TrimSpace(result)
	if trimmed == "" {
		return result
	}

	switch toolName {
	case "query_data":
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(trimmed), &payload); err == nil {
			minified, _ := json.Marshal(payload)
			return string(minified)
		}
	case "describe_data":
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(trimmed), &payload); err == nil {
			minified, _ := json.Marshal(payload)
			return string(minified)
		}
	case "run_python":
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(trimmed), &payload); err == nil {
			minified, _ := json.Marshal(payload)
			return string(minified)
		}
	case "list_tables":
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(trimmed), &payload); err == nil {
			minified, _ := json.Marshal(payload)
			return string(minified)
		}
		return strings.Join(strings.Fields(trimmed), " ")
	}

	return result
}

func toolCallSucceeded(result string, execErr error) bool {
	if execErr != nil {
		return false
	}

	var payload struct {
		OK *bool `json:"ok"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(result)), &payload); err == nil && payload.OK != nil {
		return *payload.OK
	}

	return true
}

// ProvideAskUserResult 将用户的直接回复作为 ask_user 工具的执行结果注入 LLM 对话上下文
func (e *Engine) ProvideAskUserResult(userResponse string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	var toolCallID string
	for i := len(e.messages) - 1; i >= 0; i-- {
		msg := e.messages[i]
		if msg.Role == openai.ChatMessageRoleAssistant && len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				if tc.Function.Name == "ask_user" {
					toolCallID = tc.ID
					break
				}
			}
		}
		if toolCallID != "" {
			break
		}
	}

	if toolCallID == "" {
		return fmt.Errorf("没有找到正在等待的用户确认 (ask_user) 工具调用")
	}

	e.messages = append(e.messages, openai.ChatCompletionMessage{
		Role:       openai.ChatMessageRoleTool,
		Content:    userResponse,
		ToolCallID: toolCallID,
		Name:       "ask_user",
	})

	return nil
}
