package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
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
	logger     *slog.Logger
}

func NewRetrieveMemoriesUseCase(
	repository core.MemoryRepository,
) *RetrieveMemoriesUseCase {
	return &RetrieveMemoriesUseCase{
		repository: repository,
		clock:      time.Now,
		logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func (uc *RetrieveMemoriesUseCase) SetLogger(logger *slog.Logger) error {
	if uc == nil {
		return ErrMemoryRepositoryNil
	}

	if logger == nil {
		return errors.New("logger is required")
	}

	uc.logger = logger
	return nil
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

	if len(memories) > 0 {
		usedAt := uc.clock()
		memoryIDs := make([]string, 0, len(memories))
		for _, memory := range memories {
			memoryIDs = append(memoryIDs, memory.ID)
		}

		if err := uc.repository.UpdateUsageMany(
			ctx,
			input.GuildID,
			memoryIDs,
			usedAt,
		); err != nil {
			if uc.logger != nil {
				uc.logger.Warn(
					"Failed to update memory usage; returning retrieved memories",
					"guild_id", input.GuildID,
					"memory_count", len(memoryIDs),
					"error", err,
				)
			}
		}
	}

	return memories, nil
}
