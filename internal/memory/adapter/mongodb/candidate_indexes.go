package mongodb

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (r *CandidateRepository) EnsureIndexes(ctx context.Context) error {
	if r == nil || r.collection == nil {
		return fmt.Errorf("candidate repository is not initialized")
	}

	if ctx == nil {
		return fmt.Errorf("context is required")
	}

	indexes := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "guild_id", Value: 1},
				{Key: "message_id", Value: 1},
			},
			Options: options.Index().
				SetName("idx_candidates_guild_message").
				SetUnique(true),
		},
		{
			Keys: bson.D{
				{Key: "status", Value: 1},
				{Key: "next_retry_at", Value: 1},
				{Key: "created_at", Value: 1},
			},
			Options: options.Index().
				SetName("idx_candidates_pending"),
		},
		{
			Keys: bson.D{
				{Key: "status", Value: 1},
				{Key: "locked_until", Value: 1},
			},
			Options: options.Index().
				SetName("idx_candidates_processing_lock"),
		},
		{
			Keys: bson.D{
				{Key: "expire_at", Value: 1},
			},
			Options: options.Index().
				SetName("idx_candidates_expiration"),
		},
	}

	_, err := r.collection.Indexes().CreateMany(ctx, indexes)
	if err != nil {
		return fmt.Errorf("create candidate indexes: %w", err)
	}

	return nil
}
