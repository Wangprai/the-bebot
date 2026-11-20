package config

import (
	"errors"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	AppName string
	AppEnv  string

	Discord DiscordConfig
	MongoDB MongoDBConfig
	LLM     LLMConfig

	Conversation ConversationConfig
	Memory       MemoryConfig

	LogLevel string
}

type DiscordConfig struct {
	BotToken string
}

type MongoDBConfig struct {
	URI      string
	Database string
}

type LLMConfig struct {
	Provider string
	APIKey   string
	BaseURL  string
	Model    string
}

type ConversationConfig struct {
	Timeout     time.Duration
	MaxMessages int
}

type MemoryConfig struct {
	Enabled       bool
	MaxCandidates int
	WorkerCount   int
	QueueSize     int
}

func Load() (*Config, error) {
	if err := godotenv.Load(); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}

	cfg := &Config{
		AppName: getEnv("APP_NAME", "the-bebot"),
		AppEnv:  getEnv("APP_ENV", "development"),

		Discord: DiscordConfig{
			BotToken: getEnv("DISCORD_BOT_TOKEN", ""),
		},

		MongoDB: MongoDBConfig{
			URI:      getEnv("MONGODB_URI", "mongodb://127.0.0.1:27017"),
			Database: getEnv("MONGODB_DATABASE", "the_bebot"),
		},

		LLM: LLMConfig{
			Provider: getEnv("LLM_PROVIDER", ""),
			APIKey:   getEnv("LLM_API_KEY", ""),
			BaseURL:  getEnv("LLM_BASE_URL", ""),
			Model:    getEnv("LLM_MODEL", ""),
		},

		Conversation: ConversationConfig{
			Timeout:     30 * time.Second,
			MaxMessages: 15,
		},

		Memory: MemoryConfig{
			Enabled:       true,
			MaxCandidates: 5,
			WorkerCount:   2,
			QueueSize:     100,
		},

		LogLevel: getEnv("LOG_LEVEL", "debug"),
	}

	var err error

	if cfg.Conversation.Timeout, err = getDuration(
		"CONVERSATION_TIMEOUT", cfg.Conversation.Timeout,
	); err != nil {
		return nil, err
	}

	if cfg.Conversation.MaxMessages, err = getInt(
		"MAX_CONTEXT_MESSAGES", cfg.Conversation.MaxMessages,
	); err != nil {
		return nil, err
	}

	if cfg.Memory.Enabled, err = getBool(
		"MEMORY_ENABLED", cfg.Memory.Enabled,
	); err != nil {
		return nil, err
	}

	if cfg.Memory.MaxCandidates, err = getInt(
		"MEMORY_MAX_CANDIDATES", cfg.Memory.MaxCandidates,
	); err != nil {
		return nil, err
	}

	if cfg.Memory.WorkerCount, err = getInt(
		"MEMORY_WORKER_COUNT", cfg.Memory.WorkerCount,
	); err != nil {
		return nil, err
	}

	if cfg.Memory.QueueSize, err = getInt(
		"MEMORY_QUEUE_SIZE", cfg.Memory.QueueSize,
	); err != nil {
		return nil, err
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

func getInt(key string, fallback int) (int, error) {
	value := getEnv(key, strconv.Itoa(fallback))

	result, err := strconv.Atoi(value)
	if err != nil {
		return 0, errors.New(key + " must be an integer")
	}

	return result, nil
}

func getBool(key string, fallback bool) (bool, error) {
	value := getEnv(key, strconv.FormatBool(fallback))

	result, err := strconv.ParseBool(value)
	if err != nil {
		return false, errors.New(key + " must be a boolean")
	}

	return result, nil
}

func getDuration(key string, fallback time.Duration) (time.Duration, error) {
	value := getEnv(key, fallback.String())

	result, err := time.ParseDuration(value)
	if err != nil {
		return 0, errors.New(key + " must be a valid duration")
	}

	return result, nil
}
