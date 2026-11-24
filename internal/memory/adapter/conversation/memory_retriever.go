package conversation

import (
	"context"
	"fmt"

	conversationcore "the-bebot/internal/conversation/core"
	memoryapp "the-bebot/internal/memory/app"
)

type MemoryRetrieverAdapter struct {
	useCase *memoryapp.RetrieveMemoriesUseCase
}

func NewMemoryRetrieverAdapter(
	useCase *memoryapp.RetrieveMemoriesUseCase,
) *MemoryRetrieverAdapter {
	return &MemoryRetrieverAdapter{
		useCase: useCase,
	}
}

func (a *MemoryRetrieverAdapter) RetrieveMemories(
	ctx context.Context,
	guildID string,
	userID string,
	limit int,
) ([]conversationcore.MemoryContext, error) {
	return a.RetrieveMemoriesForQuery(ctx, guildID, userID, "", limit)
}

func (a *MemoryRetrieverAdapter) RetrieveMemoriesForQuery(
	ctx context.Context,
	guildID string,
	userID string,
	query string,
	limit int,
) ([]conversationcore.MemoryContext, error) {
	if a == nil || a.useCase == nil {
		return nil, fmt.Errorf("memory retriever use case is nil")
	}

	memories, err := a.useCase.Execute(
		ctx,
		memoryapp.RetrieveMemoriesInput{
			GuildID:   guildID,
			UserID:    userID,
			QueryText: query,
			Limit:     limit,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("retrieve memories: %w", err)
	}

	result := make([]conversationcore.MemoryContext, 0, len(memories))

	for _, memory := range memories {
		result = append(result, conversationcore.MemoryContext{
			Scope:      string(memory.Scope),
			Type:       string(memory.Type),
			Content:    memory.Content,
			Importance: memory.Importance,
		})
	}

	return result, nil
}

var (
	_ conversationcore.MemoryRetriever           = (*MemoryRetrieverAdapter)(nil)
	_ conversationcore.ContextualMemoryRetriever = (*MemoryRetrieverAdapter)(nil)
)
