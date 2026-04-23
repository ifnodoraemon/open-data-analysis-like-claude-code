package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	htmlstd "html"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/ifnodoraemon/openDataAnalysis/config"
	htmlnode "golang.org/x/net/html"
)

var (
	htmlHeadingRegexp = regexp.MustCompile(`(?im)^\s*<h[1-6][^>]*>(.*?)</h[1-6]>`)
	htmlTagsRegexp    = regexp.MustCompile(`<[^>]*>`)
	mdHeadingRegexp   = regexp.MustCompile(`(?m)^\s*(?:#{1,6})\s+(.+?)(?:\r?\n|$)`)
	renderTokenRegexp = regexp.MustCompile(`[a-z0-9]+`)
)

func ResolveReportTitleFromState(state *ReportState) string {
	if state == nil {
		return ""
	}
	return state.FinalTitle
}

// RenderReportHTML 生成完整的研报 HTML（含 ECharts 图表支持）
func RenderReportHTML(title, author string, state *ReportState) string {
	if state == nil {
		state = &ReportState{}
	}
	if title == "" && state != nil {
		title = state.FinalTitle
	}
	if author == "" && state != nil {
		author = state.FinalAuthor
	}
	title = strings.TrimSpace(title)
	author = strings.TrimSpace(author)
	units := buildRenderUnits(state.Blocks, title)
	safeTitle := escapeHTMLText(title)
	titleHeaderHTML := renderReportTitleHeader(title, author)
	tocHTML := renderReportTOC(units)

	var bodyHTML strings.Builder
	chapterNum := 0

	var lastBlockID string
	var wrapperOpen bool

	for _, unit := range units {
		block := unit.Block
		if isTitleBlock(block) {
			continue
		}

		if block.ID != lastBlockID {
			if wrapperOpen {
				bodyHTML.WriteString("</div>\n")
			}
			blockKind := strings.ToLower(strings.TrimSpace(block.Kind))
			if blockKind == "" {
				blockKind = "markdown" // Default
			}
			wrapperTitle := blockDisplayTitle(block)
			bodyHTML.WriteString(fmt.Sprintf(`<div class="report-block-wrapper" data-block-id="%s" data-block-kind="%s" data-block-title="%s">`+"\n",
				escapeHTMLAttr(block.ID),
				escapeHTMLAttr(blockKind),
				escapeHTMLAttr(wrapperTitle)))
			wrapperOpen = true
			lastBlockID = block.ID
		}

		chapterNum++
		bodyHTML.WriteString(renderReportBlockHTML(block, chapterNum, state.Charts, unit.AttachedCharts))
	}

	if wrapperOpen {
		bodyHTML.WriteString("</div>\n")
	}

	chartScripts := buildChartScripts(state.Charts)
	customCSS := sanitizeCSS(state.Layout.CustomCSS)
	bodyClass := sanitizeBodyClass(state.Layout.BodyClass)

	customCSSBlock := ""
	if customCSS != "" {
		customCSSBlock = "\n" + customCSS + "\n"
	}

	echartsURL := "/assets/echarts.min.js"
	if config.Cfg != nil && config.Cfg.ReportEchartsUrl != "" {
		echartsURL = config.Cfg.ReportEchartsUrl
	}
	echartsScriptNode := fmt.Sprintf(`<script id="oda-echarts-loader" src="%s"></script>`, escapeHTMLAttr(echartsURL))

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>%s</title>
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
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, "Noto Sans SC", "PingFang SC", sans-serif;
  color: var(--text);
  line-height: 1.85;
  background: var(--bg-alt);
  -webkit-font-smoothing: antialiased;
}

