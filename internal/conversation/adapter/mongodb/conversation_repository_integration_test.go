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

	"the-bebot/internal/conversation/core"
)

func setupConversationRepository(t *testing.T) *ConversationRepository {
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

	databaseName := fmt.Sprintf("the_bebot_conv_test_%s", primitive.NewObjectID().Hex())
	database := client.Database(databaseName)

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()

		_ = database.Drop(cleanupCtx)
		_ = client.Disconnect(cleanupCtx)
	})

	repo, err := NewConversationRepository(database)
	if err != nil {
		t.Fatalf("NewConversationRepository() error = %v", err)
	}

	if err := repo.EnsureIndexes(ctx); err != nil {
		t.Fatalf("EnsureIndexes() error = %v", err)
	}

	return repo
}

func newTestConversationContext(
	t *testing.T,
	guildID string,
	channelID string,
	triggerID string,
	createdAt time.Time,
	ttl time.Duration,
	messages []core.Message,
) core.ConversationContext {
	t.Helper()

	convCtx, err := core.NewConversationContext(
		guildID,
		channelID,
		triggerID,
		messages,
		createdAt,
		ttl,
		15,
	)
	if err != nil {
		t.Fatalf("NewConversationContext() error = %v", err)
	}

	return convCtx
}

func TestConversationRepositorySaveUpsertFindLatestAndIsolation(t *testing.T) {
	repo := setupConversationRepository(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	now := time.Now().UTC().Truncate(time.Millisecond)

	first := newTestConversationContext(
		t,
		"guild-1",
		"channel-1",
		"msg-1",
		now,
		24*time.Hour,
		[]core.Message{
			{
				ID:        "msg-1",
				GuildID:   "guild-1",
				ChannelID: "channel-1",
				UserID:    "user-1",
				Content:   "Hello Bebot",
				CreatedAt: now,
			},
		},
	)

	if err := repo.Save(ctx, first); err != nil {
		t.Fatalf("first Save() error = %v", err)
	}

	// Save a newer snapshot for the same guild/channel: must upsert in place.
	second := newTestConversationContext(
		t,
		"guild-1",
		"channel-1",
		"msg-2",
		now.Add(time.Minute),
		24*time.Hour,
		[]core.Message{
			{
				ID:        "msg-1",
				GuildID:   "guild-1",
				ChannelID: "channel-1",
				UserID:    "user-1",
				Content:   "Hello Bebot",
				CreatedAt: now,
			},
			{
				ID:        "msg-2",
				GuildID:   "guild-1",
				ChannelID: "channel-1",
				UserID:    "user-2",
				Content:   "How are you?",
				CreatedAt: now.Add(time.Minute),
			},
		},
	)

	if err := repo.Save(ctx, second); err != nil {
		t.Fatalf("second Save() error = %v", err)
	}

	// Save a snapshot in another channel and another guild to verify isolation.
	otherChannel := newTestConversationContext(
		t,
		"guild-1",
		"channel-2",
		"msg-other-ch",
		now,
		24*time.Hour,
		[]core.Message{
			{
				ID:        "msg-other-ch",
				GuildID:   "guild-1",
				ChannelID: "channel-2",
				UserID:    "user-3",
				Content:   "Other channel",
				CreatedAt: now,
			},
		},
	)
	if err := repo.Save(ctx, otherChannel); err != nil {
		t.Fatalf("Save other channel error = %v", err)
	}

	otherGuild := newTestConversationContext(
		t,
		"guild-2",
		"channel-1",
		"msg-other-guild",
		now,
		24*time.Hour,
		[]core.Message{
			{
				ID:        "msg-other-guild",
				GuildID:   "guild-2",
				ChannelID: "channel-1",
				UserID:    "user-4",
				Content:   "Other guild",
				CreatedAt: now,
			},
		},
	)
	if err := repo.Save(ctx, otherGuild); err != nil {
		t.Fatalf("Save other guild error = %v", err)
	}

	// Must have exactly 1 document for (guild-1, channel-1) due to upsert.
	count, err := repo.collection.CountDocuments(ctx, bson.M{
		"guild_id":   "guild-1",
		"channel_id": "channel-1",
	})
	if err != nil {
		t.Fatalf("CountDocuments() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 upserted document for guild-1/channel-1, got %d", count)
	}

	// FindLatest returns the updated snapshot.
	latest, err := repo.FindLatest(ctx, "guild-1", "channel-1")
	if err != nil {
		t.Fatalf("FindLatest() error = %v", err)
	}
	if latest == nil {
		t.Fatal("expected latest conversation context, got nil")
	}

	if latest.TriggerMessageID != "msg-2" {
		t.Errorf("TriggerMessageID = %q, want msg-2", latest.TriggerMessageID)
	}

	if len(latest.Messages) != 2 {
		t.Fatalf("len(Messages) = %d, want 2", len(latest.Messages))
	}

	if latest.Messages[0].ID != "msg-1" || latest.Messages[1].ID != "msg-2" {
		t.Errorf("unexpected messages order/IDs: %+v", latest.Messages)
	}

	// Non-existent guild/channel returns nil, nil.
	missing, err := repo.FindLatest(ctx, "guild-missing", "channel-missing")
	if err != nil {
		t.Fatalf("FindLatest() for missing scope error = %v", err)
	}
	if missing != nil {
		t.Fatalf("expected nil for missing scope, got %+v", missing)
	}
}

func TestConversationRepositoryExpirationAndDeleteExpired(t *testing.T) {
	repo := setupConversationRepository(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	now := time.Now().UTC().Truncate(time.Millisecond)

	expired := newTestConversationContext(
		t,
		"guild-exp",
		"channel-exp",
		"msg-exp",
		now.Add(-25*time.Hour),
		24*time.Hour,
		[]core.Message{
			{
				ID:        "msg-exp",
				GuildID:   "guild-exp",
				ChannelID: "channel-exp",
				UserID:    "user-1",
				Content:   "Expired message",
				CreatedAt: now.Add(-25 * time.Hour),
			},
		},
	)

	active := newTestConversationContext(
		t,
		"guild-exp",
		"channel-active",
		"msg-active",
		now,
		24*time.Hour,
		[]core.Message{
			{
				ID:        "msg-active",
				GuildID:   "guild-exp",
				ChannelID: "channel-active",
				UserID:    "user-1",
				Content:   "Active message",
				CreatedAt: now,
			},
		},
	)

	if err := repo.Save(ctx, expired); err != nil {
		t.Fatalf("Save expired error = %v", err)
	}
	if err := repo.Save(ctx, active); err != nil {
		t.Fatalf("Save active error = %v", err)
	}

	// FindLatest must not return expired contexts even before deletion.
	gotExpired, err := repo.FindLatest(ctx, "guild-exp", "channel-exp")
	if err != nil {
		t.Fatalf("FindLatest expired error = %v", err)
	}
	if gotExpired != nil {
		t.Fatalf("expected nil for expired context, got %+v", gotExpired)
	}

	// DeleteExpired removes only expired documents.
	if err := repo.DeleteExpired(ctx, now); err != nil {
		t.Fatalf("DeleteExpired() error = %v", err)
	}

	expiredCount, err := repo.collection.CountDocuments(ctx, bson.M{
		"guild_id":   "guild-exp",
		"channel_id": "channel-exp",
	})
	if err != nil {
		t.Fatalf("CountDocuments() error = %v", err)
	}
	if expiredCount != 0 {
		t.Fatalf("expected expired document to be deleted, count = %d", expiredCount)
	}

	gotActive, err := repo.FindLatest(ctx, "guild-exp", "channel-active")
	if err != nil {
		t.Fatalf("FindLatest active error = %v", err)
	}
	if gotActive == nil || gotActive.TriggerMessageID != "msg-active" {
		t.Fatalf("expected active context to remain, got %+v", gotActive)
	}
}

func TestConversationRepositoryEnsureIndexes(t *testing.T) {
	repo := setupConversationRepository(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cursor, err := repo.collection.Indexes().List(ctx)
	if err != nil {
		t.Fatalf("list indexes error = %v", err)
	}
	defer cursor.Close(ctx)

	var indexes []struct {
		Name               string `bson:"name"`
		Unique             bool   `bson:"unique"`
		ExpireAfterSeconds *int32 `bson:"expireAfterSeconds"`
	}
	if err := cursor.All(ctx, &indexes); err != nil {
		t.Fatalf("decode indexes error = %v", err)
	}

	var foundUnique, foundTTL bool

	for _, idx := range indexes {
		if idx.Name == "idx_conversation_contexts_guild_channel" && idx.Unique {
			foundUnique = true
		}

		if idx.Name == "idx_conversation_contexts_ttl" &&
			idx.ExpireAfterSeconds != nil &&
			*idx.ExpireAfterSeconds == 0 {
			foundTTL = true
		}
	}

	if !foundUnique {
		t.Error("expected unique index idx_conversation_contexts_guild_channel")
	}

	if !foundTTL {
		t.Error("expected TTL index idx_conversation_contexts_ttl with expireAfterSeconds=0")
	}
}
