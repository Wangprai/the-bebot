package mongodb

import (
	"time"

	"the-bebot/internal/community/core"
)

type communityDocument struct {
	GuildID         string    `bson:"guild_id"`
	Name            string    `bson:"name,omitempty"`
	PrimaryLanguage string    `bson:"primary_language,omitempty"`
	ActiveSince     time.Time `bson:"active_since"`
	UpdatedAt       time.Time `bson:"updated_at"`
}

func (d communityDocument) toDomain() (core.Community, error) {
	return core.NewCommunity(core.Community{
		GuildID:         d.GuildID,
		Name:            d.Name,
		PrimaryLanguage: d.PrimaryLanguage,
		ActiveSince:     d.ActiveSince,
		UpdatedAt:       d.UpdatedAt,
	})
}

type memberDocument struct {
	GuildID          string    `bson:"guild_id"`
	UserID           string    `bson:"user_id"`
	DisplayName      string    `bson:"display_name,omitempty"`
	Nickname         string    `bson:"nickname,omitempty"`
	InteractionCount int       `bson:"interaction_count"`
	LastSeenAt       time.Time `bson:"last_seen_at"`
}

func (d memberDocument) toDomain() (core.Member, error) {
	return core.NewMember(core.Member{
		GuildID:          d.GuildID,
		UserID:           d.UserID,
		DisplayName:      d.DisplayName,
		Nickname:         d.Nickname,
		InteractionCount: d.InteractionCount,
		LastSeenAt:       d.LastSeenAt,
	})
}
