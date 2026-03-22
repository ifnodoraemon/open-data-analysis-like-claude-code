package agent

import (
	"encoding/json"
	"regexp"
	"strings"
)

var (
	// 匹配内联事件属性，如 onclick="..." onerror='...' onload=foo
	htmlEventAttrRe = regexp.MustCompile(`(?i)\s+on[a-z]+\s*=\s*(?:"[^"]*"|'[^']*'|[^\s>]*)`)

	// 匹配 href/src 中的 javascript: 协议（含空白变体）
	htmlJavascriptHrefRe = regexp.MustCompile(`(?i)((?:href|src)\s*=\s*["'])\s*javascript:[^"']*`)
)

// sanitizeReportHTML 对报告 HTML 做服务端保守清洗，移除明显的 XSS 载体。
// 策略与前端 sanitize.js 对应：
//   - 保留 <script>/<style>（ECharts 图表渲染依赖）
//   - 移除所有内联事件属性（on*=...）
//   - 将 javascript: 协议的 href/src 值置空
func sanitizeReportHTML(html string) string {
	// 1. 移除内联事件属性
	html = htmlEventAttrRe.ReplaceAllString(html, "")

	// 2. 清除 javascript: 协议链接（保留属性名称和引号，仅清空值）
	html = htmlJavascriptHrefRe.ReplaceAllStringFunc(html, func(match string) string {
		// 找到开头引号的位置，保留 `href="` 或 `src='`，截断后面的值
		idx := strings.IndexAny(match, `"'`)
		if idx < 0 {
			return match
		}
		// 返回 `href="` 部分（不含 javascript:...），后续引号由原始 HTML 的下一个 token 提供
		return match[:idx+1]
	})

	return html
}

// applyReportHTMLGuardrail 从工具返回的 JSON 结果中提取 html 字段，
// 对其进行 sanitize 后重新序列化回去。
// 若 JSON 解析失败或无 html 字段，原样返回。
func applyReportHTMLGuardrail(result string) string {
	trimmed := strings.TrimSpace(result)
	if trimmed == "" {
		return result
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return result
	}
	html, ok := payload["html"].(string)
	if !ok || strings.TrimSpace(html) == "" {
		return result
	}
	payload["html"] = sanitizeReportHTML(html)
	sanitized, err := json.Marshal(payload)
	if err != nil {
		return result
	}
	return string(sanitized)
}
