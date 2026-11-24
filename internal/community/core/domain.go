package core

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalidGuildID          = errors.New("invalid guild ID")
	ErrInvalidUserID           = errors.New("invalid user ID")
	ErrInvalidTimestamp        = errors.New("invalid timestamp")
	ErrInvalidInteractionCount = errors.New("invalid interaction count")
)

// Community stores lightweight community-level identity metadata only.
// Long-term community facts belong in Memory (scope: community).
type Community struct {
	GuildID         string
	Name            string
	PrimaryLanguage string
	ActiveSince     time.Time
	UpdatedAt       time.Time
}

func NewCommunity(c Community) (Community, error) {
	c.GuildID = strings.TrimSpace(c.GuildID)
	c.Name = strings.TrimSpace(c.Name)
	c.PrimaryLanguage = strings.TrimSpace(c.PrimaryLanguage)

	if c.GuildID == "" {
		return Community{}, ErrInvalidGuildID
	}

	if c.ActiveSince.IsZero() || c.UpdatedAt.IsZero() {
		return Community{}, ErrInvalidTimestamp
	}

	c.ActiveSince = c.ActiveSince.UTC()
	c.UpdatedAt = c.UpdatedAt.UTC()

	return c, nil
}

// Member stores lightweight member-level identity metadata only.
// Long-term member facts and preferences belong in Memory (scope: member).
type Member struct {
	GuildID          string
	UserID           string
	DisplayName      string
	Nickname         string
	InteractionCount int
	LastSeenAt       time.Time
}

func NewMember(m Member) (Member, error) {
	m.GuildID = strings.TrimSpace(m.GuildID)
	m.UserID = strings.TrimSpace(m.UserID)
	m.DisplayName = strings.TrimSpace(m.DisplayName)
	m.Nickname = strings.TrimSpace(m.Nickname)

	if m.GuildID == "" {
		return Member{}, ErrInvalidGuildID
	}

	if m.UserID == "" {
		return Member{}, ErrInvalidUserID
	}

	if m.InteractionCount < 0 {
		return Member{}, ErrInvalidInteractionCount
	}

	if m.LastSeenAt.IsZero() {
		return Member{}, ErrInvalidTimestamp
	}

	m.LastSeenAt = m.LastSeenAt.UTC()

	return m, nil
}

// PreferredName returns the member's guild nickname if present, otherwise display name.
func (m Member) PreferredName() string {
	if m.Nickname != "" {
		return m.Nickname
	}
	return m.DisplayName
}
