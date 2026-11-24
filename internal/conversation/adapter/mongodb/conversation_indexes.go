package mongodb

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func (r *ConversationRepository) EnsureIndexes(ctx context.Context) error {
	if r == nil || r.collection == nil {
		return fmt.Errorf("conversation repository is not initialized")
	}

	if ctx == nil {
		return fmt.Errorf("context is required")
	}

	indexes := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "guild_id", Value: 1},
				{Key: "channel_id", Value: 1},
			},
			Options: options.Index().
				SetName("idx_conversation_contexts_guild_channel").
				SetUnique(true),
		},
		{
			Keys: bson.D{
				{Key: "expires_at", Value: 1},
			},
			Options: options.Index().
				SetName("idx_conversation_contexts_ttl").
				SetExpireAfterSeconds(0),
		},
	}

	_, err := r.collection.Indexes().CreateMany(ctx, indexes)
	if err != nil {
		return fmt.Errorf("create conversation indexes: %w", err)
	}

	return nil
}
