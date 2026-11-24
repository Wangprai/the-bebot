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
	GuildID   string
	UserID    string
	QueryText string
	Limit     int
}

type RetrieveMemoriesUseCase struct {
	repository                core.MemoryRepository
	clock                     func() time.Time
	logger                    *slog.Logger
	minConfidence             float64
	minCommunityCorroborators int
	communityHighConfidence   float64
}

func NewRetrieveMemoriesUseCase(
	repository core.MemoryRepository,
) *RetrieveMemoriesUseCase {
	return &RetrieveMemoriesUseCase{
		repository:                repository,
		clock:                     time.Now,
		logger:                    slog.New(slog.NewTextHandler(io.Discard, nil)),
		minConfidence:             core.DefaultMinConfidence,
		minCommunityCorroborators: core.DefaultMinCommunityCorroborators,
	}
}

func (uc *RetrieveMemoriesUseCase) SetMinConfidence(minConfidence float64) error {
	if uc == nil {
		return ErrMemoryRepositoryNil
	}
	if minConfidence < 0 || minConfidence > 1 {
		return errors.New("min confidence must be between 0 and 1")
	}
	uc.minConfidence = minConfidence
	return nil
}

func (uc *RetrieveMemoriesUseCase) SetCommunityCorroboration(
	minCorroborators int,
	highConfidence float64,
) error {
	if uc == nil {
		return ErrMemoryRepositoryNil
	}
	if minCorroborators < 1 {
		return errors.New("min community corroborators must be at least 1")
	}
	if highConfidence < 0 || highConfidence > 1 {
		return errors.New("community high confidence must be between 0 and 1")
	}
	uc.minCommunityCorroborators = minCorroborators
	uc.communityHighConfidence = highConfidence
	return nil
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

	memories, err := uc.repository.FindRelevantByQuery(
		ctx,
		core.RetrievalQuery{
			GuildID:                   input.GuildID,
			UserID:                    input.UserID,
			QueryText:                 strings.TrimSpace(input.QueryText),
			Limit:                     input.Limit,
			MinConfidence:             uc.minConfidence,
			MinCommunityCorroborators: uc.minCommunityCorroborators,
			CommunityHighConfidence:   uc.communityHighConfidence,
			Now:                       uc.clock().UTC(),
		},
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
