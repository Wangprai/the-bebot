package config

import (
	"os"
	"testing"
	"time"
)

func unsetRelevantEnv(t *testing.T) {
	t.Helper()

	keys := []string{
		"APP_NAME",
		"APP_ENV",
		"DISCORD_BOT_TOKEN",
		"MONGODB_URI",
		"MONGODB_DATABASE",
		"LLM_API_KEY",
		"LLM_BASE_URL",
		"LLM_MODEL",
		"CONVERSATION_TIMEOUT",
		"MENTION_TIMEOUT",
		"MAX_CONTEXT_MESSAGES",
		"MEMORY_ENABLED",
		"MEMORY_MAX_RETRIEVED_MEMORIES",
		"MEMORY_WORKER_COUNT",
		"PERSONALITY_NAME",
		"PERSONALITY_EXTRA_TRAITS",
		"PERSONALITY_LANGUAGE_POLICY",
		"LOG_LEVEL",
	}

	for _, key := range keys {
		t.Setenv(key, "")
		_ = os.Unsetenv(key)
	}
}

func TestLoadFromEnvDefaults(t *testing.T) {
	unsetRelevantEnv(t)

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv() error = %v", err)
	}

	if cfg.Conversation.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s", cfg.Conversation.Timeout)
	}

	if cfg.Conversation.MentionTimeout != 60*time.Second {
		t.Errorf("MentionTimeout = %v, want 60s", cfg.Conversation.MentionTimeout)
	}

	if cfg.Conversation.MaxMessages != 15 {
		t.Errorf("MaxMessages = %d, want 15", cfg.Conversation.MaxMessages)
	}

	if !cfg.Memory.Enabled {
		t.Error("Memory.Enabled = false, want true")
	}

	if cfg.Memory.MaxRetrievedMemories != 5 {
		t.Errorf("MaxRetrievedMemories = %d, want 5", cfg.Memory.MaxRetrievedMemories)
	}

	if cfg.Memory.WorkerCount != 2 {
		t.Errorf("WorkerCount = %d, want 2", cfg.Memory.WorkerCount)
	}

	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want info", cfg.LogLevel)
	}
}

func TestLoadFromEnvOverrides(t *testing.T) {
	unsetRelevantEnv(t)

	t.Setenv("APP_NAME", "bebot-test")
	t.Setenv("APP_ENV", "production")
	t.Setenv("DISCORD_BOT_TOKEN", "token-123")
	t.Setenv("MONGODB_URI", "mongodb://localhost:27018")
	t.Setenv("MONGODB_DATABASE", "bebot_custom")
	t.Setenv("LLM_BASE_URL", "https://llm.example.com/v1/chat/completions")
	t.Setenv("LLM_API_KEY", "key-123")
	t.Setenv("LLM_MODEL", "model-x")
	t.Setenv("CONVERSATION_TIMEOUT", "45s")
	t.Setenv("MENTION_TIMEOUT", "90s")
	t.Setenv("MAX_CONTEXT_MESSAGES", "20")
	t.Setenv("MEMORY_ENABLED", "false")
	t.Setenv("MEMORY_MAX_RETRIEVED_MEMORIES", "8")
	t.Setenv("MEMORY_WORKER_COUNT", "4")
	t.Setenv("PERSONALITY_NAME", "Custom Bebot")
	t.Setenv("PERSONALITY_EXTRA_TRAITS", "curious, sharp , ")
	t.Setenv("PERSONALITY_LANGUAGE_POLICY", "Speak Thai naturally")
	t.Setenv("LOG_LEVEL", "debug")

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv() error = %v", err)
	}

	if cfg.AppName != "bebot-test" || cfg.AppEnv != "production" {
		t.Errorf("unexpected app settings: %+v", cfg)
	}

	if cfg.Conversation.Timeout != 45*time.Second {
		t.Errorf("Timeout = %v, want 45s", cfg.Conversation.Timeout)
	}

	if cfg.Conversation.MentionTimeout != 90*time.Second {
		t.Errorf("MentionTimeout = %v, want 90s", cfg.Conversation.MentionTimeout)
	}

	if cfg.Conversation.MaxMessages != 20 {
		t.Errorf("MaxMessages = %d, want 20", cfg.Conversation.MaxMessages)
	}

	if cfg.Memory.Enabled {
		t.Error("Memory.Enabled = true, want false")
	}

	if cfg.Memory.MaxRetrievedMemories != 8 {
		t.Errorf("MaxRetrievedMemories = %d, want 8", cfg.Memory.MaxRetrievedMemories)
	}

	if cfg.Memory.WorkerCount != 4 {
		t.Errorf("WorkerCount = %d, want 4", cfg.Memory.WorkerCount)
	}

	if cfg.Personality.Name != "Custom Bebot" {
		t.Errorf("Personality.Name = %q, want Custom Bebot", cfg.Personality.Name)
	}

	if len(cfg.Personality.ExtraTraits) != 2 ||
		cfg.Personality.ExtraTraits[0] != "curious" ||
		cfg.Personality.ExtraTraits[1] != "sharp" {
		t.Errorf("Personality.ExtraTraits = %v", cfg.Personality.ExtraTraits)
	}

	if cfg.Personality.LanguagePolicy != "Speak Thai naturally" {
		t.Errorf("Personality.LanguagePolicy = %q", cfg.Personality.LanguagePolicy)
	}

	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want debug", cfg.LogLevel)
	}
}

func TestLoadFromEnvRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name string
		key  string
		val  string
	}{
		{name: "invalid timeout format", key: "CONVERSATION_TIMEOUT", val: "not-a-duration"},
		{name: "zero timeout", key: "CONVERSATION_TIMEOUT", val: "0s"},
		{name: "negative timeout", key: "CONVERSATION_TIMEOUT", val: "-5s"},
		{name: "invalid mention timeout format", key: "MENTION_TIMEOUT", val: "not-a-duration"},
		{name: "zero mention timeout", key: "MENTION_TIMEOUT", val: "0s"},
		{name: "negative mention timeout", key: "MENTION_TIMEOUT", val: "-5s"},
		{name: "mention timeout equal to conversation timeout", key: "MENTION_TIMEOUT", val: "30s"},
		{name: "mention timeout less than conversation timeout", key: "MENTION_TIMEOUT", val: "20s"},
		{name: "non-int max messages", key: "MAX_CONTEXT_MESSAGES", val: "abc"},
		{name: "zero max messages", key: "MAX_CONTEXT_MESSAGES", val: "0"},
		{name: "negative max messages", key: "MAX_CONTEXT_MESSAGES", val: "-1"},
		{name: "invalid memory enabled", key: "MEMORY_ENABLED", val: "not-bool"},
		{name: "non-int max retrieved memories", key: "MEMORY_MAX_RETRIEVED_MEMORIES", val: "abc"},
		{name: "zero max retrieved memories", key: "MEMORY_MAX_RETRIEVED_MEMORIES", val: "0"},
		{name: "negative max retrieved memories", key: "MEMORY_MAX_RETRIEVED_MEMORIES", val: "-2"},
		{name: "non-int worker count", key: "MEMORY_WORKER_COUNT", val: "abc"},
		{name: "zero worker count", key: "MEMORY_WORKER_COUNT", val: "0"},
		{name: "negative worker count", key: "MEMORY_WORKER_COUNT", val: "-3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unsetRelevantEnv(t)
			t.Setenv("CONVERSATION_TIMEOUT", "30s")
			t.Setenv("MENTION_TIMEOUT", "60s")
			t.Setenv("MAX_CONTEXT_MESSAGES", "15")
			t.Setenv("MEMORY_ENABLED", "true")
			t.Setenv("MEMORY_MAX_RETRIEVED_MEMORIES", "5")
			t.Setenv("MEMORY_WORKER_COUNT", "2")
			t.Setenv(tt.key, tt.val)

			if _, err := LoadFromEnv(); err == nil {
				t.Fatalf("expected error for %s=%q", tt.key, tt.val)
			}
		})
	}
}
