package core

import (
	"time"
)

type MemoryService interface {
	Complete(candidate MemoryCandidate, now time.Time) (MemoryCandidate, error)
	Reject(candidate MemoryCandidate, now time.Time) (MemoryCandidate, error)
	Fail(candidate MemoryCandidate, now time.Time, retryAt *time.Time) (MemoryCandidate, error)
	Retry(candidate MemoryCandidate, now time.Time) (MemoryCandidate, error)
	RecoverExpired(candidate MemoryCandidate, now time.Time) (MemoryCandidate, error)
}

type memoryService struct{}

func NewMemoryService() MemoryService {
	return &memoryService{}
}

// Complete marks a processing candidate as completed.
func (s *memoryService) Complete(
	candidate MemoryCandidate,
	now time.Time,
) (MemoryCandidate, error) {
	if candidate.Status != CandidateStatusProcessing {
		return MemoryCandidate{}, ErrInvalidCandidateTransition
	}

	if now.IsZero() {
		return MemoryCandidate{}, ErrInvalidTimestamp
	}

	candidate.Status = CandidateStatusCompleted
	candidate.WorkerID = ""
	candidate.LockedUntil = nil
	candidate.NextRetryAt = nil
	candidate.UpdatedAt = now

	return NewMemoryCandidate(candidate)
}

// Reject marks a processing candidate as rejected.
func (s *memoryService) Reject(
	candidate MemoryCandidate,
	now time.Time,
) (MemoryCandidate, error) {
	if candidate.Status != CandidateStatusProcessing {
		return MemoryCandidate{}, ErrInvalidCandidateTransition
	}

	if now.IsZero() {
		return MemoryCandidate{}, ErrInvalidTimestamp
	}

	candidate.Status = CandidateStatusRejected
	candidate.WorkerID = ""
	candidate.LockedUntil = nil
	candidate.NextRetryAt = nil
	candidate.UpdatedAt = now

	return NewMemoryCandidate(candidate)
}

// Fail marks a processing candidate as failed.
func (s *memoryService) Fail(
	candidate MemoryCandidate,
	now time.Time,
	retryAt *time.Time,
) (MemoryCandidate, error) {
	if candidate.Status != CandidateStatusProcessing {
		return MemoryCandidate{}, ErrInvalidCandidateTransition
	}

	if now.IsZero() {
		return MemoryCandidate{}, ErrInvalidTimestamp
	}

	if retryAt != nil && !retryAt.After(now) {
		return MemoryCandidate{}, ErrInvalidCandidateRetry
	}

	candidate.Status = CandidateStatusFailed
	candidate.WorkerID = ""
	candidate.LockedUntil = nil
	candidate.NextRetryAt = copyTime(retryAt)
	candidate.UpdatedAt = now

	return NewMemoryCandidate(candidate)
}

// Retry moves a failed candidate back to pending.
func (s *memoryService) Retry(
	candidate MemoryCandidate,
	now time.Time,
) (MemoryCandidate, error) {
	if candidate.Status != CandidateStatusFailed {
		return MemoryCandidate{}, ErrInvalidCandidateTransition
	}

	if now.IsZero() {
		return MemoryCandidate{}, ErrInvalidTimestamp
	}

	if candidate.NextRetryAt == nil || now.Before(*candidate.NextRetryAt) {
		return MemoryCandidate{}, ErrInvalidCandidateRetry
	}

	candidate.Status = CandidateStatusPending
	candidate.WorkerID = ""
	candidate.LockedUntil = nil
	candidate.NextRetryAt = nil
	candidate.UpdatedAt = now

	return NewMemoryCandidate(candidate)
}

// RecoverExpired moves a processing candidate with an expired lock back to pending.
func (s *memoryService) RecoverExpired(
	candidate MemoryCandidate,
	now time.Time,
) (MemoryCandidate, error) {
	if candidate.Status != CandidateStatusProcessing {
		return MemoryCandidate{}, ErrInvalidCandidateTransition
	}

	if now.IsZero() {
		return MemoryCandidate{}, ErrInvalidTimestamp
	}

	if candidate.LockedUntil == nil || now.Before(*candidate.LockedUntil) {
		return MemoryCandidate{}, ErrInvalidCandidateLock
	}

	candidate.Status = CandidateStatusPending
	candidate.WorkerID = ""
	candidate.LockedUntil = nil
	candidate.NextRetryAt = nil
	candidate.UpdatedAt = now

	return NewMemoryCandidate(candidate)
}

func copyTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}

	copied := *value
	return &copied
}
