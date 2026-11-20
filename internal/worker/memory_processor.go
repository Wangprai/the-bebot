package worker

import (
	"context"
	"errors"
	"fmt"
	"time"

	memoryapp "the-bebot/internal/memory/app"
	"the-bebot/internal/memory/core"
)

const (
	defaultMaxAttempts = 5
	defaultRetryDelay  = time.Minute
	maxRetryDelay      = 30 * time.Minute
)

type MemoryCandidateProcessor struct {
	extractionUseCase   *memoryapp.ExtractMemoriesUseCase
	memoryRepository    core.MemoryRepository
	candidateRepository core.CandidateRepository
	candidateService    core.CandidateService

	clock       func() time.Time
	maxAttempts int
	retryDelay  time.Duration
}

func NewMemoryCandidateProcessor(
	extractionUseCase *memoryapp.ExtractMemoriesUseCase,
	memoryRepository core.MemoryRepository,
	candidateRepository core.CandidateRepository,
	candidateService core.CandidateService,
) (*MemoryCandidateProcessor, error) {
	if extractionUseCase == nil {
		return nil, errors.New("extraction use case is required")
	}

	if memoryRepository == nil {
		return nil, errors.New("memory repository is required")
	}

	if candidateRepository == nil {
		return nil, errors.New("candidate repository is required")
	}

	if candidateService == nil {
		return nil, errors.New("candidate service is required")
	}

	return &MemoryCandidateProcessor{
		extractionUseCase:   extractionUseCase,
		memoryRepository:    memoryRepository,
		candidateRepository: candidateRepository,
		candidateService:    candidateService,
		clock:               time.Now,
		maxAttempts:         defaultMaxAttempts,
		retryDelay:          defaultRetryDelay,
	}, nil
}

func (p *MemoryCandidateProcessor) Process(
	ctx context.Context,
	candidate core.MemoryCandidate,
) error {
	if p == nil {
		return errors.New("memory candidate processor is not initialized")
	}

	if ctx == nil {
		return errors.New("context is required")
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if candidate.Status != core.CandidateStatusProcessing {
		return fmt.Errorf(
			"candidate %s is not processing",
			candidate.ID,
		)
	}

	snapshot, err := core.NewConversationSnapshot(
		core.ConversationSnapshot{
			GuildID:          candidate.GuildID,
			ChannelID:        candidate.ChannelID,
			TriggerMessageID: candidate.MessageID,
			Messages:         candidate.Messages,
		},
	)
	if err != nil {
		return p.fail(ctx, candidate, fmt.Errorf("build snapshot: %w", err))
	}

	memories, err := p.extractionUseCase.Execute(
		ctx,
		memoryapp.ExtractionRequest{
			Snapshot:     snapshot,
			TargetUserID: candidate.UserID,
		},
	)
	if err != nil {
		return p.fail(ctx, candidate, fmt.Errorf("extract memories: %w", err))
	}

	if len(memories) == 0 {
		return p.reject(ctx, candidate)
	}

	for _, memory := range memories {
		if err := ctx.Err(); err != nil {
			return p.fail(ctx, candidate, err)
		}

		if _, err := p.memoryRepository.Save(ctx, memory); err != nil {
			return p.fail(
				ctx,
				candidate,
				fmt.Errorf("save memory: %w", err),
			)
		}
	}

	return p.complete(ctx, candidate)
}

func (p *MemoryCandidateProcessor) complete(
	ctx context.Context,
	candidate core.MemoryCandidate,
) error {
	now := p.clock().UTC()

	updated, err := p.candidateService.Complete(candidate, now)
	if err != nil {
		return fmt.Errorf("complete candidate: %w", err)
	}

	if err := p.candidateRepository.Update(ctx, updated); err != nil {
		return fmt.Errorf("persist completed candidate: %w", err)
	}

	return nil
}

func (p *MemoryCandidateProcessor) reject(
	ctx context.Context,
	candidate core.MemoryCandidate,
) error {
	now := p.clock().UTC()

	updated, err := p.candidateService.Reject(candidate, now)
	if err != nil {
		return fmt.Errorf("reject candidate: %w", err)
	}

	if err := p.candidateRepository.Update(ctx, updated); err != nil {
		return fmt.Errorf("persist rejected candidate: %w", err)
	}

	return nil
}

func (p *MemoryCandidateProcessor) fail(
	ctx context.Context,
	candidate core.MemoryCandidate,
	cause error,
) error {
	now := p.clock().UTC()

	retryAt := now.Add(p.retryDuration(candidate.Attempts))

	// เมื่อถึงจำนวน Attempts สูงสุด จะไม่ Retry อีก
	// กำหนด next_retry_at เป็น expire_at เพื่อรอการลบตาม TTL
	if candidate.Attempts >= p.maxAttempts {
		retryAt = candidate.ExpireAt
	}

	updated, err := p.candidateService.Fail(
		candidate,
		now,
		retryAt,
	)
	if err != nil {
		return errors.Join(
			cause,
			fmt.Errorf("mark candidate as failed: %w", err),
		)
	}

	if err := p.candidateRepository.Update(ctx, updated); err != nil {
		return errors.Join(
			cause,
			fmt.Errorf("persist failed candidate: %w", err),
		)
	}

	return cause
}

func (p *MemoryCandidateProcessor) retryDuration(attempts int) time.Duration {
	delay := p.retryDelay

	for i := 1; i < attempts; i++ {
		delay *= 2

		if delay >= maxRetryDelay {
			return maxRetryDelay
		}
	}

	if delay > maxRetryDelay {
		return maxRetryDelay
	}

	return delay
}

var _ CandidateProcessor = (*MemoryCandidateProcessor)(nil)
