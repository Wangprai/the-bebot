package core

import (
	"errors"
	"testing"
	"time"
)

func TestNewMessage(t *testing.T) {
	now := time.Now()

	message, err := NewMessage(
		"message-001",
		"guild-001",
		"channel-001",
		"user-001",
		" Hello ",
		now,
		false,
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if message.Content != "Hello" {
		t.Errorf("expected trimmed content, got %q", message.Content)
	}
}

func TestNewMessageWithEmptyContent(t *testing.T) {
	_, err := NewMessage(
		"message-001",
		"guild-001",
		"channel-001",
		"user-001",
		"   ",
		time.Now(),
		false,
	)

	if !errors.Is(err, ErrInvalidMessage) {
		t.Errorf("expected ErrInvalidMessage, got %v", err)
	}
}

func TestNewMentionEvent(t *testing.T) {
	event, err := NewMentionEvent(
		"guild-001",
		"channel-001",
		"message-001",
		"user-001",
		"Hello The Bebot",
		time.Now(),
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if event.MessageID != "message-001" {
		t.Errorf("unexpected message ID: %s", event.MessageID)
	}
}

func TestNewConversationContext(t *testing.T) {
	now := time.Now()

	messages := []Message{
		{
			ID:        "message-002",
			GuildID:   "guild-001",
			ChannelID: "channel-001",
			UserID:    "user-001",
			Content:   "Second message",
			CreatedAt: now.Add(time.Minute),
		},
		{
			ID:        "message-001",
			GuildID:   "guild-001",
			ChannelID: "channel-001",
			UserID:    "user-001",
			Content:   "First message",
			CreatedAt: now,
		},
	}

	ctx, err := NewConversationContext(
		"guild-001",
		"channel-001",
		"message-002",
		messages,
		now,
		24*time.Hour,
		15,
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ctx.Messages[0].ID != "message-001" {
		t.Errorf("messages are not sorted oldest to newest")
	}

	if len(ctx.Messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(ctx.Messages))
	}

	expectedExpiry := now.Add(24 * time.Hour)

	if !ctx.ExpiresAt.Equal(expectedExpiry) {
		t.Errorf("unexpected expiration time")
	}
}

func TestConversationContextRejectsDifferentChannel(t *testing.T) {
	now := time.Now()

	messages := []Message{
		{
			ID:        "message-001",
			GuildID:   "guild-001",
			ChannelID: "channel-002",
			UserID:    "user-001",
			Content:   "Hello",
			CreatedAt: now,
		},
	}

	_, err := NewConversationContext(
		"guild-001",
		"channel-001",
		"message-001",
		messages,
		now,
		24*time.Hour,
		15,
	)

	if !errors.Is(err, ErrInvalidConversationContext) {
		t.Errorf("expected ErrInvalidConversationContext, got %v", err)
	}
}

func TestConversationContextExpired(t *testing.T) {
	now := time.Now()

	ctx := ConversationContext{
		CreatedAt: now,
		ExpiresAt: now.Add(24 * time.Hour),
	}

	if ctx.IsExpired(now.Add(time.Hour)) {
		t.Error("context should not be expired")
	}

	if !ctx.IsExpired(now.Add(25 * time.Hour)) {
		t.Error("context should be expired")
	}
}
