package conversation

import (
	"context"
	"testing"

	moodapp "the-bebot/internal/mood/app"
	moodcore "the-bebot/internal/mood/core"
)

type stubMoodRepo struct {
	moods map[string]moodcore.CommunityMood
}

func (r *stubMoodRepo) FindByGuildID(ctx context.Context, guildID string) (*moodcore.CommunityMood, error) {
	m, ok := r.moods[guildID]
	if !ok {
		return nil, nil
	}
	copied := m
	return &copied, nil
}

func (r *stubMoodRepo) Save(ctx context.Context, mood moodcore.CommunityMood) error {
	r.moods[mood.GuildID] = mood
	return nil
}

func TestMoodResolverAdapterSuccess(t *testing.T) {
	service, err := moodapp.NewMoodService(&stubMoodRepo{
		moods: make(map[string]moodcore.CommunityMood),
	})
	if err != nil {
		t.Fatalf("NewMoodService() error = %v", err)
	}

	adapter := NewMoodResolverAdapter(service)

	moodCtx, err := adapter.ResolveMood(
		context.Background(),
		"guild-001",
		"channel-001",
		"<@bebot> thanks, awesome!",
	)
	if err != nil {
		t.Fatalf("ResolveMood() error = %v", err)
	}

	if moodCtx.EffectiveCategory != "cheerful" || moodCtx.EffectiveIntensity != 3 {
		t.Errorf("unexpected effective mood: %+v", moodCtx)
	}

	if moodCtx.CommunityCategory != "cheerful" || moodCtx.TemporaryCategory != "cheerful" {
		t.Errorf("unexpected mood components: %+v", moodCtx)
	}
}

func TestMoodResolverAdapterRejectsNil(t *testing.T) {
	var nilAdapter *MoodResolverAdapter
	if _, err := nilAdapter.ResolveMood(context.Background(), "g1", "c1", "hi"); err == nil {
		t.Fatal("expected error for nil adapter")
	}

	adapter := NewMoodResolverAdapter(nil)
	if _, err := adapter.ResolveMood(context.Background(), "g1", "c1", "hi"); err == nil {
		t.Fatal("expected error for nil service")
	}
}
