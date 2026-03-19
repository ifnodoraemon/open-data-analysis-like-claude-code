package agent

import (
	"strings"
	"testing"
)

func TestBuildPlannerPromptIncludesAmbiguityGuardrail(t *testing.T) {
	t.Parallel()

	prompt := BuildPlannerPrompt(nil)
	if !strings.Contains(prompt, "必须先向用户确认") {
		t.Fatalf("expected ambiguity guardrail in prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "字段映射") {
		t.Fatalf("expected field-mapping ambiguity to be covered, got %q", prompt)
	}
	if !strings.Contains(prompt, "已写入草稿") || !strings.Contains(prompt, "report_finalize") {
		t.Fatalf("expected draft-vs-finalize guardrail in prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "working memory") || !strings.Contains(prompt, "不能替代") {
		t.Fatalf("expected working memory delivery guardrail in prompt, got %q", prompt)
	}
}
