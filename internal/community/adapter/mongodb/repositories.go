package mongodb

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"the-bebot/internal/community/core"
)

type CommunityRepository struct {
	collection *mongo.Collection
}

var _ core.CommunityRepository = (*CommunityRepository)(nil)

func NewCommunityRepository(
	database *mongo.Database,
) (*CommunityRepository, error) {
	if database == nil {
		return nil, errors.New("database is required")
	}

	return &CommunityRepository{
		collection: database.Collection("communities"),
	}, nil
}

func (r *CommunityRepository) UpsertInteraction(
	ctx context.Context,
	input core.CommunityUpsertInput,
) (core.Community, error) {
	if r == nil || r.collection == nil {
		return core.Community{}, errors.New("community repository is not initialized")
	}

	if ctx == nil {
		return core.Community{}, errors.New("context is required")
	}

	guildID := strings.TrimSpace(input.GuildID)
	name := strings.TrimSpace(input.Name)

	if guildID == "" {
		return core.Community{}, core.ErrInvalidGuildID
	}

	if input.SeenAt.IsZero() {
		return core.Community{}, core.ErrInvalidTimestamp
	}

	seenAt := input.SeenAt.UTC()

	setFields := bson.D{
		{Key: "guild_id", Value: guildID},
		{Key: "updated_at", Value: seenAt},
	}

	if name != "" {
		setFields = append(setFields, bson.E{Key: "name", Value: name})
	}

	filter := bson.D{{Key: "guild_id", Value: guildID}}
	update := bson.D{
		{Key: "$set", Value: setFields},
		{Key: "$setOnInsert", Value: bson.D{
			{Key: "active_since", Value: seenAt},
		}},
	}

	opts := options.FindOneAndUpdate().
		SetUpsert(true).
		SetReturnDocument(options.After)

	var document communityDocument

	if err := r.collection.FindOneAndUpdate(ctx, filter, update, opts).Decode(&document); err != nil {
		return core.Community{}, fmt.Errorf("upsert community metadata: %w", err)
	}

	return document.toDomain()
}

func (r *CommunityRepository) FindByGuildID(
	ctx context.Context,
	guildID string,
) (*core.Community, error) {
	if r == nil || r.collection == nil {
		return nil, errors.New("community repository is not initialized")
	}

	if ctx == nil {
		return nil, errors.New("context is required")
	}

	guildID = strings.TrimSpace(guildID)
	if guildID == "" {
		return nil, core.ErrInvalidGuildID
	}

	var document communityDocument
	err := r.collection.FindOne(ctx, bson.D{{Key: "guild_id", Value: guildID}}).Decode(&document)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find community: %w", err)
	}

	community, err := document.toDomain()
	if err != nil {
		return nil, fmt.Errorf("map community document: %w", err)
	}

	return &community, nil
}

type MemberRepository struct {
	collection *mongo.Collection
}

var _ core.MemberRepository = (*MemberRepository)(nil)

func NewMemberRepository(
	database *mongo.Database,
) (*MemberRepository, error) {
	if database == nil {
		return nil, errors.New("database is required")
	}

	return &MemberRepository{
		collection: database.Collection("members"),
	}, nil
}

func (r *MemberRepository) UpsertInteraction(
	ctx context.Context,
	input core.MemberUpsertInput,
) (core.Member, error) {
	if r == nil || r.collection == nil {
		return core.Member{}, errors.New("member repository is not initialized")
	}

	if ctx == nil {
		return core.Member{}, errors.New("context is required")
	}

	guildID := strings.TrimSpace(input.GuildID)
	userID := strings.TrimSpace(input.UserID)
	displayName := strings.TrimSpace(input.DisplayName)
	nickname := strings.TrimSpace(input.Nickname)

	if guildID == "" {
		return core.Member{}, core.ErrInvalidGuildID
	}

	if userID == "" {
		return core.Member{}, core.ErrInvalidUserID
	}

	if input.SeenAt.IsZero() {
		return core.Member{}, core.ErrInvalidTimestamp
	}

	seenAt := input.SeenAt.UTC()

	setFields := bson.D{
		{Key: "guild_id", Value: guildID},
		{Key: "user_id", Value: userID},
		{Key: "last_seen_at", Value: seenAt},
	}

	if displayName != "" {
		setFields = append(setFields, bson.E{Key: "display_name", Value: displayName})
	}

	if nickname != "" {
		setFields = append(setFields, bson.E{Key: "nickname", Value: nickname})
	}

	filter := bson.D{
		{Key: "guild_id", Value: guildID},
		{Key: "user_id", Value: userID},
	}

	update := bson.D{
		{Key: "$set", Value: setFields},
		{Key: "$inc", Value: bson.D{
			{Key: "interaction_count", Value: 1},
		}},
	}

	opts := options.FindOneAndUpdate().
		SetUpsert(true).
		SetReturnDocument(options.After)

	var document memberDocument

	if err := r.collection.FindOneAndUpdate(ctx, filter, update, opts).Decode(&document); err != nil {
		return core.Member{}, fmt.Errorf("upsert member metadata: %w", err)
	}

	return document.toDomain()
}

func (r *MemberRepository) FindByGuildAndUser(
	ctx context.Context,
	guildID string,
	userID string,
) (*core.Member, error) {
	if r == nil || r.collection == nil {
		return nil, errors.New("member repository is not initialized")
	}

	if ctx == nil {
		return nil, errors.New("context is required")
	}

	guildID = strings.TrimSpace(guildID)
	userID = strings.TrimSpace(userID)

	if guildID == "" {
		return nil, core.ErrInvalidGuildID
	}

	if userID == "" {
		return nil, core.ErrInvalidUserID
	}

	var document memberDocument
	err := r.collection.FindOne(ctx, bson.D{
		{Key: "guild_id", Value: guildID},
		{Key: "user_id", Value: userID},
	}).Decode(&document)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find member: %w", err)
	}

	member, err := document.toDomain()
	if err != nil {
		return nil, fmt.Errorf("map member document: %w", err)
	}

	return &member, nil
}
