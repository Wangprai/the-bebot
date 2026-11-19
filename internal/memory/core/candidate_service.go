package core

import (
	"errors"
	"time"
)

var (
	ErrCandidateLockNotExpired = errors.New("candidate lock has not expired")
	ErrCandidateRetryNotReady  = errors.New("candidate retry is not ready")
)

type CandidateService interface {
	StartProcessing(candidate MemoryCandidate, workerID string, now time.Time) (MemoryCandidate, error)
	Complete(candidate MemoryCandidate, now time.Time) (MemoryCandidate, error)
	Reject(candidate MemoryCandidate, now time.Time) (MemoryCandidate, error)
	Fail(candidate MemoryCandidate, now time.Time, retryAt time.Time) (MemoryCandidate, error)
	Retry(candidate MemoryCandidate, now time.Time) (MemoryCandidate, error)
	Recover(candidate MemoryCandidate, now time.Time) (MemoryCandidate, error)
}

type candidateService struct{}

func NewCandidateService() CandidateService {
	return &candidateService{}
}

func (s *candidateService) StartProcessing(
	candidate MemoryCandidate,
	workerID string,
	now time.Time,
) (MemoryCandidate, error) {
	if candidate.Status != CandidateStatusPending {
		return MemoryCandidate{}, ErrInvalidCandidateTransition
	}

	if workerID == "" || now.IsZero() {
		return MemoryCandidate{}, errors.New("invalid processing information")
	}

	candidate.Status = CandidateStatusProcessing
	candidate.WorkerID = workerID
	lockedUntil := now.Add(5 * time.Minute)
	candidate.LockedUntil = &lockedUntil
	candidate.Attempts++
	candidate.UpdatedAt = now

	return candidate, nil
}

func (s *candidateService) Complete(
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

	return candidate, nil
}

func (s *candidateService) Reject(
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

	return candidate, nil
}

func (s *candidateService) Fail(
	candidate MemoryCandidate,
	now time.Time,
	retryAt time.Time,
) (MemoryCandidate, error) {
	if candidate.Status != CandidateStatusProcessing {
		return MemoryCandidate{}, ErrInvalidCandidateTransition
	}

	if now.IsZero() || retryAt.IsZero() || !retryAt.After(now) {
		return MemoryCandidate{}, errors.New("invalid retry time")
	}

	candidate.Status = CandidateStatusFailed
	candidate.WorkerID = ""
	candidate.LockedUntil = nil
	candidate.NextRetryAt = &retryAt
	candidate.UpdatedAt = now

	return candidate, nil
}

func (s *candidateService) Retry(
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
		return MemoryCandidate{}, ErrCandidateRetryNotReady
	}

	candidate.Status = CandidateStatusPending
	candidate.WorkerID = ""
	candidate.LockedUntil = nil
	candidate.NextRetryAt = nil
	candidate.UpdatedAt = now

	return candidate, nil
}

func (s *candidateService) Recover(
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
		return MemoryCandidate{}, ErrCandidateLockNotExpired
	}

	candidate.Status = CandidateStatusPending
	candidate.WorkerID = ""
	candidate.LockedUntil = nil
	candidate.UpdatedAt = now

	return candidate, nil
}
