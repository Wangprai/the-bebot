package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"the-bebot/internal/memory/core"
)

var (
	ErrInvalidRetrievalInput = errors.New("invalid memory retrieval input")
	ErrMemoryRepositoryNil   = errors.New("memory repository is nil")
)

type RetrieveMemoriesInput struct {
	GuildID string
	UserID  string
	Limit   int
}

type RetrieveMemoriesUseCase struct {
	repository core.MemoryRepository
	clock      func() time.Time
}

func NewRetrieveMemoriesUseCase(
	repository core.MemoryRepository,
) *RetrieveMemoriesUseCase {
	return &RetrieveMemoriesUseCase{
		repository: repository,
		clock:      time.Now,
	}
}

func (uc *RetrieveMemoriesUseCase) Execute(
	ctx context.Context,
	input RetrieveMemoriesInput,
) ([]core.Memory, error) {
	if uc == nil || uc.repository == nil {
		return nil, ErrMemoryRepositoryNil
	}

	if ctx == nil {
		return nil, ErrInvalidRetrievalInput
	}

	input.GuildID = strings.TrimSpace(input.GuildID)
	input.UserID = strings.TrimSpace(input.UserID)

	if input.GuildID == "" ||
		input.UserID == "" ||
		input.Limit <= 0 {
		return nil, ErrInvalidRetrievalInput
	}

	memories, err := uc.repository.FindRelevant(
		ctx,
		input.GuildID,
		input.UserID,
		input.Limit,
	)
	if err != nil {
		return nil, fmt.Errorf("find relevant memories: %w", err)
	}

	usedAt := uc.clock()

	for _, memory := range memories {
		if err := uc.repository.UpdateUsage(
			ctx,
			input.GuildID,
			memory.ID,
			usedAt,
		); err != nil {
			return nil, fmt.Errorf(
				"update memory usage %s: %w",
				memory.ID,
				err,
			)
		}
	}

	return memories, nil
}
