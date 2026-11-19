package core

import (
	"errors"
	"testing"
	"time"
)

func processingCandidate() MemoryCandidate {
	now := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	lockedUntil := now.Add(5 * time.Minute)

	return MemoryCandidate{
		ID:          "candidate-001",
		GuildID:     "guild-001",
		ChannelID:   "channel-001",
		MessageID:   "message-001",
		UserID:      "user-001",
		Content:     "User likes Go programming",
		Status:      CandidateStatusProcessing,
		Attempts:    1,
		WorkerID:    "worker-001",
		LockedUntil: &lockedUntil,
		ExpireAt:    now.Add(24 * time.Hour),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func TestMemoryServiceComplete(t *testing.T) {
	service := NewMemoryService()
	now := time.Date(2026, 11, 19, 11, 0, 0, 0, time.UTC)

	candidate := processingCandidate()

	result, err := service.Complete(candidate, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Status != CandidateStatusCompleted {
		t.Fatalf("expected completed, got %s", result.Status)
	}

	if result.WorkerID != "" || result.LockedUntil != nil {
		t.Fatal("expected worker and lock to be cleared")
	}

	if result.UpdatedAt != now {
		t.Fatal("expected UpdatedAt to be updated")
	}

	// Original candidate must remain unchanged.
	if candidate.Status != CandidateStatusProcessing {
		t.Fatal("original candidate was modified")
	}
}

func TestMemoryServiceReject(t *testing.T) {
	service := NewMemoryService()
	now := time.Date(2026, 11, 19, 11, 0, 0, 0, time.UTC)

	result, err := service.Reject(processingCandidate(), now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Status != CandidateStatusRejected {
		t.Fatalf("expected rejected, got %s", result.Status)
	}
}

func TestMemoryServiceFailAndRetry(t *testing.T) {
	service := NewMemoryService()
	now := time.Date(2026, 11, 19, 11, 0, 0, 0, time.UTC)
	retryAt := now.Add(time.Minute)

	failed, err := service.Fail(processingCandidate(), now, &retryAt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if failed.Status != CandidateStatusFailed {
		t.Fatalf("expected failed, got %s", failed.Status)
	}

	if failed.NextRetryAt == nil {
		t.Fatal("expected NextRetryAt to be set")
	}

	// Retry before scheduled time must fail.
	_, err = service.Retry(failed, now)
	if !errors.Is(err, ErrInvalidCandidateRetry) {
		t.Fatalf("expected ErrInvalidCandidateRetry, got %v", err)
	}

	// Retry after scheduled time must succeed.
	retried, err := service.Retry(failed, retryAt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if retried.Status != CandidateStatusPending {
		t.Fatalf("expected pending, got %s", retried.Status)
	}

	if retried.NextRetryAt != nil {
		t.Fatal("expected NextRetryAt to be cleared")
	}
}

func TestMemoryServiceRecoverExpired(t *testing.T) {
	service := NewMemoryService()
	candidate := processingCandidate()

	now := candidate.LockedUntil.Add(time.Second)

	result, err := service.RecoverExpired(candidate, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Status != CandidateStatusPending {
		t.Fatalf("expected pending, got %s", result.Status)
	}

	if result.WorkerID != "" || result.LockedUntil != nil {
		t.Fatal("expected worker and lock to be cleared")
	}
}

func TestMemoryServiceRecoverBeforeLockExpires(t *testing.T) {
	service := NewMemoryService()
	candidate := processingCandidate()

	now := candidate.LockedUntil.Add(-time.Minute)

	_, err := service.RecoverExpired(candidate, now)
	if !errors.Is(err, ErrInvalidCandidateLock) {
		t.Fatalf("expected ErrInvalidCandidateLock, got %v", err)
	}
}

func TestMemoryServiceRejectInvalidTransition(t *testing.T) {
	service := NewMemoryService()
	now := time.Date(2026, 11, 19, 11, 0, 0, 0, time.UTC)

	candidate := validMemoryCandidate()

	_, err := service.Complete(candidate, now)
	if !errors.Is(err, ErrInvalidCandidateTransition) {
		t.Fatalf("expected ErrInvalidCandidateTransition, got %v", err)
	}
}

func validMemoryCandidate() MemoryCandidate {
	now := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)

	return MemoryCandidate{
		ID:        "candidate-001",
		GuildID:   "guild-001",
		ChannelID: "channel-001",
		MessageID: "message-001",
		UserID:    "user-001",
		Content:   "User likes Go programming",
		Status:    CandidateStatusPending,
		Attempts:  0,
		ExpireAt:  now.Add(24 * time.Hour),
		CreatedAt: now,
		UpdatedAt: now,
	}
}
