package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"the-bebot/internal/conversation/core"
)

func TestConversationPipelineIntegration(t *testing.T) {
	now := time.Now().UTC()

	event := core.MentionEvent{
		GuildID:     "guild-001",
		ChannelID:   "channel-001",
		MessageID:   "message-003",
		UserID:      "user-001",
		Content:     "@TheBebot hello",
		MentionedAt: now,
	}

	// Simulate messages retrieved from Discord.
	provider := &mockContextProvider{
		messages: []core.Message{
			{
				ID:        "message-001",
				GuildID:   "guild-001",
				ChannelID: "channel-001",
				UserID:    "user-001",
				Content:   "Hello everyone",
				CreatedAt: now.Add(-2 * time.Minute),
			},
			{
				ID:        "message-002",
				GuildID:   "guild-001",
				ChannelID: "channel-001",
				UserID:    "user-002",
				Content:   "How are you?",
				CreatedAt: now.Add(-time.Minute),
			},
		},
	}

	// Use the real ConversationService.
	conversationService := core.NewConversationService()

	// Simulate LLM response.
	generator := &mockResponseGenerator{
		response: "Hello! What's happening here?",
	}

	// Simulate Discord message sender.
	sender := &mockMessageSender{}

	candidateCreator := &mockCandidateCreator{}

	useCase := NewHandleMentionUseCase(
		provider,
		conversationService,
		generator,
		sender,
		candidateCreator,
		15,
		24*time.Hour,
	)

	err := useCase.HandleMention(context.Background(), event)
	if err != nil {
		t.Fatalf("HandleMention failed: %v", err)
	}

	// Verify ContextProvider.
	if !provider.called {
		t.Fatal("ContextProvider was not called")
	}

	if provider.receivedGuildID != event.GuildID {
		t.Errorf(
			"unexpected guild ID: got %s, want %s",
			provider.receivedGuildID,
			event.GuildID,
		)
	}

	if provider.receivedChannelID != event.ChannelID {
		t.Errorf(
			"unexpected channel ID: got %s, want %s",
			provider.receivedChannelID,
			event.ChannelID,
		)
	}

	if provider.receivedLimit != 15 {
		t.Errorf(
			"unexpected message limit: got %d, want 15",
			provider.receivedLimit,
		)
	}

	// Verify ResponseGenerator received valid context.
	if !generator.called {
		t.Fatal("ResponseGenerator was not called")
	}

	conversationContext := generator.receivedContext

	if conversationContext.GuildID != event.GuildID {
		t.Errorf("unexpected context guild ID")
	}

	if conversationContext.ChannelID != event.ChannelID {
		t.Errorf("unexpected context channel ID")
	}

	if conversationContext.TriggerMessageID != event.MessageID {
		t.Errorf("unexpected trigger message ID")
	}

	if len(conversationContext.Messages) != 3 {
		t.Fatalf(
			"expected 3 context messages, got %d",
			len(conversationContext.Messages),
		)
	}

	// Verify chronological ordering.
	messages := conversationContext.Messages

	for i := 1; i < len(messages); i++ {
		if messages[i].CreatedAt.Before(messages[i-1].CreatedAt) {
			t.Errorf("messages are not sorted chronologically")
		}
	}

	// Verify trigger message exists.
	lastMessage := messages[len(messages)-1]

	if lastMessage.ID != event.MessageID {
		t.Errorf(
			"expected trigger message at the end, got %s",
			lastMessage.ID,
		)
	}

	if lastMessage.Content != event.Content {
		t.Errorf("trigger message content does not match event")
	}

	// Verify MessageSender.
	if !sender.called {
		t.Fatal("MessageSender was not called")
	}

	if sender.receivedGuildID != event.GuildID {
		t.Errorf("unexpected sender guild ID")
	}

	if sender.receivedChannelID != event.ChannelID {
		t.Errorf("unexpected sender channel ID")
	}

	if sender.content != generator.response {
		t.Errorf(
			"unexpected response: got %q, want %q",
			sender.content,
			generator.response,
		)
	}
}

func TestConversationPipelineAddsMissingTrigger(t *testing.T) {
	now := time.Now().UTC()

	event := core.MentionEvent{
		GuildID:     "guild-001",
		ChannelID:   "channel-001",
		MessageID:   "message-002",
		UserID:      "user-001",
		Content:     "@TheBebot hello",
		MentionedAt: now,
	}

	// ContextProvider does not return the trigger message.
	provider := &mockContextProvider{
		messages: []core.Message{
			{
				ID:        "message-001",
				GuildID:   event.GuildID,
				ChannelID: event.ChannelID,
				UserID:    "user-001",
				Content:   "Hello",
				CreatedAt: now.Add(-time.Minute),
			},
		},
	}

	// Use the real ConversationService.
	conversationService := core.NewConversationService()

	generator := &mockResponseGenerator{
		response: "Hello! What's up?",
	}

	sender := &mockMessageSender{}
	candidateCreator := &mockCandidateCreator{}

	useCase := NewHandleMentionUseCase(
		provider,
		conversationService,
		generator,
		sender,
		candidateCreator,
		15,
		24*time.Hour,
	)

	err := useCase.HandleMention(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !generator.called {
		t.Fatal("ResponseGenerator should be called")
	}

	messages := generator.receivedContext.Messages

	var triggerFound bool

	for _, message := range messages {
		if message.ID == event.MessageID {
			triggerFound = true

			if message.Content != event.Content {
				t.Error("trigger content does not match event")
			}
		}
	}

	if !triggerFound {
		t.Fatal("trigger message should be added to context")
	}

	if !sender.called {
		t.Error("MessageSender should be called")
	}
}

func TestConversationPipelineStopsOnLLMError(t *testing.T) {
	useCase, _, _, generator, sender, event := newTestUseCase()

	generator.err = errors.New("LLM unavailable")

	err := useCase.HandleMention(context.Background(), event)

	if err == nil {
		t.Fatal("expected LLM error")
	}

	if sender.called {
		t.Error("MessageSender should not be called")
	}
}

func TestConversationPipelineRejectsEmptyLLMResponse(t *testing.T) {
	useCase, _, _, generator, sender, event := newTestUseCase()

	generator.response = "   "

	err := useCase.HandleMention(context.Background(), event)

	if !errors.Is(err, ErrEmptyResponse) {
		t.Fatalf("expected ErrEmptyResponse, got %v", err)
	}

	if sender.called {
		t.Error("MessageSender should not be called")
	}
}
