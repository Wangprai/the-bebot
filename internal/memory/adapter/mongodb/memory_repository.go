package mongodb

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
	collection             *mongo.Collection
	unusedTTL              time.Duration
	unusedTTLMaxImportance int
	clock                  func() time.Time
}

var _ core.MemoryRepository = (*MemoryRepository)(nil)

func NewMemoryRepository(database *mongo.Database) (*MemoryRepository, error) {
	if database == nil {
		return nil, errors.New("database is required")
	}

	return &MemoryRepository{
		collection:             database.Collection("memories"),
		unusedTTLMaxImportance: core.DefaultUnusedTTLMaxImportance,
		clock:                  time.Now,
	}, nil
}

// SetUnusedTTL configures optional expiration for lower-importance memories
// that are never used. Passing ttl == 0 disables expiration (default off-safe).
func (r *MemoryRepository) SetUnusedTTL(ttl time.Duration, maxImportance int) error {
	if r == nil {
		return errors.New("memory repository is not initialized")
	}
	if ttl < 0 {
		return errors.New("unused TTL must be zero or greater")
	}
	if maxImportance < 1 || maxImportance > 5 {
		return errors.New("unused TTL max importance must be between 1 and 5")
	}

	r.unusedTTL = ttl
	r.unusedTTLMaxImportance = maxImportance
	return nil
}

