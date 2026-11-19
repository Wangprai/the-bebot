package core

import (
	"fmt"
	"time"
)

type ConversationService interface {
	BuildContext(
		event MentionEvent,
		messages []Message,
		maxMessages int,
		ttl time.Duration,
	) (ConversationContext, error)

	ValidateContext(
		conversationContext ConversationContext,
		now time.Time,
	) error
}

type conversationService struct {
	clock func() time.Time
}

func NewConversationService() ConversationService {
	return &conversationService{
		clock: time.Now,
	}
}

func (s *conversationService) BuildContext(
	event MentionEvent,
	messages []Message,
	maxMessages int,
	ttl time.Duration,
) (ConversationContext, error) {
	if err := event.Validate(); err != nil {
		return ConversationContext{}, err
	}

	// Ensure the trigger message is included.
	triggerFound := false

	for _, message := range messages {
		if message.ID == event.MessageID {
			triggerFound = true
			break
		}
	}

	if !triggerFound {
		return ConversationContext{}, ErrTriggerMessageNotFound
	}

	return NewConversationContext(
		event.GuildID,
		event.ChannelID,
		event.MessageID,
		messages,
		s.clock(),
		ttl,
		maxMessages,
	)
}

func (s *conversationService) ValidateContext(
	conversationContext ConversationContext,
	now time.Time,
) error {
	if conversationContext.GuildID == "" ||
		conversationContext.ChannelID == "" ||
		conversationContext.TriggerMessageID == "" {
		return fmt.Errorf(
			"%w: missing context identifiers",
			ErrInvalidConversationContext,
		)
	}

	if conversationContext.CreatedAt.IsZero() ||
		conversationContext.ExpiresAt.IsZero() {
		return fmt.Errorf(
			"%w: missing context timestamps",
			ErrInvalidConversationContext,
		)
	}

	if conversationContext.IsExpired(now) {
		return fmt.Errorf(
			"%w: conversation context has expired",
			ErrInvalidConversationContext,
		)
	}

	return nil
}
