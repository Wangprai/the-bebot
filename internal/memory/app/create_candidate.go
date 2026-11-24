package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"the-bebot/internal/memory/core"
)

var ErrInvalidCandidateInput = errors.New("invalid candidate input")

type CreateCandidateInput struct {
	Snapshot core.ConversationSnapshot
}

type CreateCandidateUseCase struct {
	repository   core.CandidateRepository
	evaluator    core.CandidateRuleEvaluator
	clock        func() time.Time
	candidateTTL time.Duration
}

func NewCreateCandidateUseCase(
	repository core.CandidateRepository,
	evaluator core.CandidateRuleEvaluator,
	candidateTTL time.Duration,
) *CreateCandidateUseCase {
	return &CreateCandidateUseCase{
		repository:   repository,
		evaluator:    evaluator,
		clock:        time.Now,
		candidateTTL: candidateTTL,
	}
}

func (uc *CreateCandidateUseCase) Execute(
	ctx context.Context,
	input CreateCandidateInput,
) (bool, error) {
	if uc == nil {
		return false, errors.New("create candidate use case is nil")
	}

	if ctx == nil {
		return false, errors.New("context is required")
	}

	if err := ctx.Err(); err != nil {
		return false, err
	}

	if uc.repository == nil {
		return false, errors.New("candidate repository is required")
	}

	if uc.evaluator == nil {
		return false, errors.New("candidate rule evaluator is required")
	}

	if uc.clock == nil {
		return false, errors.New("clock is required")
	}

	if uc.candidateTTL <= 0 {
		return false, errors.New("candidate TTL must be greater than zero")
	}

	snapshot, err := core.NewConversationSnapshot(input.Snapshot)
	if err != nil {
		return false, fmt.Errorf(
			"%w: %v",
			ErrInvalidCandidateInput,
			err,
		)
	}

	shouldCreate, err := uc.evaluator.ShouldCreateCandidate(snapshot)
	if err != nil {
		return false, fmt.Errorf("evaluate candidate rules: %w", err)
	}

	if !shouldCreate {
		return false, nil
	}

	var triggerMessage *core.ConversationMessage

	for i := range snapshot.Messages {
		if snapshot.Messages[i].MessageID == snapshot.TriggerMessageID {
			triggerMessage = &snapshot.Messages[i]
			break
		}
	}

	if triggerMessage == nil {
		return false, fmt.Errorf(
			"%w: trigger message not found",
			ErrInvalidCandidateInput,
		)
	}

	now := uc.clock().UTC()

	candidateID, err := newCandidateID()
	if err != nil {
		return false, err
	}

	candidate, err := core.NewMemoryCandidate(core.MemoryCandidate{
		ID:        candidateID,
		GuildID:   snapshot.GuildID,
		ChannelID: snapshot.ChannelID,
		MessageID: snapshot.TriggerMessageID,
		UserID:    strings.TrimSpace(triggerMessage.UserID),
		Content:   strings.TrimSpace(triggerMessage.Content),
		Status:    core.CandidateStatusPending,
		Messages:  snapshot.Messages,
		Attempts:  0,
		ExpireAt:  now.Add(uc.candidateTTL),
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		return false, fmt.Errorf("create memory candidate: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return false, err
	}

	if err := uc.repository.Create(ctx, candidate); err != nil {
		if errors.Is(err, core.ErrCandidateAlreadyExists) {
			return false, nil
		}

		return false, fmt.Errorf("save memory candidate: %w", err)
	}

	return true, nil
}

func newCandidateID() (string, error) {
	id := make([]byte, 12)

	if _, err := rand.Read(id); err != nil {
		return "", fmt.Errorf("generate candidate ID: %w", err)
	}

	return hex.EncodeToString(id), nil
}
