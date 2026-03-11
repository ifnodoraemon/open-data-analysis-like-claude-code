package tools

import (
	"fmt"
	"strings"
	"time"
)

// generateReportHTML 生成完整的研报 HTML
func generateReportHTML(title, author string, sections []ReportSection) string {
	now := time.Now().Format("2006年01月02日")

	var tocHTML strings.Builder
	var bodyHTML strings.Builder
	chapterNum := 0

	for _, sec := range sections {
		switch sec.Type {
		case "title":
			// 标题在封面显示
			continue
		case "summary":
			chapterNum++
			tocHTML.WriteString(fmt.Sprintf(`<li><a href="#section-%d">%s</a></li>`, chapterNum, sec.Title))
			bodyHTML.WriteString(fmt.Sprintf(`
				<div class="section summary" id="section-%d">
					<h2>%s</h2>
					<div class="summary-box">%s</div>
				</div>`, chapterNum, sec.Title, markdownToHTML(sec.Content)))
		case "overview", "analysis":
			chapterNum++
			tocHTML.WriteString(fmt.Sprintf(`<li><a href="#section-%d">%s</a></li>`, chapterNum, sec.Title))
			bodyHTML.WriteString(fmt.Sprintf(`
				<div class="section" id="section-%d">
					<h2>%d. %s</h2>
					<div class="content">%s</div>
				</div>`, chapterNum, chapterNum, sec.Title, markdownToHTML(sec.Content)))
		case "chart":
			chapterNum++
			tocHTML.WriteString(fmt.Sprintf(`<li><a href="#section-%d">%s</a></li>`, chapterNum, sec.Title))
			bodyHTML.WriteString(fmt.Sprintf(`
				<div class="section chart-section" id="section-%d">
					<h2>%d. %s</h2>
					<div class="chart-container">%s</div>
				</div>`, chapterNum, chapterNum, sec.Title, sec.Content))
		case "conclusion":
			chapterNum++
			tocHTML.WriteString(fmt.Sprintf(`<li><a href="#section-%d">%s</a></li>`, chapterNum, sec.Title))
			bodyHTML.WriteString(fmt.Sprintf(`
				<div class="section conclusion" id="section-%d">
					<h2>%d. %s</h2>
					<div class="conclusion-box">%s</div>
				</div>`, chapterNum, chapterNum, sec.Title, markdownToHTML(sec.Content)))
		}
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>%s</title>
<style>
:root {
  --primary: #1a365d;
  --primary-light: #2a4a7f;
  --accent: #d69e2e;
  --text: #2d3748;
  --text-light: #718096;
  --bg: #ffffff;
  --bg-alt: #f7fafc;
  --border: #e2e8f0;
}
* { margin: 0; padding: 0; box-sizing: border-box; }
body {
  font-family: 'Noto Sans SC', 'PingFang SC', -apple-system, sans-serif;
  color: var(--text);
  line-height: 1.8;
  background: var(--bg);
}
.cover {
  min-height: 60vh;
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: center;
  background: linear-gradient(135deg, var(--primary) 0%%, var(--primary-light) 100%%);
  color: white;
  text-align: center;
  padding: 4rem 2rem;
  page-break-after: always;
}
.cover h1 { font-size: 2.5rem; margin-bottom: 1rem; font-weight: 700; }
.cover .meta { font-size: 1.1rem; opacity: 0.85; }
.cover .meta span { margin: 0 1rem; }
.cover .divider { width: 80px; height: 3px; background: var(--accent); margin: 1.5rem auto; }
.toc {
  max-width: 800px;
  margin: 3rem auto;
  padding: 2rem;
  page-break-after: always;
}
.toc h2 { color: var(--primary); border-bottom: 2px solid var(--accent); padding-bottom: 0.5rem; margin-bottom: 1.5rem; }
.toc ol { counter-reset: toc; padding-left: 0; }
.toc li {
  list-style: none;
  counter-increment: toc;
  padding: 0.5rem 0;
  border-bottom: 1px dashed var(--border);
}
.toc li::before { content: counter(toc) ". "; color: var(--accent); font-weight: 600; }
.toc a { color: var(--text); text-decoration: none; }
.toc a:hover { color: var(--primary); }
.section {
  max-width: 800px;
  margin: 2rem auto;
  padding: 2rem;
}
.section h2 {
  color: var(--primary);
  font-size: 1.5rem;
  margin-bottom: 1rem;
  padding-bottom: 0.5rem;
  border-bottom: 2px solid var(--accent);
}
.content p { margin-bottom: 1rem; text-indent: 2em; }
.summary-box {
  background: var(--bg-alt);
  border-left: 4px solid var(--accent);
  padding: 1.5rem;
  border-radius: 0 8px 8px 0;
  margin: 1rem 0;
}
.conclusion-box {
  background: linear-gradient(135deg, #ebf8ff 0%%, #e6fffa 100%%);
  border: 1px solid #bee3f8;
  padding: 1.5rem;
  border-radius: 8px;
  margin: 1rem 0;
}
table {
  width: 100%%;
  border-collapse: collapse;
  margin: 1rem 0;
  font-size: 0.9rem;
}
th {
  background: var(--primary);
  color: white;
  padding: 0.75rem;
  text-align: left;
  font-weight: 500;
}
td { padding: 0.6rem 0.75rem; border-bottom: 1px solid var(--border); }
tr:nth-child(even) { background: var(--bg-alt); }
tr:hover { background: #edf2f7; }
.chart-container { margin: 1rem 0; min-height: 300px; }
.footer {
  max-width: 800px;
  margin: 3rem auto;
  padding: 2rem;
  text-align: center;
  color: var(--text-light);
  font-size: 0.85rem;
  border-top: 1px solid var(--border);
}
@media print {
  .cover { min-height: 100vh; }
  .section { page-break-inside: avoid; }
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
</body>
</html>`, title, title, author, now, tocHTML.String(), bodyHTML.String(), now)
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
			continue // 跳过表格分隔行
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
			html.WriteString(fmt.Sprintf("<li>%s</li>\n", trimmed[2:]))
			continue
		}

		// 标题
		if strings.HasPrefix(trimmed, "### ") {
			html.WriteString(fmt.Sprintf("<h4>%s</h4>\n", trimmed[4:]))
			continue
		}

		// 加粗
		processed := trimmed
		for strings.Contains(processed, "**") {
			processed = strings.Replace(processed, "**", "<strong>", 1)
			processed = strings.Replace(processed, "**", "</strong>", 1)
		}

		html.WriteString(fmt.Sprintf("<p>%s</p>\n", processed))
	}

	if inList {
		html.WriteString("</ul>\n")
	}
	if inTable {
		html.WriteString("</tbody></table>\n")
	}

	return html.String()
}
