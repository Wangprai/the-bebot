package core

import (
	"context"
	"time"
)

// ContextProvider retrieves recent messages from Discord.
type ContextProvider interface {
	FetchRecentMessages(
		ctx context.Context,
		guildID string,
		channelID string,
		limit int,
	) ([]Message, error)
}

// ConversationRepository manages conversation contexts.
type ConversationRepository interface {
	Save(
		ctx context.Context,
		conversationContext ConversationContext,
	) error

	FindLatest(
		ctx context.Context,
		guildID string,
		channelID string,
	) (*ConversationContext, error)

	DeleteExpired(
		ctx context.Context,
		before time.Time,
	) error
}

// ResponseGenerator generates a response from conversation context.
type ResponseGenerator interface {
	Generate(
		ctx context.Context,
		conversationContext ConversationContext,
	) (string, error)
}

// MessageSender sends a message to Discord.
type MessageSender interface {
	Send(
		ctx context.Context,
		guildID string,
		channelID string,
		replyToMessageID string,
		content string,
	) (Message, error)
}

// TypingIndicator broadcasts typing status in a channel while generating a response.
type TypingIndicator interface {
	StartTyping(
		ctx context.Context,
		channelID string,
	) error
}

type CandidateCreator interface {
	CreateCandidate(
		ctx context.Context,
		conversationContext ConversationContext,
	) error
}

// MemoryRetriever retrieves relevant long-term memories for conversation context.
type MemoryRetriever interface {
	RetrieveMemories(
		ctx context.Context,
		guildID string,
		userID string,
		limit int,
	) ([]MemoryContext, error)
}

// PersonalityProvider supplies The Bebot's underlying behavioral personality.
type PersonalityProvider interface {
	GetPersonality(
		ctx context.Context,
	) (PersonalityContext, error)
}

// MoodResolver resolves and updates effective Mood for an incoming interaction.
type MoodResolver interface {
	ResolveMood(
		ctx context.Context,
		guildID string,
		channelID string,
		userID string,
		triggerContent string,
	) (MoodContext, error)
}

// CommunityContextSyncer updates and returns lightweight community and member metadata.
type CommunityContextSyncer interface {
	SyncMentionContext(
		ctx context.Context,
		event MentionEvent,
	) (CommunityContext, error)
}
