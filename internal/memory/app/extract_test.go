package app

import (
	"context"
	"errors"
	"testing"

	"the-bebot/internal/memory/core"
)

type mockExtractionRules struct {
	shouldExtract bool
	err           error
	called        bool
}

func (m *mockExtractionRules) ShouldCreateCandidate(
	snapshot core.ConversationSnapshot,
) (bool, error) {
	m.called = true
	return m.shouldExtract, m.err
}

type mockMemoryExtractor struct {
	memories []core.ExtractedMemory
	err      error
	called   bool
	input    core.ExtractionInput
}

func (m *mockMemoryExtractor) Extract(
	ctx context.Context,
	input core.ExtractionInput,
) ([]core.ExtractedMemory, error) {
	m.called = true
	m.input = input
	return m.memories, m.err
}

type mockExtractionValidator struct {
	memory core.Memory
	err    error
	called bool
	input  core.ExtractedMemory
	source core.ExtractionSource
}

func (m *mockExtractionValidator) Validate(
	input core.ExtractedMemory,
	source core.ExtractionSource,
) (core.Memory, error) {
	m.called = true
	m.input = input
	m.source = source
	return m.memory, m.err
}

func newValidExtractionRequest() ExtractionRequest {
	return ExtractionRequest{
		TargetUserID: "user-001",
		Snapshot: core.ConversationSnapshot{
			GuildID:          "guild-001",
			ChannelID:        "channel-001",
			TriggerMessageID: "message-003",
			Messages: []core.ConversationMessage{
				{
					MessageID: "message-001",
					UserID:    "user-001",
					Content:   "I like Go programming",
				},
				{
					MessageID: "message-002",
					UserID:    "user-002",
					Content:   "Python is also interesting",
				},
				{
					MessageID: "message-003",
					UserID:    "user-003",
					Content:   "@TheBebot",
				},
			},
		},
	}
}

func newValidExtractedMemory() core.ExtractedMemory {
	return core.ExtractedMemory{
		Scope:           core.MemoryScopeMember,
		Type:            core.MemoryTypePreference,
		Content:         "Likes Go programming",
		Importance:      3,
		Confidence:      0.95,
		Tags:            []string{"Go"},
		SourceMessageID: "message-001",
	}
}

func newValidDomainMemory() core.Memory {
	return core.Memory{
		ID:              "memory-001",
		GuildID:         "guild-001",
		UserID:          "user-001",
		Scope:           core.MemoryScopeMember,
		Type:            core.MemoryTypePreference,
		Content:         "Likes Go programming",
		Importance:      3,
		Confidence:      0.95,
		SourceMessageID: "message-001",
	}
}

func newExtractionUseCase(
	rules *mockExtractionRules,
	extractor *mockMemoryExtractor,
	validator *mockExtractionValidator,
) *ExtractMemoriesUseCase {
	return NewExtractMemoriesUseCase(
		rules,
		extractor,
		validator,
	)
}

