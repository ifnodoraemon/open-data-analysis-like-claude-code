package config

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	// LLM 配置
	LLMProvider    string // "openai" 或 "anthropic"
	LLMBaseURL     string
	LLMAPIEndpoint string
	LLMAPIKey      string
	LLMModel       string
	LLMDebug       bool
	LLMDebugDir    string

	// 服务配置
	ServerPort           string
	StorageRoot          string
	CacheRoot            string
	MetadataDBPath       string
	TempDir              string
	PythonMCPURL         string
	AuthSecret           string
	DefaultUserID        string
	DefaultUserEmail     string
	DefaultUserName      string
	DefaultUserPassword  string
	DefaultWorkspaceID   string
	DefaultWorkspaceName string

	// 生命周期管理
	SessionTTLHours    int    // 空闲 session 自动清理阈值（小时），0 = 不自动清理
	TraceRetentionDays int    // LLM debug trace 保留天数，0 = 永久保留
	TempCleanupOnStart bool   // 启动时清理 TempDir
	ReportEchartsUrl   string // ECharts 资源路径，默认为前端自托管静态资源
}

var Cfg *Config

func Load() {
	err := godotenv.Load()
	if err != nil {
		if _, statErr := os.Stat(".env"); statErr == nil {
			log.Printf("Warning: failed to load .env: %v", err)
		}
	}

	provider := strings.ToLower(getEnv("LLM_PROVIDER", "openai"))

	// 根据 Provider 设置默认值
	defaultBaseURL := "https://api.openai.com"
	defaultAPIEndpoint := "https://api.openai.com/v1/responses"
	defaultModel := "gpt-4o"
	if provider == "anthropic" {
		defaultBaseURL = "https://api.anthropic.com"
		defaultAPIEndpoint = "https://api.anthropic.com/v1/messages"
		defaultModel = "claude-sonnet-4-20250514"
	}

	Cfg = &Config{
		LLMProvider:          provider,
		LLMBaseURL:           getEnv("LLM_BASE_URL", defaultBaseURL),
		LLMAPIEndpoint:       getEnv("LLM_API_ENDPOINT", defaultAPIEndpoint),
		LLMAPIKey:            getEnv("LLM_API_KEY", ""),
		LLMModel:             getEnv("LLM_MODEL", defaultModel),
		LLMDebug:             getEnvBool("LLM_DEBUG", false),
		LLMDebugDir:          getEnv("LLM_DEBUG_DIR", "./data/llm-debug"),
		ServerPort:           getEnv("SERVER_PORT", "8080"),
		StorageRoot:          getEnv("STORAGE_ROOT", "./data/storage"),
		CacheRoot:            getEnv("CACHE_ROOT", "./data/cache"),
		MetadataDBPath:       getEnv("METADATA_DB_PATH", "./data/metadata/app.db"),
		TempDir:              getEnv("TEMP_DIR", "./data/tmp"),
		PythonMCPURL:         getEnv("PYTHON_MCP_URL", ""),
		AuthSecret:           getEnv("AUTH_SECRET", ""),
		DefaultUserID:        getEnv("DEFAULT_USER_ID", ""),
		DefaultUserEmail:     getEnv("DEFAULT_USER_EMAIL", ""),
		DefaultUserName:      getEnv("DEFAULT_USER_NAME", ""),
		DefaultUserPassword:  getEnv("DEFAULT_USER_PASSWORD", ""),
		DefaultWorkspaceID:   getEnv("DEFAULT_WORKSPACE_ID", ""),
		DefaultWorkspaceName: getEnv("DEFAULT_WORKSPACE_NAME", ""),

		SessionTTLHours:    getEnvInt("SESSION_TTL_HOURS", 0),
		TraceRetentionDays: getEnvInt("TRACE_RETENTION_DAYS", 0),
		TempCleanupOnStart: getEnvBool("TEMP_CLEANUP_ON_START", false),
		ReportEchartsUrl:   getEnv("REPORT_ECHARTS_URL", "/assets/echarts.min.js"),
	}

	if Cfg.LLMAPIKey == "" {
		log.Println("Warning: LLM_API_KEY is not set")
	}

	if Cfg.AuthSecret == "" || Cfg.AuthSecret == "replace-with-a-long-random-secret" {
		log.Println("CRITICAL: AUTH_SECRET is not set or uses the default placeholder. Tokens may be forgeable. Set a strong random secret.")
	}

	if len(Cfg.AuthSecret) < 32 {
		log.Printf("Warning: AUTH_SECRET is too short (%d chars). Recommend at least 32 characters.", len(Cfg.AuthSecret))
	}

	if Cfg.ReportEchartsUrl != "" &&
		!strings.HasPrefix(Cfg.ReportEchartsUrl, "/") &&
		!strings.HasPrefix(Cfg.ReportEchartsUrl, "https://") &&
		!strings.HasPrefix(Cfg.ReportEchartsUrl, "http://") {
		log.Printf("Warning: REPORT_ECHARTS_URL does not start with / or http(s)://, ignoring: %s", Cfg.ReportEchartsUrl)
		Cfg.ReportEchartsUrl = ""
	}

	log.Printf("config loaded llm_provider=%s llm_model=%s llm_endpoint=%s", Cfg.LLMProvider, Cfg.LLMModel, Cfg.LLMAPIEndpoint)
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func getEnvInt(key string, fallback int) int {
	value, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	result, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return result
}
