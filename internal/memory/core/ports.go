package core

import (
	"context"
	"time"
)

type MemoryRepository interface {
	Save(ctx context.Context, memory Memory) error

	FindByID(
		ctx context.Context,
		guildID string,
		memoryID string,
	) (*Memory, error)

	FindRelevant(
		ctx context.Context,
		guildID string,
		userID string,
		limit int,
	) ([]Memory, error)

	UpdateUsage(
		ctx context.Context,
		guildID string,
		memoryID string,
		usedAt time.Time,
	) error

	DeleteByID(
		ctx context.Context,
		guildID string,
		memoryID string,
	) error
}

type CandidateRepository interface {
	Create(
		ctx context.Context,
		candidate MemoryCandidate,
	) error

	ClaimNext(
		ctx context.Context,
		workerID string,
		now time.Time,
		lockDuration time.Duration,
	) (*MemoryCandidate, error)

	Update(
		ctx context.Context,
		candidate MemoryCandidate,
	) error

	DeleteExpired(
		ctx context.Context,
		before time.Time,
	) error
}

type MemoryExtractor interface {
	Extract(
		ctx context.Context,
		input ExtractionInput,
	) ([]ExtractedMemory, error)
}

type MemoryValidator interface {
	Validate(
		input ExtractedMemory,
		source ExtractionSource,
	) (Memory, error)
}

type CandidateRuleEvaluator interface {
	ShouldCreateCandidate(
		context ConversationSnapshot,
	) (bool, error)
}