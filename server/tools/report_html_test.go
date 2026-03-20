package tools

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRenderReportHTMLConvertsMarkdownHeadings(t *testing.T) {
	t.Parallel()

	html := RenderReportHTML("测试报告", "AI", &ReportState{
		Blocks: []ReportBlock{
			{
				ID:      "analysis-1",
				Kind:    "markdown",
				Title:   "销售分析",
				Content: "## 销售总体表现\n\n- **收入** 增长 20%",
			},
		},
	})

	if !strings.Contains(html, "<h3>销售总体表现</h3>") {
		t.Fatalf("expected h2 markdown to render as heading, got: %s", html)
	}
	if !strings.Contains(html, "<li><strong>收入</strong> 增长 20%</li>") {
		t.Fatalf("expected list markdown to render as html, got: %s", html)
	}
}

func TestRenderReportHTMLDoesNotAppendUnreferencedCharts(t *testing.T) {
	t.Parallel()

	option, err := json.Marshal(map[string]any{
		"xAxis": map[string]any{"type": "category", "data": []string{"1月"}},
		"yAxis": map[string]any{"type": "value"},
		"series": []map[string]any{
			{"type": "bar", "data": []int{100}},
		},
	})
	if err != nil {
		t.Fatalf("marshal option: %v", err)
	}

	html := RenderReportHTML("测试报告", "AI", &ReportState{
		Blocks: []ReportBlock{
			{
				ID:      "analysis-1",
				Kind:    "markdown",
				Title:   "销售分析",
				Content: "这里只写了解读，没有引用图表。",
			},
		},
		Charts: []ChartData{
			{ID: "chart_sales", Option: option, Height: "360px"},
		},
	})

	if strings.Contains(html, "图表附录") {
		t.Fatalf("did not expect appendix for unreferenced charts")
	}
}

func TestRenderReportHTMLSupportsCustomSectionKinds(t *testing.T) {
	t.Parallel()

	html := RenderReportHTML("测试报告", "AI", &ReportState{
		Blocks: []ReportBlock{
			{
				ID:      "risks-1",
				Kind:    "markdown",
				Title:   "主要风险",
				Content: "需求口径仍可能调整。",
			},
			{
				ID:      "method-1",
				Kind:    "markdown",
				Title:   "分析方法",
				Content: "基于聚合 SQL 与图表交叉验证。",
			},
		},
	})

	if !strings.Contains(html, "1. 主要风险") {
		t.Fatalf("expected custom risks section to render with numbered heading")
	}
	if !strings.Contains(html, "2. 分析方法") {
		t.Fatalf("expected custom methodology section to render with numbered heading")
	}
}

func TestRenderReportHTMLNormalizesPrefixedSectionTitles(t *testing.T) {
	t.Parallel()

	html := RenderReportHTML("测试报告", "AI", &ReportState{
		Blocks: []ReportBlock{
			{ID: "sec-1", Kind: "markdown", Title: "一、数据概览", Content: "内容 1"},
			{ID: "sec-2", Kind: "markdown", Title: "02 各维度分布", Content: "内容 2"},
			{ID: "sec-3", Kind: "markdown", Title: "第3章 趋势变化", Content: "内容 3"},
		},
	})

	if !strings.Contains(html, `<li><a href="#section-1">数据概览</a></li>`) {
		t.Fatalf("expected toc title to strip ordinal prefix, got: %s", html)
	}
	if !strings.Contains(html, `<li><a href="#section-2">各维度分布</a></li>`) {
		t.Fatalf("expected numeric space prefix to be stripped, got: %s", html)
	}
	if !strings.Contains(html, `<li><a href="#section-3">趋势变化</a></li>`) {
		t.Fatalf("expected chapter prefix to be stripped, got: %s", html)
	}
	if !strings.Contains(html, `<h2>1. 数据概览</h2>`) {
		t.Fatalf("expected heading to keep single generated ordinal, got: %s", html)
	}
	if !strings.Contains(html, `<h2>2. 各维度分布</h2>`) {
		t.Fatalf("expected heading to normalize prefixed title, got: %s", html)
	}
	if !strings.Contains(html, `<h2>3. 趋势变化</h2>`) {
		t.Fatalf("expected heading to normalize chapter title, got: %s", html)
	}
}

func TestRenderReportHTMLSupportsLayoutOverrides(t *testing.T) {
	t.Parallel()

	html := RenderReportHTML("测试报告", "AI", &ReportState{
		Layout: ReportLayout{
			CustomCSS: ".hero{color:red;}",
			CustomJS:  "console.log('layout')",
			BodyClass: "magazine",
			HideCover: true,
			HideTOC:   true,
		},
		Blocks: []ReportBlock{
			{
				ID:      "body-1",
				Kind:    "markdown",
				Title:   "正文",
				Content: "内容",
			},
		},
	})

	if strings.Contains(html, `<div class="cover">`) {
		t.Fatalf("expected cover to be hidden")
	}
	if strings.Contains(html, `<div class="toc">`) {
		t.Fatalf("expected toc to be hidden")
	}
	if !strings.Contains(html, `body class="magazine"`) {
		t.Fatalf("expected body class override")
	}
	if !strings.Contains(html, ".hero{color:red;}") {
		t.Fatalf("expected custom css to be injected")
	}
	if strings.Contains(html, "console.log('layout')") {
		t.Fatalf("expected custom js to be ignored")
	}
}