.report-titlebar {
  max-width: 780px;
  margin: 2rem auto 0.75rem;
  padding: 2rem 2.5rem 1.75rem;
  background: var(--bg);
  border-top: 4px solid var(--primary);
  border-radius: var(--radius);
  box-shadow: var(--shadow-sm);
}
.report-titlebar h1 {
  color: var(--primary);
  font-size: 1.9rem;
  line-height: 1.25;
  font-weight: 800;
  margin: 0;
}
.report-titlebar .meta {
  margin-top: 0.75rem;
  color: var(--text-light);
  font-size: 0.9rem;
}
.report-toc {
  max-width: 780px;
  margin: 1rem auto 1.5rem;
  padding: 1.4rem 2rem;
  background: var(--bg);
  border-radius: var(--radius);
  box-shadow: var(--shadow-sm);
}
.report-toc h2 {
  color: var(--primary);
  font-size: 1rem;
  font-weight: 800;
  margin-bottom: 0.75rem;
}
.report-toc ol {
  list-style: decimal;
  padding-left: 1.25rem;
  margin: 0;
}
.report-toc li {
  color: var(--text-light);
  padding: 0.3rem 0;
  border-bottom: 1px solid var(--border-light);
}
.report-toc li:last-child {
  border-bottom: none;
}
.report-toc a {
  color: var(--text);
  text-decoration: none;
}
.report-toc a:hover {
  color: var(--primary-light);
}

/* === Sections === */
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

/* === Charts === */
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
/* === Tables === */
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

/* === Print === */
@media print {
  @page { margin: 18mm 16mm; }
  body { background: white; }
  .report-titlebar { box-shadow: none; page-break-after: avoid; }
  .report-toc { box-shadow: none; page-break-after: avoid; }
  .section {
    box-shadow: none;
    break-inside: auto;
    page-break-inside: auto;
    margin: 0.75rem auto;
  }
  .section h2,
  .content h3,
  .content h4,
  .content h5 {
    break-after: avoid;
    page-break-after: avoid;
  }
  .chart-box {
    box-shadow: none;
    break-inside: avoid;
    page-break-inside: avoid;
  }
  table {
    box-shadow: none;
    break-inside: auto;
    page-break-inside: auto;
  }
  tr {
    break-inside: avoid;
    page-break-inside: avoid;
  }
}
/* === Responsive === */
@media (max-width: 860px) {
  .section { margin-left: 1rem; margin-right: 1rem; }
}
%s
</style>
%s
</head>
<body class="%s">
%s
%s
%s
%s
</body>
</html>`, safeTitle, customCSSBlock, echartsScriptNode, bodyClass, titleHeaderHTML, tocHTML, bodyHTML.String(), chartScripts)
}

func renderReportTitleHeader(title, author string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}
	meta := ""
	if strings.TrimSpace(author) != "" {
		meta = fmt.Sprintf(`<div class="meta">%s</div>`, escapeHTMLText(strings.TrimSpace(author)))
	}
	return fmt.Sprintf(`<header class="report-titlebar" data-report-title="true">
  <h1>%s</h1>
  %s
