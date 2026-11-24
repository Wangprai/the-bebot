package mongodb

import (
	"fmt"
	"time"

	"the-bebot/internal/mood/core"
)

type communityMoodDocument struct {
	GuildID   string    `bson:"guild_id"`
	Category  string    `bson:"category"`
	Intensity int       `bson:"intensity"`
	CreatedAt time.Time `bson:"created_at"`
	UpdatedAt time.Time `bson:"updated_at"`
}

func toCommunityMoodDocument(mood core.CommunityMood) (communityMoodDocument, error) {
	validated, err := core.NewCommunityMood(mood)
	if err != nil {
		return communityMoodDocument{}, fmt.Errorf("validate community mood: %w", err)
	}

	return communityMoodDocument{
		GuildID:   validated.GuildID,
		Category:  string(validated.Category),
		Intensity: validated.Intensity,
		CreatedAt: validated.CreatedAt,
		UpdatedAt: validated.UpdatedAt,
	}, nil
}

func (d communityMoodDocument) toDomain() (core.CommunityMood, error) {
	return core.NewCommunityMood(core.CommunityMood{
		GuildID:   d.GuildID,
		Category:  core.Category(d.Category),
		Intensity: d.Intensity,
		CreatedAt: d.CreatedAt,
		UpdatedAt: d.UpdatedAt,
	})
}
