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
	document.ContentDedupKey = newMemoryContentDedupKey(validated)

	// 1. Preserve candidate-level idempotency so retries of the same
	// (guild_id, candidate_id, candidate_index) never create a second memory,
	// even if the LLM rewords the extracted text.
	if document.CandidateID != "" && document.CandidateIndex != nil {
		candidateFilter := bson.M{
			"guild_id":        document.GuildID,
			"candidate_id":    document.CandidateID,
			"candidate_index": *document.CandidateIndex,
		}

		var existing memoryDocument
		err := r.collection.FindOne(ctx, candidateFilter).Decode(&existing)
		if err == nil {
			return existing.ID.Hex(), nil
		}
		if !errors.Is(err, mongo.ErrNoDocuments) {
			return "", fmt.Errorf("check existing candidate memory: %w", err)
		}
	}

	// 2. Content-based deduplication upsert per guild + scope + type + user (member scope)
	// + normalized content. On a duplicate, advance importance and updated_at to max.
	dedupFilter := bson.M{
		"guild_id":          document.GuildID,
		"content_dedup_key": document.ContentDedupKey,
	}

	setOnInsert := bson.M{
		"_id":               document.ID,
		"guild_id":          document.GuildID,
		"scope":             document.Scope,
		"type":              document.Type,
		"content":           document.Content,
		"confidence":        document.Confidence,
		"tags":              document.Tags,
		"source_message_id": document.SourceMessageID,
		"usage_count":       document.UsageCount,
		"idempotency_key":   document.IdempotencyKey,
		"content_dedup_key": document.ContentDedupKey,
		"last_used_at":      document.LastUsedAt,
		"created_at":        document.CreatedAt,
	}
	if document.UserID != "" {
		setOnInsert["user_id"] = document.UserID
	}
	if document.CandidateID != "" {
		setOnInsert["candidate_id"] = document.CandidateID
	}
	if document.CandidateIndex != nil {
		setOnInsert["candidate_index"] = *document.CandidateIndex
	}

	maxUpdate := bson.M{
		"importance": document.Importance,
		"updated_at": document.UpdatedAt,
	}

	update := bson.M{
		"$setOnInsert": setOnInsert,
		"$max":         maxUpdate,
	}

	_, err = r.collection.UpdateOne(
		ctx,
		dedupFilter,
		update,
		options.Update().SetUpsert(true),
	)
	if err != nil {
		if !mongo.IsDuplicateKeyError(err) {
			return "", fmt.Errorf("upsert memory: %w", err)
		}

		// Handle concurrent race on candidate_item index.
		if document.CandidateID != "" && document.CandidateIndex != nil {
			var existing memoryDocument
			if findErr := r.collection.FindOne(ctx, bson.M{
				"guild_id":        document.GuildID,
				"candidate_id":    document.CandidateID,
				"candidate_index": *document.CandidateIndex,
			}).Decode(&existing); findErr == nil {
				return existing.ID.Hex(), nil
			}
		}

		// Handle concurrent race on content_dedup_key by applying the $max update.
		res, retryErr := r.collection.UpdateOne(
			ctx,
			dedupFilter,
			bson.M{"$max": maxUpdate},
		)
		if retryErr != nil {
			return "", fmt.Errorf("update duplicate memory: %w", retryErr)
		}

		// Handle legacy pre-migration documents matched only by idempotency_key.
		if res.MatchedCount == 0 {
			legacyFilter := bson.M{"idempotency_key": document.IdempotencyKey}
			var legacy memoryDocument
			if legacyErr := r.collection.FindOne(ctx, legacyFilter).Decode(&legacy); legacyErr == nil {
				_, _ = r.collection.UpdateOne(ctx, legacyFilter, bson.M{
					"$set": bson.M{"content_dedup_key": document.ContentDedupKey},
					"$max": maxUpdate,
				})
				return legacy.ID.Hex(), nil
			}
		}
	}

	var saved memoryDocument
	if err := r.collection.FindOne(ctx, dedupFilter).Decode(&saved); err != nil {
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
