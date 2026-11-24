package core

import (
	"context"
	"time"
)

type CommunityUpsertInput struct {
	GuildID string
	Name    string
	SeenAt  time.Time
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
	UpsertInteraction(
		ctx context.Context,
		input MemberUpsertInput,
	) (Member, error)

	FindByGuildAndUser(
		ctx context.Context,
		guildID string,
		userID string,
	) (*Member, error)
}
