package tools

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// ResolveReportTitle 从章节中解析报告标题
func ResolveReportTitle(sections []ReportSection, fallback string) string {
	for _, sec := range sections {
		if sec.Type == "title" && sec.Title != "" {
			return sec.Title
		}
	}
	return fallback
}

// RenderReportHTML 生成完整的研报 HTML（含 ECharts 图表支持）
func RenderReportHTML(title, author string, state *ReportState) string {
	if state == nil {
		state = &ReportState{}
	}
	if title == "" {
		title = ResolveReportTitle(state.Sections, "数据分析报告")
	}
	if author == "" {
		author = "AI 数据分析师"
	}

	now := time.Now().Format("2006年01月02日")

	var tocHTML strings.Builder
	var bodyHTML strings.Builder
	chapterNum := 0

	for _, sec := range state.Sections {
		switch sec.Type {
		case "title":
			continue
		case "summary":
			chapterNum++
			tocHTML.WriteString(fmt.Sprintf(`<li><a href="#section-%d">%s</a></li>`, chapterNum, sec.Title))
			bodyHTML.WriteString(fmt.Sprintf(`
				<div class="section summary" id="section-%d">
					<h2>%s</h2>
					<div class="summary-box">%s</div>
				</div>`, chapterNum, sec.Title, processContent(sec.Content, state.Charts)))
		case "overview", "analysis":
			chapterNum++
			tocHTML.WriteString(fmt.Sprintf(`<li><a href="#section-%d">%s</a></li>`, chapterNum, sec.Title))
			bodyHTML.WriteString(fmt.Sprintf(`
				<div class="section" id="section-%d">
					<h2>%d. %s</h2>
					<div class="content">%s</div>
				</div>`, chapterNum, chapterNum, sec.Title, processContent(sec.Content, state.Charts)))
		case "chart":
			chapterNum++
			tocHTML.WriteString(fmt.Sprintf(`<li><a href="#section-%d">%s</a></li>`, chapterNum, sec.Title))
			bodyHTML.WriteString(fmt.Sprintf(`
				<div class="section chart-section" id="section-%d">
					<h2>%d. %s</h2>
					<div class="content">%s</div>
				</div>`, chapterNum, chapterNum, sec.Title, processContent(sec.Content, state.Charts)))
		case "conclusion":
			chapterNum++
			tocHTML.WriteString(fmt.Sprintf(`<li><a href="#section-%d">%s</a></li>`, chapterNum, sec.Title))
			bodyHTML.WriteString(fmt.Sprintf(`
				<div class="section conclusion" id="section-%d">
					<h2>%d. %s</h2>
					<div class="conclusion-box">%s</div>
				</div>`, chapterNum, chapterNum, sec.Title, processContent(sec.Content, state.Charts)))
		}
	}

	// 生成图表初始化脚本
	var chartScripts strings.Builder
	if len(state.Charts) > 0 {
		chartScripts.WriteString("<script>\ndocument.addEventListener('DOMContentLoaded', function() {\n")
		for _, ch := range state.Charts {
			optionStr := string(ch.Option)
			if optionStr == "" || optionStr == "null" {
				optionStr = "{}"
			}
			chartScripts.WriteString(fmt.Sprintf(`
  (function() {
    var el = document.getElementById('%s');
    if (el) {
      try {
        var chart = echarts.init(el);
        var option = %s;
        if (option && typeof option === 'object' && Object.keys(option).length > 0) {
          if (!option.tooltip) option.tooltip = {trigger: 'axis'};
          if (!option.grid) option.grid = {left:'3%%',right:'4%%',bottom:'3%%',containLabel:true};
          chart.setOption(option);
          window.addEventListener('resize', function() { chart.resize(); });
        } else {
          el.innerHTML = '<div style="display:flex;align-items:center;justify-content:center;height:100%%;color:#999;font-size:14px;">图表数据为空</div>';
        }
      } catch(e) {
        el.innerHTML = '<div style="display:flex;align-items:center;justify-content:center;height:100%%;color:#e53e3e;font-size:14px;">图表渲染失败: ' + e.message + '</div>';
      }
    }
  })();
`, ch.ID, optionStr))
		}
		chartScripts.WriteString("});\n</script>")
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>%s</title>
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&family=Noto+Sans+SC:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
:root {
  --primary: #0f2b46;
  --primary-light: #1a4a7a;
  --primary-soft: #e8f0fe;
  --accent: #e8a838;
  --accent-light: #fef3c7;
  --text: #1e293b;
  --text-light: #64748b;
  --text-muted: #94a3b8;
  --bg: #ffffff;
  --bg-alt: #f8fafc;
  --bg-warm: #fffbf0;
  --border: #e2e8f0;
  --border-light: #f1f5f9;
  --shadow-sm: 0 1px 3px rgba(0,0,0,0.06), 0 1px 2px rgba(0,0,0,0.04);
  --shadow-md: 0 4px 12px rgba(0,0,0,0.08), 0 2px 4px rgba(0,0,0,0.04);
  --shadow-lg: 0 10px 30px rgba(0,0,0,0.1), 0 4px 8px rgba(0,0,0,0.05);
  --radius: 12px;
  --radius-sm: 8px;
}
* { margin: 0; padding: 0; box-sizing: border-box; }
body {
  font-family: 'Inter', 'Noto Sans SC', 'PingFang SC', -apple-system, sans-serif;
  color: var(--text);
  line-height: 1.85;
  background: var(--bg-alt);
  -webkit-font-smoothing: antialiased;
}

/* === 封面 === */
.cover {
  min-height: 70vh;
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: center;
  background: linear-gradient(160deg, var(--primary) 0%%, var(--primary-light) 50%%, #2563eb 100%%);
  color: white;
  text-align: center;
  padding: 5rem 2rem;
  position: relative;
  overflow: hidden;
  page-break-after: always;
}
.cover::before {
  content: '';
  position: absolute;
  top: -50%%; left: -50%%;
  width: 200%%; height: 200%%;
  background: radial-gradient(circle at 30%% 70%%, rgba(255,255,255,0.06) 0%%, transparent 50%%),
              radial-gradient(circle at 70%% 30%%, rgba(232,168,56,0.1) 0%%, transparent 40%%);
  animation: coverShift 20s ease-in-out infinite alternate;
}
@keyframes coverShift {
  0%% { transform: translate(0, 0) rotate(0deg); }
  100%% { transform: translate(-2%%, 2%%) rotate(1deg); }
}
.cover h1 {
  font-size: 2.8rem;
  margin-bottom: 1rem;
  font-weight: 700;
  letter-spacing: -0.02em;
  position: relative;
  z-index: 1;
  text-shadow: 0 2px 20px rgba(0,0,0,0.15);
}
.cover .subtitle {
  font-size: 1.15rem;
  opacity: 0.8;
  max-width: 600px;
  position: relative;
  z-index: 1;
}
.cover .divider {
  width: 60px;
  height: 3px;
  background: linear-gradient(90deg, var(--accent), #f7c948);
  margin: 2rem auto;
  border-radius: 2px;
  position: relative;
  z-index: 1;
}
.cover .meta {
  font-size: 1rem;
  opacity: 0.75;
  position: relative;
  z-index: 1;
  display: flex;
  gap: 2rem;
  align-items: center;
}
.cover .meta span {
  display: flex;
  align-items: center;
  gap: 0.4rem;
}

/* === 目录 === */
.toc {
  max-width: 780px;
  margin: 3rem auto;
  padding: 2.5rem;
  background: var(--bg);
  border-radius: var(--radius);
  box-shadow: var(--shadow-md);
  page-break-after: always;
}
.toc h2 {
  color: var(--primary);
  font-size: 1.35rem;
  font-weight: 700;
  padding-bottom: 0.75rem;
  margin-bottom: 1.5rem;
  border-bottom: 3px solid var(--accent);
  display: flex;
  align-items: center;
  gap: 0.5rem;
}
.toc h2::before { content: '📑'; }
.toc ol { counter-reset: toc; padding-left: 0; }
.toc li {
  list-style: none;
  counter-increment: toc;
  padding: 0.7rem 0.5rem;
  border-bottom: 1px solid var(--border-light);
  transition: all 0.2s ease;
  border-radius: 6px;
}
.toc li:hover { background: var(--bg-alt); padding-left: 1rem; }
.toc li::before {
  content: '0' counter(toc);
  color: var(--accent);
  font-weight: 700;
  font-size: 0.85rem;
  margin-right: 0.75rem;
  display: inline-block;
  min-width: 1.8rem;
}
.toc li:nth-child(n+10)::before { content: counter(toc); }
.toc a { color: var(--text); text-decoration: none; font-weight: 500; }
.toc a:hover { color: var(--primary-light); }

/* === 章节 === */
.section {
  max-width: 780px;
  margin: 1.5rem auto;
  padding: 2.5rem;
  background: var(--bg);
  border-radius: var(--radius);
  box-shadow: var(--shadow-sm);
  transition: box-shadow 0.3s ease;
}
.section:hover { box-shadow: var(--shadow-md); }
.section h2 {
  color: var(--primary);
  font-size: 1.4rem;
  font-weight: 700;
  margin-bottom: 1.25rem;
  padding-bottom: 0.75rem;
  border-bottom: 2px solid var(--border);
  position: relative;
}
.section h2::after {
  content: '';
  position: absolute;
  bottom: -2px;
  left: 0;
  width: 60px;
  height: 2px;
  background: linear-gradient(90deg, var(--accent), transparent);
}
.content p {
  margin-bottom: 1.1rem;
  text-indent: 2em;
  color: var(--text);
  font-size: 0.95rem;
}
.content h4 {
  color: var(--primary);
  font-size: 1.05rem;
  font-weight: 600;
  margin: 1.5rem 0 0.75rem;
  padding-left: 0.75rem;
  border-left: 3px solid var(--accent);
}
.content ul {
  margin: 0.75rem 0;
  padding-left: 1.5rem;
}
.content li {
  margin-bottom: 0.4rem;
  font-size: 0.95rem;
  color: var(--text);
}

/* === 摘要框 === */
.summary-box {
  background: linear-gradient(135deg, var(--bg-warm) 0%%, var(--accent-light) 100%%);
  border-left: 4px solid var(--accent);
  padding: 1.75rem 2rem;
  border-radius: 0 var(--radius-sm) var(--radius-sm) 0;
  margin: 1rem 0;
  font-size: 0.95rem;
  line-height: 1.9;
  box-shadow: var(--shadow-sm);
}

/* === 结论框 === */
.conclusion-box {
  background: linear-gradient(135deg, #f0f9ff 0%%, #ecfdf5 100%%);
  border: 1px solid #bae6fd;
  border-left: 4px solid #0ea5e9;
  padding: 1.75rem 2rem;
  border-radius: var(--radius-sm);
  margin: 1rem 0;
  box-shadow: var(--shadow-sm);
}

/* === 图表 === */
.chart-box {
  width: 100%%;
  height: 420px;
  margin: 1.5rem 0;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  background: var(--bg);
  box-shadow: var(--shadow-sm);
  transition: box-shadow 0.3s ease;
}
.chart-box:hover { box-shadow: var(--shadow-md); }

/* === 表格 === */
table {
  width: 100%%;
  border-collapse: separate;
  border-spacing: 0;
  margin: 1.25rem 0;
  font-size: 0.88rem;
  border-radius: var(--radius-sm);
  overflow: hidden;
  box-shadow: var(--shadow-sm);
}
th {
  background: linear-gradient(135deg, var(--primary) 0%%, var(--primary-light) 100%%);
  color: white;
  padding: 0.85rem 1rem;
  text-align: left;
  font-weight: 600;
  font-size: 0.82rem;
  text-transform: uppercase;
  letter-spacing: 0.03em;
}
td {
  padding: 0.7rem 1rem;
  border-bottom: 1px solid var(--border-light);
  transition: background 0.15s ease;
}
tr:nth-child(even) { background: var(--bg-alt); }
tr:hover td { background: var(--primary-soft); }
strong { color: var(--primary); font-weight: 600; }

/* === 页脚 === */
.footer {
  max-width: 780px;
  margin: 2rem auto 3rem;
  padding: 2rem;
  text-align: center;
  color: var(--text-muted);
  font-size: 0.82rem;
  position: relative;
}
.footer::before {
  content: '';
  display: block;
  width: 60px;
  height: 2px;
  background: linear-gradient(90deg, transparent, var(--border), transparent);
  margin: 0 auto 1.5rem;
}
.footer p { line-height: 1.6; }

/* === 打印样式 === */
@media print {
  body { background: white; }
  .cover { min-height: 100vh; }
  .section { box-shadow: none; page-break-inside: avoid; margin: 1rem auto; }
  .toc { box-shadow: none; }
  .chart-box { box-shadow: none; }
  table { box-shadow: none; }
}
/* === 响应式 === */
@media (max-width: 860px) {
  .section, .toc, .footer { margin-left: 1rem; margin-right: 1rem; }
  .cover h1 { font-size: 2rem; }
}
</style>
<script src="https://cdn.jsdelivr.net/npm/echarts@5/dist/echarts.min.js"></script>
</head>
<body>
<div class="cover">
  <h1>%s</h1>
  <div class="divider"></div>
  <div class="meta">
    <span>📊 %s</span>
    <span>📅 %s</span>
  </div>
</div>
<div class="toc">
  <h2>目录</h2>
  <ol>%s</ol>
</div>
%s
<div class="footer">
  <p>本报告由 AI 数据分析智能体自动生成 | %s</p>
</div>
%s
</body>
</html>`, title, title, author, now, tocHTML.String(), bodyHTML.String(), now, chartScripts.String())
}

// processContent 处理内容：Markdown 转 HTML + 替换图表占位符
func processContent(content string, charts []ChartData) string {
	html := markdownToHTML(content)

	// 替换 {{chart:chart_id}} 占位符为 ECharts 容器
	re := regexp.MustCompile(`\{\{chart:(\w+)\}\}`)
	html = re.ReplaceAllStringFunc(html, func(match string) string {
		chartID := re.FindStringSubmatch(match)[1]
		// 查找对应图表
		for _, ch := range charts {
			if ch.ID == chartID {
				height := ch.Height
				if height == "" {
					height = "400px"
				}
				return fmt.Sprintf(`<div id="%s" class="chart-box" style="height:%s;"></div>`, ch.ID, height)
			}
		}
		return fmt.Sprintf(`<div class="chart-box" style="display:flex;align-items:center;justify-content:center;color:#999;">图表 %s 未找到</div>`, chartID)
	})

	return html
}

// markdownToHTML 简单的 Markdown → HTML 转换
func markdownToHTML(md string) string {
	lines := strings.Split(md, "\n")
	var html strings.Builder
	inList := false
	inTable := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if inList {
				html.WriteString("</ul>\n")
				inList = false
			}
			if inTable {
				html.WriteString("</tbody></table>\n")
				inTable = false
			}
			continue
		}

		// 表格
		if strings.Contains(trimmed, "|") && !inTable {
			html.WriteString("<table><thead><tr>")
			cells := strings.Split(strings.Trim(trimmed, "|"), "|")
			for _, cell := range cells {
				html.WriteString(fmt.Sprintf("<th>%s</th>", strings.TrimSpace(cell)))
			}
			html.WriteString("</tr></thead><tbody>\n")
			inTable = true
			continue
		}
		if inTable && strings.Contains(trimmed, "---") {
			continue
		}
		if inTable && strings.Contains(trimmed, "|") {
			html.WriteString("<tr>")
			cells := strings.Split(strings.Trim(trimmed, "|"), "|")
			for _, cell := range cells {
				html.WriteString(fmt.Sprintf("<td>%s</td>", strings.TrimSpace(cell)))
			}
			html.WriteString("</tr>\n")
			continue
		}

		// 列表
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			if !inList {
				html.WriteString("<ul>\n")
				inList = true
			}
			html.WriteString(fmt.Sprintf("<li>%s</li>\n", formatInline(trimmed[2:])))
			continue
		}

		// 标题
		if strings.HasPrefix(trimmed, "### ") {
			html.WriteString(fmt.Sprintf("<h4>%s</h4>\n", trimmed[4:]))
			continue
		}

		// 图表占位符 - 保持原样，让 processContent 处理
		if strings.Contains(trimmed, "{{chart:") {
			html.WriteString(trimmed + "\n")
			continue
		}

		html.WriteString(fmt.Sprintf("<p>%s</p>\n", formatInline(trimmed)))
	}

	if inList {
		html.WriteString("</ul>\n")
	}
	if inTable {
		html.WriteString("</tbody></table>\n")
	}

	return html.String()
}

// formatInline 处理行内格式（加粗）
func formatInline(text string) string {
	for strings.Contains(text, "**") {
		text = strings.Replace(text, "**", "<strong>", 1)
		text = strings.Replace(text, "**", "</strong>", 1)
	}
	return text
}
