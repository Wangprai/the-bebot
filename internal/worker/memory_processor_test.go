package worker

import (
	"context"
	"errors"
	"testing"
	"time"

	memoryapp "the-bebot/internal/memory/app"
	"the-bebot/internal/memory/core"
)

// ---------- Mocks ----------

type mockCandidateRules struct {
	shouldCreate bool
	err          error
}

func (m *mockCandidateRules) ShouldCreateCandidate(
	snapshot core.ConversationSnapshot,
) (bool, error) {
	return m.shouldCreate, m.err
}

type mockMemoryExtractor struct {
	memories []core.ExtractedMemory
	err      error
}

func (m *mockMemoryExtractor) Extract(
	ctx context.Context,
	input core.ExtractionInput,
) ([]core.ExtractedMemory, error) {
	return m.memories, m.err
}

type mockMemoryValidator struct {
	memory core.Memory
	err    error
}

func (m *mockMemoryValidator) Validate(
	input core.ExtractedMemory,
	source core.ExtractionSource,
) (core.Memory, error) {
	return m.memory, m.err
}

type mockProcessorMemoryRepository struct {
	saved   []core.Memory
	saveErr error
}

func (m *mockProcessorMemoryRepository) Save(
	ctx context.Context,
	memory core.Memory,
) (string, error) {
	if m.saveErr != nil {
		return "", m.saveErr
	}

	m.saved = append(m.saved, memory)
	return memory.ID, nil
}

func (m *mockProcessorMemoryRepository) FindByID(
	ctx context.Context,
	guildID, memoryID string,
) (*core.Memory, error) {
	return nil, nil
}

func (m *mockProcessorMemoryRepository) FindRelevant(
	ctx context.Context,
	guildID, userID string,
	limit int,
) ([]core.Memory, error) {
	return nil, nil
}

func (m *mockProcessorMemoryRepository) UpdateUsage(
	ctx context.Context,
	guildID, memoryID string,
	usedAt time.Time,
) error {
	return nil
}

func (m *mockProcessorMemoryRepository) DeleteByID(
	ctx context.Context,
	guildID, memoryID string,
) error {
	return nil
}

type mockProcessorCandidateRepository struct {
	updated   []core.MemoryCandidate
	updateErr error
}

func (m *mockProcessorCandidateRepository) Create(
	ctx context.Context,
	candidate core.MemoryCandidate,
) error {
	return nil
}

func (m *mockProcessorCandidateRepository) ClaimNext(
	ctx context.Context,
	workerID string,
	now time.Time,
	lockDuration time.Duration,
) (*core.MemoryCandidate, error) {
	return nil, nil
}

func (m *mockProcessorCandidateRepository) Update(
	ctx context.Context,
	candidate core.MemoryCandidate,
) error {
	if m.updateErr != nil {
		return m.updateErr
	}

	m.updated = append(m.updated, candidate)
	return nil
}

func (m *mockProcessorCandidateRepository) DeleteExpired(
	ctx context.Context,
	before time.Time,
) error {
	return nil
}

// ---------- Test Helpers ----------

