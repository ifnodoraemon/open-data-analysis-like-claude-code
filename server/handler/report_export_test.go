package handler

import (
	"strings"
	"testing"
)

func TestSanitizeExportHTMLRemovesActiveContentAndRemoteResources(t *testing.T) {
	t.Parallel()

	input := `<html><body>
	<script>alert(1)</script>
	<iframe src="https://evil.example/x"></iframe>
	<img src="https://evil.example/a.png" onerror="alert(2)">
	<a href="javascript:alert(3)" onclick="alert(4)">bad</a>
	<p>safe text</p>
	</body></html>`

	out := sanitizeExportHTML(input)
	lower := strings.ToLower(out)

	badFragments := []string{
		"<script",
		"<iframe",
		"onerror=",
		"onclick=",
		"https://evil.example",
		"javascript:",
	}
	for _, frag := range badFragments {
		if strings.Contains(lower, strings.ToLower(frag)) {
			t.Fatalf("expected sanitized export html to drop %q, got: %s", frag, out)
		}
	}
	if !strings.Contains(out, "safe text") {
		t.Fatalf("expected safe text to remain, got: %s", out)
	}
}

func TestSanitizeExportHTMLPreservesSafeDataImage(t *testing.T) {
	t.Parallel()

	input := `<html><body>
	<img src="data:image/png;base64,aGVsbG8=">
	<img src="data:text/html;base64,PHNjcmlwdD5iYWQoKTwvc2NyaXB0Pg==">
	</body></html>`

	out := sanitizeExportHTML(input)

	if !strings.Contains(out, `src="data:image/png;base64,aGVsbG8="`) {
		t.Fatalf("expected safe data image to remain, got: %s", out)
	}
	if strings.Contains(strings.ToLower(out), "data:text/html") {
		t.Fatalf("expected non-image data URL to be stripped, got: %s", out)
	}
}
