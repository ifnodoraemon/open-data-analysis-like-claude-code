package agent

import (
	"strings"
	"testing"
)

func TestSanitizeReportHTML_RemovesEventHandlers(t *testing.T) {
	tests := []struct {
		name  string
		input string
		bad   []string
	}{
		{"onclick", `<div onclick="alert(1)">hello</div>`, []string{"onclick"}},
		{"onerror", `<img onerror="alert(1)" src="x">`, []string{"onerror"}},
		{"onload", `<body onload="alert(1)">`, []string{"onload"}},
		{"no-space onerror", `<img/onerror="alert(1)" src="x">`, []string{"onerror"}},
		{"case-insensitive", `<div ONCLICK="alert(1)">`, []string{"ONCLICK"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeReportHTML(tt.input)
			for _, b := range tt.bad {
				if strings.Contains(result, b) {
					t.Errorf("result still contains %q: %s", b, result)
				}
			}
		})
	}
}

func TestSanitizeReportHTML_BlocksDangerousProtocols(t *testing.T) {
	tests := []struct {
		name  string
		input string
		bad   string
	}{
		{"javascript href", `<a href="javascript:alert(1)">click</a>`, "javascript:"},
		{"vbscript href", `<a href="vbscript:msgbox">click</a>`, "vbscript:"},
		{"data src", `<img src="data:text/html,<script>alert(1)</script>">`, "data:"},
		{"blob src", `<img src="blob:http://evil">`, "blob:"},
		{"javascript with spaces", `<a href="  javascript:alert(1)">click</a>`, "javascript:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeReportHTML(tt.input)
			if strings.Contains(result, tt.bad) {
				t.Errorf("result still contains %q: %s", tt.bad, result)
			}
		})
	}
}

func TestSanitizeReportHTML_PreservesSafeContent(t *testing.T) {
	input := `<a href="https://example.com">link</a><p>text</p>`
	result := sanitizeReportHTML(input)
	if !strings.Contains(result, `href="https://example.com"`) {
		t.Errorf("safe href was removed: %s", result)
	}
	if !strings.Contains(result, "<p>text</p>") {
		t.Errorf("safe content was removed: %s", result)
	}
}

func TestApplyReportHTMLGuardrail(t *testing.T) {
	input := `{"html":"<div onclick=\"alert(1)\">bad</div>","other":"safe"}`
	result := applyReportHTMLGuardrail(input)
	if strings.Contains(result, "onclick") {
		t.Errorf("onclick not removed: %s", result)
	}
	if !strings.Contains(result, `"other":"safe"`) {
		t.Errorf("non-html field modified: %s", result)
	}
}

func TestApplyReportHTMLGuardrail_NoHTMLField(t *testing.T) {
	input := `{"message":"hello"}`
	result := applyReportHTMLGuardrail(input)
	if result != input {
		t.Errorf("non-html payload was modified: %s", result)
	}
}
