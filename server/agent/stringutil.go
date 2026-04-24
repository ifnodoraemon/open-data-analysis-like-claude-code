package agent

import (
	"encoding/json"
	"strings"
)

// clipText 将输入截断到最多 max 个 Unicode 码位，末尾加省略标记。
// 仅用于日志/调试输出场景（允许丢信息）；LLM 历史摘要请使用 digestSummary。
func clipText(input string, max int) string {
	input = strings.TrimSpace(input)
	if input == "" || max <= 0 {
		return input
	}
	runes := []rune(input)
	if len(runes) <= max {
		return input
	}
	return string(runes[:max]) + "...(truncated)"
}

// digestSummary 为 LLM 历史摘要提取最有价值的文本片段，避免无意义的硬截断。
//
// 策略（按优先级）：
//  1. 若文本可解析为 JSON 且含 ui_summary / message / result 字段，
//     直接返回该字段（工具刻意设计为简短摘要，不应再截断）。
//  2. 若输入为多段文字，取最后一段非空内容（assistant 可见状态文本的结论往往在末尾）。
//  3. 回退：对最后一段做截断。
func digestSummary(input string, max int) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return ""
	}

	// 1. 尝试解析 JSON，优先使用语义摘要字段
	if len(trimmed) > 0 && trimmed[0] == '{' {
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(trimmed), &payload); err == nil {
			for _, key := range []string{"ui_summary", "message", "result"} {
				if v, ok := payload[key].(string); ok && strings.TrimSpace(v) != "" {
					return strings.TrimSpace(v)
				}
			}
		}
	}

	// 2. 取最后一段非空内容（适合 assistant 可见状态文本，结论通常在末尾）
	paragraphs := strings.Split(trimmed, "\n")
	for i := len(paragraphs) - 1; i >= 0; i-- {
		para := strings.TrimSpace(paragraphs[i])
		if para == "" {
			continue
		}
		runes := []rune(para)
		if max > 0 && len(runes) > max {
			return string(runes[:max]) + "...(truncated)"
		}
		return para
	}

	// 3. 回退截断
	return clipText(trimmed, max)
}
