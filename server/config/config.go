package config

import (
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	// LLM 配置
	LLMProvider string // "openai" 或 "anthropic"
	LLMBaseURL  string
	LLMAPIKey   string
	LLMModel    string

	// 服务配置
	ServerPort           string
	StorageRoot          string
	CacheRoot            string
	MetadataDBPath       string
	TempDir              string
	DefaultUserID        string
	DefaultUserEmail     string
	DefaultUserName      string
	DefaultWorkspaceID   string
	DefaultWorkspaceName string
}

var Cfg *Config

func Load() {
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: .env file not found, using environment variables")
	}

	provider := strings.ToLower(getEnv("LLM_PROVIDER", "openai"))

	// 根据 Provider 设置默认值
	defaultBaseURL := "https://api.openai.com/v1"
	defaultModel := "gpt-4o"
	if provider == "anthropic" {
		defaultBaseURL = "https://api.anthropic.com"
		defaultModel = "claude-sonnet-4-20250514"
	}

	Cfg = &Config{
		LLMProvider:          provider,
		LLMBaseURL:           getEnv("LLM_BASE_URL", defaultBaseURL),
		LLMAPIKey:            getEnv("LLM_API_KEY", ""),
		LLMModel:             getEnv("LLM_MODEL", defaultModel),
		ServerPort:           getEnv("SERVER_PORT", "8080"),
		StorageRoot:          getEnv("STORAGE_ROOT", "./storage"),
		CacheRoot:            getEnv("CACHE_ROOT", "./data/cache"),
		MetadataDBPath:       getEnv("METADATA_DB_PATH", "./data/metadata/app.db"),
		TempDir:              getEnv("TEMP_DIR", "./tmp"),
		DefaultUserID:        getEnv("DEFAULT_USER_ID", "u_demo"),
		DefaultUserEmail:     getEnv("DEFAULT_USER_EMAIL", "demo@example.com"),
		DefaultUserName:      getEnv("DEFAULT_USER_NAME", "Demo User"),
		DefaultWorkspaceID:   getEnv("DEFAULT_WORKSPACE_ID", "w_demo"),
		DefaultWorkspaceName: getEnv("DEFAULT_WORKSPACE_NAME", "Default Workspace"),
	}

	if Cfg.LLMAPIKey == "" {
		log.Println("Warning: LLM_API_KEY is not set")
	}

	log.Printf("LLM Provider: %s, Model: %s, Base URL: %s", Cfg.LLMProvider, Cfg.LLMModel, Cfg.LLMBaseURL)
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
