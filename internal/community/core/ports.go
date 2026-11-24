package core

import (
	"context"
	"time"
)

type CommunityUpsertInput struct {
	GuildID           string
	Name              string
	AuthoritativeName bool
	SeenAt            time.Time
}

type MemberUpsertInput struct {
	GuildID     string
	UserID      string
	DisplayName string
	Nickname    string
	SeenAt      time.Time
}

type CommunityRepository interface {
	UpsertInteraction(
		ctx context.Context,
		input CommunityUpsertInput,
	) (Community, error)

	FindByGuildID(
		ctx context.Context,
		guildID string,
	) (*Community, error)
}

type MemberRepository interface {
	// UpsertInteraction touches/upserts member profile metadata without
	// incrementing interaction_count (initializing it to 0 on insert).
	UpsertInteraction(
		ctx context.Context,
		input MemberUpsertInput,
	) (Member, error)

	// IncrementInteractionCount atomically increments a member's interaction_count
	// after a reply has been sent successfully.
	IncrementInteractionCount(
		ctx context.Context,
		guildID string,
		userID string,
		interactedAt time.Time,
	) error

	FindByGuildAndUser(
		ctx context.Context,
		guildID string,
		userID string,
	) (*Member, error)
}
