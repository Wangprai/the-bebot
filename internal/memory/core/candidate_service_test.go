package core

import (
	"errors"
	"testing"
	"time"
)

func newTestCandidate() MemoryCandidate {
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

func TestStartProcessing(t *testing.T) {
	service := NewCandidateService()
	now := time.Now()

	candidate := newTestCandidate()

	result, err := service.StartProcessing(candidate, "worker-001", now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Status != CandidateStatusProcessing {
		t.Errorf("expected processing, got %s", result.Status)
	}

	if result.WorkerID != "worker-001" {
		t.Errorf("expected worker-001, got %s", result.WorkerID)
	}

	if result.Attempts != 1 {
		t.Errorf("expected attempts 1, got %d", result.Attempts)
	}

	if result.LockedUntil == nil {
		t.Fatal("expected lock expiration")
	}

	if !result.LockedUntil.Equal(now.Add(5 * time.Minute)) {
		t.Errorf("unexpected lock expiration: %v", result.LockedUntil)
	}
}

func TestStartProcessingRejectsInvalidTransition(t *testing.T) {
	service := NewCandidateService()
	candidate := newTestCandidate()
	candidate.Status = CandidateStatusCompleted

	_, err := service.StartProcessing(candidate, "worker-001", time.Now())

	if !errors.Is(err, ErrInvalidCandidateTransition) {
		t.Fatalf("expected ErrInvalidCandidateTransition, got %v", err)
	}
}

func TestCompleteCandidate(t *testing.T) {
	service := NewCandidateService()
	now := time.Now()

	candidate := newTestCandidate()
	candidate.Status = CandidateStatusProcessing
	candidate.WorkerID = "worker-001"

	lock := now.Add(5 * time.Minute)
	candidate.LockedUntil = &lock

	result, err := service.Complete(candidate, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Status != CandidateStatusCompleted {
		t.Errorf("expected completed, got %s", result.Status)
	}

	if result.WorkerID != "" {
		t.Errorf("expected empty worker ID, got %s", result.WorkerID)
	}

	if result.LockedUntil != nil {
		t.Error("expected lock to be cleared")
	}

	if result.NextRetryAt != nil {
		t.Error("expected retry time to be cleared")
	}
}

func TestRejectCandidate(t *testing.T) {
	service := NewCandidateService()
	now := time.Now()

	candidate := newTestCandidate()
	candidate.Status = CandidateStatusProcessing
	candidate.WorkerID = "worker-001"

	result, err := service.Reject(candidate, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Status != CandidateStatusRejected {
		t.Errorf("expected rejected, got %s", result.Status)
	}

	if result.WorkerID != "" {
		t.Error("expected worker ID to be cleared")
	}
}

func TestFailCandidate(t *testing.T) {
	service := NewCandidateService()
	now := time.Now()
	retryAt := now.Add(1 * time.Minute)

	candidate := newTestCandidate()
	candidate.Status = CandidateStatusProcessing
	candidate.WorkerID = "worker-001"

	result, err := service.Fail(candidate, now, retryAt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Status != CandidateStatusFailed {
		t.Errorf("expected failed, got %s", result.Status)
	}

	if result.NextRetryAt == nil || !result.NextRetryAt.Equal(retryAt) {
		t.Error("expected retry time to be set")
	}

	if result.WorkerID != "" {
		t.Error("expected worker ID to be cleared")
	}
}

func TestRetryCandidate(t *testing.T) {
	service := NewCandidateService()
	now := time.Now()

	candidate := newTestCandidate()
	candidate.Status = CandidateStatusFailed

	retryAt := now.Add(-1 * time.Minute)
	candidate.NextRetryAt = &retryAt

	result, err := service.Retry(candidate, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Status != CandidateStatusPending {
		t.Errorf("expected pending, got %s", result.Status)
	}

	if result.NextRetryAt != nil {
		t.Error("expected retry time to be cleared")
	}
}

func TestRetryCandidateNotReady(t *testing.T) {
	service := NewCandidateService()
	now := time.Now()

	candidate := newTestCandidate()
	candidate.Status = CandidateStatusFailed

	retryAt := now.Add(5 * time.Minute)
	candidate.NextRetryAt = &retryAt

	_, err := service.Retry(candidate, now)

	if !errors.Is(err, ErrCandidateRetryNotReady) {
		t.Fatalf("expected ErrCandidateRetryNotReady, got %v", err)
	}
}

func TestRecoverExpiredCandidate(t *testing.T) {
	service := NewCandidateService()
	now := time.Now()

	candidate := newTestCandidate()
	candidate.Status = CandidateStatusProcessing
	candidate.WorkerID = "worker-001"

	lock := now.Add(-1 * time.Minute)
	candidate.LockedUntil = &lock

	result, err := service.Recover(candidate, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Status != CandidateStatusPending {
		t.Errorf("expected pending, got %s", result.Status)
	}

	if result.WorkerID != "" {
		t.Error("expected worker ID to be cleared")
	}

	if result.LockedUntil != nil {
		t.Error("expected lock to be cleared")
	}
}

func TestRecoverCandidateBeforeLockExpires(t *testing.T) {
	service := NewCandidateService()
	now := time.Now()

	candidate := newTestCandidate()
	candidate.Status = CandidateStatusProcessing

	lock := now.Add(5 * time.Minute)
	candidate.LockedUntil = &lock

	_, err := service.Recover(candidate, now)

	if !errors.Is(err, ErrCandidateLockNotExpired) {
		t.Fatalf("expected ErrCandidateLockNotExpired, got %v", err)
	}
}