func newProcessorTestCandidate(t *testing.T) core.MemoryCandidate {
	t.Helper()

	now := time.Now().UTC()
	lockedUntil := now.Add(5 * time.Minute)

	candidate, err := core.NewMemoryCandidate(core.MemoryCandidate{
		ID:        "0123456789abcdef01234567",
		GuildID:   "guild-123",
		ChannelID: "channel-123",
		MessageID: "message-123",
		UserID:    "user-123",
		Content:   "I like Go programming",
		Messages: []core.ConversationMessage{
			{
				MessageID: "message-123",
				UserID:    "user-123",
				Content:   "I like Go programming",
			},
		},
		Status:      core.CandidateStatusProcessing,
		Attempts:    1,
		WorkerID:    "worker-1",
		LockedUntil: &lockedUntil,
		ExpireAt:    now.Add(24 * time.Hour),
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("create test candidate: %v", err)
	}

	return candidate
}

func newProcessorTestMemory(t *testing.T) core.Memory {
	t.Helper()

	now := time.Now().UTC()

	memory, err := core.NewMemory(core.Memory{
		ID:              "0123456789abcdef01234567",
		GuildID:         "guild-123",
		UserID:          "user-123",
		Scope:           core.MemoryScopeMember,
		Type:            core.MemoryTypePreference,
		Content:         "User likes Go programming",
		Importance:      3,
		Confidence:      0.9,
		SourceMessageID: "message-123",
		CreatedAt:       now,
		UpdatedAt:       now,
	})
	if err != nil {
		t.Fatalf("create test memory: %v", err)
	}

	return memory
}

func newProcessorTestExtractor(
	memories []core.ExtractedMemory,
	extractErr error,
	memory core.Memory,
) *memoryapp.ExtractMemoriesUseCase {
	return memoryapp.NewExtractMemoriesUseCase(
		&mockCandidateRules{shouldCreate: true},
		&mockMemoryExtractor{
			memories: memories,
			err:      extractErr,
		},
		&mockMemoryValidator{
			memory: memory,
		},
	)
}

// ---------- Tests ----------

func TestMemoryCandidateProcessorSuccess(t *testing.T) {
	memory := newProcessorTestMemory(t)

	extraction := core.ExtractedMemory{
		Scope:           core.MemoryScopeMember,
		Type:            core.MemoryTypePreference,
		Content:         "User likes Go programming",
		Importance:      3,
		Confidence:      0.9,
		SourceMessageID: "message-123",
	}

	extractionUseCase := newProcessorTestExtractor(
		[]core.ExtractedMemory{extraction},
		nil,
		memory,
	)

	memoryRepo := &mockProcessorMemoryRepository{}
	candidateRepo := &mockProcessorCandidateRepository{}

	processor, err := NewMemoryCandidateProcessor(
		extractionUseCase,
		memoryRepo,
		candidateRepo,
		core.NewCandidateService(),
	)
	if err != nil {
		t.Fatalf("create processor: %v", err)
	}

	err = processor.Process(
		context.Background(),
		newProcessorTestCandidate(t),
	)
	if err != nil {
		t.Fatalf("process candidate: %v", err)
	}

	if len(memoryRepo.saved) != 1 {
		t.Fatalf("expected 1 saved memory, got %d", len(memoryRepo.saved))
	}

	if len(candidateRepo.updated) != 1 {
		t.Fatalf("expected 1 candidate update, got %d", len(candidateRepo.updated))
	}

	if candidateRepo.updated[0].Status != core.CandidateStatusCompleted {
		t.Fatalf(
			"expected completed status, got %s",
			candidateRepo.updated[0].Status,
		)
	}

	if candidateRepo.updated[0].WorkerID != "" {
		t.Fatal("expected worker ID to be cleared")
	}
}

func TestMemoryCandidateProcessorRejectsEmptyExtraction(t *testing.T) {
	extractionUseCase := newProcessorTestExtractor(
		[]core.ExtractedMemory{},
		nil,
		core.Memory{},
	)

	memoryRepo := &mockProcessorMemoryRepository{}
	candidateRepo := &mockProcessorCandidateRepository{}

	processor, err := NewMemoryCandidateProcessor(
		extractionUseCase,
		memoryRepo,
		candidateRepo,
		core.NewCandidateService(),
	)
	if err != nil {
		t.Fatalf("create processor: %v", err)
	}

	err = processor.Process(
		context.Background(),
		newProcessorTestCandidate(t),
	)
	if err != nil {
		t.Fatalf("process candidate: %v", err)
	}

	if len(memoryRepo.saved) != 0 {
		t.Fatal("expected no memories to be saved")
	}

	if len(candidateRepo.updated) != 1 {
		t.Fatalf("expected 1 candidate update, got %d", len(candidateRepo.updated))
	}

	if candidateRepo.updated[0].Status != core.CandidateStatusRejected {
		t.Fatalf(
			"expected rejected status, got %s",
			candidateRepo.updated[0].Status,
		)
	}
}

func TestMemoryCandidateProcessorExtractionError(t *testing.T) {
	extractionUseCase := newProcessorTestExtractor(
		nil,
		errors.New("LLM unavailable"),
		core.Memory{},
	)

	memoryRepo := &mockProcessorMemoryRepository{}
	candidateRepo := &mockProcessorCandidateRepository{}

	processor, err := NewMemoryCandidateProcessor(
		extractionUseCase,
		memoryRepo,
		candidateRepo,
		core.NewCandidateService(),
	)
	if err != nil {
		t.Fatalf("create processor: %v", err)
	}

	err = processor.Process(
		context.Background(),
		newProcessorTestCandidate(t),
	)
	if err == nil {
		t.Fatal("expected extraction error")
	}

	if len(candidateRepo.updated) != 1 {
		t.Fatalf("expected 1 candidate update, got %d", len(candidateRepo.updated))
	}

	updated := candidateRepo.updated[0]

	if updated.Status != core.CandidateStatusFailed {
		t.Fatalf("expected failed status, got %s", updated.Status)
	}

	if updated.NextRetryAt.IsZero() {
		t.Fatal("expected retry time to be set")
	}

	if updated.WorkerID != "" {
		t.Fatal("expected worker ID to be cleared")
	}
}

func TestMemoryCandidateProcessorSaveError(t *testing.T) {
	memory := newProcessorTestMemory(t)

	extraction := core.ExtractedMemory{
		Scope:           core.MemoryScopeMember,
		Type:            core.MemoryTypePreference,
		Content:         "User likes Go programming",
		Importance:      3,
		Confidence:      0.9,
		SourceMessageID: "message-123",
	}

	extractionUseCase := newProcessorTestExtractor(
		[]core.ExtractedMemory{extraction},
		nil,
		memory,
	)

	memoryRepo := &mockProcessorMemoryRepository{
		saveErr: errors.New("database unavailable"),
	}
	candidateRepo := &mockProcessorCandidateRepository{}

	processor, err := NewMemoryCandidateProcessor(
		extractionUseCase,
		memoryRepo,
		candidateRepo,
		core.NewCandidateService(),
	)
	if err != nil {
		t.Fatalf("create processor: %v", err)
	}

	err = processor.Process(
		context.Background(),
		newProcessorTestCandidate(t),
	)
	if err == nil {
		t.Fatal("expected save error")
	}

	if len(candidateRepo.updated) != 1 {
		t.Fatalf("expected 1 candidate update, got %d", len(candidateRepo.updated))
	}

	if candidateRepo.updated[0].Status != core.CandidateStatusFailed {
		t.Fatalf(
			"expected failed status, got %s",
			candidateRepo.updated[0].Status,
		)
	}
}

func TestMemoryCandidateProcessorRejectsNonProcessingCandidate(t *testing.T) {
	extractionUseCase := newProcessorTestExtractor(
		nil,
		nil,
		core.Memory{},
	)

	processor, err := NewMemoryCandidateProcessor(
		extractionUseCase,
		&mockProcessorMemoryRepository{},
		&mockProcessorCandidateRepository{},
		core.NewCandidateService(),
	)
	if err != nil {
		t.Fatalf("create processor: %v", err)
	}

	candidate := newProcessorTestCandidate(t)
	candidate.Status = core.CandidateStatusPending

	err = processor.Process(context.Background(), candidate)
	if err == nil {
		t.Fatal("expected error for non-processing candidate")
	}
}

func TestMemoryCandidateProcessorRespectsCanceledContext(t *testing.T) {
	extractionUseCase := newProcessorTestExtractor(
		nil,
		nil,
		core.Memory{},
	)

	processor, err := NewMemoryCandidateProcessor(
		extractionUseCase,
		&mockProcessorMemoryRepository{},
		&mockProcessorCandidateRepository{},
		core.NewCandidateService(),
	)
	if err != nil {
		t.Fatalf("create processor: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = processor.Process(ctx, newProcessorTestCandidate(t))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}
