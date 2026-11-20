package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"the-bebot/internal/memory/core"
)

type mockMemoryRepository struct {
	memories  []core.Memory
	findErr   error
	updateErr error

	findCalled   bool
	updateCalled int

	guildID    string
	userID     string
	limit      int
	updatedIDs []string
	usedAt     time.Time
}

func (m *mockMemoryRepository) Save(
	ctx context.Context,
	memory core.Memory,
) (string, error) {
	return memory.ID, nil
}

func (m *mockMemoryRepository) FindByID(
	ctx context.Context,
	guildID string,
	memoryID string,
) (*core.Memory, error) {
	return nil, nil
}

func (m *mockMemoryRepository) FindRelevant(
	ctx context.Context,
	guildID string,
	userID string,
	limit int,
) ([]core.Memory, error) {
	m.findCalled = true
	m.guildID = guildID
	m.userID = userID
	m.limit = limit

	return m.memories, m.findErr
}

func (m *mockMemoryRepository) UpdateUsage(
	ctx context.Context,
	guildID string,
	memoryID string,
	usedAt time.Time,
) error {
	m.updateCalled++
	m.updatedIDs = append(m.updatedIDs, memoryID)
	m.usedAt = usedAt

	return m.updateErr
}

func (m *mockMemoryRepository) DeleteByID(
	ctx context.Context,
	guildID string,
	memoryID string,
) error {
	return nil
}

func newTestMemories() []core.Memory {
	now := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)

	return []core.Memory{
		{
			ID:         "memory-001",
			GuildID:    "guild-001",
			UserID:     "",
			Scope:      core.MemoryScopeCommunity,
			Type:       core.MemoryTypeRunningJoke,
			Content:    "Everyone jokes about Go bugs",
			Importance: 4,
			Confidence: 0.9,
			CreatedAt:  now,
			UpdatedAt:  now,
		},
		{
			ID:         "memory-002",
			GuildID:    "guild-001",
			UserID:     "user-001",
			Scope:      core.MemoryScopeMember,
			Type:       core.MemoryTypePreference,
			Content:    "Likes Go programming",
			Importance: 3,
			Confidence: 0.9,
			CreatedAt:  now,
			UpdatedAt:  now,
		},
	}
}

func TestRetrieveMemories(t *testing.T) {
	repository := &mockMemoryRepository{
		memories: newTestMemories(),
	}

	useCase := NewRetrieveMemoriesUseCase(repository)

	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	useCase.clock = func() time.Time {
		return now
	}

	result, err := useCase.Execute(context.Background(), RetrieveMemoriesInput{
		GuildID: " guild-001 ",
		UserID:  " user-001 ",
		Limit:   10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 memories, got %d", len(result))
	}

	if !repository.findCalled {
		t.Fatal("expected FindRelevant to be called")
	}

	if repository.guildID != "guild-001" {
		t.Errorf("unexpected guild ID: %s", repository.guildID)
	}

	if repository.userID != "user-001" {
		t.Errorf("unexpected user ID: %s", repository.userID)
	}

	if repository.limit != 10 {
		t.Errorf("unexpected limit: %d", repository.limit)
	}

	if repository.updateCalled != 2 {
		t.Errorf("expected 2 usage updates, got %d", repository.updateCalled)
	}

	if !repository.usedAt.Equal(now) {
		t.Errorf("unexpected usage timestamp: %v", repository.usedAt)
	}
}

func TestRetrieveMemoriesRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name  string
		input RetrieveMemoriesInput
	}{
		{
			name: "empty guild ID",
			input: RetrieveMemoriesInput{
				UserID: "user-001",
				Limit:  10,
			},
		},
		{
			name: "empty user ID",
			input: RetrieveMemoriesInput{
				GuildID: "guild-001",
				Limit:   10,
			},
		},
		{
			name: "invalid limit",
			input: RetrieveMemoriesInput{
				GuildID: "guild-001",
				UserID:  "user-001",
				Limit:   0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository := &mockMemoryRepository{}
			useCase := NewRetrieveMemoriesUseCase(repository)

			_, err := useCase.Execute(context.Background(), tt.input)

			if !errors.Is(err, ErrInvalidRetrievalInput) {
				t.Fatalf("expected ErrInvalidRetrievalInput, got %v", err)
			}

			if repository.findCalled {
				t.Error("repository should not be called")
			}
		})
	}
}

func TestRetrieveMemoriesFindError(t *testing.T) {
	expectedErr := errors.New("database connection failed")

	repository := &mockMemoryRepository{
		findErr: expectedErr,
	}

	useCase := NewRetrieveMemoriesUseCase(repository)

	_, err := useCase.Execute(context.Background(), RetrieveMemoriesInput{
		GuildID: "guild-001",
		UserID:  "user-001",
		Limit:   10,
	})

	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected repository error, got %v", err)
	}

	if repository.updateCalled != 0 {
		t.Error("usage should not be updated when retrieval fails")
	}
}

func TestRetrieveMemoriesUpdateUsageError(t *testing.T) {
	expectedErr := errors.New("update failed")

	repository := &mockMemoryRepository{
		memories:  newTestMemories(),
		updateErr: expectedErr,
	}

	useCase := NewRetrieveMemoriesUseCase(repository)

	_, err := useCase.Execute(context.Background(), RetrieveMemoriesInput{
		GuildID: "guild-001",
		UserID:  "user-001",
		Limit:   10,
	})

	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected update error, got %v", err)
	}
}

func TestRetrieveMemoriesRejectsNilContext(t *testing.T) {
	repository := &mockMemoryRepository{}
	useCase := NewRetrieveMemoriesUseCase(repository)

	ctx := context.Context(nil)

	_, err := useCase.Execute(ctx, RetrieveMemoriesInput{
		GuildID: "guild-001",
		UserID:  "user-001",
		Limit:   10,
	})

	if !errors.Is(err, ErrInvalidRetrievalInput) {
		t.Fatalf("expected ErrInvalidRetrievalInput, got %v", err)
	}

	if repository.findCalled {
		t.Error("repository should not be called")
	}
}

func TestRetrieveMemoriesRejectsNilUseCase(t *testing.T) {
	var useCase *RetrieveMemoriesUseCase

	_, err := useCase.Execute(context.Background(), RetrieveMemoriesInput{
		GuildID: "guild-001",
		UserID:  "user-001",
		Limit:   10,
	})

	if !errors.Is(err, ErrMemoryRepositoryNil) {
		t.Fatalf("expected ErrMemoryRepositoryNil, got %v", err)
	}
}
