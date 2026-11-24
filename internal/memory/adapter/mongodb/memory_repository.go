package mongodb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"the-bebot/internal/memory/core"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	ErrMemoryNotFound  = errors.New("memory not found")
	ErrInvalidMemoryID = errors.New("invalid memory ID")
)

type MemoryRepository struct {
	collection *mongo.Collection
}

var _ core.MemoryRepository = (*MemoryRepository)(nil)

func NewMemoryRepository(database *mongo.Database) (*MemoryRepository, error) {
	if database == nil {
		return nil, errors.New("database is required")
	}

	return &MemoryRepository{
		collection: database.Collection("memories"),
	}, nil
}

func (r *MemoryRepository) Save(
	ctx context.Context,
	memory core.Memory,
) (string, error) {
	if r == nil || r.collection == nil {
		return "", errors.New("memory repository is not initialized")
	}

	if ctx == nil {
		return "", errors.New("context is required")
	}

	validated, err := core.NewMemory(memory)
	if err != nil {
		return "", fmt.Errorf("validate memory: %w", err)
	}

	document := toMemoryDocument(validated)
	document.ID = primitive.NewObjectID()
	document.IdempotencyKey = newMemoryIdempotencyKey(validated)

	filter := bson.M{
		"idempotency_key": document.IdempotencyKey,
	}
	if document.CandidateID != "" && document.CandidateIndex != nil {
		filter = bson.M{
			"guild_id":        document.GuildID,
			"candidate_id":    document.CandidateID,
			"candidate_index": *document.CandidateIndex,
		}
	}

	update := bson.M{
		"$setOnInsert": document,
	}

	_, err = r.collection.UpdateOne(
		ctx,
		filter,
		update,
		options.Update().SetUpsert(true),
	)

	if err != nil && !mongo.IsDuplicateKeyError(err) {
		return "", fmt.Errorf("upsert memory: %w", err)
	}

	var saved memoryDocument

	if err := r.collection.FindOne(ctx, filter).Decode(&saved); err != nil {
		return "", fmt.Errorf("find saved memory: %w", err)
	}

	return saved.ID.Hex(), nil
}

func (r *MemoryRepository) FindByID(
	ctx context.Context,
	guildID string,
	memoryID string,
) (*core.Memory, error) {
	if r == nil || r.collection == nil {
		return nil, errors.New("memory repository is not initialized")
	}

	if ctx == nil {
		return nil, errors.New("context is required")
	}

	objectID, err := primitive.ObjectIDFromHex(memoryID)
	if err != nil {
		return nil, ErrInvalidMemoryID
	}

	filter := bson.M{
		"_id":      objectID,
		"guild_id": guildID,
	}

	var document memoryDocument

	err = r.collection.FindOne(ctx, filter).Decode(&document)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, ErrMemoryNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find memory: %w", err)
	}

	memory, err := document.toDomain()
	if err != nil {
		return nil, fmt.Errorf("map memory document: %w", err)
	}

	return &memory, nil
}

func (r *MemoryRepository) FindRelevant(
	ctx context.Context,
	guildID string,
	userID string,
	limit int,
) ([]core.Memory, error) {
	if r == nil || r.collection == nil {
		return nil, errors.New("memory repository is not initialized")
	}

	if ctx == nil {
		return nil, errors.New("context is required")
	}

	if limit <= 0 {
		return nil, errors.New("limit must be greater than zero")
	}

	filter := bson.M{
		"guild_id": guildID,
		"$or": []bson.M{
			{
				"scope": core.MemoryScopeCommunity,
			},
			{
				"scope":   core.MemoryScopeMember,
				"user_id": userID,
			},
		},
	}

	findOptions := options.Find().
		SetSort(bson.D{
			{Key: "importance", Value: -1},
			{Key: "updated_at", Value: -1},
		}).
		SetLimit(int64(limit))

	cursor, err := r.collection.Find(ctx, filter, findOptions)
	if err != nil {
		return nil, fmt.Errorf("find relevant memories: %w", err)
	}
	defer cursor.Close(ctx)

	memories := make([]core.Memory, 0)

	for cursor.Next(ctx) {
		var document memoryDocument

		if err := cursor.Decode(&document); err != nil {
			return nil, fmt.Errorf("decode memory: %w", err)
		}

		memory, err := document.toDomain()
		if err != nil {
			return nil, fmt.Errorf("map memory document: %w", err)
		}

		memories = append(memories, memory)
	}

	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("iterate memories: %w", err)
	}

	return memories, nil
}

func (r *MemoryRepository) UpdateUsage(
	ctx context.Context,
	guildID string,
	memoryID string,
	usedAt time.Time,
) error {
	if r == nil || r.collection == nil {
		return errors.New("memory repository is not initialized")
	}

	if ctx == nil {
		return errors.New("context is required")
	}

	objectID, err := primitive.ObjectIDFromHex(memoryID)
	if err != nil {
		return ErrInvalidMemoryID
	}

	filter := bson.M{
		"_id":      objectID,
		"guild_id": guildID,
	}

	update := bson.M{
		"$inc": bson.M{
			"usage_count": 1,
		},
		"$set": bson.M{
			"last_used_at": usedAt,
		},
	}

	result, err := r.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("update memory usage: %w", err)
	}

	if result.MatchedCount == 0 {
		return ErrMemoryNotFound
	}

	return nil
}

func (r *MemoryRepository) UpdateUsageMany(
	ctx context.Context,
	guildID string,
	memoryIDs []string,
	usedAt time.Time,
) error {
	if r == nil || r.collection == nil {
		return errors.New("memory repository is not initialized")
	}

	if ctx == nil {
		return errors.New("context is required")
	}

	if len(memoryIDs) == 0 {
		return nil
	}

	objectIDs := make([]primitive.ObjectID, 0, len(memoryIDs))
	for _, memoryID := range memoryIDs {
		objectID, err := primitive.ObjectIDFromHex(memoryID)
		if err != nil {
			return ErrInvalidMemoryID
		}
		objectIDs = append(objectIDs, objectID)
	}

	filter := bson.M{
		"guild_id": guildID,
		"_id": bson.M{
			"$in": objectIDs,
		},
	}

	update := bson.M{
		"$inc": bson.M{
			"usage_count": 1,
		},
		"$set": bson.M{
			"last_used_at": usedAt,
		},
	}

	if _, err := r.collection.UpdateMany(ctx, filter, update); err != nil {
		return fmt.Errorf("bulk update memory usage: %w", err)
	}

	return nil
}

func (r *MemoryRepository) DeleteByID(
	ctx context.Context,
	guildID string,
	memoryID string,
) error {
	if r == nil || r.collection == nil {
		return errors.New("memory repository is not initialized")
	}

	if ctx == nil {
		return errors.New("context is required")
	}

	objectID, err := primitive.ObjectIDFromHex(memoryID)
	if err != nil {
		return ErrInvalidMemoryID
	}

	result, err := r.collection.DeleteOne(ctx, bson.M{
		"_id":      objectID,
		"guild_id": guildID,
	})
	if err != nil {
		return fmt.Errorf("delete memory: %w", err)
	}

	if result.DeletedCount == 0 {
		return ErrMemoryNotFound
	}

	return nil
}
