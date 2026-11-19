package core

import (
	"errors"
	"testing"
	"time"
)

func validMemory() Memory {
	now := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)

	return Memory{
		ID:         "memory-001",
		GuildID:    "guild-001",
		UserID:     "user-001",
		Scope:      MemoryScopeMember,
		Type:       MemoryTypePreference,
		Content:    "  Likes Go programming  ",
		Importance: 3,
		Confidence: 0.9,
		Tags:       []string{"Go", "Backend"},
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

func TestNewMemory(t *testing.T) {
	memory, err := NewMemory(validMemory())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if memory.Content != "Likes Go programming" {
		t.Errorf("content was not trimmed: %q", memory.Content)
	}

	if len(memory.Tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(memory.Tags))
	}

	if memory.Tags[0] != "go" || memory.Tags[1] != "backend" {
		t.Errorf("tags were not normalized: %v", memory.Tags)
	}
}

func TestNewMemoryRejectsInvalidData(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Memory)
		wantErr error
	}{
		{
			name: "empty ID",
			modify: func(m *Memory) {
				m.ID = ""
			},
			wantErr: ErrInvalidMemoryID,
		},
		{
			name: "empty guild ID",
			modify: func(m *Memory) {
				m.GuildID = ""
			},
			wantErr: ErrInvalidGuildID,
		},
		{
			name: "empty content",
			modify: func(m *Memory) {
				m.Content = "   "
			},
			wantErr: ErrInvalidMemoryContent,
		},
		{
			name: "invalid scope",
			modify: func(m *Memory) {
				m.Scope = "unknown"
			},
			wantErr: ErrInvalidMemoryScope,
		},
		{
			name: "invalid type",
			modify: func(m *Memory) {
				m.Type = "unknown"
			},
			wantErr: ErrInvalidMemoryType,
		},
		{
			name: "member scope without user ID",
			modify: func(m *Memory) {
				m.UserID = ""
			},
			wantErr: ErrInvalidUserID,
		},
		{
			name: "community scope with user ID",
			modify: func(m *Memory) {
				m.Scope = MemoryScopeCommunity
			},
			wantErr: ErrInvalidUserID,
		},
		{
			name: "importance below minimum",
			modify: func(m *Memory) {
				m.Importance = 0
			},
			wantErr: ErrInvalidImportance,
		},
		{
			name: "importance above maximum",
			modify: func(m *Memory) {
				m.Importance = 6
			},
			wantErr: ErrInvalidImportance,
		},
		{
			name: "confidence below minimum",
			modify: func(m *Memory) {
				m.Confidence = -0.1
			},
			wantErr: ErrInvalidConfidence,
		},
		{
			name: "confidence above maximum",
			modify: func(m *Memory) {
				m.Confidence = 1.1
			},
			wantErr: ErrInvalidConfidence,
		},
		{
			name: "missing created at",
			modify: func(m *Memory) {
				m.CreatedAt = time.Time{}
			},
			wantErr: ErrInvalidTimestamp,
		},
		{
			name: "missing updated at",
			modify: func(m *Memory) {
				m.UpdatedAt = time.Time{}
			},
			wantErr: ErrInvalidTimestamp,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			memory := validMemory()
			tt.modify(&memory)

			_, err := NewMemory(memory)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf(
					"expected error %v, got %v",
					tt.wantErr,
					err,
				)
			}
		})
	}
}

func TestNewMemoryCommunityScope(t *testing.T) {
	memory := validMemory()
	memory.Scope = MemoryScopeCommunity
	memory.UserID = ""

	result, err := NewMemory(memory)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.UserID != "" {
		t.Error("community memory should not have a user ID")
	}
}

func TestNewMemoryNormalizesTags(t *testing.T) {
	memory := validMemory()
	memory.Tags = []string{
		" Go ",
		"backend",
		"GO",
		"",
		"  ",
		"AI",
	}

	result, err := NewMemory(memory)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"go", "backend", "ai"}

	if len(result.Tags) != len(expected) {
		t.Fatalf("expected %v, got %v", expected, result.Tags)
	}

	for i, tag := range expected {
		if result.Tags[i] != tag {
			t.Errorf(
				"tag[%d]: expected %q, got %q",
				i,
				tag,
				result.Tags[i],
			)
		}
	}
}
