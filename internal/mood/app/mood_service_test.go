package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"the-bebot/internal/mood/core"
)

type inMemoryCommunityMoodRepo struct {
	mu      sync.Mutex
	moods   map[string]core.CommunityMood
	findErr error
	saveErr error
	saves   int
}

func newInMemoryCommunityMoodRepo() *inMemoryCommunityMoodRepo {
	return &inMemoryCommunityMoodRepo{
		moods: make(map[string]core.CommunityMood),
	}
}

func (r *inMemoryCommunityMoodRepo) FindByGuildID(
	ctx context.Context,
	guildID string,
) (*core.CommunityMood, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.findErr != nil {
		return nil, r.findErr
	}

	mood, exists := r.moods[guildID]
	if !exists {
		return nil, nil
	}

	copied := mood
	return &copied, nil
}

func (r *inMemoryCommunityMoodRepo) Save(
	ctx context.Context,
	mood core.CommunityMood,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.saveErr != nil {
		return r.saveErr
	}

	r.moods[mood.GuildID] = mood
	r.saves++
	return nil
}

func TestMoodServiceInitializesDefaultAndSurvivesNeutralInteraction(t *testing.T) {
	repo := newInMemoryCommunityMoodRepo()
	service, err := NewMoodService(repo)
	if err != nil {
		t.Fatalf("NewMoodService() error = %v", err)
	}

	now := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	service.clock = func() time.Time { return now }

	// First neutral interaction initializes default (playful, 2) with no temporary mood.
	effective, err := service.ResolveForInteraction(
		context.Background(),
		"guild-1",
		"channel-1",
		"<@bebot> what is the weather?",
	)
	if err != nil {
		t.Fatalf("ResolveForInteraction() error = %v", err)
	}

	if effective.EffectiveCategory != core.CategoryPlayful || effective.EffectiveIntensity != 2 {
		t.Fatalf("effective = (%s, %d), want (playful, 2)", effective.EffectiveCategory, effective.EffectiveIntensity)
	}
	if effective.Temporary != nil {
		t.Fatalf("neutral interaction must not create temporary mood, got %+v", effective.Temporary)
	}
	if repo.saves != 1 {
		t.Fatalf("expected 1 initial save, got %d", repo.saves)
	}

	// Second neutral interaction shortly after must NOT mutate or rewrite mood.
	service.clock = func() time.Time { return now.Add(time.Minute) }
	effective2, err := service.ResolveForInteraction(
		context.Background(),
		"guild-1",
		"channel-1",
		"<@bebot> tell me about Go",
	)
	if err != nil {
		t.Fatalf("second ResolveForInteraction() error = %v", err)
	}
	if effective2.EffectiveCategory != core.CategoryPlayful || effective2.EffectiveIntensity != 2 {
		t.Errorf("unexpected mood change on neutral message: %+v", effective2)
	}
	if repo.saves != 1 {
		t.Errorf("expected no extra save on unchanged neutral interaction, got %d saves", repo.saves)
	}
}

