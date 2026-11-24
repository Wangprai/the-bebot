package core

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalidMemoryID            = errors.New("invalid memory ID")
	ErrInvalidGuildID             = errors.New("invalid guild ID")
	ErrInvalidUserID              = errors.New("invalid user ID")
	ErrInvalidMemoryContent       = errors.New("invalid memory content")
	ErrInvalidMemoryType          = errors.New("invalid memory type")
	ErrInvalidMemoryScope         = errors.New("invalid memory scope")
	ErrInvalidImportance          = errors.New("invalid memory importance")
	ErrInvalidConfidence          = errors.New("invalid memory confidence")
	ErrInvalidTimestamp           = errors.New("invalid memory timestamp")
	ErrInvalidCandidateStatus     = errors.New("invalid candidate status")
	ErrInvalidCandidateWorker     = errors.New("invalid candidate worker")
	ErrInvalidCandidateLock       = errors.New("invalid candidate lock")
	ErrInvalidCandidateRetry      = errors.New("candidate is not ready for retry")
	ErrInvalidCandidateTransition = errors.New("invalid candidate state transition")
	ErrCandidateAlreadyExists     = errors.New("memory candidate already exists")
)

type MemoryType string

const (
	MemoryTypeRunningJoke        MemoryType = "running_joke"
	MemoryTypePersonalFact       MemoryType = "personal_fact"
	MemoryTypePreference         MemoryType = "preference"
	MemoryTypeEvent              MemoryType = "event"
	MemoryTypePromise            MemoryType = "promise"
	MemoryTypeCatchphrase        MemoryType = "catchphrase"
	MemoryTypeInteractionPattern MemoryType = "interaction_pattern"
)

type MemoryScope string

const (
	MemoryScopeCommunity MemoryScope = "community"
	MemoryScopeMember    MemoryScope = "member"
)

type Memory struct {
	ID         string
	GuildID    string
	UserID     string
	Scope      MemoryScope
	Type       MemoryType
	Content    string
	Importance int
	Confidence float64
	Tags       []string

	SourceMessageID string
	CandidateID     string
	CandidateIndex  int

	UsageCount int
	LastUsedAt *time.Time

	CreatedAt time.Time
	UpdatedAt time.Time
}

func NewMemory(memory Memory) (Memory, error) {
	memory.ID = strings.TrimSpace(memory.ID)
	memory.GuildID = strings.TrimSpace(memory.GuildID)
	memory.UserID = strings.TrimSpace(memory.UserID)
	memory.Content = strings.TrimSpace(memory.Content)
	memory.SourceMessageID = strings.TrimSpace(memory.SourceMessageID)
	memory.CandidateID = strings.TrimSpace(memory.CandidateID)

	if memory.CandidateID != "" && memory.CandidateIndex < 0 {
		return Memory{}, ErrInvalidMemoryID
	}

	if memory.ID == "" {
		return Memory{}, ErrInvalidMemoryID
	}

	if memory.GuildID == "" {
		return Memory{}, ErrInvalidGuildID
	}

	if memory.Content == "" {
		return Memory{}, ErrInvalidMemoryContent
	}

	if !memory.Scope.IsValid() {
		return Memory{}, ErrInvalidMemoryScope
	}

	if !memory.Type.IsValid() {
		return Memory{}, ErrInvalidMemoryType
	}

	if memory.Scope == MemoryScopeCommunity && memory.UserID != "" {
		return Memory{}, ErrInvalidUserID
	}

	if memory.Scope == MemoryScopeMember && memory.UserID == "" {
		return Memory{}, ErrInvalidUserID
	}

	if memory.Importance < 1 || memory.Importance > 5 {
		return Memory{}, ErrInvalidImportance
	}

	if memory.Confidence < 0 || memory.Confidence > 1 {
		return Memory{}, ErrInvalidConfidence
	}

	if memory.CreatedAt.IsZero() || memory.UpdatedAt.IsZero() {
		return Memory{}, ErrInvalidTimestamp
	}

	memory.Tags = normalizeTags(memory.Tags)

	return memory, nil
}

func (t MemoryType) IsValid() bool {
	switch t {
	case MemoryTypeRunningJoke,
		MemoryTypePersonalFact,
		MemoryTypePreference,
		MemoryTypeEvent,
		MemoryTypePromise,
		MemoryTypeCatchphrase,
		MemoryTypeInteractionPattern:
		return true
	default:
		return false
	}
}

func (s MemoryScope) IsValid() bool {
	return s == MemoryScopeCommunity || s == MemoryScopeMember
}

