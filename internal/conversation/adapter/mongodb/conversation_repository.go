package mongodb

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"the-bebot/internal/conversation/core"
)

type ConversationRepository struct {
	collection *mongo.Collection
	clock      func() time.Time
}

var _ core.ConversationRepository = (*ConversationRepository)(nil)

func NewConversationRepository(
	database *mongo.Database,
) (*ConversationRepository, error) {
	if database == nil {
		return nil, errors.New("database is required")
	}

	return &ConversationRepository{
		collection: database.Collection("conversation_contexts"),
		clock:      time.Now,
	}, nil
}

func (r *ConversationRepository) Save(
	ctx context.Context,
	conversationContext core.ConversationContext,
) error {
	if r == nil || r.collection == nil {
		return errors.New("conversation repository is not initialized")
	}

	if ctx == nil {
		return errors.New("context is required")
	}

	document, err := toConversationDocument(conversationContext)
	if err != nil {
		return fmt.Errorf("convert conversation document: %w", err)
	}

	filter := bson.D{
		{Key: "guild_id", Value: document.GuildID},
		{Key: "channel_id", Value: document.ChannelID},
	}

	update := bson.D{
		{Key: "$set", Value: document},
	}

	_, err = r.collection.UpdateOne(
		ctx,
		filter,
		update,
		options.Update().SetUpsert(true),
	)
	if err != nil {
		return fmt.Errorf("save conversation context: %w", err)
	}

	return nil
}

func (r *ConversationRepository) FindLatest(
	ctx context.Context,
	guildID string,
	channelID string,
) (*core.ConversationContext, error) {
	if r == nil || r.collection == nil {
		return nil, errors.New("conversation repository is not initialized")
	}

	if ctx == nil {
		return nil, errors.New("context is required")
	}

	guildID = strings.TrimSpace(guildID)
	channelID = strings.TrimSpace(channelID)

	if guildID == "" || channelID == "" {
		return nil, errors.New("guild ID and channel ID are required")
	}

	now := r.clock().UTC()

	filter := bson.D{
		{Key: "guild_id", Value: guildID},
		{Key: "channel_id", Value: channelID},
		{Key: "expires_at", Value: bson.D{
			{Key: "$gt", Value: now},
		}},
	}

	var document conversationDocument

	err := r.collection.FindOne(ctx, filter).Decode(&document)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find latest conversation context: %w", err)
	}

	conversationContext, err := document.toDomain()
	if err != nil {
		return nil, fmt.Errorf("map conversation document: %w", err)
	}

	return &conversationContext, nil
}

func (r *ConversationRepository) DeleteExpired(
	ctx context.Context,
	before time.Time,
) error {
	if r == nil || r.collection == nil {
		return errors.New("conversation repository is not initialized")
	}

	if ctx == nil {
		return errors.New("context is required")
	}

	if before.IsZero() {
		return errors.New("expiration time is required")
	}

	_, err := r.collection.DeleteMany(
		ctx,
		bson.D{
			{Key: "expires_at", Value: bson.D{
				{Key: "$lte", Value: before.UTC()},
			}},
		},
	)
	if err != nil {
		return fmt.Errorf("delete expired conversation contexts: %w", err)
	}

	return nil
}