func TestMoodServiceTemporaryChannelIsolationAndTTLExpiration(t *testing.T) {
	repo := newInMemoryCommunityMoodRepo()
	service, _ := NewMoodService(repo)

	now := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	service.clock = func() time.Time { return now }

	// Sarcastic cue in channel-1 sets temporary mood for channel-1.
	eff1, err := service.ResolveForInteraction(
		context.Background(),
		"guild-1",
		"channel-1",
		"<@bebot> yeah right, sure buddy",
	)
	if err != nil {
		t.Fatalf("ResolveForInteraction() error = %v", err)
	}
	if eff1.Temporary == nil || eff1.Temporary.Category != core.CategorySarcastic || eff1.Temporary.Intensity != 3 {
		t.Fatalf("expected temporary sarcastic(3), got %+v", eff1.Temporary)
	}

	// Channel-2 in same guild must NOT inherit channel-1's temporary mood (sees community mood only).
	effCh2, err := service.ResolveForInteraction(
		context.Background(),
		"guild-1",
		"channel-2",
		"<@bebot> normal question",
	)
	if err != nil {
		t.Fatalf("channel-2 ResolveForInteraction() error = %v", err)
	}
	if effCh2.Temporary != nil {
		t.Fatalf("channel-2 should not have channel-1 temporary mood: %+v", effCh2.Temporary)
	}
	if effCh2.EffectiveCategory != core.CategorySarcastic || effCh2.EffectiveIntensity != 2 {
		t.Errorf("channel-2 should see community mood sarcastic(2), got (%s, %d)", effCh2.EffectiveCategory, effCh2.EffectiveIntensity)
	}

	// After 15 minutes, channel-1 temporary mood expires.
	service.clock = func() time.Time { return now.Add(core.TemporaryMoodTTL) }
	effExpired, err := service.ResolveForInteraction(
		context.Background(),
		"guild-1",
		"channel-1",
		"<@bebot> still here?",
	)
	if err != nil {
		t.Fatalf("expired ResolveForInteraction() error = %v", err)
	}
	if effExpired.Temporary != nil {
		t.Fatalf("expected temporary mood to expire after 15m, got %+v", effExpired.Temporary)
	}
}

func TestMoodServiceApplies6HourDecay(t *testing.T) {
	repo := newInMemoryCommunityMoodRepo()
	base := time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)

	repo.moods["guild-1"] = core.CommunityMood{
		GuildID:   "guild-1",
		Category:  core.CategoryCheerful,
		Intensity: 3,
		CreatedAt: base,
		UpdatedAt: base,
	}

	service, _ := NewMoodService(repo)
	service.clock = func() time.Time { return base.Add(12 * time.Hour) }

	eff, err := service.ResolveForInteraction(
		context.Background(),
		"guild-1",
		"channel-1",
		"<@bebot> neutral check",
	)
	if err != nil {
		t.Fatalf("ResolveForInteraction() error = %v", err)
	}

	if eff.Community.Category != core.CategoryCheerful || eff.Community.Intensity != 1 {
		t.Fatalf("expected decayed cheerful(1), got (%s, %d)", eff.Community.Category, eff.Community.Intensity)
	}

	// After another 6 hours, intensity falls below 1 -> resets to default playful(2).
	service.clock = func() time.Time { return base.Add(18 * time.Hour) }
	effReset, err := service.ResolveForInteraction(
		context.Background(),
		"guild-1",
		"channel-1",
		"<@bebot> neutral check",
	)
	if err != nil {
		t.Fatalf("reset ResolveForInteraction() error = %v", err)
	}
	if effReset.Community.Category != core.CategoryPlayful || effReset.Community.Intensity != 2 {
		t.Fatalf("expected reset playful(2), got (%s, %d)", effReset.Community.Category, effReset.Community.Intensity)
	}
}

func TestMoodServiceConcurrentAccess(t *testing.T) {
	repo := newInMemoryCommunityMoodRepo()
	service, _ := NewMoodService(repo)

	var wg sync.WaitGroup
	errCh := make(chan error, 20)

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			msg := "<@bebot> haha 555"
			if i%2 == 0 {
				msg = "<@bebot> hello thanks"
			}
			_, err := service.ResolveForInteraction(
				context.Background(),
				"guild-concurrent",
				"channel-concurrent",
				msg,
			)
			if err != nil {
				errCh <- err
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatalf("concurrent ResolveForInteraction() error = %v", err)
	}
}

func TestMoodServiceErrorHandling(t *testing.T) {
	if _, err := NewMoodService(nil); err == nil {
		t.Fatal("expected error for nil repository")
	}

	repo := newInMemoryCommunityMoodRepo()
	repo.findErr = errors.New("db down")
	service, _ := NewMoodService(repo)

	if _, err := service.ResolveForInteraction(context.Background(), "guild-1", "ch-1", "hi"); err == nil {
		t.Fatal("expected repository find error")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.ResolveForInteraction(ctx, "guild-1", "ch-1", "hi"); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}