func TestRenderReportHTMLIgnoresCustomShell(t *testing.T) {
	t.Parallel()

	html := RenderReportHTML("测试报告", "AI", &ReportState{
		Layout: ReportLayout{
			CustomHTMLShell: `<!DOCTYPE html><html><head><style>{{custom_css}}</style></head><body class="{{body_class}}"><main>{{content}}</main><aside>{{toc}}</aside><footer>{{author}} {{date}}</footer>{{chart_scripts}}<script>{{custom_js}}</script></body></html>`,
			CustomCSS:       ".page{padding:24px;}",
			CustomJS:        "window.__customShell = true;",
			BodyClass:       "page",
		},
		Blocks: []ReportBlock{
			{
				ID:      "custom-1",
				Kind:    "markdown",
				Title:   "自定义正文",
				Content: "Hello shell",
			},
		},
	})

	if !strings.Contains(html, "Hello shell") {
		t.Fatalf("expected report content to render")
	}
	if !strings.Contains(html, `body class="page"`) {
		t.Fatalf("expected body class to remain effective")
	}
	if strings.Contains(html, "<main>") || strings.Contains(html, "window.__customShell = true;") {
		t.Fatalf("expected custom shell/js to be ignored")
	}
}

func TestRenderReportHTMLUsesBlocksAsPrimaryModel(t *testing.T) {
	t.Parallel()

	html := RenderReportHTML("测试报告", "AI", &ReportState{
		Blocks: []ReportBlock{
			{ID: "intro", Kind: "markdown", Title: "导言", Content: "这是导言。"},
			{ID: "raw", Kind: "html", Title: "自定义块", Content: `<div class="custom">RAW</div>`},
			{ID: "trend", Kind: "chart", Title: "趋势图", ChartID: "chart_sales", Content: "图表解读"},
		},
		Charts: []ChartData{
			{ID: "chart_sales", Option: json.RawMessage(`{"xAxis":{"type":"category","data":["1月"]},"yAxis":{"type":"value"},"series":[{"type":"bar","data":[100]}]}`)},
		},
	})

	if !strings.Contains(html, "导言") || !strings.Contains(html, `class="custom"`) {
		t.Fatalf("expected markdown/html blocks to render")
	}
	if !strings.Contains(html, `data-chart-id="chart_sales"`) || !strings.Contains(html, "图表解读") {
		t.Fatalf("expected chart block to render chart and commentary")
	}
	if strings.Contains(html, `data-block-id="trend" data-block-kind="chart" data-chart-id="chart_sales"`) {
		t.Fatalf("expected chart block wrapper not to carry data-chart-id, got: %s", html)
	}
	if !strings.Contains(html, `document.querySelectorAll('.chart-box[data-chart-id="chart_sales"]')`) {
		t.Fatalf("expected chart runtime to target chart boxes only, got: %s", html)
	}
}

func TestRenderReportHTMLSanitizesUnsafeHTML(t *testing.T) {
	t.Parallel()

	html := RenderReportHTML(`<img src=x onerror=alert(1)>`, "AI", &ReportState{
		Blocks: []ReportBlock{
			{ID: "raw", Kind: "html", Title: "原始块", Content: `<div class="custom" onclick="alert(1)"><script>alert(1)</script><a href="javascript:alert(1)">bad</a><a href="https://example.com">good</a></div>`},
			{ID: "md", Kind: "markdown", Title: "正文", Content: `<script>alert(1)</script> **ok**`},
		},
	})

	if strings.Contains(html, "<script>alert(1)</script>") || strings.Contains(html, "onclick=") || strings.Contains(html, `href="javascript:alert(1)"`) {
		t.Fatalf("expected unsafe html/js to be removed, got: %s", html)
	}
	if !strings.Contains(html, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Fatalf("expected markdown html to be escaped, got: %s", html)
	}
	if !strings.Contains(html, `<a href="https://example.com" rel="noopener noreferrer" target="_blank">good</a>`) {
		t.Fatalf("expected safe link to remain, got: %s", html)
	}
	if !strings.Contains(html, "&lt;img src=x onerror=alert(1)&gt;") {
		t.Fatalf("expected title to be escaped, got: %s", html)
	}
}

func TestRenderReportHTMLAllowsRepeatedChartReferences(t *testing.T) {
	t.Parallel()

	html := RenderReportHTML("测试报告", "AI", &ReportState{
		Blocks: []ReportBlock{
			{ID: "analysis", Kind: "markdown", Title: "分析", Content: "{{chart:chart_sales}}\n\n{{chart:chart_sales}}"},
		},
		Charts: []ChartData{
			{ID: "chart_sales", Option: json.RawMessage(`{"series":[{"type":"bar","data":[1]}]}`)},
		},
	})

	if strings.Count(html, `data-chart-id="chart_sales" class="chart-box"`) != 2 {
		t.Fatalf("expected repeated chart references to render 2 containers, got html: %s", html)
	}
	if !strings.Contains(html, `id="chart_sales-ref-1"`) || !strings.Contains(html, `id="chart_sales-ref-2"`) {
		t.Fatalf("expected unique container ids for repeated chart references, got html: %s", html)
	}
}
