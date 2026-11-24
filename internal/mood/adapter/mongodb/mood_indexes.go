package mongodb

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (r *CommunityMoodRepository) EnsureIndexes(ctx context.Context) error {
	if r == nil || r.collection == nil {
		return fmt.Errorf("community mood repository is not initialized")
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
				SetName("idx_community_moods_guild_id").
				SetUnique(true),
		},
	}

	_, err := r.collection.Indexes().CreateMany(ctx, indexes)
	if err != nil {
		return fmt.Errorf("create community mood indexes: %w", err)
	}

	return nil
}
