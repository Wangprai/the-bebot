package mongodb

import (
	"context"
	"errors"
	"testing"
	"time"

	"the-bebot/internal/memory/core"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

func setupCandidateRepository(t *testing.T) *CandidateRepository {
	t.Helper()

	memoryRepository := setupMemoryRepository(t)
	database := memoryRepository.collection.Database()

	repository, err := NewCandidateRepository(database)
	if err != nil {
		t.Fatalf("create candidate repository failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := repository.EnsureIndexes(ctx); err != nil {
		t.Fatalf("create candidate indexes failed: %v", err)
	}

	return repository
}

func newTestCandidate(
	t *testing.T,
	guildID string,
	channelID string,
	messageID string,
	userID string,
	expireAt time.Time,
) core.MemoryCandidate {
	t.Helper()

	now := time.Now().UTC()

	candidate, err := core.NewMemoryCandidate(core.MemoryCandidate{
		ID:        primitive.NewObjectID().Hex(),
		GuildID:   guildID,
		ChannelID: channelID,
		MessageID: messageID,
		UserID:    userID,
		Content:   "User likes Go programming",
		Status:    core.CandidateStatusPending,

		Messages: []core.ConversationMessage{
			{
				MessageID: messageID,
				UserID:    userID,
				Content:   "User likes Go programming",
			},
		},

		ExpireAt:  expireAt,
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("create test candidate failed: %v", err)
	}

	return candidate
}

func TestCandidateRepositoryCreateAndClaim(t *testing.T) {
	repository := setupCandidateRepository(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	now := time.Now().UTC().Truncate(time.Millisecond)
	expireAt := now.Add(time.Hour)

	candidate := newTestCandidate(
		t,
		"guild-123",
		"channel-123",
		"message-123",
		"user-123",
		expireAt,
	)

	// Create.
	if err := repository.Create(ctx, candidate); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Duplicate protection.
	err := repository.Create(ctx, candidate)
	if !errors.Is(err, ErrCandidateAlreadyExists) {
		t.Fatalf("expected ErrCandidateAlreadyExists, got %v", err)
	}

	// First worker claims the candidate.
	claimed, err := repository.ClaimNext(
		ctx,
		"worker-1",
		now,
		5*time.Minute,
	)
	if err != nil {
		t.Fatalf("ClaimNext failed: %v", err)
	}

	if claimed == nil {
		t.Fatal("expected a candidate to be claimed")
	}

	if claimed.Status != core.CandidateStatusProcessing {
		t.Errorf("expected processing status, got %q", claimed.Status)
	}

	if claimed.WorkerID != "worker-1" {
		t.Errorf("expected worker-1, got %q", claimed.WorkerID)
	}

	if claimed.Attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", claimed.Attempts)
	}

	if claimed.LockedUntil == nil {
		t.Fatal("expected LockedUntil to be set")
	}

	expectedLock := now.Add(5 * time.Minute)
	if !claimed.LockedUntil.Equal(expectedLock) {
		t.Errorf(
			"expected lock until %v, got %v",
			expectedLock,
			*claimed.LockedUntil,
		)
	}

	// Verify that the conversation snapshot was persisted correctly.
	if len(claimed.Messages) != len(candidate.Messages) {
		t.Fatalf(
			"expected %d messages, got %d",
			len(candidate.Messages),
			len(claimed.Messages),
		)
	}

	for i, message := range candidate.Messages {
		got := claimed.Messages[i]

		if got.MessageID != message.MessageID {
			t.Errorf(
				"message[%d] ID mismatch: expected %s, got %s",
				i, message.MessageID, got.MessageID,
			)
		}

		if got.UserID != message.UserID {
			t.Errorf(
				"message[%d] UserID mismatch: expected %s, got %s",
				i, message.UserID, got.UserID,
			)
		}

		if got.Content != message.Content {
			t.Errorf(
				"message[%d] Content mismatch: expected %s, got %s",
				i, message.Content, got.Content,
			)
		}

		if got.IsBot != message.IsBot {
			t.Errorf(
				"message[%d] IsBot mismatch: expected %v, got %v",
				i, message.IsBot, got.IsBot,
			)
		}
	}

	// Another worker must not claim an active lock.
	secondClaim, err := repository.ClaimNext(
		ctx,
		"worker-2",
		now.Add(time.Minute),
		5*time.Minute,
	)
	if err != nil {
		t.Fatalf("second ClaimNext failed: %v", err)
	}

	if secondClaim != nil {
		t.Fatal("another worker claimed a candidate with an active lock")
	}

	// Simulate an expired lock.
	expiredLock := now.Add(-time.Minute)
	claimed.LockedUntil = &expiredLock
	claimed.UpdatedAt = now.Add(2 * time.Minute)

	if err := repository.Update(ctx, *claimed); err != nil {
		t.Fatalf("Update expired lock failed: %v", err)
	}

	// Another worker can now recover and claim it.
	recovered, err := repository.ClaimNext(
		ctx,
		"worker-2",
		now.Add(3*time.Minute),
		5*time.Minute,
	)
	if err != nil {
		t.Fatalf("claim after lock expiration failed: %v", err)
	}

	if recovered == nil {
		t.Fatal("expected expired candidate to be claimed again")
	}

	if recovered.WorkerID != "worker-2" {
		t.Errorf("expected worker-2, got %q", recovered.WorkerID)
	}

	if recovered.Attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", recovered.Attempts)
	}
}

func TestCandidateRepositoryDeleteExpired(t *testing.T) {
	repository := setupCandidateRepository(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	now := time.Now().UTC()

	expiredCandidate := newTestCandidate(
		t,
		"guild-123",
		"channel-123",
		"message-expired",
		"user-123",
		now.Add(-time.Hour),
	)

	activeCandidate := newTestCandidate(
		t,
		"guild-123",
		"channel-123",
		"message-active",
		"user-123",
		now.Add(time.Hour),
	)

	if err := repository.Create(ctx, expiredCandidate); err != nil {
		t.Fatalf("create expired candidate failed: %v", err)
	}

	if err := repository.Create(ctx, activeCandidate); err != nil {
		t.Fatalf("create active candidate failed: %v", err)
	}

	if err := repository.DeleteExpired(ctx, now); err != nil {
		t.Fatalf("DeleteExpired failed: %v", err)
	}

	// Expired candidate must no longer be claimable.
	claimed, err := repository.ClaimNext(
		ctx,
		"worker-1",
		now,
		5*time.Minute,
	)
	if err != nil {
		t.Fatalf("ClaimNext after expiration failed: %v", err)
	}

	if claimed == nil {
		t.Fatal("expected active candidate to remain")
	}

	if claimed.MessageID != activeCandidate.MessageID {
		t.Errorf(
			"expected active candidate %q, got %q",
			activeCandidate.MessageID,
			claimed.MessageID,
		)
	}
}

func TestCandidateRepositoryClaimFailedCandidateAfterRetry(t *testing.T) {
	repository := setupCandidateRepository(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	now := time.Now().UTC().Truncate(time.Millisecond)

	candidate := newTestCandidate(
		t,
		"guild-retry",
		"channel-retry",
		"message-retry",
		"user-retry",
		now.Add(time.Hour),
	)

	if err := repository.Create(ctx, candidate); err != nil {
		t.Fatalf("create candidate: %v", err)
	}

	claimed, err := repository.ClaimNext(
		ctx,
		"worker-1",
		now,
		5*time.Minute,
	)
	if err != nil {
		t.Fatalf("claim candidate: %v", err)
	}

	if claimed == nil {
		t.Fatal("expected candidate to be claimed")
	}

	retryAt := now.Add(time.Minute)

	candidateService := core.NewCandidateService()

	failed, err := candidateService.Fail(
		*claimed,
		now,
		retryAt,
	)
	if err != nil {
		t.Fatalf("fail candidate: %v", err)
	}

	if err := repository.Update(ctx, failed); err != nil {
		t.Fatalf("update failed candidate: %v", err)
	}

	beforeRetry, err := repository.ClaimNext(
		ctx,
		"worker-2",
		retryAt.Add(-time.Second),
		5*time.Minute,
	)
	if err != nil {
		t.Fatalf("claim before retry: %v", err)
	}

	if beforeRetry != nil {
		t.Fatal("expected no candidate before retry time")
	}

	retried, err := repository.ClaimNext(
		ctx,
		"worker-2",
		retryAt,
		5*time.Minute,
	)
	if err != nil {
		t.Fatalf("claim after retry: %v", err)
	}

	if retried == nil {
		t.Fatal("expected failed candidate to be claimed after retry time")
	}

	if retried.Status != core.CandidateStatusProcessing {
		t.Fatalf("expected processing status, got %s", retried.Status)
	}

	if retried.WorkerID != "worker-2" {
		t.Fatalf("expected worker-2, got %s", retried.WorkerID)
	}

	if retried.Attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", retried.Attempts)
	}
}


func TestCandidateRepositoryUpdateProcessingResult(t *testing.T) {
	repository := setupCandidateRepository(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	now := time.Now().UTC().Truncate(time.Millisecond)
	candidate := newTestCandidate(
		t,
		"guild-update",
		"channel-update",
		"message-update",
		"user-update",
		now.Add(time.Hour),
	)

	if err := repository.Create(ctx, candidate); err != nil {
		t.Fatalf("create candidate: %v", err)
	}

	claimed, err := repository.ClaimNext(
		ctx,
		"worker-1",
		now,
		5*time.Minute,
	)
	if err != nil {
		t.Fatalf("claim candidate: %v", err)
	}

	if claimed == nil {
		t.Fatal("expected candidate to be claimed")
	}

	candidateService := core.NewCandidateService()

	// Case 1: The current owner can save the processing result.
	completed, err := candidateService.Complete(*claimed, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("complete candidate: %v", err)
	}

	if err := repository.UpdateProcessingResult(
		ctx,
		completed,
		"worker-1",
	); err != nil {
		t.Fatalf("expected current worker to update candidate: %v", err)
	}

	// Case 2: A different worker cannot update the candidate.
	err = repository.UpdateProcessingResult(
		ctx,
		completed,
		"worker-2",
	)
	if !errors.Is(err, ErrCandidateOwnershipLost) {
		t.Fatalf("expected ErrCandidateOwnershipLost, got %v", err)
	}

	// Case 3: A candidate that is not processing cannot be updated.
	pendingCandidate := newTestCandidate(
		t,
		"guild-update",
		"channel-update",
		"message-pending",
		"user-update",
		now.Add(time.Hour),
	)

	if err := repository.Create(ctx, pendingCandidate); err != nil {
		t.Fatalf("create pending candidate: %v", err)
	}

	err = repository.UpdateProcessingResult(
		ctx,
		pendingCandidate,
		"worker-1",
	)
	if !errors.Is(err, ErrCandidateOwnershipLost) {
		t.Fatalf("expected ErrCandidateOwnershipLost for pending candidate, got %v", err)
	}

	// Case 4: An expired lock cannot be updated.
	expiringCandidate := newTestCandidate(
		t,
		"guild-update",
		"channel-update",
		"message-expired-lock",
		"user-update",
		now.Add(time.Hour),
	)

	if err := repository.Create(ctx, expiringCandidate); err != nil {
		t.Fatalf("create expiring candidate: %v", err)
	}

	expiringClaim, err := repository.ClaimNext(
		ctx,
		"worker-3",
		now,
		5*time.Minute,
	)
	if err != nil {
		t.Fatalf("claim expiring candidate: %v", err)
	}

	if expiringClaim == nil {
		t.Fatal("expected expiring candidate to be claimed")
	}

	expiredLock := now.Add(-time.Minute)
	expiringClaim.LockedUntil = &expiredLock

	if err := repository.Update(ctx, *expiringClaim); err != nil {
		t.Fatalf("update expired lock: %v", err)
	}

	expiredResult, err := candidateService.Complete(
		*expiringClaim,
		now.Add(time.Minute),
	)
	if err != nil {
		t.Fatalf("complete expired candidate: %v", err)
	}

	err = repository.UpdateProcessingResult(
		ctx,
		expiredResult,
		"worker-3",
	)
	if !errors.Is(err, ErrCandidateOwnershipLost) {
		t.Fatalf("expected ErrCandidateOwnershipLost for expired lock, got %v", err)
	}
}
