package mongodb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"the-bebot/internal/memory/core"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	ErrCandidateNotFound      = errors.New("memory candidate not found")
	ErrCandidateAlreadyExists = errors.New("memory candidate already exists")
	ErrInvalidCandidateID     = errors.New("invalid memory candidate ID")
	ErrCandidateOwnershipLost = errors.New("memory candidate ownership lost")
)

type CandidateRepository struct {
	collection *mongo.Collection
}

var _ core.CandidateRepository = (*CandidateRepository)(nil)

func NewCandidateRepository(database *mongo.Database) (*CandidateRepository, error) {
	if database == nil {
		return nil, fmt.Errorf("database is required")
	}

	return &CandidateRepository{
		collection: database.Collection("memory_candidates"),
	}, nil
}

func (r *CandidateRepository) Create(
	ctx context.Context,
	candidate core.MemoryCandidate,
) error {
	if r == nil || r.collection == nil {
		return fmt.Errorf("candidate repository is not initialized")
	}

	if ctx == nil {
		return fmt.Errorf("context is required")
	}

	document, err := toCandidateDocument(candidate)
	if err != nil {
		return fmt.Errorf("convert candidate document: %w", err)
	}

	_, err = r.collection.InsertOne(ctx, document)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return fmt.Errorf("%w: %v", ErrCandidateAlreadyExists, err)
		}

		return fmt.Errorf("create memory candidate: %w", err)
	}

	return nil
}

func (r *CandidateRepository) ClaimNext(
	ctx context.Context,
	workerID string,
	now time.Time,
	lockDuration time.Duration,
) (*core.MemoryCandidate, error) {
	if r == nil || r.collection == nil {
		return nil, fmt.Errorf("candidate repository is not initialized")
	}

	if ctx == nil {
		return nil, fmt.Errorf("context is required")
	}

	if workerID == "" {
		return nil, fmt.Errorf("worker ID is required")
	}

	if now.IsZero() {
		return nil, fmt.Errorf("claim time is required")
	}

	if lockDuration <= 0 {
		return nil, fmt.Errorf("lock duration must be positive")
	}

	lockUntil := now.Add(lockDuration)

	filter := bson.D{
		{Key: "expire_at", Value: bson.D{
			{Key: "$gt", Value: now},
		}},
		{Key: "$or", Value: bson.A{
			bson.D{
				{Key: "status", Value: string(core.CandidateStatusPending)},
				{Key: "$or", Value: bson.A{
					bson.D{
						{Key: "next_retry_at", Value: bson.D{
							{Key: "$exists", Value: false},
						}},
					},
					bson.D{
						{Key: "next_retry_at", Value: bson.D{
							{Key: "$lte", Value: now},
						}},
					},
				}},
			},
			bson.D{
				{Key: "status", Value: string(core.CandidateStatusFailed)},
				{Key: "next_retry_at", Value: bson.D{
					{Key: "$lte", Value: now},
				}},
			},
			bson.D{
				{Key: "status", Value: string(core.CandidateStatusProcessing)},
				{Key: "locked_until", Value: bson.D{
					{Key: "$lte", Value: now},
				}},
			},
		}},
	}

	update := bson.D{
		{Key: "$set", Value: bson.D{
			{Key: "status", Value: string(core.CandidateStatusProcessing)},
			{Key: "worker_id", Value: workerID},
			{Key: "locked_until", Value: lockUntil},
			{Key: "updated_at", Value: now},
		}},
		{Key: "$inc", Value: bson.D{
			{Key: "attempts", Value: 1},
		}},
		{Key: "$unset", Value: bson.D{
			{Key: "next_retry_at", Value: ""},
		}},
	}

	opts := options.FindOneAndUpdate().
		SetSort(bson.D{
			{Key: "created_at", Value: 1},
		}).
		SetReturnDocument(options.After)

	var document candidateDocument

	err := r.collection.FindOneAndUpdate(
		ctx,
		filter,
		update,
		opts,
	).Decode(&document)

	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("claim memory candidate: %w", err)
	}

	candidate, err := document.toDomain()
	if err != nil {
		return nil, fmt.Errorf("convert claimed candidate: %w", err)
	}

	return &candidate, nil
}

func (r *CandidateRepository) Update(
	ctx context.Context,
	candidate core.MemoryCandidate,
) error {
	if r == nil || r.collection == nil {
		return fmt.Errorf("candidate repository is not initialized")
	}

	if ctx == nil {
		return fmt.Errorf("context is required")
	}

	document, err := toCandidateDocument(candidate)
	if err != nil {
		return fmt.Errorf("convert candidate document: %w", err)
	}

	result, err := r.collection.ReplaceOne(
		ctx,
		bson.D{{Key: "_id", Value: document.ID}},
		document,
	)
	if err != nil {
		return fmt.Errorf("update memory candidate: %w", err)
	}

	if result.MatchedCount == 0 {
		return ErrCandidateNotFound
	}

	return nil
}

func (r *CandidateRepository) DeleteExpired(
	ctx context.Context,
	before time.Time,
) error {
	if r == nil || r.collection == nil {
		return fmt.Errorf("candidate repository is not initialized")
	}

	if ctx == nil {
		return fmt.Errorf("context is required")
	}

	if before.IsZero() {
		return fmt.Errorf("expiration time is required")
	}

	_, err := r.collection.DeleteMany(
		ctx,
		bson.D{
			{Key: "expire_at", Value: bson.D{
				{Key: "$lte", Value: before},
			}},
		},
	)
	if err != nil {
		return fmt.Errorf("delete expired memory candidates: %w", err)
	}

	return nil
}

func (r *CandidateRepository) UpdateProcessingResult(
	ctx context.Context,
	candidate core.MemoryCandidate,
	expectedWorkerID string,
) error {
	if r == nil || r.collection == nil {
		return fmt.Errorf("candidate repository is not initialized")
	}

	if ctx == nil {
		return fmt.Errorf("context is required")
	}

	if expectedWorkerID == "" {
		return fmt.Errorf("expected worker ID is required")
	}

	document, err := toCandidateDocument(candidate)
	if err != nil {
		return fmt.Errorf("convert candidate document: %w", err)
	}

	now := time.Now().UTC()
	filter := bson.D{
		{Key: "_id", Value: document.ID},
		{Key: "status", Value: string(core.CandidateStatusProcessing)},
		{Key: "worker_id", Value: expectedWorkerID},
		{Key: "locked_until", Value: bson.D{
			{Key: "$gt", Value: now},
		}},
	}

	result, err := r.collection.ReplaceOne(ctx, filter, document)
	if err != nil {
		return fmt.Errorf("update processing result: %w", err)
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf(
			"%w: candidate %s is no longer owned by worker %s",
			ErrCandidateOwnershipLost,
			candidate.ID,
			expectedWorkerID,
		)
	}

	return nil
}
