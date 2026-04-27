package handler

import "testing"

func TestIsPlaceholderSecret(t *testing.T) {
	for _, value := range []string{
		"CHANGE_ME_generate-a-long-random-secret-here",
		"replace-with-a-long-random-secret",
		"admin",
		"password",
		"changeme",
	} {
		if !isPlaceholderSecret(value) {
			t.Fatalf("expected placeholder to be rejected: %q", value)
		}
	}

	if isPlaceholderSecret("8f3995c9dbd14f1eb1cf8d3f9a296e11") {
		t.Fatal("expected random-looking value to be accepted")
	}
}
