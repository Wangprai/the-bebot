package conversation

import (
	"context"
	"errors"
	"testing"
	"time"

	conversationcore "the-bebot/internal/conversation/core"
	memoryapp "the-bebot/internal/memory/app"
	memorycore "the-bebot/internal/memory/core"
)

type mockCandidateRepository struct {
	candidate memorycore.MemoryCandidate
	called    bool
	err       error
}

func (m *mockCandidateRepository) Create(
	ctx context.Context,
	candidate memorycore.MemoryCandidate,
) error {
	m.called = true
	m.candidate = candidate
	return m.err
}

func (m *mockCandidateRepository) ClaimNext(
	ctx context.Context,
	workerID string,
	now time.Time,
	lockDuration time.Duration,
) (*memorycore.MemoryCandidate, error) {
	return nil, nil
}

func (m *mockCandidateRepository) Update(
	ctx context.Context,
	candidate memorycore.MemoryCandidate,
) error {
	return nil
}

func (m *mockCandidateRepository) UpdateProcessingResult(
	ctx context.Context,
	candidate memorycore.MemoryCandidate,
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
	err          error
	called       bool
	snapshot     memorycore.ConversationSnapshot
}

func (m *mockCandidateRuleEvaluator) ShouldCreateCandidate(
	snapshot memorycore.ConversationSnapshot,
) (bool, error) {
	m.called = true
	m.snapshot = snapshot
	return m.shouldCreate, m.err
}

func newTestCandidateCreator(
	shouldCreate bool,
) (*CandidateCreatorAdapter, *mockCandidateRepository, *mockCandidateRuleEvaluator) {
	repository := &mockCandidateRepository{}
	evaluator := &mockCandidateRuleEvaluator{
		shouldCreate: shouldCreate,
	}

	useCase := memoryapp.NewCreateCandidateUseCase(
		repository,
		evaluator,
		time.Hour,
	)

	return NewCandidateCreatorAdapter(useCase), repository, evaluator
}

func newTestConversationContext() conversationcore.ConversationContext {
	return conversationcore.ConversationContext{
		GuildID:          "guild-123",
		ChannelID:        "channel-123",
		TriggerMessageID: "message-002",
		Messages: []conversationcore.Message{
			{
				ID:        "message-001",
				GuildID:   "guild-123",
				ChannelID: "channel-123",
				UserID:    "user-001",
				Content:   "I like Go",
				IsBot:     false,
			},
			{
				ID:        "message-002",
				GuildID:   "guild-123",
				ChannelID: "channel-123",
				UserID:    "user-002",
				Content:   "I prefer backend development",
				IsBot:     false,
			},
		},
	}
}

func TestCandidateCreatorAdapterCreateCandidate(t *testing.T) {
	adapter, repository, evaluator := newTestCandidateCreator(true)

	err := adapter.CreateCandidate(
		context.Background(),
		newTestConversationContext(),
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !evaluator.called {
		t.Fatal("expected evaluator to be called")
	}

	if !repository.called {
		t.Fatal("expected repository to be called")
	}

	candidate := repository.candidate

	if candidate.GuildID != "guild-123" {
		t.Errorf("unexpected guild ID: %s", candidate.GuildID)
	}

	if candidate.ChannelID != "channel-123" {
		t.Errorf("unexpected channel ID: %s", candidate.ChannelID)
	}

	if candidate.MessageID != "message-002" {
		t.Errorf("unexpected trigger message ID: %s", candidate.MessageID)
	}

	if candidate.UserID != "user-002" {
		t.Errorf("unexpected user ID: %s", candidate.UserID)
	}

	if candidate.Content != "I prefer backend development" {
		t.Errorf("unexpected content: %s", candidate.Content)
	}

	if len(candidate.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(candidate.Messages))
	}

	first := candidate.Messages[0]

	if first.MessageID != "message-001" ||
		first.UserID != "user-001" ||
		first.Content != "I like Go" ||
		first.IsBot {
		t.Errorf("first message was not mapped correctly: %+v", first)
	}

	second := candidate.Messages[1]

	if second.MessageID != "message-002" ||
		second.UserID != "user-002" ||
		second.Content != "I prefer backend development" ||
		second.IsBot {
		t.Errorf("second message was not mapped correctly: %+v", second)
	}
}

func TestCandidateCreatorAdapterDoesNotCreateCandidateWhenRulesReject(t *testing.T) {
	adapter, repository, evaluator := newTestCandidateCreator(false)

	err := adapter.CreateCandidate(
		context.Background(),
		newTestConversationContext(),
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !evaluator.called {
		t.Fatal("expected evaluator to be called")
	}

	if repository.called {
		t.Fatal("repository should not be called when rules reject candidate")
	}
}

func TestCandidateCreatorAdapterReturnsRepositoryError(t *testing.T) {
	adapter, repository, _ := newTestCandidateCreator(true)

	expectedErr := errors.New("database unavailable")
	repository.err = expectedErr

	err := adapter.CreateCandidate(
		context.Background(),
		newTestConversationContext(),
	)

	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected repository error %v, got %v", expectedErr, err)
	}
}

func TestCandidateCreatorAdapterTreatsDuplicateCandidateAsIdempotent(t *testing.T) {
	adapter, repository, _ := newTestCandidateCreator(true)
	repository.err = memorycore.ErrCandidateAlreadyExists

	err := adapter.CreateCandidate(
		context.Background(),
		newTestConversationContext(),
	)
	if err != nil {
		t.Fatalf("expected nil error for duplicate candidate, got %v", err)
	}
}

func TestCandidateCreatorAdapterRejectsNilAdapter(t *testing.T) {
	var adapter *CandidateCreatorAdapter

	err := adapter.CreateCandidate(
		context.Background(),
		newTestConversationContext(),
	)

	if err == nil {
		t.Fatal("expected error for nil adapter")
	}
}

func TestCandidateCreatorAdapterRejectsNilUseCase(t *testing.T) {
	adapter := NewCandidateCreatorAdapter(nil)

	err := adapter.CreateCandidate(
		context.Background(),
		newTestConversationContext(),
	)

	if err == nil {
		t.Fatal("expected error for nil use case")
	}
}

func TestCandidateCreatorAdapterReturnsContextError(t *testing.T) {
	adapter, repository, _ := newTestCandidateCreator(true)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := adapter.CreateCandidate(
		ctx,
		newTestConversationContext(),
	)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}

	if repository.called {
		t.Fatal("repository should not be called for canceled context")
	}
}
