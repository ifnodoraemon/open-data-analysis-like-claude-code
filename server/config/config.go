package config

import (
	"log"
	"os"
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
	}

	if Cfg.LLMAPIKey == "" {
		log.Println("Warning: LLM_API_KEY is not set")
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
