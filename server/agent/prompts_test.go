package agent

import (
	"strings"
	"testing"
)

func TestPolicyPromptIncludesGuardrails(t *testing.T) {
	t.Parallel()

	prompt := BuildPolicyPrompt()
	if !strings.Contains(prompt, "必须先向用户确认") {
		t.Fatalf("expected ambiguity guardrail in prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "字段映射") {
		t.Fatalf("expected field-mapping ambiguity to be covered, got %q", prompt)
	}
	if !strings.Contains(prompt, "经过 finalize 的内容才可交付") {
		t.Fatalf("expected finalize guardrail in prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "working memory") {
		t.Fatalf("expected working memory guardrail in prompt, got %q", prompt)
	}
}
