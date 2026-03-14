package tools

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

func ResolveReportTitleFromState(state *ReportState, fallback string) string {
	if state == nil {
		return fallback
	}
	return resolveReportTitleFromBlocks(state.Blocks, fallback)
}

// RenderReportHTML 生成完整的研报 HTML（含 ECharts 图表支持）
func RenderReportHTML(title, author string, state *ReportState) string {
	if state == nil {
		state = &ReportState{}
	}
	blocks := state.Blocks
	if title == "" {
		title = ResolveReportTitleFromState(state, "数据分析报告")
	}
	if author == "" {
		author = "AI 数据分析师"
	}

	now := time.Now().Format("2006年01月02日")

	var tocHTML strings.Builder
	var bodyHTML strings.Builder
	chapterNum := 0
	referencedCharts := collectReferencedCharts(blocks)

	for _, block := range blocks {
		if isTitleBlock(block) {
			continue
		}
		chapterNum++
		tocHTML.WriteString(fmt.Sprintf(`<li><a href="#section-%d">%s</a></li>`, chapterNum, blockDisplayTitle(block, chapterNum)))
		bodyHTML.WriteString(renderReportBlockHTML(block, chapterNum, state.Charts))
	}
	if appendixHTML := buildChartAppendix(state.Charts, referencedCharts, &chapterNum, &tocHTML); appendixHTML != "" {
		bodyHTML.WriteString(appendixHTML)
	}

	chartScripts := buildChartScripts(state.Charts)
	customCSS := strings.TrimSpace(state.Layout.CustomCSS)
	customJS := strings.TrimSpace(state.Layout.CustomJS)
	bodyClass := strings.TrimSpace(state.Layout.BodyClass)
	coverHTML := ""
	if !state.Layout.HideCover {
		coverHTML = fmt.Sprintf(`<div class="cover">
  <h1>%s</h1>
  <div class="divider"></div>
  <div class="meta">
    <span>📊 %s</span>
    <span>📅 %s</span>
  </div>
</div>`, title, author, now)
	}
	tocBlockHTML := ""
	if !state.Layout.HideTOC {
		tocBlockHTML = fmt.Sprintf(`<div class="toc">
  <h2>目录</h2>
  <ol>%s</ol>
</div>`, tocHTML.String())
	}
	if strings.TrimSpace(state.Layout.CustomHTMLShell) != "" {
		return renderCustomShell(state.Layout.CustomHTMLShell, reportShellData{
			Title:        title,
			Author:       author,
			Date:         now,
			TOC:          tocBlockHTML,
			Content:      bodyHTML.String(),
			ChartScripts: chartScripts,
			CustomCSS:    customCSS,
			CustomJS:     customJS,
			BodyClass:    bodyClass,
		})
	}

	customCSSBlock := ""
	if customCSS != "" {
		customCSSBlock = "\n" + customCSS + "\n"
	}
	customJSBlock := ""
	if customJS != "" {
		customJSBlock = fmt.Sprintf("\n<script>\n%s\n</script>", customJS)
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
.content h3 {
  color: var(--primary);
  font-size: 1.15rem;
  font-weight: 700;
  margin: 1.6rem 0 0.9rem;
}
.content h5 {
  color: var(--primary-light);
  font-size: 1rem;
  font-weight: 600;
  margin: 1.25rem 0 0.65rem;
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
%s
</style>
<script src="https://cdn.jsdelivr.net/npm/echarts@5/dist/echarts.min.js"></script>
</head>
<body class="%s">
%s
%s
%s
<div class="footer">
  <p>本报告由 AI 数据分析智能体自动生成 | %s</p>
</div>
%s
%s
</body>
</html>`, title, customCSSBlock, bodyClass, coverHTML, tocBlockHTML, bodyHTML.String(), now, chartScripts, customJSBlock)
}

type reportShellData struct {
	Title        string
	Author       string
	Date         string
	TOC          string
	Content      string
	ChartScripts string
	CustomCSS    string
	CustomJS     string
	BodyClass    string
}

type sectionRenderPreset struct {
	classes   []string
	bodyClass string
	rawTitle  bool
}

var sectionRenderPresets = map[string]sectionRenderPreset{
	"summary":           {classes: []string{"section", "summary"}, bodyClass: "summary-box", rawTitle: true},
	"executive_summary": {classes: []string{"section", "summary"}, bodyClass: "summary-box", rawTitle: true},
	"conclusion":        {classes: []string{"section", "conclusion"}, bodyClass: "conclusion-box"},
	"appendix":          {classes: []string{"section", "appendix"}, bodyClass: "content"},
	"risks":             {classes: []string{"section", "risks"}, bodyClass: "content"},
	"methodology":       {classes: []string{"section", "methodology"}, bodyClass: "content"},
}

func buildChartScripts(charts []ChartData) string {
	var chartScripts strings.Builder
	if len(charts) == 0 {
		return ""
	}
	chartScripts.WriteString("<script>\ndocument.addEventListener('DOMContentLoaded', function() {\n")
	for _, ch := range charts {
		optionStr := string(ch.Option)
		if optionStr == "" || optionStr == "null" {
			optionStr = "{}"
		}
		chartScripts.WriteString(fmt.Sprintf(`
  (function() {
    var nodes = document.querySelectorAll('[data-chart-id="%s"]');
    if (nodes.length > 0) {
      var option = %s;
      nodes.forEach(function(el) {
        try {
          var chart = echarts.init(el);
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
      });
    }
  })();
`, ch.ID, optionStr))
	}
	chartScripts.WriteString("});\n</script>")
	return chartScripts.String()
}

func renderCustomShell(shell string, data reportShellData) string {
	replacements := strings.NewReplacer(
		"{{title}}", data.Title,
		"{{author}}", data.Author,
		"{{date}}", data.Date,
		"{{toc}}", data.TOC,
		"{{content}}", data.Content,
		"{{chart_scripts}}", data.ChartScripts,
		"{{custom_css}}", data.CustomCSS,
		"{{custom_js}}", data.CustomJS,
		"{{body_class}}", data.BodyClass,
	)
	rendered := replacements.Replace(shell)
	if !strings.Contains(rendered, "{{content}}") && !strings.Contains(shell, "{{content}}") && !strings.Contains(rendered, data.Content) {
		rendered += data.Content
	}
	return rendered
}

func resolveReportTitleFromBlocks(blocks []ReportBlock, fallback string) string {
	for _, block := range blocks {
		if isTitleBlock(block) && strings.TrimSpace(block.Title) != "" {
			return block.Title
		}
	}
	return fallback
}

func collectReferencedCharts(blocks []ReportBlock) map[string]struct{} {
	re := regexp.MustCompile(`\{\{chart:(\w+)\}\}`)
	refs := make(map[string]struct{})
	for _, block := range blocks {
		for _, match := range re.FindAllStringSubmatch(block.Content, -1) {
			if len(match) > 1 {
				refs[match[1]] = struct{}{}
			}
		}
		if strings.TrimSpace(block.Kind) == "chart" && strings.TrimSpace(block.ChartID) != "" {
			refs[strings.TrimSpace(block.ChartID)] = struct{}{}
		}
	}
	return refs
}

func isTitleBlock(block ReportBlock) bool {
	return strings.EqualFold(strings.TrimSpace(block.Kind), "title")
}

func blockDisplayTitle(block ReportBlock, chapterNum int) string {
	title := strings.TrimSpace(block.Title)
	if title != "" {
		return title
	}
	switch strings.ToLower(strings.TrimSpace(block.Kind)) {
	case "chart":
		if block.ChartID != "" {
			return fmt.Sprintf("图表 %s", block.ChartID)
		}
	case "html":
		return fmt.Sprintf("HTML Block %d", chapterNum)
	}
	return fmt.Sprintf("Block %d", chapterNum)
}

type reportBlockRenderer func(ReportBlock, int, []ChartData) string

var reportBlockRenderers = map[string]reportBlockRenderer{
	"markdown": renderMarkdownBlockHTML,
	"html":     renderHTMLBlock,
	"chart":    renderChartBlockHTML,
}

func renderReportBlockHTML(block ReportBlock, chapterNum int, charts []ChartData) string {
	kind := strings.ToLower(strings.TrimSpace(block.Kind))
	renderer, ok := reportBlockRenderers[kind]
	if !ok {
		renderer = renderMarkdownBlockHTML
	}
	return renderer(block, chapterNum, charts)
}

func renderMarkdownBlockHTML(block ReportBlock, chapterNum int, charts []ChartData) string {
	preset := inferMarkdownBlockPreset(block)
	title := blockDisplayTitle(block, chapterNum)
	heading := fmt.Sprintf("%d. %s", chapterNum, title)
	if preset.rawTitle {
		heading = title
	}
	return fmt.Sprintf(`
		<div class="%s" id="section-%d" data-block-id="%s">
			<h2>%s</h2>
			<div class="%s">%s</div>
		</div>`, strings.Join(preset.classes, " "), chapterNum, block.ID, heading, preset.bodyClass, processContent(block.Content, charts))
}

func inferMarkdownBlockPreset(block ReportBlock) sectionRenderPreset {
	defaultPreset := sectionRenderPreset{classes: []string{"section"}, bodyClass: "content"}
	hint := strings.ToLower(strings.TrimSpace(block.ID + " " + block.Title))
	switch {
	case strings.Contains(hint, "summary"), strings.Contains(hint, "摘要"):
		if preset, ok := sectionRenderPresets["summary"]; ok {
			return preset
		}
	case strings.Contains(hint, "conclusion"), strings.Contains(hint, "结论"):
		if preset, ok := sectionRenderPresets["conclusion"]; ok {
			return preset
		}
	case strings.Contains(hint, "risk"), strings.Contains(hint, "风险"):
		if preset, ok := sectionRenderPresets["risks"]; ok {
			return preset
		}
	case strings.Contains(hint, "method"), strings.Contains(hint, "方法"):
		if preset, ok := sectionRenderPresets["methodology"]; ok {
			return preset
		}
	}
	return defaultPreset
}

func renderHTMLBlock(block ReportBlock, chapterNum int, charts []ChartData) string {
	title := blockDisplayTitle(block, chapterNum)
	return fmt.Sprintf(`
		<div class="section html-block" id="section-%d" data-block-id="%s">
			<h2>%d. %s</h2>
			<div class="content">%s</div>
		</div>`, chapterNum, block.ID, chapterNum, title, block.Content)
}

func renderChartBlockHTML(block ReportBlock, chapterNum int, charts []ChartData) string {
	title := blockDisplayTitle(block, chapterNum)
	content := fmt.Sprintf("{{chart:%s}}", block.ChartID)
	if strings.TrimSpace(block.Content) != "" {
		content += "\n\n" + block.Content
	}
	return fmt.Sprintf(`
		<div class="section chart-block" id="section-%d" data-block-id="%s">
			<h2>%d. %s</h2>
			<div class="content">%s</div>
		</div>`, chapterNum, block.ID, chapterNum, title, processContent(content, charts))
}

func buildChartAppendix(charts []ChartData, referenced map[string]struct{}, chapterNum *int, tocHTML *strings.Builder) string {
	var missing []string
	for _, ch := range charts {
		if _, ok := referenced[ch.ID]; ok {
			continue
		}
		missing = append(missing, fmt.Sprintf("{{chart:%s}}", ch.ID))
	}
	if len(missing) == 0 {
		return ""
	}

	*chapterNum = *chapterNum + 1
	tocHTML.WriteString(fmt.Sprintf(`<li><a href="#section-%d">%s</a></li>`, *chapterNum, "图表附录"))
	return fmt.Sprintf(`
		<div class="section" id="section-%d">
			<h2>%d. %s</h2>
			<div class="content">
				<p>以下图表已经生成，但正文尚未引用，系统已自动补入附录以避免遗漏。</p>
				%s
			</div>
		</div>`, *chapterNum, *chapterNum, "图表附录", processContent(strings.Join(missing, "\n\n"), charts))
}

// processContent 处理内容：Markdown 转 HTML + 替换图表占位符
func processContent(content string, charts []ChartData) string {
	html := markdownToHTML(content)

	// 替换 {{chart:chart_id}} 占位符为 ECharts 容器
	re := regexp.MustCompile(`\{\{chart:(\w+)\}\}`)
	chartRefCounts := make(map[string]int)
	html = re.ReplaceAllStringFunc(html, func(match string) string {
		chartID := re.FindStringSubmatch(match)[1]
		// 查找对应图表
		for _, ch := range charts {
			if ch.ID == chartID {
				height := ch.Height
				if height == "" {
					height = "400px"
				}
				chartRefCounts[chartID]++
				containerID := fmt.Sprintf("%s-ref-%d", chartID, chartRefCounts[chartID])
				return fmt.Sprintf(`<div id="%s" data-chart-id="%s" class="chart-box" style="height:%s;"></div>`, containerID, ch.ID, height)
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
		if strings.HasPrefix(trimmed, "#### ") {
			html.WriteString(fmt.Sprintf("<h5>%s</h5>\n", formatInline(trimmed[5:])))
			continue
		}
		if strings.HasPrefix(trimmed, "### ") {
			html.WriteString(fmt.Sprintf("<h4>%s</h4>\n", formatInline(trimmed[4:])))
			continue
		}
		if strings.HasPrefix(trimmed, "## ") {
			html.WriteString(fmt.Sprintf("<h3>%s</h3>\n", formatInline(trimmed[3:])))
			continue
		}
		if strings.HasPrefix(trimmed, "# ") {
			html.WriteString(fmt.Sprintf("<h2>%s</h2>\n", formatInline(trimmed[2:])))
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
