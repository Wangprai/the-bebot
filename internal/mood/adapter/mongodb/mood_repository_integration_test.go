package mongodb

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	driver "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"the-bebot/internal/mood/core"
)

func setupCommunityMoodRepository(t *testing.T) (*CommunityMoodRepository, *driver.Database) {
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

	dbName := fmt.Sprintf("the_bebot_mood_test_%s", primitive.NewObjectID().Hex())
	db := client.Database(dbName)

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_ = db.Drop(cleanupCtx)
		_ = client.Disconnect(cleanupCtx)
	})

	repo, err := NewCommunityMoodRepository(db)
	if err != nil {
		t.Fatalf("NewCommunityMoodRepository() error = %v", err)
	}

	if err := repo.EnsureIndexes(ctx); err != nil {
		t.Fatalf("EnsureIndexes() error = %v", err)
	}

	return repo, db
}

func TestCommunityMoodRepositorySaveReloadAndRestartSurvival(t *testing.T) {
	repo, db := setupCommunityMoodRepository(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	now := time.Now().UTC().Truncate(time.Millisecond)

	// Missing guild returns nil, nil.
	missing, err := repo.FindByGuildID(ctx, "guild-missing")
	if err != nil {
		t.Fatalf("FindByGuildID() error = %v", err)
	}
	if missing != nil {
		t.Fatalf("expected nil for missing guild, got %+v", missing)
	}

	initial, err := core.NewCommunityMood(core.CommunityMood{
		GuildID:   "guild-001",
		Category:  core.CategoryCheerful,
		Intensity: 3,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("NewCommunityMood() error = %v", err)
	}

	if err := repo.Save(ctx, initial); err != nil {
		t.Fatalf("first Save() error = %v", err)
	}

	// Update existing guild mood (upsert, preserving CreatedAt).
	updated, err := core.NewCommunityMood(core.CommunityMood{
		GuildID:   "guild-001",
		Category:  core.CategorySarcastic,
		Intensity: 4,
		CreatedAt: now.Add(time.Hour),
		UpdatedAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("NewCommunityMood() error = %v", err)
	}

	if err := repo.Save(ctx, updated); err != nil {
		t.Fatalf("second Save() error = %v", err)
	}

	// Simulate application restart by creating a brand new repository instance against the same DB.
	restartedRepo, err := NewCommunityMoodRepository(db)
	if err != nil {
		t.Fatalf("restarted NewCommunityMoodRepository() error = %v", err)
	}

	loaded, err := restartedRepo.FindByGuildID(ctx, "guild-001")
	if err != nil {
		t.Fatalf("FindByGuildID() after restart error = %v", err)
	}
	if loaded == nil {
		t.Fatal("expected mood to survive restart")
	}

	if loaded.Category != core.CategorySarcastic || loaded.Intensity != 4 {
		t.Errorf("loaded = (%s, %d), want (sarcastic, 4)", loaded.Category, loaded.Intensity)
	}

	if !loaded.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt = %v, want original %v", loaded.CreatedAt, now)
	}

	if !loaded.UpdatedAt.Equal(now.Add(time.Hour)) {
		t.Errorf("UpdatedAt = %v, want %v", loaded.UpdatedAt, now.Add(time.Hour))
	}

	count, err := db.Collection("community_moods").CountDocuments(ctx, bson.M{"guild_id": "guild-001"})
	if err != nil {
		t.Fatalf("CountDocuments() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 document for guild-001, got %d", count)
	}
}

func TestCommunityMoodRepositoryEnsureIndexes(t *testing.T) {
	repo, _ := setupCommunityMoodRepository(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cursor, err := repo.collection.Indexes().List(ctx)
	if err != nil {
		t.Fatalf("list indexes error = %v", err)
	}
	defer cursor.Close(ctx)

	var indexes []struct {
		Name   string `bson:"name"`
		Unique bool   `bson:"unique"`
	}
	if err := cursor.All(ctx, &indexes); err != nil {
		t.Fatalf("decode indexes error = %v", err)
	}

	foundUnique := false
	for _, idx := range indexes {
		if idx.Name == "idx_community_moods_guild_id" && idx.Unique {
			foundUnique = true
		}
	}

	if !foundUnique {
		t.Fatal("expected unique index idx_community_moods_guild_id")
	}
}