func (r *MemoryRepository) now() time.Time {
	if r != nil && r.clock != nil {
		return r.clock().UTC()
	}
	return time.Now().UTC()
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

	if r.unusedTTL > 0 &&
		validated.Importance <= r.unusedTTLMaxImportance &&
		validated.UsageCount == 0 &&
		document.ExpiresAt == nil {
		expiresAt := document.UpdatedAt.UTC().Add(r.unusedTTL)
		document.ExpiresAt = &expiresAt
	}

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
	// + normalized content. On a duplicate, advance importance, confidence, and updated_at
	// to max and accumulate distinct source_user_ids for corroboration.
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
	if document.ExpiresAt != nil {
		setOnInsert["expires_at"] = document.ExpiresAt
	}

	maxUpdate := bson.M{
		"importance": document.Importance,
		"confidence": document.Confidence,
		"updated_at": document.UpdatedAt,
	}

	update := bson.M{
		"$setOnInsert": setOnInsert,
		"$max":         maxUpdate,
	}
	if len(document.SourceUserIDs) > 0 {
		update["$addToSet"] = bson.M{
			"source_user_ids": bson.M{"$each": document.SourceUserIDs},
		}
	}
	if document.ExpiresAt == nil && r.unusedTTL > 0 && validated.Importance > r.unusedTTLMaxImportance {
		update["$unset"] = bson.M{"expires_at": ""}
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

		// Handle concurrent race on content_dedup_key by applying the $max/$addToSet update.
		retryUpdate := bson.M{"$max": maxUpdate}
		if len(document.SourceUserIDs) > 0 {
			retryUpdate["$addToSet"] = bson.M{
				"source_user_ids": bson.M{"$each": document.SourceUserIDs},
			}
		}
		if document.ExpiresAt == nil && r.unusedTTL > 0 && validated.Importance > r.unusedTTLMaxImportance {
			retryUpdate["$unset"] = bson.M{"expires_at": ""}
		}
		res, retryErr := r.collection.UpdateOne(ctx, dedupFilter, retryUpdate)
		if retryErr != nil {
			return "", fmt.Errorf("update duplicate memory: %w", retryErr)
		}

		// Handle legacy pre-migration documents matched only by idempotency_key.
		if res.MatchedCount == 0 {
			legacyFilter := bson.M{"idempotency_key": document.IdempotencyKey}
			var legacy memoryDocument
			if legacyErr := r.collection.FindOne(ctx, legacyFilter).Decode(&legacy); legacyErr == nil {
				legacyUpdate := bson.M{
					"$set": bson.M{"content_dedup_key": document.ContentDedupKey},
					"$max": maxUpdate,
				}
				if len(document.SourceUserIDs) > 0 {
					legacyUpdate["$addToSet"] = bson.M{
						"source_user_ids": bson.M{"$each": document.SourceUserIDs},
					}
				}
				_, _ = r.collection.UpdateOne(ctx, legacyFilter, legacyUpdate)
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
	return r.FindRelevantByQuery(ctx, core.RetrievalQuery{
		GuildID:                   guildID,
		UserID:                    userID,
		Limit:                     limit,
		MinConfidence:             core.DefaultMinConfidence,
		MinCommunityCorroborators: core.DefaultMinCommunityCorroborators,
		Now:                       r.now(),
	})
}

func (r *MemoryRepository) FindRelevantByQuery(
	ctx context.Context,
	query core.RetrievalQuery,
) ([]core.Memory, error) {
	if r == nil || r.collection == nil {
		return nil, errors.New("memory repository is not initialized")
	}

	if ctx == nil {
		return nil, errors.New("context is required")
	}

	if query.Limit <= 0 {
		return nil, errors.New("limit must be greater than zero")
	}

	now := query.Now
	if now.IsZero() {
		now = r.now()
	}
	query.Now = now

	poolLimit := query.Limit * 4
	if poolLimit < 20 {
		poolLimit = 20
	}
	if poolLimit > 100 {
		poolLimit = 100
	}

	baseAnd := buildBaseRetrievalFilters(query, now)

	candidatesByID := make(map[string]core.ScoredMemory, poolLimit)

	// 1. Query MongoDB text index when a search query is present.
	cleanedQuery := core.CleanRetrievalQuery(query.QueryText)
	if cleanedQuery != "" {
		textFilter := bson.M{
			"$text": bson.M{"$search": cleanedQuery},
			"$and":  baseAnd,
		}
		textOpts := options.Find().
			SetProjection(bson.M{"text_score": bson.M{"$meta": "textScore"}}).
			SetSort(bson.D{
				{Key: "text_score", Value: bson.M{"$meta": "textScore"}},
				{Key: "importance", Value: -1},
				{Key: "updated_at", Value: -1},
			}).
			SetLimit(int64(poolLimit))

		textCursor, err := r.collection.Find(ctx, textFilter, textOpts)
		if err != nil {
			if !isTextIndexMissingError(err) {
				return nil, fmt.Errorf("find text-relevant memories: %w", err)
			}
		} else {
			defer textCursor.Close(ctx)
			for textCursor.Next(ctx) {
				var scoredDoc memoryDocument
				if err := textCursor.Decode(&scoredDoc); err != nil {
					return nil, fmt.Errorf("decode scored memory: %w", err)
				}
				mem, err := scoredDoc.toDomain()
				if err != nil {
					return nil, fmt.Errorf("map memory document: %w", err)
				}
				candidatesByID[mem.ID] = core.ScoredMemory{
					Memory:    mem,
					TextScore: scoredDoc.TextScore,
				}
			}
			if err := textCursor.Err(); err != nil {
				return nil, fmt.Errorf("iterate text-relevant memories: %w", err)
			}
		}
	}

	// 2. Also fetch importance/recency fallback candidates (covers queries with no
	// MongoDB $text match and unspaced Thai queries ranked via lexical overlap).
	if len(candidatesByID) < poolLimit {
		fallbackFilter := bson.M{"$and": baseAnd}
		fallbackOpts := options.Find().
			SetSort(bson.D{
				{Key: "importance", Value: -1},
				{Key: "confidence", Value: -1},
				{Key: "updated_at", Value: -1},
			}).
			SetLimit(int64(poolLimit))

		cursor, err := r.collection.Find(ctx, fallbackFilter, fallbackOpts)
		if err != nil {
			return nil, fmt.Errorf("find relevant memories: %w", err)
		}
		defer cursor.Close(ctx)

		for cursor.Next(ctx) {
			var document memoryDocument
			if err := cursor.Decode(&document); err != nil {
				return nil, fmt.Errorf("decode memory: %w", err)
			}
			mem, err := document.toDomain()
			if err != nil {
				return nil, fmt.Errorf("map memory document: %w", err)
			}
			if _, exists := candidatesByID[mem.ID]; !exists {
				candidatesByID[mem.ID] = core.ScoredMemory{Memory: mem}
			}
		}
		if err := cursor.Err(); err != nil {
			return nil, fmt.Errorf("iterate memories: %w", err)
		}
	}

	pool := make([]core.ScoredMemory, 0, len(candidatesByID))
	for _, candidate := range candidatesByID {
		pool = append(pool, candidate)
	}

	return core.RankMemories(pool, query), nil
}

func buildBaseRetrievalFilters(query core.RetrievalQuery, now time.Time) []bson.M {
	communityFilter := bson.M{"scope": core.MemoryScopeCommunity}
	if query.MinCommunityCorroborators > 1 {
		corroborationOr := []bson.M{
			{
				"$expr": bson.M{
					"$gte": bson.A{
						bson.M{"$size": bson.M{"$ifNull": bson.A{"$source_user_ids", bson.A{}}}},
						query.MinCommunityCorroborators,
					},
				},
			},
		}
		if query.CommunityHighConfidence > 0 {
			corroborationOr = append(corroborationOr, bson.M{
				"confidence": bson.M{"$gte": query.CommunityHighConfidence},
			})
		}
		communityFilter["$or"] = corroborationOr
	}

	clauses := []bson.M{
		{"guild_id": query.GuildID},
		{
			"$or": []bson.M{
				communityFilter,
				{
					"scope":   core.MemoryScopeMember,
					"user_id": query.UserID,
				},
			},
		},
		{
			"$or": []bson.M{
				{"expires_at": bson.M{"$exists": false}},
				{"expires_at": nil},
				{"expires_at": bson.M{"$gt": now}},
			},
		},
	}

	if query.MinConfidence > 0 {
		clauses = append(clauses, bson.M{
			"confidence": bson.M{"$gte": query.MinConfidence},
		})
	}

	return clauses
}

func isTextIndexMissingError(err error) bool {
	if err == nil {
		return false
	}
	var cmdErr mongo.CommandError
	if errors.As(err, &cmdErr) && cmdErr.Code == 27 {
		return true
	}
	return strings.Contains(err.Error(), "text index required")
}

// DeleteExpiredMemories removes memories whose expires_at is at or before before.
func (r *MemoryRepository) DeleteExpiredMemories(
	ctx context.Context,
	before time.Time,
) (int64, error) {
	if r == nil || r.collection == nil {
		return 0, errors.New("memory repository is not initialized")
	}
	if ctx == nil {
		return 0, errors.New("context is required")
	}
	if before.IsZero() {
		return 0, errors.New("expiration time is required")
	}

	res, err := r.collection.DeleteMany(ctx, bson.M{
		"expires_at": bson.M{
			"$type": "date",
			"$lte":  before.UTC(),
		},
	})
	if err != nil {
		return 0, fmt.Errorf("delete expired memories: %w", err)
	}

	return res.DeletedCount, nil
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
		"$unset": bson.M{
			"expires_at": "",
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
		"$unset": bson.M{
			"expires_at": "",
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
