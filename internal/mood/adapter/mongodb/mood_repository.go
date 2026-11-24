package mongodb

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"the-bebot/internal/mood/core"
)

type CommunityMoodRepository struct {
	collection *mongo.Collection
}

var _ core.CommunityMoodRepository = (*CommunityMoodRepository)(nil)

func NewCommunityMoodRepository(
	database *mongo.Database,
) (*CommunityMoodRepository, error) {
	if database == nil {
		return nil, errors.New("database is required")
	}

	return &CommunityMoodRepository{
		collection: database.Collection("community_moods"),
	}, nil
}

func (r *CommunityMoodRepository) FindByGuildID(
	ctx context.Context,
	guildID string,
) (*core.CommunityMood, error) {
	if r == nil || r.collection == nil {
		return nil, errors.New("community mood repository is not initialized")
	}

	if ctx == nil {
		return nil, errors.New("context is required")
	}

	guildID = strings.TrimSpace(guildID)
	if guildID == "" {
		return nil, core.ErrInvalidGuildID
	}

	var document communityMoodDocument

	err := r.collection.FindOne(ctx, bson.D{{Key: "guild_id", Value: guildID}}).Decode(&document)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find community mood: %w", err)
	}

	mood, err := document.toDomain()
	if err != nil {
		return nil, fmt.Errorf("map community mood document: %w", err)
	}

	return &mood, nil
}

func (r *CommunityMoodRepository) Save(
	ctx context.Context,
	mood core.CommunityMood,
) error {
	if r == nil || r.collection == nil {
		return errors.New("community mood repository is not initialized")
	}

	if ctx == nil {
		return errors.New("context is required")
	}

	document, err := toCommunityMoodDocument(mood)
	if err != nil {
		return err
	}

	filter := bson.D{{Key: "guild_id", Value: document.GuildID}}
	update := bson.D{
		{Key: "$set", Value: bson.D{
			{Key: "guild_id", Value: document.GuildID},
			{Key: "category", Value: document.Category},
			{Key: "intensity", Value: document.Intensity},
			{Key: "updated_at", Value: document.UpdatedAt},
		}},
		{Key: "$setOnInsert", Value: bson.D{
			{Key: "created_at", Value: document.CreatedAt},
		}},
	}

	_, err = r.collection.UpdateOne(
		ctx,
		filter,
		update,
		options.Update().SetUpsert(true),
	)
	if err != nil {
		return fmt.Errorf("save community mood: %w", err)
	}

	return nil
}
