package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
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
	Mood         MoodConfig
	Personality  PersonalityConfig

	LogLevel string
}

type MoodConfig struct {
	UserCooldown time.Duration
}

type PersonalityConfig struct {
	Name           string
	ExtraTraits    []string
	LanguagePolicy string
}

type DiscordConfig struct {
	BotToken string
}

type MongoDBConfig struct {
	URI      string
	Database string
}

type LLMConfig struct {
	APIKey    string
	BaseURL   string
	Model     string
	MaxTokens int
}

type ConversationConfig struct {
	Timeout               time.Duration
	MentionTimeout        time.Duration
	MaxMessages           int
	UserCooldown          time.Duration
	MaxConcurrentMentions int
}

type MemoryConfig struct {
	Enabled              bool
	MaxRetrievedMemories int
	WorkerCount          int
}

func Load() (*Config, error) {
	if err := godotenv.Load(); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}

	return LoadFromEnv()
}

func LoadFromEnv() (*Config, error) {
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
			APIKey:    getEnv("LLM_API_KEY", ""),
			BaseURL:   getEnv("LLM_BASE_URL", ""),
			Model:     getEnv("LLM_MODEL", ""),
			MaxTokens: 600,
		},

		Conversation: ConversationConfig{
			Timeout:               30 * time.Second,
			MentionTimeout:        60 * time.Second,
			MaxMessages:           15,
			UserCooldown:          5 * time.Second,
			MaxConcurrentMentions: 8,
		},

		Memory: MemoryConfig{
			Enabled:              true,
			MaxRetrievedMemories: 5,
			WorkerCount:          2,
		},

		Mood: MoodConfig{
			UserCooldown: 2 * time.Minute,
		},

		Personality: PersonalityConfig{
			Name:           strings.TrimSpace(getEnv("PERSONALITY_NAME", "")),
			ExtraTraits:    getCSV("PERSONALITY_EXTRA_TRAITS"),
			LanguagePolicy: strings.TrimSpace(getEnv("PERSONALITY_LANGUAGE_POLICY", "")),
		},

		LogLevel: getEnv("LOG_LEVEL", "info"),
	}

	var err error

	if cfg.LLM.MaxTokens, err = getInt(
		"LLM_MAX_TOKENS", cfg.LLM.MaxTokens,
	); err != nil {
		return nil, err
	}

	if cfg.Conversation.Timeout, err = getDuration(
		"CONVERSATION_TIMEOUT", cfg.Conversation.Timeout,
	); err != nil {
		return nil, err
	}

	if cfg.Conversation.MentionTimeout, err = getDuration(
		"MENTION_TIMEOUT", cfg.Conversation.MentionTimeout,
	); err != nil {
		return nil, err
	}

	if cfg.Conversation.UserCooldown, err = getDuration(
		"MENTION_USER_COOLDOWN", cfg.Conversation.UserCooldown,
	); err != nil {
		return nil, err
	}

	if cfg.Conversation.MaxConcurrentMentions, err = getInt(
		"MAX_CONCURRENT_MENTIONS", cfg.Conversation.MaxConcurrentMentions,
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

	if cfg.Memory.MaxRetrievedMemories, err = getInt(
		"MEMORY_MAX_RETRIEVED_MEMORIES", cfg.Memory.MaxRetrievedMemories,
	); err != nil {
		return nil, err
	}

	if cfg.Memory.WorkerCount, err = getInt(
		"MEMORY_WORKER_COUNT", cfg.Memory.WorkerCount,
	); err != nil {
		return nil, err
	}

	if cfg.Mood.UserCooldown, err = getDuration(
		"MOOD_USER_COOLDOWN", cfg.Mood.UserCooldown,
	); err != nil {
		return nil, err
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	if c == nil {
		return errors.New("config is required")
	}

	if c.LLM.MaxTokens <= 0 {
		return errors.New("LLM_MAX_TOKENS must be greater than zero")
	}

	if c.Conversation.Timeout <= 0 {
		return errors.New("CONVERSATION_TIMEOUT must be greater than zero")
	}

	if c.Conversation.MentionTimeout <= 0 {
		return errors.New("MENTION_TIMEOUT must be greater than zero")
	}

	if c.Conversation.MentionTimeout <= c.Conversation.Timeout {
		return errors.New("MENTION_TIMEOUT must be greater than CONVERSATION_TIMEOUT")
	}

	if c.Conversation.UserCooldown < 0 {
		return errors.New("MENTION_USER_COOLDOWN must be zero or greater")
	}

	if c.Conversation.MaxConcurrentMentions <= 0 {
		return errors.New("MAX_CONCURRENT_MENTIONS must be greater than zero")
	}

	if c.Conversation.MaxMessages <= 0 {
		return errors.New("MAX_CONTEXT_MESSAGES must be greater than zero")
	}

	if c.Memory.MaxRetrievedMemories <= 0 {
		return errors.New("MEMORY_MAX_RETRIEVED_MEMORIES must be greater than zero")
	}

	if c.Memory.WorkerCount <= 0 {
		return errors.New("MEMORY_WORKER_COUNT must be greater than zero")
	}

	if c.Mood.UserCooldown < 0 {
		return errors.New("MOOD_USER_COOLDOWN must be zero or greater")
	}

	return nil
}

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

func getCSV(key string) []string {
	raw := strings.TrimSpace(getEnv(key, ""))
	if raw == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))

	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}

	return result
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
