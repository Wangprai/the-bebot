package core

import (
	"errors"
	"testing"
	"time"
)

func TestBuildContext(t *testing.T) {
	now := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)

	service := &conversationService{
		clock: func() time.Time {
			return now
		},
	}

	event := MentionEvent{
		GuildID:     "guild-001",
		ChannelID:   "channel-001",
		MessageID:   "message-002",
		UserID:      "user-001",
		Content:     "@TheBebot hello",
		MentionedAt: now,
	}

	messages := []Message{
		{
			ID:        "message-001",
			GuildID:   "guild-001",
			ChannelID: "channel-001",
			UserID:    "user-001",
			Content:   "Hello",
			CreatedAt: now.Add(-time.Minute),
		},
		{
			ID:        "message-002",
			GuildID:   "guild-001",
			ChannelID: "channel-001",
			UserID:    "user-001",
			Content:   "@TheBebot hello",
			CreatedAt: now,
		},
	}

	ctx, err := service.BuildContext(
		event,
		messages,
		15,
		24*time.Hour,
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ctx.TriggerMessageID != event.MessageID {
		t.Error("trigger message ID does not match")
	}

	if len(ctx.Messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(ctx.Messages))
	}
}

func TestBuildContextRejectsMissingTrigger(t *testing.T) {
	now := time.Now()

	service := NewConversationService()

	event := MentionEvent{
		GuildID:     "guild-001",
		ChannelID:   "channel-001",
		MessageID:   "missing-message",
		UserID:      "user-001",
		Content:     "Hello",
		MentionedAt: now,
	}

	messages := []Message{
		{
			ID:        "message-001",
			GuildID:   "guild-001",
			ChannelID: "channel-001",
			UserID:    "user-001",
			Content:   "Hello",
			CreatedAt: now,
		},
	}

	_, err := service.BuildContext(
		event,
		messages,
		15,
		24*time.Hour,
	)

	if !errors.Is(err, ErrTriggerMessageNotFound) {
		t.Errorf("expected ErrTriggerMessageNotFound, got %v", err)
	}
}

func TestValidateExpiredContext(t *testing.T) {
	now := time.Now()

	service := NewConversationService()

	ctx := ConversationContext{
		GuildID:          "guild-001",
		ChannelID:        "channel-001",
		TriggerMessageID: "message-001",
		CreatedAt:        now.Add(-25 * time.Hour),
		ExpiresAt:        now.Add(-time.Hour),
	}

	err := service.ValidateContext(ctx, now)

	if err == nil {
		t.Fatal("expected expired context error")
	}
}