func TestExtractMemoriesSuccess(t *testing.T) {
	rules := &mockExtractionRules{
		shouldExtract: true,
	}

	extractor := &mockMemoryExtractor{
		memories: []core.ExtractedMemory{
			newValidExtractedMemory(),
		},
	}

	validator := &mockExtractionValidator{
		memory: newValidDomainMemory(),
	}

	uc := newExtractionUseCase(rules, extractor, validator)

	memories, err := uc.Execute(
		context.Background(),
		newValidExtractionRequest(),
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if len(memories) != 1 {
		t.Fatalf("memories count = %d, want 1", len(memories))
	}

	if !rules.called || !extractor.called || !validator.called {
		t.Fatal("expected all pipeline components to be called")
	}

	if extractor.input.GuildID != "guild-001" {
		t.Errorf("extractor GuildID = %s", extractor.input.GuildID)
	}

	if extractor.input.UserID != "user-001" {
		t.Errorf("extractor UserID = %s", extractor.input.UserID)
	}

	if validator.source.MessageID != "message-001" {
		t.Errorf(
			"source MessageID = %s, want message-001",
			validator.source.MessageID,
		)
	}

	if validator.source.UserID != "user-001" {
		t.Errorf(
			"source UserID = %s, want user-001",
			validator.source.UserID,
		)
	}
}

func TestExtractMemoriesSkipsWithoutCandidateSignal(t *testing.T) {
	rules := &mockExtractionRules{
		shouldExtract: false,
	}

	extractor := &mockMemoryExtractor{}
	validator := &mockExtractionValidator{}

	uc := newExtractionUseCase(rules, extractor, validator)

	memories, err := uc.Execute(
		context.Background(),
		newValidExtractionRequest(),
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if len(memories) != 0 {
		t.Errorf("memories count = %d, want 0", len(memories))
	}

	if extractor.called {
		t.Error("extractor should not be called")
	}

	if validator.called {
		t.Error("validator should not be called")
	}
}

func TestExtractMemoriesEmptyLLMResult(t *testing.T) {
	rules := &mockExtractionRules{
		shouldExtract: true,
	}

	extractor := &mockMemoryExtractor{
		memories: []core.ExtractedMemory{},
	}

	validator := &mockExtractionValidator{}

	uc := newExtractionUseCase(rules, extractor, validator)

	memories, err := uc.Execute(
		context.Background(),
		newValidExtractionRequest(),
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if len(memories) != 0 {
		t.Errorf("memories count = %d, want 0", len(memories))
	}

	if validator.called {
		t.Error("validator should not be called")
	}
}

func TestExtractMemoriesRejectsWrongSourceUser(t *testing.T) {
	rules := &mockExtractionRules{
		shouldExtract: true,
	}

	extracted := newValidExtractedMemory()
	extracted.SourceMessageID = "message-002"

	extractor := &mockMemoryExtractor{
		memories: []core.ExtractedMemory{extracted},
	}

	validator := &mockExtractionValidator{}

	uc := newExtractionUseCase(rules, extractor, validator)

	_, err := uc.Execute(
		context.Background(),
		newValidExtractionRequest(),
	)

	if !errors.Is(err, core.ErrInvalidExtractionSource) {
		t.Errorf(
			"error = %v, want ErrInvalidExtractionSource",
			err,
		)
	}

	if validator.called {
		t.Error("validator should not be called for invalid source")
	}
}

func TestExtractMemoriesRejectsBotSource(t *testing.T) {
	rules := &mockExtractionRules{
		shouldExtract: true,
	}

	request := newValidExtractionRequest()
	request.Snapshot.Messages[0].IsBot = true

	extractor := &mockMemoryExtractor{
		memories: []core.ExtractedMemory{
			newValidExtractedMemory(),
		},
	}

	validator := &mockExtractionValidator{}

	uc := newExtractionUseCase(rules, extractor, validator)

	_, err := uc.Execute(context.Background(), request)

	if !errors.Is(err, core.ErrInvalidExtractionSource) {
		t.Errorf(
			"error = %v, want ErrInvalidExtractionSource",
			err,
		)
	}

	if validator.called {
		t.Error("validator should not be called")
	}
}

func TestExtractMemoriesStopsOnRuleError(t *testing.T) {
	expectedErr := errors.New("rule evaluation failed")

	rules := &mockExtractionRules{
		err: expectedErr,
	}

	extractor := &mockMemoryExtractor{}
	validator := &mockExtractionValidator{}

	uc := newExtractionUseCase(rules, extractor, validator)

	_, err := uc.Execute(
		context.Background(),
		newValidExtractionRequest(),
	)

	if !errors.Is(err, expectedErr) {
		t.Errorf("error = %v, want rule error", err)
	}

	if extractor.called || validator.called {
		t.Error("pipeline should stop after rule error")
	}
}

func TestExtractMemoriesStopsOnLLMError(t *testing.T) {
	expectedErr := errors.New("LLM unavailable")

	rules := &mockExtractionRules{
		shouldExtract: true,
	}

	extractor := &mockMemoryExtractor{
		err: expectedErr,
	}

	validator := &mockExtractionValidator{}

	uc := newExtractionUseCase(rules, extractor, validator)

	_, err := uc.Execute(
		context.Background(),
		newValidExtractionRequest(),
	)

	if !errors.Is(err, expectedErr) {
		t.Errorf("error = %v, want LLM error", err)
	}

	if validator.called {
		t.Error("validator should not be called after LLM error")
	}
}

func TestExtractMemoriesStopsOnValidationError(t *testing.T) {
	expectedErr := core.ErrInvalidExtractedMemory

	rules := &mockExtractionRules{
		shouldExtract: true,
	}

	extractor := &mockMemoryExtractor{
		memories: []core.ExtractedMemory{
			newValidExtractedMemory(),
		},
	}

	validator := &mockExtractionValidator{
		err: expectedErr,
	}

	uc := newExtractionUseCase(rules, extractor, validator)

	_, err := uc.Execute(
		context.Background(),
		newValidExtractionRequest(),
	)

	if !errors.Is(err, expectedErr) {
		t.Errorf("error = %v, want validation error", err)
	}
}

func TestExtractMemoriesRejectsInvalidRequest(t *testing.T) {
	rules := &mockExtractionRules{
		shouldExtract: true,
	}

	extractor := &mockMemoryExtractor{}
	validator := &mockExtractionValidator{}

	uc := newExtractionUseCase(rules, extractor, validator)

	request := newValidExtractionRequest()
	request.TargetUserID = ""

	_, err := uc.Execute(context.Background(), request)

	if !errors.Is(err, ErrInvalidExtractionRequest) {
		t.Errorf(
			"error = %v, want ErrInvalidExtractionRequest",
			err,
		)
	}

	if rules.called || extractor.called || validator.called {
		t.Error("pipeline components should not be called")
	}
}

func TestExtractMemoriesRejectsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	rules := &mockExtractionRules{
		shouldExtract: true,
	}

	extractor := &mockMemoryExtractor{}
	validator := &mockExtractionValidator{}

	uc := newExtractionUseCase(rules, extractor, validator)

	_, err := uc.Execute(ctx, newValidExtractionRequest())

	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}

	if extractor.called || validator.called {
		t.Error("pipeline should stop after context cancellation")
	}
}
