package mongodb

import (
	"fmt"
	"time"

	"the-bebot/internal/conversation/core"
)

type conversationMessageDocument struct {
	ID         string    `bson:"id"`
	GuildID    string    `bson:"guild_id"`
	ChannelID  string    `bson:"channel_id"`
	UserID     string    `bson:"user_id"`
	AuthorName string    `bson:"author_name,omitempty"`
	Content    string    `bson:"content"`
	CreatedAt  time.Time `bson:"created_at"`
	IsBot      bool      `bson:"is_bot"`
	IsSelf     bool      `bson:"is_self,omitempty"`
}

type conversationDocument struct {
	GuildID          string                        `bson:"guild_id"`
	ChannelID        string                        `bson:"channel_id"`
	TriggerMessageID string                        `bson:"trigger_message_id"`
	Messages         []conversationMessageDocument `bson:"messages"`
	CreatedAt        time.Time                     `bson:"created_at"`
	ExpiresAt        time.Time                     `bson:"expires_at"`
}

func toConversationDocument(
	conversationContext core.ConversationContext,
) (conversationDocument, error) {
	if conversationContext.GuildID == "" ||
		conversationContext.ChannelID == "" ||
		conversationContext.TriggerMessageID == "" {
		return conversationDocument{}, fmt.Errorf(
			"%w: guild ID, channel ID, and trigger message ID are required",
			core.ErrInvalidConversationContext,
		)
	}

	if conversationContext.CreatedAt.IsZero() ||
		conversationContext.ExpiresAt.IsZero() ||
		!conversationContext.ExpiresAt.After(conversationContext.CreatedAt) {
		return conversationDocument{}, fmt.Errorf(
			"%w: valid created_at and expires_at timestamps are required",
			core.ErrInvalidConversationContext,
		)
	}

	if len(conversationContext.Messages) == 0 {
		return conversationDocument{}, fmt.Errorf(
			"%w: messages cannot be empty",
			core.ErrInvalidConversationContext,
		)
	}

	messages := make([]conversationMessageDocument, 0, len(conversationContext.Messages))

	for _, message := range conversationContext.Messages {
		if err := message.Validate(); err != nil {
			return conversationDocument{}, err
		}

		if message.GuildID != conversationContext.GuildID ||
			message.ChannelID != conversationContext.ChannelID {
			return conversationDocument{}, fmt.Errorf(
				"%w: message scope mismatch",
				core.ErrInvalidConversationContext,
			)
		}

		messages = append(messages, conversationMessageDocument{
			ID:         message.ID,
			GuildID:    message.GuildID,
			ChannelID:  message.ChannelID,
			UserID:     message.UserID,
			AuthorName: message.AuthorName,
			Content:    message.Content,
			CreatedAt:  message.CreatedAt.UTC(),
			IsBot:      message.IsBot,
			IsSelf:     message.IsSelf,
		})
	}

	return conversationDocument{
		GuildID:          conversationContext.GuildID,
		ChannelID:        conversationContext.ChannelID,
		TriggerMessageID: conversationContext.TriggerMessageID,
		Messages:         messages,
		CreatedAt:        conversationContext.CreatedAt.UTC(),
		ExpiresAt:        conversationContext.ExpiresAt.UTC(),
	}, nil
}

func (d conversationDocument) toDomain() (core.ConversationContext, error) {
	messages := make([]core.Message, 0, len(d.Messages))

	for _, message := range d.Messages {
		messages = append(messages, core.Message{
			ID:         message.ID,
			GuildID:    message.GuildID,
			ChannelID:  message.ChannelID,
			UserID:     message.UserID,
			AuthorName: message.AuthorName,
			Content:    message.Content,
			CreatedAt:  message.CreatedAt.UTC(),
			IsBot:      message.IsBot,
			IsSelf:     message.IsSelf,
		})
	}

	ttl := d.ExpiresAt.Sub(d.CreatedAt)

	return core.NewConversationContext(
		d.GuildID,
		d.ChannelID,
		d.TriggerMessageID,
		messages,
		d.CreatedAt.UTC(),
		ttl,
		len(messages),
	)
}
