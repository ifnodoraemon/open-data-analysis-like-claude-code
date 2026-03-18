package handler

import (
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
