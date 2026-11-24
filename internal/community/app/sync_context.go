package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"the-bebot/internal/community/core"
)

type SyncInteractionInput struct {
	GuildID     string
	GuildName   string
	UserID      string
	DisplayName string
	Nickname    string
	SeenAt      time.Time
}

type SyncedContext struct {
	Community core.Community
	Member    core.Member
}

type SyncContextUseCase struct {
	communityRepository core.CommunityRepository
	memberRepository    core.MemberRepository
	clock               func() time.Time
}

func NewSyncContextUseCase(
	communityRepository core.CommunityRepository,
	memberRepository core.MemberRepository,
) (*SyncContextUseCase, error) {
	if communityRepository == nil {
		return nil, errors.New("community repository is required")
	}

	if memberRepository == nil {
		return nil, errors.New("member repository is required")
	}

	return &SyncContextUseCase{
		communityRepository: communityRepository,
		memberRepository:    memberRepository,
		clock:               time.Now,
	}, nil
}

func (uc *SyncContextUseCase) Execute(
	ctx context.Context,
	input SyncInteractionInput,
) (SyncedContext, error) {
	if uc == nil || uc.communityRepository == nil || uc.memberRepository == nil {
		return SyncedContext{}, errors.New("sync context use case is not initialized")
	}

	if ctx == nil {
		return SyncedContext{}, errors.New("context is required")
	}

	if err := ctx.Err(); err != nil {
		return SyncedContext{}, err
	}

	seenAt := input.SeenAt
	if seenAt.IsZero() {
		seenAt = uc.clock()
	}
	seenAt = seenAt.UTC()

	community, err := uc.communityRepository.UpsertInteraction(ctx, core.CommunityUpsertInput{
		GuildID: input.GuildID,
		Name:    input.GuildName,
		SeenAt:  seenAt,
	})
	if err != nil {
		return SyncedContext{}, fmt.Errorf("sync community metadata: %w", err)
	}

	member, err := uc.memberRepository.UpsertInteraction(ctx, core.MemberUpsertInput{
		GuildID:     input.GuildID,
		UserID:      input.UserID,
		DisplayName: input.DisplayName,
		Nickname:    input.Nickname,
		SeenAt:      seenAt,
	})
	if err != nil {
		return SyncedContext{}, fmt.Errorf("sync member metadata: %w", err)
	}

	return SyncedContext{
		Community: community,
		Member:    member,
	}, nil
}
