package core

import "context"

// CommunityMoodRepository persists guild-level CommunityMood records.
type CommunityMoodRepository interface {
	FindByGuildID(
		ctx context.Context,
		guildID string,
	) (*CommunityMood, error)

	Save(
		ctx context.Context,
		mood CommunityMood,
	) error
}
