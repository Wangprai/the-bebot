package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"the-bebot/internal/memory/core"
)

type mockCandidateRepository struct {
	createCalled bool
	candidate    core.MemoryCandidate
	createErr    error
}

func (m *mockCandidateRepository) Create(
	ctx context.Context,
	candidate core.MemoryCandidate,
) error {
	m.createCalled = true
	m.candidate = candidate
	return m.createErr
}

func (m *mockCandidateRepository) ClaimNext(
	ctx context.Context,
	workerID string,
	now time.Time,
	lockDuration time.Duration,
) (*core.MemoryCandidate, error) {
	return nil, nil
}

func (m *mockCandidateRepository) Update(
	ctx context.Context,
	candidate core.MemoryCandidate,
) error {
	return nil
}

func (m *mockCandidateRepository) UpdateProcessingResult(
	ctx context.Context,
	candidate core.MemoryCandidate,
	expectedWorkerID string,
) error {
	return nil
}

func (m *mockCandidateRepository) DeleteExpired(
	ctx context.Context,
	before time.Time,
) error {
	return nil
}

type mockCandidateRuleEvaluator struct {
	shouldCreate bool
	evaluateErr  error
	called       bool
}

func (m *mockCandidateRuleEvaluator) ShouldCreateCandidate(
	snapshot core.ConversationSnapshot,
) (bool, error) {
	m.called = true
	return m.shouldCreate, m.evaluateErr
}

func newTestConversationSnapshot() core.ConversationSnapshot {
	return core.ConversationSnapshot{
		GuildID:          "guild-123",
		ChannelID:        "channel-123",
		TriggerMessageID: "message-123",
		Messages: []core.ConversationMessage{
			{
				MessageID: "message-122",
				UserID:    "user-456",
				Content:   "I like programming",
			},
			{
				MessageID: "message-123",
				UserID:    "user-123",
				Content:   "I prefer Go for backend development",
			},
		},
	}
}

func TestCreateCandidateSuccess(t *testing.T) {
	now := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	ttl := 24 * time.Hour

	repository := &mockCandidateRepository{}
	evaluator := &mockCandidateRuleEvaluator{
		shouldCreate: true,
	}

	useCase := NewCreateCandidateUseCase(
		repository,
		evaluator,
		ttl,
	)

	useCase.clock = func() time.Time {
		return now
	}

	created, err := useCase.Execute(
		context.Background(),
		CreateCandidateInput{
			Snapshot: newTestConversationSnapshot(),
		},
	)

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !created {
		t.Fatal("expected candidate to be created")
	}

	if !repository.createCalled {
		t.Fatal("expected repository Create to be called")
	}

	if !evaluator.called {
		t.Fatal("expected evaluator to be called")
	}

	candidate := repository.candidate

	if candidate.GuildID != "guild-123" {
		t.Errorf("unexpected GuildID: %s", candidate.GuildID)
	}

	if candidate.ChannelID != "channel-123" {
		t.Errorf("unexpected ChannelID: %s", candidate.ChannelID)
	}

	if candidate.MessageID != "message-123" {
		t.Errorf("unexpected MessageID: %s", candidate.MessageID)
	}

	if candidate.UserID != "user-123" {
		t.Errorf("unexpected UserID: %s", candidate.UserID)
	}

	if candidate.Content != "I prefer Go for backend development" {
		t.Errorf("unexpected Content: %s", candidate.Content)
	}

	if candidate.Status != core.CandidateStatusPending {
		t.Errorf("unexpected Status: %s", candidate.Status)
	}

	if candidate.Attempts != 0 {
		t.Errorf("expected 0 attempts, got %d", candidate.Attempts)
	}

	if !candidate.ExpireAt.Equal(now.Add(ttl)) {
		t.Errorf("unexpected ExpireAt: %v", candidate.ExpireAt)
	}

	if len(candidate.Messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(candidate.Messages))
	}
}

func TestCreateCandidateRejectedByRules(t *testing.T) {
	repository := &mockCandidateRepository{}
	evaluator := &mockCandidateRuleEvaluator{
		shouldCreate: false,
	}

	useCase := NewCreateCandidateUseCase(
		repository,
		evaluator,
		24*time.Hour,
	)

	created, err := useCase.Execute(
		context.Background(),
		CreateCandidateInput{
			Snapshot: newTestConversationSnapshot(),
		},
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if created {
		t.Fatal("expected candidate not to be created")
	}

	if repository.createCalled {
		t.Fatal("repository Create must not be called")
	}
}

func TestCreateCandidateEvaluatorError(t *testing.T) {
	expectedErr := errors.New("rule evaluation failed")

	repository := &mockCandidateRepository{}
	evaluator := &mockCandidateRuleEvaluator{
		evaluateErr: expectedErr,
	}

	useCase := NewCreateCandidateUseCase(
		repository,
		evaluator,
		24*time.Hour,
	)

	_, err := useCase.Execute(
		context.Background(),
		CreateCandidateInput{
			Snapshot: newTestConversationSnapshot(),
		},
	)

	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected evaluator error, got %v", err)
	}

	if repository.createCalled {
		t.Fatal("repository Create must not be called")
	}
}

func TestCreateCandidateRepositoryError(t *testing.T) {
	expectedErr := errors.New("database unavailable")

	repository := &mockCandidateRepository{
		createErr: expectedErr,
	}

	evaluator := &mockCandidateRuleEvaluator{
		shouldCreate: true,
	}

	useCase := NewCreateCandidateUseCase(
		repository,
		evaluator,
		24*time.Hour,
	)

	_, err := useCase.Execute(
		context.Background(),
		CreateCandidateInput{
			Snapshot: newTestConversationSnapshot(),
		},
	)

	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected repository error, got %v", err)
	}
}

func TestCreateCandidateDuplicateIsIdempotent(t *testing.T) {
	repository := &mockCandidateRepository{
		createErr: core.ErrCandidateAlreadyExists,
	}

	evaluator := &mockCandidateRuleEvaluator{
		shouldCreate: true,
	}

	useCase := NewCreateCandidateUseCase(
		repository,
		evaluator,
		24*time.Hour,
	)

	created, err := useCase.Execute(
		context.Background(),
		CreateCandidateInput{
			Snapshot: newTestConversationSnapshot(),
		},
	)
	if err != nil {
		t.Fatalf("expected nil error for duplicate candidate, got %v", err)
	}

	if created {
		t.Fatal("expected created = false for duplicate candidate")
	}
}

func TestCreateCandidateCanceledContext(t *testing.T) {
	repository := &mockCandidateRepository{}
	evaluator := &mockCandidateRuleEvaluator{
		shouldCreate: true,
	}

	useCase := NewCreateCandidateUseCase(
		repository,
		evaluator,
		24*time.Hour,
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := useCase.Execute(
		ctx,
		CreateCandidateInput{
			Snapshot: newTestConversationSnapshot(),
		},
	)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	if evaluator.called {
		t.Fatal("evaluator must not be called with canceled context")
	}

	if repository.createCalled {
		t.Fatal("repository Create must not be called")
	}
}
