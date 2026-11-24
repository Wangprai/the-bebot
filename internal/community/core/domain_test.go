package core

import (
	"errors"
	"testing"
	"time"
)

func TestNewCommunityValidation(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)

	c, err := NewCommunity(Community{
		GuildID:     " guild-1 ",
		Name:        " Dev Hangout ",
		ActiveSince: now,
		UpdatedAt:   now,
	})
	if err != nil {
		t.Fatalf("NewCommunity() error = %v", err)
	}

	if c.GuildID != "guild-1" || c.Name != "Dev Hangout" || c.PrimaryLanguage != "" {
		t.Errorf("unexpected community: %+v", c)
	}

	if _, err := NewCommunity(Community{ActiveSince: now, UpdatedAt: now}); !errors.Is(err, ErrInvalidGuildID) {
		t.Errorf("expected ErrInvalidGuildID, got %v", err)
	}

	if _, err := NewCommunity(Community{GuildID: "guild-1"}); !errors.Is(err, ErrInvalidTimestamp) {
		t.Errorf("expected ErrInvalidTimestamp, got %v", err)
	}
}

func TestNewMemberValidationAndPreferredName(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)

	m, err := NewMember(Member{
		GuildID:          "guild-1",
		UserID:           "user-1",
		DisplayName:      "Alice",
		Nickname:         "Ali",
		InteractionCount: 3,
		LastSeenAt:       now,
	})
	if err != nil {
		t.Fatalf("NewMember() error = %v", err)
	}

	if m.PreferredName() != "Ali" {
		t.Errorf("PreferredName() = %q, want Ali", m.PreferredName())
	}

	m.Nickname = ""
	if m.PreferredName() != "Alice" {
		t.Errorf("PreferredName() fallback = %q, want Alice", m.PreferredName())
	}

	if _, err := NewMember(Member{UserID: "u1", LastSeenAt: now}); !errors.Is(err, ErrInvalidGuildID) {
		t.Errorf("expected ErrInvalidGuildID, got %v", err)
	}

	if _, err := NewMember(Member{GuildID: "g1", LastSeenAt: now}); !errors.Is(err, ErrInvalidUserID) {
		t.Errorf("expected ErrInvalidUserID, got %v", err)
	}

	if _, err := NewMember(Member{GuildID: "g1", UserID: "u1", InteractionCount: -1, LastSeenAt: now}); !errors.Is(err, ErrInvalidInteractionCount) {
		t.Errorf("expected ErrInvalidInteractionCount, got %v", err)
	}

	if _, err := NewMember(Member{GuildID: "g1", UserID: "u1"}); !errors.Is(err, ErrInvalidTimestamp) {
		t.Errorf("expected ErrInvalidTimestamp, got %v", err)
	}
}
