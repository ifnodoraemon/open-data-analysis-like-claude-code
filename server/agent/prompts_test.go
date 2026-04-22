package agent

import (
	"strings"
	"testing"
)

func TestPolicyPromptIncludesGuardrails(t *testing.T) {
	t.Parallel()

	prompt := BuildPolicyPrompt()
	lower := strings.ToLower(prompt)
	if !strings.Contains(lower, "ambiguity") {
		t.Fatalf("expected ambiguity guardrail in prompt, got %q", prompt)
	}
	if !strings.Contains(lower, "assumptions") {
		t.Fatalf("expected assumption-related guidance in prompt, got %q", prompt)
	}
	if !strings.Contains(lower, "finalize") {
		t.Fatalf("expected finalize guardrail in prompt, got %q", prompt)
	}
	if !strings.Contains(lower, "data analysis") {
		t.Fatalf("expected domain boundary constraint in prompt, got %q", prompt)
	}
}
