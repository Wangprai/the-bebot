package conversation

import (
	"context"
	"errors"
	"testing"
	"time"

	memoryapp "the-bebot/internal/memory/app"
	memorycore "the-bebot/internal/memory/core"
)

type mockMemoryRepository struct {
	memories []memorycore.Memory
	findErr  error

	guildID      string
	userID       string
	limit        int
	updateCalled int
}

func (m *mockMemoryRepository) Save(
	ctx context.Context,
	memory memorycore.Memory,
) (string, error) {
	return memory.ID, nil
}

func (m *mockMemoryRepository) FindByID(
	ctx context.Context,
	guildID string,
	memoryID string,
) (*memorycore.Memory, error) {
	return nil, nil
}

func (m *mockMemoryRepository) FindRelevant(
	ctx context.Context,
	guildID string,
	userID string,
	limit int,
) ([]memorycore.Memory, error) {
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
	return nil
}

func (m *mockMemoryRepository) UpdateUsageMany(
	ctx context.Context,
	guildID string,
	memoryIDs []string,
	usedAt time.Time,
) error {
	m.updateCalled++
	return nil
}

func (m *mockMemoryRepository) DeleteByID(
	ctx context.Context,
	guildID string,
	memoryID string,
) error {
	return nil
}

func TestMemoryRetrieverAdapterRetrieveMemories(t *testing.T) {
	now := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)

	repo := &mockMemoryRepository{
		memories: []memorycore.Memory{
			{
				ID:         "mem-1",
				GuildID:    "guild-001",
				Scope:      memorycore.MemoryScopeCommunity,
				Type:       memorycore.MemoryTypeRunningJoke,
				Content:    "Community jokes about late deploys",
				Importance: 4,
				Confidence: 0.9,
				CreatedAt:  now,
				UpdatedAt:  now,
			},
			{
				ID:         "mem-2",
				GuildID:    "guild-001",
				UserID:     "user-001",
				Scope:      memorycore.MemoryScopeMember,
				Type:       memorycore.MemoryTypePreference,
				Content:    "Likes Go",
				Importance: 3,
				Confidence: 0.95,
				CreatedAt:  now,
				UpdatedAt:  now,
			},
		},
	}

	useCase := memoryapp.NewRetrieveMemoriesUseCase(repo)
	adapter := NewMemoryRetrieverAdapter(useCase)

	memories, err := adapter.RetrieveMemories(
		context.Background(),
		"guild-001",
		"user-001",
		5,
	)
	if err != nil {
		t.Fatalf("RetrieveMemories() error = %v", err)
	}

	if len(memories) != 2 {
		t.Fatalf("expected 2 memories, got %d", len(memories))
	}

	if memories[0].Scope != "community" ||
		memories[0].Type != "running_joke" ||
		memories[0].Content != "Community jokes about late deploys" ||
		memories[0].Importance != 4 {
		t.Errorf("first memory mapped incorrectly: %+v", memories[0])
	}

	if memories[1].Scope != "member" ||
		memories[1].Type != "preference" ||
		memories[1].Content != "Likes Go" ||
		memories[1].Importance != 3 {
		t.Errorf("second memory mapped incorrectly: %+v", memories[1])
	}

	if repo.limit != 5 || repo.updateCalled != 1 {
		t.Errorf("unexpected repository call state: limit=%d updateCalled=%d", repo.limit, repo.updateCalled)
	}
}

func TestMemoryRetrieverAdapterReturnsError(t *testing.T) {
	expectedErr := errors.New("mongodb timeout")

	repo := &mockMemoryRepository{
		findErr: expectedErr,
	}

	adapter := NewMemoryRetrieverAdapter(memoryapp.NewRetrieveMemoriesUseCase(repo))

	_, err := adapter.RetrieveMemories(context.Background(), "guild-001", "user-001", 5)
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}
}

func TestMemoryRetrieverAdapterRejectsNil(t *testing.T) {
	var nilAdapter *MemoryRetrieverAdapter
	if _, err := nilAdapter.RetrieveMemories(context.Background(), "guild-001", "user-001", 5); err == nil {
		t.Fatal("expected error for nil adapter")
	}

	adapter := NewMemoryRetrieverAdapter(nil)
	if _, err := adapter.RetrieveMemories(context.Background(), "guild-001", "user-001", 5); err == nil {
		t.Fatal("expected error for nil use case")
	}
}
