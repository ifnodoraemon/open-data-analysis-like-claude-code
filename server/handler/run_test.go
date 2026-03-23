package handler

import (
	"strings"
	"testing"

	"github.com/ifnodoraemon/openDataAnalysis/domain"
)

func TestSummarizeRunMessagePrefersUISummary(t *testing.T) {
	t.Parallel()

	msg := domain.RunMessage{
		Type:    "tool_result",
		Name:    "task_delegate",
		Content: `{"tool":"task_delegate","ui_summary":"子 Agent researcher 已完成: 收集到了 3 个事实","summary_text":"旧摘要"}`,
	}

	summary := summarizeRunMessage(msg)
	if summary != "子 Agent researcher 已完成: 收集到了 3 个事实" {
		t.Fatalf("expected ui_summary to win, got %q", summary)
	}
}

func TestSummarizeRunMessageFallsBackToSummaryText(t *testing.T) {
	t.Parallel()

	msg := domain.RunMessage{
		Type:    "tool_result",
		Name:    "data_list_tables",
		Content: `{"tool":"data_list_tables","summary_text":"已导入 2 张表"}`,
	}

	summary := summarizeRunMessage(msg)
	if summary != "已导入 2 张表" {
		t.Fatalf("expected summary_text fallback, got %q", summary)
	}
}

func TestRenderReportHTMLFromSnapshotRegeneratesCurrentTemplate(t *testing.T) {
	t.Parallel()

	report := &domain.Report{
		Title: "测试报告",
		SnapshotJSON: `{
			"version":"v3",
			"title":"测试报告",
			"author":"AI",
			"blocks":[
				{"id":"blk_1","kind":"markdown","title":"一、概览","content":"说明"},
				{"id":"blk_chart","kind":"chart","title":"趋势图","content":"图表说明","chartId":"chart_sales"}
			],
			"charts":[
				{"id":"chart_sales","option":{"series":[{"type":"bar","data":[1]}]}}
			]
		}`,
	}

	html, ok := renderReportHTMLFromSnapshot(report)
	if !ok {
		t.Fatal("expected snapshot to be rendered")
	}
	if !strings.Contains(html, `<h2>一、概览</h2>`) {
		t.Fatalf("expected regenerated html to retain original prefixed titles, got: %s", html)
	}
	if !strings.Contains(html, `document.querySelectorAll('.chart-box[data-chart-id="chart_sales"]')`) {
		t.Fatalf("expected regenerated html to use chart-box-only selector, got: %s", html)
	}
	if strings.Contains(html, `data-block-id="blk_chart" data-block-kind="chart" data-chart-id="chart_sales"`) {
		t.Fatalf("expected chart wrapper not to retain duplicate data-chart-id, got: %s", html)
	}
}
