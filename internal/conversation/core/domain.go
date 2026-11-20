package core

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var (
	ErrInvalidMessage             = errors.New("invalid message")
	ErrInvalidMentionEvent        = errors.New("invalid mention event")
	ErrInvalidConversationContext = errors.New("invalid conversation context")
	ErrTriggerMessageNotFound     = errors.New("trigger message not found in context")
	ErrTooManyMessages            = errors.New("conversation context exceeds maximum messages")
)

type Message struct {
	ID        string
	GuildID   string
	ChannelID string
	UserID    string
	Content   string
	CreatedAt time.Time
	IsBot     bool
}

func NewMessage(
	id string,
	guildID string,
	channelID string,
	userID string,
	content string,
	createdAt time.Time,
	isBot bool,
) (Message, error) {
	message := Message{
		ID:        id,
		GuildID:   guildID,
		ChannelID: channelID,
		UserID:    userID,
		Content:   strings.TrimSpace(content),
		CreatedAt: createdAt,
		IsBot:     isBot,
	}

	if err := message.Validate(); err != nil {
		return Message{}, err
	}

	return message, nil
}

func (m Message) Validate() error {
	if m.ID == "" {
		return fmt.Errorf("%w: message ID is required", ErrInvalidMessage)
	}

	if m.GuildID == "" {
		return fmt.Errorf("%w: guild ID is required", ErrInvalidMessage)
	}

	if m.ChannelID == "" {
		return fmt.Errorf("%w: channel ID is required", ErrInvalidMessage)
	}

	if m.UserID == "" {
		return fmt.Errorf("%w: user ID is required", ErrInvalidMessage)
	}

	if strings.TrimSpace(m.Content) == "" {
		return fmt.Errorf("%w: content is required", ErrInvalidMessage)
	}

	if m.CreatedAt.IsZero() {
		return fmt.Errorf("%w: created at is required", ErrInvalidMessage)
	}

	return nil
}

type MentionEvent struct {
	GuildID     string
	ChannelID   string
	MessageID   string
	UserID      string
	Content     string
	MentionedAt time.Time
}

func NewMentionEvent(
	guildID string,
	channelID string,
	messageID string,
	userID string,
	content string,
	mentionedAt time.Time,
) (MentionEvent, error) {
	event := MentionEvent{
		GuildID:     guildID,
		ChannelID:   channelID,
		MessageID:   messageID,
		UserID:      userID,
		Content:     strings.TrimSpace(content),
		MentionedAt: mentionedAt,
	}

	if err := event.Validate(); err != nil {
		return MentionEvent{}, err
	}

	return event, nil
}

func (e MentionEvent) Validate() error {
	if e.GuildID == "" {
		return fmt.Errorf("%w: guild ID is required", ErrInvalidMentionEvent)
	}

	if e.ChannelID == "" {
		return fmt.Errorf("%w: channel ID is required", ErrInvalidMentionEvent)
	}

	if e.MessageID == "" {
		return fmt.Errorf("%w: message ID is required", ErrInvalidMentionEvent)
	}

	if e.UserID == "" {
		return fmt.Errorf("%w: user ID is required", ErrInvalidMentionEvent)
	}

	if e.Content == "" {
		return fmt.Errorf("%w: content is required", ErrInvalidMentionEvent)
	}

	if e.MentionedAt.IsZero() {
		return fmt.Errorf("%w: mentioned at is required", ErrInvalidMentionEvent)
	}

	return nil
}

type MemoryContext struct {
	Scope      string
	Type       string
	Content    string
	Importance int
}

type ConversationContext struct {
	GuildID          string
	ChannelID        string
	TriggerMessageID string
	Messages         []Message
	Memories         []MemoryContext
	CreatedAt        time.Time
	ExpiresAt        time.Time
}

func NewConversationContext(
	guildID string,
	channelID string,
	triggerMessageID string,
	messages []Message,
	createdAt time.Time,
	ttl time.Duration,
	maxMessages int,
) (ConversationContext, error) {
	if guildID == "" || channelID == "" || triggerMessageID == "" {
		return ConversationContext{}, fmt.Errorf(
			"%w: guild ID, channel ID and trigger message ID are required",
			ErrInvalidConversationContext,
		)
	}

	if createdAt.IsZero() || ttl <= 0 || maxMessages <= 0 {
		return ConversationContext{}, fmt.Errorf(
			"%w: invalid creation time, TTL or maximum messages",
			ErrInvalidConversationContext,
		)
	}

	if len(messages) == 0 {
		return ConversationContext{}, fmt.Errorf(
			"%w: messages cannot be empty",
			ErrInvalidConversationContext,
		)
	}

	if len(messages) > maxMessages {
		return ConversationContext{}, ErrTooManyMessages
	}

	// Copy the slice to avoid modifying the caller's data.
	contextMessages := make([]Message, len(messages))
	copy(contextMessages, messages)

	// Ensure messages are ordered oldest to newest.
	sort.Slice(contextMessages, func(i, j int) bool {
		return contextMessages[i].CreatedAt.Before(
			contextMessages[j].CreatedAt,
		)
	})

	triggerFound := false
	seen := make(map[string]struct{}, len(contextMessages))

	for _, message := range contextMessages {
		if err := message.Validate(); err != nil {
			return ConversationContext{}, err
		}

		if message.GuildID != guildID ||
			message.ChannelID != channelID {
			return ConversationContext{}, fmt.Errorf(
				"%w: messages must belong to the same guild and channel",
				ErrInvalidConversationContext,
			)
		}

		if _, exists := seen[message.ID]; exists {
			return ConversationContext{}, fmt.Errorf(
				"%w: duplicate message ID %s",
				ErrInvalidConversationContext,
				message.ID,
			)
		}

		seen[message.ID] = struct{}{}

		if message.ID == triggerMessageID {
			triggerFound = true
		}
	}

	if !triggerFound {
		return ConversationContext{}, ErrTriggerMessageNotFound
	}

	return ConversationContext{
		GuildID:          guildID,
		ChannelID:        channelID,
		TriggerMessageID: triggerMessageID,
		Messages:         contextMessages,
		CreatedAt:        createdAt,
		ExpiresAt:        createdAt.Add(ttl),
	}, nil
}

func (c ConversationContext) IsExpired(now time.Time) bool {
	return !now.Before(c.ExpiresAt)
}
