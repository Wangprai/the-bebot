package mongodb

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (r *MemoryRepository) EnsureIndexes(ctx context.Context) error {
	if r == nil || r.collection == nil {
		return fmt.Errorf("memory repository is not initialized")
	}

	if ctx == nil {
		return fmt.Errorf("context is required")
	}

	indexes := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "guild_id", Value: 1},
				{Key: "scope", Value: 1},
				{Key: "importance", Value: -1},
				{Key: "updated_at", Value: -1},
			},
			Options: options.Index().SetName("idx_memories_community_relevance"),
		},
		{
			Keys: bson.D{
				{Key: "guild_id", Value: 1},
				{Key: "user_id", Value: 1},
				{Key: "scope", Value: 1},
				{Key: "importance", Value: -1},
				{Key: "updated_at", Value: -1},
			},
			Options: options.Index().SetName("idx_memories_member_relevance"),
		},
		{
			Keys: bson.D{
				{Key: "idempotency_key", Value: 1},
			},
			Options: options.Index().
				SetName("idx_memories_idempotency").
				SetUnique(true).
				SetPartialFilterExpression(bson.M{
					"idempotency_key": bson.M{
						"$type": "string",
					},
				}),
		},
		{
			Keys: bson.D{
				{Key: "guild_id", Value: 1},
				{Key: "candidate_id", Value: 1},
				{Key: "candidate_index", Value: 1},
			},
			Options: options.Index().
				SetName("idx_memories_candidate_item").
				SetUnique(true).
				SetPartialFilterExpression(bson.M{
					"candidate_id": bson.M{
						"$type": "string",
					},
				}),
		},
	}

	_, err := r.collection.Indexes().CreateMany(ctx, indexes)
	if err != nil {
		return fmt.Errorf("create memory indexes: %w", err)
	}

	return nil
}
