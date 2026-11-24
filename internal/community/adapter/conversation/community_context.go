package conversation

import (
	"context"
	"fmt"

	communityapp "the-bebot/internal/community/app"
	conversationcore "the-bebot/internal/conversation/core"
)

type CommunityContextAdapter struct {
	useCase *communityapp.SyncContextUseCase
}

func NewCommunityContextAdapter(
	useCase *communityapp.SyncContextUseCase,
) *CommunityContextAdapter {
	return &CommunityContextAdapter{
		useCase: useCase,
	}
}

func (a *CommunityContextAdapter) SyncMentionContext(
	ctx context.Context,
	event conversationcore.MentionEvent,
) (conversationcore.CommunityContext, error) {
	if a == nil || a.useCase == nil {
		return conversationcore.CommunityContext{}, fmt.Errorf("community sync use case is nil")
	}

	synced, err := a.useCase.Execute(ctx, communityapp.SyncInteractionInput{
		GuildID:     event.GuildID,
		GuildName:   event.GuildName,
		UserID:      event.UserID,
		DisplayName: event.UserDisplayName,
		Nickname:    event.UserNickname,
		SeenAt:      event.MentionedAt,
	})
	if err != nil {
		return conversationcore.CommunityContext{}, fmt.Errorf("sync community context: %w", err)
	}

	return conversationcore.CommunityContext{
		GuildID:          synced.Community.GuildID,
		GuildName:        synced.Community.Name,
		PrimaryLanguage:  synced.Community.PrimaryLanguage,
		MemberUserID:     synced.Member.UserID,
		MemberName:       synced.Member.DisplayName,
		MemberNickname:   synced.Member.Nickname,
		InteractionCount: synced.Member.InteractionCount,
	}, nil
}

func (a *CommunityContextAdapter) RecordInteraction(
	ctx context.Context,
	event conversationcore.MentionEvent,
) error {
	if a == nil || a.useCase == nil {
		return fmt.Errorf("community sync use case is nil")
	}

	if err := a.useCase.RecordCompletedInteraction(
		ctx,
		event.GuildID,
		event.UserID,
		event.MentionedAt,
	); err != nil {
		return fmt.Errorf("record community interaction: %w", err)
	}

	return nil
}

var _ conversationcore.CommunityContextSyncer = (*CommunityContextAdapter)(nil)
