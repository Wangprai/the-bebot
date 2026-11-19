package mongodb

import (
	"fmt"
	"time"

	"the-bebot/internal/memory/core"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type candidateMessageDocument struct {
	MessageID string `bson:"message_id"`
	UserID    string `bson:"user_id"`
	Content   string `bson:"content"`
	IsBot     bool   `bson:"is_bot"`
}

type candidateDocument struct {
	ID          primitive.ObjectID         `bson:"_id,omitempty"`
	GuildID     string                     `bson:"guild_id"`
	ChannelID   string                     `bson:"channel_id"`
	MessageID   string                     `bson:"message_id"`
	UserID      string                     `bson:"user_id"`
	Content     string                     `bson:"content"`
	Messages    []candidateMessageDocument `bson:"messages"`
	Status      string                     `bson:"status"`
	Attempts    int                        `bson:"attempts"`
	WorkerID    string                     `bson:"worker_id,omitempty"`
	LockedUntil *time.Time                 `bson:"locked_until,omitempty"`
	NextRetryAt *time.Time                 `bson:"next_retry_at,omitempty"`
	ExpireAt    time.Time                  `bson:"expire_at"`
	CreatedAt   time.Time                  `bson:"created_at"`
	UpdatedAt   time.Time                  `bson:"updated_at"`
}

func toCandidateDocument(candidate core.MemoryCandidate) (candidateDocument, error) {
	objectID, err := primitive.ObjectIDFromHex(candidate.ID)
	if err != nil {
		return candidateDocument{}, fmt.Errorf("invalid candidate ID: %w", err)
	}

	messages := make([]candidateMessageDocument, 0, len(candidate.Messages))

	for _, message := range candidate.Messages {
		messages = append(messages, candidateMessageDocument{
			MessageID: message.MessageID,
			UserID:    message.UserID,
			Content:   message.Content,
			IsBot:     message.IsBot,
		})
	}

	return candidateDocument{
		ID:          objectID,
		GuildID:     candidate.GuildID,
		ChannelID:   candidate.ChannelID,
		MessageID:   candidate.MessageID,
		UserID:      candidate.UserID,
		Content:     candidate.Content,
		Messages:    messages,
		Status:      string(candidate.Status),
		Attempts:    candidate.Attempts,
		WorkerID:    candidate.WorkerID,
		LockedUntil: candidate.LockedUntil,
		NextRetryAt: candidate.NextRetryAt,
		ExpireAt:    candidate.ExpireAt,
		CreatedAt:   candidate.CreatedAt,
		UpdatedAt:   candidate.UpdatedAt,
	}, nil
}

func (d candidateDocument) toDomain() (core.MemoryCandidate, error) {
	messages := make([]core.ConversationMessage, 0, len(d.Messages))

	for _, message := range d.Messages {
		messages = append(messages, core.ConversationMessage{
			MessageID: message.MessageID,
			UserID:    message.UserID,
			Content:   message.Content,
			IsBot:     message.IsBot,
		})
	}

	return core.NewMemoryCandidate(core.MemoryCandidate{
		ID:          d.ID.Hex(),
		GuildID:     d.GuildID,
		ChannelID:   d.ChannelID,
		MessageID:   d.MessageID,
		UserID:      d.UserID,
		Content:     d.Content,
		Messages:    messages,
		Status:      core.CandidateStatus(d.Status),
		Attempts:    d.Attempts,
		WorkerID:    d.WorkerID,
		LockedUntil: d.LockedUntil,
		NextRetryAt: d.NextRetryAt,
		ExpireAt:    d.ExpireAt,
		CreatedAt:   d.CreatedAt,
		UpdatedAt:   d.UpdatedAt,
	})
}