func normalizeTags(tags []string) []string {
	normalized := make([]string, 0, len(tags))
	seen := make(map[string]struct{})

	for _, tag := range tags {
		tag = strings.ToLower(strings.TrimSpace(tag))

		if tag == "" {
			continue
		}

		if _, exists := seen[tag]; exists {
			continue
		}

		seen[tag] = struct{}{}
		normalized = append(normalized, tag)
	}

	return normalized
}

type CandidateStatus string

const (
	CandidateStatusPending    CandidateStatus = "pending"
	CandidateStatusProcessing CandidateStatus = "processing"
	CandidateStatusCompleted  CandidateStatus = "completed"
	CandidateStatusRejected   CandidateStatus = "rejected"
	CandidateStatusFailed     CandidateStatus = "failed"
)

type MemoryCandidate struct {
	ID        string
	GuildID   string
	ChannelID string
	MessageID string
	UserID    string
	Content   string

	// Context snapshot at the time of mention.
	Messages []ConversationMessage

	Status      CandidateStatus
	Attempts    int
	WorkerID    string
	LockedUntil *time.Time
	NextRetryAt *time.Time
	ExpireAt    time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func NewMemoryCandidate(candidate MemoryCandidate) (MemoryCandidate, error) {
	candidate.ID = strings.TrimSpace(candidate.ID)
	candidate.GuildID = strings.TrimSpace(candidate.GuildID)
	candidate.ChannelID = strings.TrimSpace(candidate.ChannelID)
	candidate.MessageID = strings.TrimSpace(candidate.MessageID)
	candidate.UserID = strings.TrimSpace(candidate.UserID)
	candidate.Content = strings.TrimSpace(candidate.Content)
	candidate.WorkerID = strings.TrimSpace(candidate.WorkerID)

	// Copy messages to prevent modifying the original slice.
	candidate.Messages = append(
		[]ConversationMessage(nil),
		candidate.Messages...,
	)

	if candidate.ID == "" {
		return MemoryCandidate{}, ErrInvalidMemoryID
	}

	if candidate.GuildID == "" {
		return MemoryCandidate{}, ErrInvalidGuildID
	}

	if candidate.ChannelID == "" || candidate.MessageID == "" {
		return MemoryCandidate{}, errors.New("invalid candidate source")
	}

	if candidate.UserID == "" {
		return MemoryCandidate{}, ErrInvalidUserID
	}

	if candidate.Content == "" {
		return MemoryCandidate{}, ErrInvalidMemoryContent
	}

	if !candidate.Status.IsValid() {
		return MemoryCandidate{}, errors.New("invalid candidate status")
	}

	if candidate.Attempts < 0 {
		return MemoryCandidate{}, errors.New("invalid candidate attempts")
	}

	if candidate.ExpireAt.IsZero() ||
		candidate.CreatedAt.IsZero() ||
		candidate.UpdatedAt.IsZero() {
		return MemoryCandidate{}, ErrInvalidTimestamp
	}

	if candidate.Status == CandidateStatusProcessing &&
		candidate.WorkerID == "" {
		return MemoryCandidate{}, errors.New("processing candidate requires worker ID")
	}

	// Validate conversation messages.
	if len(candidate.Messages) == 0 {
		return MemoryCandidate{}, errors.New("candidate messages are required")
	}

	seenMessageIDs := make(map[string]struct{}, len(candidate.Messages))

	for i := range candidate.Messages {
		message := &candidate.Messages[i]

		message.MessageID = strings.TrimSpace(message.MessageID)
		message.UserID = strings.TrimSpace(message.UserID)
		message.Content = strings.TrimSpace(message.Content)

		if message.MessageID == "" {
			return MemoryCandidate{}, errors.New("candidate message ID is required")
		}

		if message.UserID == "" {
			return MemoryCandidate{}, errors.New("candidate message user ID is required")
		}

		if message.Content == "" {
			return MemoryCandidate{}, errors.New("candidate message content is required")
		}

		if _, exists := seenMessageIDs[message.MessageID]; exists {
			return MemoryCandidate{}, errors.New("duplicate candidate message ID")
		}

		seenMessageIDs[message.MessageID] = struct{}{}
	}

	// Ensure the trigger message exists in the conversation snapshot.
	if _, exists := seenMessageIDs[candidate.MessageID]; !exists {
		return MemoryCandidate{}, errors.New("candidate trigger message not found in messages")
	}

	return candidate, nil
}

func (s CandidateStatus) IsValid() bool {
	switch s {
	case CandidateStatusPending,
		CandidateStatusProcessing,
		CandidateStatusCompleted,
		CandidateStatusRejected,
		CandidateStatusFailed:
		return true
	default:
		return false
	}
}
