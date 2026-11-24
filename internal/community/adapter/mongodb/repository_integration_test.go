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

	"the-bebot/internal/community/core"
)

func setupCommunityRepositories(t *testing.T) (*CommunityRepository, *MemberRepository, *driver.Database) {
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

	dbName := fmt.Sprintf("the_bebot_community_test_%s", primitive.NewObjectID().Hex())
	db := client.Database(dbName)

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_ = db.Drop(cleanupCtx)
		_ = client.Disconnect(cleanupCtx)
	})

	communityRepo, err := NewCommunityRepository(db)
	if err != nil {
		t.Fatalf("NewCommunityRepository() error = %v", err)
	}
	if err := communityRepo.EnsureIndexes(ctx); err != nil {
		t.Fatalf("communityRepo.EnsureIndexes() error = %v", err)
	}

	memberRepo, err := NewMemberRepository(db)
	if err != nil {
		t.Fatalf("NewMemberRepository() error = %v", err)
	}
	if err := memberRepo.EnsureIndexes(ctx); err != nil {
		t.Fatalf("memberRepo.EnsureIndexes() error = %v", err)
	}

	return communityRepo, memberRepo, db
}

func TestCommunityRepositoryUpsertPreservesActiveSinceAndNeverGuessesLanguage(t *testing.T) {
	communityRepo, _, db := setupCommunityRepositories(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	firstSeen := time.Now().UTC().Truncate(time.Millisecond)

	first, err := communityRepo.UpsertInteraction(ctx, core.CommunityUpsertInput{
		GuildID: "guild-100",
		Name:    "Bebot Squad",
		SeenAt:  firstSeen,
	})
	if err != nil {
		t.Fatalf("first UpsertInteraction() error = %v", err)
	}

	if first.GuildID != "guild-100" || first.Name != "Bebot Squad" {
		t.Errorf("unexpected first community: %+v", first)
	}
	if first.PrimaryLanguage != "" {
		t.Errorf("PrimaryLanguage must remain unset when unknown, got %q", first.PrimaryLanguage)
	}
	if !first.ActiveSince.Equal(firstSeen) {
		t.Errorf("ActiveSince = %v, want %v", first.ActiveSince, firstSeen)
	}

	// Second interaction without name must preserve existing name and active_since.
	later := firstSeen.Add(10 * time.Minute)
	second, err := communityRepo.UpsertInteraction(ctx, core.CommunityUpsertInput{
		GuildID: "guild-100",
		Name:    "",
		SeenAt:  later,
	})
	if err != nil {
		t.Fatalf("second UpsertInteraction() error = %v", err)
	}

	if second.Name != "Bebot Squad" {
		t.Errorf("expected existing guild name to be preserved, got %q", second.Name)
	}
	if !second.ActiveSince.Equal(firstSeen) {
		t.Errorf("ActiveSince changed from %v to %v", firstSeen, second.ActiveSince)
	}
	if !second.UpdatedAt.Equal(later) {
		t.Errorf("UpdatedAt = %v, want %v", second.UpdatedAt, later)
	}

	count, err := db.Collection("communities").CountDocuments(ctx, bson.M{"guild_id": "guild-100"})
	if err != nil {
		t.Fatalf("CountDocuments() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 community doc, got %d", count)
	}
}

func TestMemberRepositoryUpsertIncrementsInteractionCountAndIsolatesGuilds(t *testing.T) {
	_, memberRepo, db := setupCommunityRepositories(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	t1 := time.Now().UTC().Truncate(time.Millisecond)

	m1, err := memberRepo.UpsertInteraction(ctx, core.MemberUpsertInput{
		GuildID:     "guild-1",
		UserID:      "user-1",
		DisplayName: "Alice",
		Nickname:    "Ali",
		SeenAt:      t1,
	})
	if err != nil {
		t.Fatalf("first UpsertInteraction() error = %v", err)
	}
	if m1.InteractionCount != 1 || m1.DisplayName != "Alice" || m1.Nickname != "Ali" {
		t.Fatalf("unexpected first member: %+v", m1)
	}

	t2 := t1.Add(time.Minute)
	m2, err := memberRepo.UpsertInteraction(ctx, core.MemberUpsertInput{
		GuildID:     "guild-1",
		UserID:      "user-1",
		DisplayName: "Alice",
		Nickname:    "Alice The Dev",
		SeenAt:      t2,
	})
	if err != nil {
		t.Fatalf("second UpsertInteraction() error = %v", err)
	}
	if m2.InteractionCount != 2 || m2.Nickname != "Alice The Dev" || !m2.LastSeenAt.Equal(t2) {
		t.Fatalf("unexpected second member: %+v", m2)
	}

	// Same user in a different guild is tracked separately.
	otherGuildMember, err := memberRepo.UpsertInteraction(ctx, core.MemberUpsertInput{
		GuildID:     "guild-2",
		UserID:      "user-1",
		DisplayName: "Alice",
		SeenAt:      t2,
	})
	if err != nil {
		t.Fatalf("other guild UpsertInteraction() error = %v", err)
	}
	if otherGuildMember.InteractionCount != 1 {
		t.Fatalf("other guild interaction count = %d, want 1", otherGuildMember.InteractionCount)
	}

	count, err := db.Collection("members").CountDocuments(ctx, bson.M{"user_id": "user-1"})
	if err != nil {
		t.Fatalf("CountDocuments() error = %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 guild-isolated member docs, got %d", count)
	}
}

func TestCommunityAndMemberEnsureIndexes(t *testing.T) {
	communityRepo, memberRepo, _ := setupCommunityRepositories(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	assertUniqueIndex := func(coll *driver.Collection, expectedName string) {
		t.Helper()
		cursor, err := coll.Indexes().List(ctx)
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

		for _, idx := range indexes {
			if idx.Name == expectedName && idx.Unique {
				return
			}
		}

		t.Errorf("expected unique index %q on collection %s", expectedName, coll.Name())
	}

	assertUniqueIndex(communityRepo.collection, "idx_communities_guild_id")
	assertUniqueIndex(memberRepo.collection, "idx_members_guild_user")
}
