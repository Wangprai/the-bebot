package conversation

import (
	"context"
	"fmt"

	conversationcore "the-bebot/internal/conversation/core"
	memoryapp "the-bebot/internal/memory/app"
	memorycore "the-bebot/internal/memory/core"
)

type CandidateCreatorAdapter struct {
	useCase *memoryapp.CreateCandidateUseCase
}

func NewCandidateCreatorAdapter(
	useCase *memoryapp.CreateCandidateUseCase,
) *CandidateCreatorAdapter {
	return &CandidateCreatorAdapter{
		useCase: useCase,
	}
}

func (a *CandidateCreatorAdapter) CreateCandidate(
	ctx context.Context,
	conversationContext conversationcore.ConversationContext,
) error {
	if a == nil || a.useCase == nil {
		return fmt.Errorf("candidate creator use case is nil")
	}

	messages := make([]memorycore.ConversationMessage, 0, len(conversationContext.Messages))

	for _, message := range conversationContext.Messages {
		messages = append(messages, memorycore.ConversationMessage{
			MessageID: message.ID,
			UserID:    message.UserID,
			Content:   message.Content,
			IsBot:     message.IsBot,
		})
	}

	snapshot := memorycore.ConversationSnapshot{
		GuildID:          conversationContext.GuildID,
		ChannelID:        conversationContext.ChannelID,
		TriggerMessageID: conversationContext.TriggerMessageID,
		Messages:         messages,
	}

	_, err := a.useCase.Execute(ctx, memoryapp.CreateCandidateInput{
		Snapshot: snapshot,
	})
	if err != nil {
		return fmt.Errorf("create memory candidate: %w", err)
	}

	return nil
}

var _ conversationcore.CandidateCreator = (*CandidateCreatorAdapter)(nil)
