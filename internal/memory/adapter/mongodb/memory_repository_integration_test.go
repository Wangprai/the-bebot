package mongodb

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"the-bebot/internal/memory/core"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	driver "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func setupMemoryRepository(t *testing.T) *MemoryRepository {
	t.Helper()

	uri := os.Getenv("MONGODB_TEST_URI")
	if uri == "" {
		uri = "mongodb://127.0.0.1:27017"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := driver.Connect(ctx, options.Client().
		ApplyURI(uri).
		SetServerSelectionTimeout(3*time.Second),
	)
	if err != nil {
		t.Skipf("MongoDB is not available: %v", err)
	}

	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		t.Skipf("MongoDB is not available: %v", err)
	}

	databaseName := fmt.Sprintf("the_bebot_test_%s", primitive.NewObjectID().Hex())
	database := client.Database(databaseName)

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)
		defer cleanupCancel()

		_ = database.Drop(cleanupCtx)
		_ = client.Disconnect(cleanupCtx)
	})

	repository, err := NewMemoryRepository(database)
	if err != nil {
		t.Fatalf("create memory repository failed: %v", err)
	}

	return repository
}

func newTestMemory(scope core.MemoryScope, userID, content string) core.Memory {
	now := time.Now().UTC()

	return core.Memory{
		ID:              primitive.NewObjectID().Hex(),
		GuildID:         "guild-123",
		UserID:          userID,
		Scope:           scope,
		Type:            core.MemoryTypePersonalFact,
		Content:         content,
		Importance:      4,
		Confidence:      0.9,
		Tags:            []string{"test", "integration"},
		SourceMessageID: "message-123",
		UsageCount:      0,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

func TestMemoryRepositoryIntegration(t *testing.T) {
	repository := setupMemoryRepository(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Save community memory.
	communityMemory := newTestMemory(
		core.MemoryScopeCommunity,
		"",
		"Community likes playing games",
	)

	communityID, err := repository.Save(ctx, communityMemory)
	if err != nil {
		t.Fatalf("Save community memory failed: %v", err)
	}

	if _, err := primitive.ObjectIDFromHex(communityID); err != nil {
		t.Fatalf("Save returned invalid ObjectID: %v", err)
	}

	// Save member memory.
	memberMemory := newTestMemory(
		core.MemoryScopeMember,
		"user-123",
		"User prefers Go programming",
	)

	memberID, err := repository.Save(ctx, memberMemory)
	if err != nil {
		t.Fatalf("Save member memory failed: %v", err)
	}

	// Saving the same memory again should return the existing ID.
	duplicateID, err := repository.Save(ctx, memberMemory)
	if err != nil {
		t.Fatalf("Save duplicate memory failed: %v", err)
	}

	if duplicateID != memberID {
		t.Fatalf(
			"expected same memory ID %q, got %q",
			memberID,
			duplicateID,
		)
	}

	// Save another member's memory.
	otherMemberMemory := newTestMemory(
		core.MemoryScopeMember,
		"user-456",
		"Another user prefers Python",
	)

	_, err = repository.Save(ctx, otherMemberMemory)
	if err != nil {
		t.Fatalf("Save other member memory failed: %v", err)
	}

	count, err := repository.collection.CountDocuments(
		ctx,
		bson.M{"guild_id": "guild-123"},
	)
	if err != nil {
		t.Fatalf("count memories failed: %v", err)
	}

	if count != 3 {
		t.Fatalf("expected 3 memories, got %d", count)
	}

	if err := repository.EnsureIndexes(ctx); err != nil {
		t.Fatalf("EnsureIndexes failed: %v", err)
	}

	// FindByID.
	found, err := repository.FindByID(ctx, "guild-123", memberID)
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}

	if found.ID != memberID {
		t.Errorf("expected ID %q, got %q", memberID, found.ID)
	}

	if found.Content != memberMemory.Content {
		t.Errorf("expected content %q, got %q", memberMemory.Content, found.Content)
	}

	// FindRelevant should return community + target member memories.
	relevant, err := repository.FindRelevant(ctx, "guild-123", "user-123", 10)
	if err != nil {
		t.Fatalf("FindRelevant failed: %v", err)
	}

	if len(relevant) != 2 {
		t.Fatalf("expected 2 relevant memories, got %d", len(relevant))
	}

	for _, memory := range relevant {
		if memory.Scope == core.MemoryScopeMember && memory.UserID != "user-123" {
			t.Errorf("unexpected member memory from user %q", memory.UserID)
		}
	}

	// UpdateUsage must NOT modify updated_at (only usage_count and last_used_at).
	originalUpdatedAt := found.UpdatedAt
	usedAt := time.Now().UTC().Add(time.Hour).Truncate(time.Millisecond)

	if err := repository.UpdateUsage(ctx, "guild-123", memberID, usedAt); err != nil {
		t.Fatalf("UpdateUsage failed: %v", err)
	}

	updated, err := repository.FindByID(ctx, "guild-123", memberID)
	if err != nil {
		t.Fatalf("FindByID after UpdateUsage failed: %v", err)
	}

	if updated.UsageCount != 1 {
		t.Errorf("expected UsageCount 1, got %d", updated.UsageCount)
	}

	if updated.LastUsedAt == nil {
		t.Fatal("expected LastUsedAt to be set")
	}

	if !updated.LastUsedAt.Equal(usedAt) {
		t.Errorf("expected LastUsedAt %v, got %v", usedAt, *updated.LastUsedAt)
	}

	if !updated.UpdatedAt.Equal(originalUpdatedAt) {
		t.Errorf("UpdateUsage must not touch updated_at: got %v, want %v", updated.UpdatedAt, originalUpdatedAt)
	}

	// Wrong guild must not find the memory.
	_, err = repository.FindByID(ctx, "another-guild", memberID)
	if !errors.Is(err, ErrMemoryNotFound) {
		t.Errorf("expected ErrMemoryNotFound, got %v", err)
	}

	// DeleteByID.
	if err := repository.DeleteByID(ctx, "guild-123", memberID); err != nil {
		t.Fatalf("DeleteByID failed: %v", err)
	}

	_, err = repository.FindByID(ctx, "guild-123", memberID)
	if !errors.Is(err, ErrMemoryNotFound) {
		t.Errorf("expected ErrMemoryNotFound after deletion, got %v", err)
	}

	// Other records should remain.
	if _, err := repository.FindByID(ctx, "guild-123", communityID); err != nil {
		t.Errorf("community memory should still exist: %v", err)
	}
}

func TestMemoryRepositoryEnsureIndexes(t *testing.T) {
	repository := setupMemoryRepository(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := repository.EnsureIndexes(ctx); err != nil {
		t.Fatalf("EnsureIndexes failed: %v", err)
	}

	cursor, err := repository.collection.Indexes().List(ctx)
	if err != nil {
		t.Fatalf("list indexes failed: %v", err)
	}
	defer cursor.Close(ctx)

	var indexes []struct {
		Name string `bson:"name"`
	}

	if err := cursor.All(ctx, &indexes); err != nil {
		t.Fatalf("decode indexes failed: %v", err)
	}

	found := make(map[string]bool)

	for _, index := range indexes {
		found[index.Name] = true
	}

	expected := []string{
		"_id_",
		"idx_memories_community_relevance",
		"idx_memories_member_relevance",
		"idx_memories_idempotency",
		"idx_memories_candidate_item",
	}

	for _, name := range expected {
		if !found[name] {
			t.Errorf("expected index %q was not created", name)
		}
	}
}

func TestMemoryRepositoryUpdateUsageManyPreservesUpdatedAtAndRanking(t *testing.T) {
	repository := setupMemoryRepository(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := repository.EnsureIndexes(ctx); err != nil {
		t.Fatalf("EnsureIndexes failed: %v", err)
	}

	olderTime := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	newerTime := time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)

	older := newTestMemory(core.MemoryScopeMember, "user-rank", "Older preference")
	older.CreatedAt = olderTime
	older.UpdatedAt = olderTime
	olderID, err := repository.Save(ctx, older)
	if err != nil {
		t.Fatalf("Save older failed: %v", err)
	}

	newer := newTestMemory(core.MemoryScopeMember, "user-rank", "Newer preference")
	newer.CreatedAt = newerTime
	newer.UpdatedAt = newerTime
	newerID, err := repository.Save(ctx, newer)
	if err != nil {
		t.Fatalf("Save newer failed: %v", err)
	}

	// Bulk update usage for both memories at a much later timestamp.
	usedAt := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	if err := repository.UpdateUsageMany(ctx, "guild-123", []string{olderID, newerID}, usedAt); err != nil {
		t.Fatalf("UpdateUsageMany failed: %v", err)
	}

	// Also update older one more time; it still must NOT overtake newer in FindRelevant.
	laterUse := usedAt.Add(time.Hour)
	if err := repository.UpdateUsageMany(ctx, "guild-123", []string{olderID}, laterUse); err != nil {
		t.Fatalf("second UpdateUsageMany failed: %v", err)
	}

	reloadedOlder, err := repository.FindByID(ctx, "guild-123", olderID)
	if err != nil {
		t.Fatalf("FindByID older failed: %v", err)
	}
	if reloadedOlder.UsageCount != 2 || !reloadedOlder.UpdatedAt.Equal(olderTime) {
		t.Errorf("older usage=%d updated_at=%v, want usage=2 updated_at=%v",
			reloadedOlder.UsageCount, reloadedOlder.UpdatedAt, olderTime)
	}

	relevant, err := repository.FindRelevant(ctx, "guild-123", "user-rank", 5)
	if err != nil {
		t.Fatalf("FindRelevant failed: %v", err)
	}
	if len(relevant) != 2 {
		t.Fatalf("expected 2 relevant memories, got %d", len(relevant))
	}
	if relevant[0].ID != newerID {
		t.Errorf("newer memory should rank first (updated_at unchanged by usage): got %q first, want %q", relevant[0].ID, newerID)
	}
}

func TestMemoryRepositoryRewordedRetryProducesNoDuplicatesAndNormalizesContent(t *testing.T) {
	repository := setupMemoryRepository(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := repository.EnsureIndexes(ctx); err != nil {
		t.Fatalf("EnsureIndexes failed: %v", err)
	}

	// 1. Candidate-backed memories: reworded retry at same (candidate_id, index) produces NO duplicate.
	firstAttempt := newTestMemory(core.MemoryScopeMember, "user-retry", "Likes Go for backend services")
	firstAttempt.CandidateID = "cand-abc"
	firstAttempt.CandidateIndex = 0

	firstID, err := repository.Save(ctx, firstAttempt)
	if err != nil {
		t.Fatalf("first Save failed: %v", err)
	}

	rewordedRetry := newTestMemory(core.MemoryScopeMember, "user-retry", "Prefers Golang for backend engineering")
	rewordedRetry.CandidateID = "cand-abc"
	rewordedRetry.CandidateIndex = 0

	retryID, err := repository.Save(ctx, rewordedRetry)
	if err != nil {
		t.Fatalf("reworded retry Save failed: %v", err)
	}
	if retryID != firstID {
		t.Fatalf("reworded retry created duplicate: got ID %q, want %q", retryID, firstID)
	}

	// Different index for the same candidate IS saved as a second memory.
	secondItem := newTestMemory(core.MemoryScopeMember, "user-retry", "Uses MongoDB")
	secondItem.CandidateID = "cand-abc"
	secondItem.CandidateIndex = 1
	secondID, err := repository.Save(ctx, secondItem)
	if err != nil {
		t.Fatalf("second item Save failed: %v", err)
	}
	if secondID == firstID {
		t.Fatal("distinct candidate index should create a distinct memory")
	}

	// 2. Non-candidate memories (no candidate_id): partial unique index allows multiple docs,
	//    and content-based idempotency normalizes case & whitespace.
	legacyA := newTestMemory(core.MemoryScopeMember, "user-norm", "Likes   Go\tProgramming")
	idA, err := repository.Save(ctx, legacyA)
	if err != nil {
		t.Fatalf("Save legacyA failed: %v", err)
	}

	legacyB := newTestMemory(core.MemoryScopeMember, "user-norm", "likes go programming")
	idB, err := repository.Save(ctx, legacyB)
	if err != nil {
		t.Fatalf("Save legacyB failed: %v", err)
	}
	if idB != idA {
		t.Errorf("normalized content should deduplicate: idA=%q idB=%q", idA, idB)
	}

	legacyDifferent := newTestMemory(core.MemoryScopeMember, "user-norm", "likes rust programming")
	if _, err := repository.Save(ctx, legacyDifferent); err != nil {
		t.Fatalf("second legacy doc without candidate_id should coexist under partial index: %v", err)
	}

	candCount, err := repository.collection.CountDocuments(ctx, bson.M{"candidate_id": "cand-abc"})
	if err != nil {
		t.Fatalf("CountDocuments failed: %v", err)
	}
	if candCount != 2 {
		t.Errorf("candidate memory count = %d, want 2", candCount)
	}
}