</header>`, escapeHTMLText(title), meta)
}

type reportTOCItem struct {
	Anchor string
	Title  string
}

func renderReportTOC(units []reportRenderUnit) string {
	items := make([]reportTOCItem, 0, len(units))
	sectionNum := 0
	for _, unit := range units {
		block := unit.Block
		if isTitleBlock(block) {
			continue
		}
		sectionNum++
		title := strings.TrimSpace(blockDisplayTitle(block))
		if title == "" {
			continue
		}
		items = append(items, reportTOCItem{
			Anchor: fmt.Sprintf("section-%d", sectionNum),
			Title:  title,
		})
	}
	if len(items) < 2 {
		return ""
	}

	var html strings.Builder
	html.WriteString(`<nav class="report-toc" aria-label="报告目录">` + "\n")
	html.WriteString("<h2>目录</h2>\n<ol>\n")
	for _, item := range items {
		html.WriteString(fmt.Sprintf(`<li><a href="#%s">%s</a></li>`+"\n", escapeHTMLAttr(item.Anchor), escapeHTMLText(item.Title)))
	}
	html.WriteString("</ol>\n</nav>")
	return html.String()
}

func buildChartScripts(charts []ChartData) string {
	if len(charts) == 0 {
		return ""
	}
	return `<script id="oda-chart-runtime" src="/oda-chart-runtime.js"></script>`
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

func blockDisplayTitle(block ReportBlock) string {
	if title := extractContentHeadingTitle(block.Content); title != "" {
		return title
	}
	return strings.TrimSpace(block.Title)
}

func extractContentHeadingTitle(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}

	htmlLoc := htmlHeadingRegexp.FindStringIndex(content)
	mdLoc := mdHeadingRegexp.FindStringIndex(content)

	var firstMatch []string
	if htmlLoc != nil && (mdLoc == nil || htmlLoc[0] < mdLoc[0]) {
		firstMatch = htmlHeadingRegexp.FindStringSubmatch(content)
	} else if mdLoc != nil {
		firstMatch = mdHeadingRegexp.FindStringSubmatch(content)
	}

	if len(firstMatch) > 1 {
		return strings.TrimSpace(htmlTagsRegexp.ReplaceAllString(firstMatch[1], ""))
	}
	return ""
}

type reportBlockRenderer func(ReportBlock, int, []ChartData) string

var reportBlockRenderers = map[string]reportBlockRenderer{
	"markdown": renderMarkdownBlockHTMLStandalone,
	"html":     renderHTMLBlockStandalone,
	"chart":    renderChartBlockHTML,
}

type reportRenderUnit struct {
	Block          ReportBlock
	AttachedCharts []ReportBlock
}

func renderReportBlockHTML(block ReportBlock, chapterNum int, charts []ChartData, attachedCharts []ReportBlock) string {
	kind := strings.ToLower(strings.TrimSpace(block.Kind))
	switch kind {
	case "markdown":
		return renderMarkdownBlockHTML(block, chapterNum, charts, attachedCharts)
	case "html":
		return renderHTMLBlock(block, chapterNum, charts, attachedCharts)
	default:
		renderer, ok := reportBlockRenderers[kind]
		if !ok {
			renderer = renderMarkdownBlockHTMLStandalone
		}
		return renderer(block, chapterNum, charts)
	}
}

func renderMarkdownBlockHTMLStandalone(block ReportBlock, chapterNum int, charts []ChartData) string {
	return renderMarkdownBlockHTML(block, chapterNum, charts, nil)
}

func renderMarkdownBlockHTML(block ReportBlock, chapterNum int, charts []ChartData, attachedCharts []ReportBlock) string {
	displayTitle := blockDisplayTitle(block)

	headingHTML := ""
	if displayTitle != "" && extractContentHeadingTitle(block.Content) == "" {
		headingHTML = fmt.Sprintf("<h2>%s</h2>\n", escapeHTMLText(displayTitle))
	}

	contentHTML := headingHTML + processContent(block.Content, charts) + renderAttachedChartsInline(attachedCharts, charts)

	return fmt.Sprintf(`
			<div class="section" id="section-%d">
				<div class="content">%s</div>
			</div>`, chapterNum, contentHTML)
}

func renderHTMLBlock(block ReportBlock, chapterNum int, charts []ChartData, attachedCharts []ReportBlock) string {
	displayTitle := blockDisplayTitle(block)

	headingHTML := ""
	if displayTitle != "" && extractContentHeadingTitle(block.Content) == "" {
		headingHTML = fmt.Sprintf("<h2>%s</h2>\n", escapeHTMLText(displayTitle))
	}

	contentHTML := headingHTML + sanitizeHTMLFragment(block.Content) + renderAttachedChartsInline(attachedCharts, charts)
	return fmt.Sprintf(`
		<div class="section html-block" id="section-%d">
			<div class="content">%s</div>
		</div>`, chapterNum, contentHTML)
}

func renderHTMLBlockStandalone(block ReportBlock, chapterNum int, charts []ChartData) string {
	return renderHTMLBlock(block, chapterNum, charts, nil)
}

func renderChartBlockHTML(block ReportBlock, chapterNum int, charts []ChartData) string {
	title := blockDisplayTitle(block)
	content := fmt.Sprintf("{{chart:%s}}", block.ChartID)
	if strings.TrimSpace(block.Content) != "" {
		content += "\n\n" + block.Content
	}
	var headingHTML string
	if title != "" {
		headingHTML = fmt.Sprintf("<h2>%s</h2>", escapeHTMLText(title))
	}
	return fmt.Sprintf(`
		<div class="section chart-block" id="section-%d">
			%s
			<div class="content">%s</div>
		</div>`, chapterNum, headingHTML, processContent(content, charts))
}

func renderAttachedChartsInline(attachedCharts []ReportBlock, charts []ChartData) string {
	if len(attachedCharts) == 0 {
		return ""
	}
	var html strings.Builder
	for _, block := range attachedCharts {
		title := blockDisplayTitle(block)
		if title != "" {
			html.WriteString(fmt.Sprintf("<h4>%s</h4>\n", escapeHTMLText(title)))
		}
		content := fmt.Sprintf("{{chart:%s}}", block.ChartID)
		if strings.TrimSpace(block.Content) != "" {
			content += "\n\n" + block.Content
		}
		html.WriteString(processContent(content, charts))
	}
	return html.String()
}

func buildRenderUnits(blocks []ReportBlock, reportTitle string) []reportRenderUnit {
	if len(blocks) == 0 {
		return nil
	}
	blocks = normalizeReportBlocksForRendering(blocks, reportTitle)
	attachments := make(map[int][]ReportBlock)
	attachedCharts := make(map[int]struct{})
	for idx, block := range blocks {
		if !shouldAttachChartInline(block) {
			continue
		}
		target := findInlineChartAnchorIndex(blocks, idx)
		if target < 0 {
			continue
		}
		attachments[target] = append(attachments[target], block)
		attachedCharts[idx] = struct{}{}
	}

	baseUnits := make([]reportRenderUnit, 0, len(blocks))
	for idx, block := range blocks {
		if _, attached := attachedCharts[idx]; attached {
			continue
		}
		unit := reportRenderUnit{Block: block}
		if len(attachments[idx]) > 0 {
			unit.AttachedCharts = append(unit.AttachedCharts, attachments[idx]...)
		}
		baseUnits = append(baseUnits, unit)
	}

	units := make([]reportRenderUnit, 0, len(baseUnits))
	for _, unit := range baseUnits {
		units = append(units, splitRenderUnitSections(unit)...)
	}
	return units
}

func normalizeReportBlocksForRendering(blocks []ReportBlock, reportTitle string) []ReportBlock {
	normalized := make([]ReportBlock, len(blocks))
	copy(normalized, blocks)

	reportTitle = strings.TrimSpace(reportTitle)
	if reportTitle == "" {
		return normalized
	}

	for idx, block := range normalized {
		if isTitleBlock(block) || !strings.EqualFold(strings.TrimSpace(block.Kind), "markdown") {
			continue
		}
		heading, content, ok := stripLeadingMarkdownDocumentTitle(block.Content, reportTitle)
		if !ok {
			continue
		}
		block.Content = content
		if strings.TrimSpace(block.Title) == "" || comparableTitle(block.Title) == comparableTitle(heading) {
			block.Title = ""
		}
		normalized[idx] = block
		break
	}

	return normalized
}

func stripLeadingMarkdownDocumentTitle(content, reportTitle string) (string, string, bool) {
	lines := strings.Split(content, "\n")
	firstContentLine := -1
	for idx, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		firstContentLine = idx
		break
	}
	if firstContentLine < 0 {
		return "", content, false
	}

	trimmed := strings.TrimSpace(lines[firstContentLine])
	if !strings.HasPrefix(trimmed, "# ") || strings.HasPrefix(trimmed, "## ") {
		return "", content, false
	}

	heading := strings.TrimSpace(trimmed[2:])
	if !titlesReferToSameReport(heading, reportTitle) {
		return "", content, false
	}

	remaining := append([]string{}, lines[:firstContentLine]...)
	remaining = append(remaining, lines[firstContentLine+1:]...)
	return heading, strings.TrimSpace(strings.Join(remaining, "\n")), true
}

func titlesReferToSameReport(heading, reportTitle string) bool {
	headingKey := comparableTitle(heading)
	titleKey := comparableTitle(reportTitle)
	if headingKey == "" || titleKey == "" {
		return false
	}
	if headingKey == titleKey {
		return true
	}
	if len(headingKey) < 10 || len(titleKey) < 10 {
		return false
	}
	return strings.Contains(titleKey, headingKey) || strings.Contains(headingKey, titleKey)
}

func comparableTitle(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= '\u4e00' && r <= '\u9fff':
			b.WriteRune(r)
		}
	}
	return b.String()
}

func splitRenderUnitSections(unit reportRenderUnit) []reportRenderUnit {
	if !strings.EqualFold(strings.TrimSpace(unit.Block.Kind), "markdown") {
		return []reportRenderUnit{unit}
	}

	fragments := splitMarkdownIntoTopLevelSections(unit.Block.Content)
	if len(fragments) <= 1 {
		return []reportRenderUnit{unit}
	}

	units := make([]reportRenderUnit, 0, len(fragments))
	for i, fragment := range fragments {
		block := unit.Block
		block.Content = fragment
		fragmentUnit := reportRenderUnit{Block: block}
		if i == 0 && len(unit.AttachedCharts) > 0 {
			fragmentUnit.AttachedCharts = append(fragmentUnit.AttachedCharts, unit.AttachedCharts...)
		}
		units = append(units, fragmentUnit)
	}
	return units
}

func splitMarkdownIntoTopLevelSections(content string) []string {
	lines := strings.Split(content, "\n")
	minLevel := 0
	headingCount := 0
	for _, line := range lines {
		level, ok := markdownHeadingLevel(line)
		if !ok || level > 2 {
			continue
		}
		if minLevel == 0 || level < minLevel {
			minLevel = level
			headingCount = 1
			continue
		}
		if level == minLevel {
			headingCount++
		}
	}
	if minLevel == 0 || headingCount <= 1 {
		return []string{content}
	}

	parts := make([]string, 0, headingCount)
	current := make([]string, 0, len(lines))
	for _, line := range lines {
		level, ok := markdownHeadingLevel(line)
		if ok && level == minLevel && len(current) > 0 {
			parts = append(parts, strings.TrimSpace(strings.Join(current, "\n")))
			current = current[:0]
		}
		current = append(current, line)
	}
	if len(current) > 0 {
		parts = append(parts, strings.TrimSpace(strings.Join(current, "\n")))
	}
	if len(parts) <= 1 {
		return []string{content}
	}
	return parts
}

func markdownHeadingLevel(line string) (int, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || trimmed[0] != '#' {
		return 0, false
	}
	level := 0
	for level < len(trimmed) && trimmed[level] == '#' {
		level++
	}
	if level == 0 || level >= len(trimmed) || trimmed[level] != ' ' {
		return 0, false
	}
	return level, true
}

func shouldAttachChartInline(block ReportBlock) bool {
	return strings.EqualFold(strings.TrimSpace(block.Kind), "chart") &&
		strings.TrimSpace(block.ChartID) != "" &&
		strings.TrimSpace(block.Title) == ""
}

func findInlineChartAnchorIndex(blocks []ReportBlock, chartIndex int) int {
	chartBlock := blocks[chartIndex]
	prevIdx := findAdjacentTextBlockIndex(blocks, chartIndex, -1)
	nextIdx := findAdjacentTextBlockIndex(blocks, chartIndex, 1)
	if prevIdx < 0 {
		return nextIdx
	}
	if nextIdx < 0 {
		return prevIdx
	}

	prevScore := scoreInlineChartAnchor(chartBlock, blocks[prevIdx], chartIndex-prevIdx, false)
	nextScore := scoreInlineChartAnchor(chartBlock, blocks[nextIdx], nextIdx-chartIndex, true)
	if nextScore > prevScore {
		return nextIdx
	}
	return prevIdx
}

func findAdjacentTextBlockIndex(blocks []ReportBlock, start, step int) int {
	for idx := start + step; idx >= 0 && idx < len(blocks); idx += step {
		if isTextRenderBlock(blocks[idx]) {
			return idx
		}
	}
	return -1
}

func isTextRenderBlock(block ReportBlock) bool {
	kind := strings.ToLower(strings.TrimSpace(block.Kind))
	return kind == "markdown" || kind == "html"
}

func scoreInlineChartAnchor(chartBlock, textBlock ReportBlock, distance int, _ bool) int {
	score := positiveDistanceScore(distance)
	score += tokenOverlapScore(chartBlock, textBlock) * 10
	if isOverviewLikeBlock(textBlock) {
		score -= 4
	}
	return score
}

func positiveDistanceScore(distance int) int {
	if distance >= 8 {
		return 0
	}
	return 8 - distance
}

func tokenOverlapScore(chartBlock, textBlock ReportBlock) int {
	chartTokens := blockRenderTokens(chartBlock)
	textTokens := blockRenderTokens(textBlock)
	if len(chartTokens) == 0 || len(textTokens) == 0 {
		return 0
	}
	score := 0
	for token := range chartTokens {
		if _, ok := textTokens[token]; ok {
			score++
		}
	}
	return score
}

func blockRenderTokens(block ReportBlock) map[string]struct{} {
	source := strings.ToLower(strings.TrimSpace(strings.Join([]string{
		block.ID,
		block.Title,
		block.ChartID,
		blockDisplayTitle(block),
	}, " ")))
	matches := renderTokenRegexp.FindAllString(source, -1)
	tokens := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		tokens[match] = struct{}{}
	}
	return tokens
}

func isOverviewLikeBlock(block ReportBlock) bool {
	hint := strings.ToLower(strings.TrimSpace(strings.Join([]string{
		block.ID,
		block.Title,
		blockDisplayTitle(block),
	}, " ")))
	return strings.Contains(hint, "overview") ||
		strings.Contains(hint, "summary") ||
		strings.Contains(hint, "exec") ||
		strings.Contains(hint, "摘要") ||
		strings.Contains(hint, "概览")
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
				optionAttr := escapeHTMLAttr(safeJSONForInlineScript(ch.Option))
				return fmt.Sprintf(`<div id="%s" data-chart-id="%s" data-chart-option="%s" class="chart-box" style="height:%s;"></div>`, escapeHTMLAttr(containerID), escapeHTMLAttr(ch.ID), optionAttr, escapeHTMLAttr(height))
			}
		}
		return ""
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
				html.WriteString(fmt.Sprintf("<th>%s</th>", formatInline(strings.TrimSpace(cell))))
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
				html.WriteString(fmt.Sprintf("<td>%s</td>", formatInline(strings.TrimSpace(cell))))
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
	text = escapeHTMLText(text)
	for strings.Contains(text, "**") {
		text = strings.Replace(text, "**", "<strong>", 1)
		text = strings.Replace(text, "**", "</strong>", 1)
	}
	return text
}

func escapeHTMLText(value string) string {
	return htmlstd.EscapeString(strings.TrimSpace(value))
}

func escapeHTMLAttr(value string) string {
	return htmlstd.EscapeString(strings.TrimSpace(value))
}

func sanitizeBodyClass(value string) string {
	fields := strings.Fields(value)
	safe := make([]string, 0, len(fields))
	for _, field := range fields {
		if field == "" {
			continue
		}
		valid := true
		for _, r := range field {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
				continue
			}
			valid = false
			break
		}
		if valid {
			safe = append(safe, field)
		}
	}
	return strings.Join(safe, " ")
}

func sanitizeCSS(value string) string {
	css := strings.TrimSpace(value)
	if css == "" {
		return ""
	}
	replacements := []string{
		"</style", "",
		"</Style", "",
		"</STYLE", "",
		"@import", "",
		"@Import", "",
		"@IMPORT", "",
		"expression(", "",
		"Expression(", "",
		"EXPRESSION(", "",
		"javascript:", "",
		"Javascript:", "",
		"JAVASCRIPT:", "",
		"behavior:", "",
		"Behavior:", "",
		"BEHAVIOR:", "",
		"vbscript:", "",
		"Vbscript:", "",
		"VBSCRIPT:", "",
	}
	replacer := strings.NewReplacer(replacements...)
	css = replacer.Replace(css)
	css = cssStripRe.ReplaceAllString(css, "")
	return css
}

var cssStripRe = regexp.MustCompile(`(?i)(@[\s]*import|expression[\s]*\(|behavior[\s]*:)`)

func safeJSONForInlineScript(raw json.RawMessage) string {
	option := strings.TrimSpace(string(raw))
	if option == "" || option == "null" {
		return "{}"
	}
	var parsed any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "{}"
	}
	normalized, err := json.Marshal(parsed)
	if err != nil {
		return "{}"
	}
	return escapeInlineScript(string(normalized))
}

func escapeInlineScript(value string) string {
	replacer := strings.NewReplacer(
		"</script", "<\\/script",
		"<!--", "<\\!--",
		"-->", "--\\>",
		"\u2028", "\\u2028",
		"\u2029", "\\u2029",
	)
	return replacer.Replace(value)
}

var allowedHTMLBlockTags = map[string]struct{}{
	"a": {}, "b": {}, "blockquote": {}, "br": {}, "code": {}, "div": {}, "em": {}, "h1": {}, "h2": {}, "h3": {}, "h4": {}, "h5": {}, "h6": {},
	"hr": {}, "i": {}, "li": {}, "ol": {}, "p": {}, "pre": {}, "span": {}, "strong": {}, "table": {}, "tbody": {}, "td": {}, "th": {}, "thead": {}, "tr": {}, "ul": {},
}

func sanitizeHTMLFragment(fragment string) string {
	doc, err := htmlnode.Parse(strings.NewReader("<!DOCTYPE html><html><body><div id=\"__oda_root__\">" + fragment + "</div></body></html>"))
	if err != nil {
		return fmt.Sprintf("<p>%s</p>", escapeHTMLText(fragment))
	}
	root := findHTMLNodeByID(doc, "__oda_root__")
	if root == nil {
		return fmt.Sprintf("<p>%s</p>", escapeHTMLText(fragment))
	}
	container := &htmlnode.Node{Type: htmlnode.ElementNode, Data: "div"}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		sanitizeHTMLNode(container, child)
	}
	var out bytes.Buffer
	for child := container.FirstChild; child != nil; child = child.NextSibling {
		if err := htmlnode.Render(&out, child); err != nil {
			return fmt.Sprintf("<p>%s</p>", escapeHTMLText(fragment))
		}
	}
	return out.String()
}

func sanitizeHTMLNode(parent, node *htmlnode.Node) {
	switch node.Type {
	case htmlnode.TextNode:
		parent.AppendChild(&htmlnode.Node{Type: htmlnode.TextNode, Data: node.Data})
	case htmlnode.ElementNode:
		tag := strings.ToLower(strings.TrimSpace(node.Data))
		switch tag {
		case "script", "style", "iframe", "object", "embed", "link", "meta", "base":
			return
		}
		if _, ok := allowedHTMLBlockTags[tag]; !ok {
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				sanitizeHTMLNode(parent, child)
			}
			return
		}
		safeNode := &htmlnode.Node{Type: htmlnode.ElementNode, Data: tag}
		safeNode.Attr = sanitizeHTMLAttrs(tag, node.Attr)
		parent.AppendChild(safeNode)
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			sanitizeHTMLNode(safeNode, child)
		}
	default:
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			sanitizeHTMLNode(parent, child)
		}
	}
}

func sanitizeHTMLAttrs(tag string, attrs []htmlnode.Attribute) []htmlnode.Attribute {
	safe := make([]htmlnode.Attribute, 0, len(attrs))
	for _, attr := range attrs {
		key := strings.ToLower(strings.TrimSpace(attr.Key))
		value := strings.TrimSpace(attr.Val)
		if key == "" || strings.HasPrefix(key, "on") {
			continue
		}
		switch key {
		case "class":
			className := sanitizeBodyClass(strings.ReplaceAll(value, ":", " "))
			className = strings.ReplaceAll(className, " ", "-")
			if className == "" {
				continue
			}
			safe = append(safe, htmlnode.Attribute{Key: key, Val: className})
		case "href":
			href, ok := sanitizeURL(value)
			if !ok || tag != "a" {
				continue
			}
			safe = append(safe, htmlnode.Attribute{Key: key, Val: href})
			safe = append(safe, htmlnode.Attribute{Key: "rel", Val: "noopener noreferrer"})
			safe = append(safe, htmlnode.Attribute{Key: "target", Val: "_blank"})
		case "title":
			safe = append(safe, htmlnode.Attribute{Key: key, Val: value})
		case "colspan", "rowspan":
			if _, err := strconv.Atoi(value); err == nil {
				safe = append(safe, htmlnode.Attribute{Key: key, Val: value})
			}
		}
	}
	return safe
}

func sanitizeURL(value string) (string, bool) {
	if value == "" {
		return "", false
	}
	if strings.HasPrefix(value, "#") || strings.HasPrefix(value, "/") {
		return value, true
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https", "mailto":
		return value, true
	default:
		return "", false
	}
}

func findHTMLNodeByID(node *htmlnode.Node, id string) *htmlnode.Node {
	if node == nil {
		return nil
	}
	if node.Type == htmlnode.ElementNode {
		for _, attr := range node.Attr {
			if attr.Key == "id" && attr.Val == id {
				return node
			}
		}
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := findHTMLNodeByID(child, id); found != nil {
			return found
		}
	}
	return nil
}
