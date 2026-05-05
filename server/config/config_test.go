package config

import (
	"os"
	"testing"
)

func TestDefaultLLMAPIEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		baseURL  string
		want     string
	}{
		{"openai default", "openai", "https://api.openai.com", "https://api.openai.com/v1/responses"},
		{"anthropic default", "anthropic", "https://api.anthropic.com", "https://api.anthropic.com/v1/messages"},
		{"deepseek auto chat.completions", "openai", "https://api.deepseek.com", "https://api.deepseek.com/chat/completions"},
		{"general openai-compat with /v1", "openai", "https://api.groq.com/v1", "https://api.groq.com/v1/chat/completions"},
		{"general openai-compat without /v1", "openai", "https://api.groq.com", "https://api.groq.com/v1/chat/completions"},
		{"empty baseURL", "openai", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := defaultLLMAPIEndpoint(tt.provider, tt.baseURL)
			if got != tt.want {
				t.Errorf("defaultLLMAPIEndpoint(%q, %q) = %q, want %q", tt.provider, tt.baseURL, got, tt.want)
			}
		})
	}
}

func TestTrustedReportScriptURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"http://cdn.jsdelivr.net/npm/echarts@5/dist/echarts.min.js", false},
		{"https://cdn.jsdelivr.net/npm/echarts@5/dist/echarts.min.js", true},
		{"https://cdnjs.cloudflare.com/ajax/libs/echarts/5.5.0/echarts.min.js", true},
		{"https://evil.com/echarts.min.js", false},
		{"/assets/echarts.min.js", true},
		{"", false},
		{"//cdn.jsdelivr.net/npm/echarts/dist/echarts.min.js", false},
	}

	for _, tt := range tests {
		t.Run("url="+tt.url, func(t *testing.T) {
			got := trustedReportScriptURL(tt.url)
			if got != tt.want {
				t.Errorf("trustedReportScriptURL(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestLoadLLMTimeoutConfig(t *testing.T) {
	previous := Cfg
	defer func() { Cfg = previous }()

	t.Setenv("LLM_API_KEY", "test-key")
	t.Setenv("AUTH_SECRET", "abcdefghijklmnopqrstuvwxyz123456")
	t.Setenv("LLM_HTTP_TIMEOUT_SECONDS", "321")
	t.Setenv("LLM_RETRY_BUDGET_SECONDS", "654")

	Load()

	if Cfg.LLMHTTPTimeoutSec != 321 {
		t.Fatalf("expected LLMHTTPTimeoutSec=321, got %d", Cfg.LLMHTTPTimeoutSec)
	}
	if Cfg.LLMRetryBudgetSec != 654 {
		t.Fatalf("expected LLMRetryBudgetSec=654, got %d", Cfg.LLMRetryBudgetSec)
	}
}

func TestGetEnv(t *testing.T) {
	os.Unsetenv("TEST_GETENV_KEY")

	got := getEnv("TEST_GETENV_KEY", "default")
	if got != "default" {
		t.Errorf("expected default, got %q", got)
	}

	os.Setenv("TEST_GETENV_KEY", "custom")
	defer os.Unsetenv("TEST_GETENV_KEY")

	got = getEnv("TEST_GETENV_KEY", "default")
	if got != "custom" {
		t.Errorf("expected custom, got %q", got)
	}
}

func TestGetEnvBool(t *testing.T) {
	os.Unsetenv("TEST_BOOL_KEY")

	if getEnvBool("TEST_BOOL_KEY", true) != true {
		t.Error("expected default true")
	}

	for _, val := range []string{"1", "true", "TRUE", "yes", "on"} {
		os.Setenv("TEST_BOOL_KEY", val)
		if !getEnvBool("TEST_BOOL_KEY", false) {
			t.Errorf("expected true for value %q", val)
		}
	}

	for _, val := range []string{"0", "false", "FALSE", "no", "off"} {
		os.Setenv("TEST_BOOL_KEY", val)
		if getEnvBool("TEST_BOOL_KEY", true) {
			t.Errorf("expected false for value %q", val)
		}
	}

	os.Unsetenv("TEST_BOOL_KEY")
}

func TestGetEnvInt(t *testing.T) {
	os.Unsetenv("TEST_INT_KEY")

	if getEnvInt("TEST_INT_KEY", 42) != 42 {
		t.Error("expected default 42")
	}

	os.Setenv("TEST_INT_KEY", "100")
	defer os.Unsetenv("TEST_INT_KEY")

	if getEnvInt("TEST_INT_KEY", 42) != 100 {
		t.Error("expected 100")
	}

	os.Setenv("TEST_INT_KEY", "not-a-number")
	if getEnvInt("TEST_INT_KEY", 42) != 42 {
		t.Error("expected default 42 for invalid input")
	}
}
