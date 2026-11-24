package mongodb

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (r *CommunityRepository) EnsureIndexes(ctx context.Context) error {
	if r == nil || r.collection == nil {
		return fmt.Errorf("community repository is not initialized")
	}

	if ctx == nil {
		return fmt.Errorf("context is required")
	}

	indexes := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "guild_id", Value: 1},
			},
			Options: options.Index().
				SetName("idx_communities_guild_id").
				SetUnique(true),
		},
	}

	_, err := r.collection.Indexes().CreateMany(ctx, indexes)
	if err != nil {
		return fmt.Errorf("create community indexes: %w", err)
	}

	return nil
}

func (r *MemberRepository) EnsureIndexes(ctx context.Context) error {
	if r == nil || r.collection == nil {
		return fmt.Errorf("member repository is not initialized")
	}

	if ctx == nil {
		return fmt.Errorf("context is required")
	}

	indexes := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "guild_id", Value: 1},
				{Key: "user_id", Value: 1},
			},
			Options: options.Index().
				SetName("idx_members_guild_user").
				SetUnique(true),
		},
	}

	_, err := r.collection.Indexes().CreateMany(ctx, indexes)
	if err != nil {
		return fmt.Errorf("create member indexes: %w", err)
	}

	return nil
}
