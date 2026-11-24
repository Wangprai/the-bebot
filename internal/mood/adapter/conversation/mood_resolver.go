package conversation

import (
	"context"
	"fmt"

	conversationcore "the-bebot/internal/conversation/core"
	moodapp "the-bebot/internal/mood/app"
)

type MoodResolverAdapter struct {
	service *moodapp.MoodService
}

func NewMoodResolverAdapter(
	service *moodapp.MoodService,
) *MoodResolverAdapter {
	return &MoodResolverAdapter{
		service: service,
	}
}

func (a *MoodResolverAdapter) ResolveMood(
	ctx context.Context,
	guildID string,
	channelID string,
	triggerContent string,
) (conversationcore.MoodContext, error) {
	if a == nil || a.service == nil {
		return conversationcore.MoodContext{}, fmt.Errorf("mood service is nil")
	}

	effective, err := a.service.ResolveForInteraction(
		ctx,
		guildID,
		channelID,
		triggerContent,
	)
	if err != nil {
		return conversationcore.MoodContext{}, fmt.Errorf("resolve mood: %w", err)
	}

	moodCtx := conversationcore.MoodContext{
		EffectiveCategory:  string(effective.EffectiveCategory),
		EffectiveIntensity: effective.EffectiveIntensity,
		CommunityCategory:  string(effective.Community.Category),
		CommunityIntensity: effective.Community.Intensity,
	}

	if effective.Temporary != nil {
		moodCtx.TemporaryCategory = string(effective.Temporary.Category)
		moodCtx.TemporaryIntensity = effective.Temporary.Intensity
	}

	return moodCtx, nil
}

var _ conversationcore.MoodResolver = (*MoodResolverAdapter)(nil)
