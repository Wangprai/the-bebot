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
		"user-1",
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
		"user-1",
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
		"user-1",
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
		"user-2",
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
		"user-1",
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
		"user-1",
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
		"user-1",
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
				"user-"+string(rune('a'+(i%5))),
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

	if got := service.guildLockCount(); got != 0 {
		t.Errorf("guildLockCount() after concurrent calls = %d, want 0", got)
	}
}

func TestMoodServiceErrorHandling(t *testing.T) {
	if _, err := NewMoodService(nil); err == nil {
		t.Fatal("expected error for nil repository")
	}

	repo := newInMemoryCommunityMoodRepo()
	repo.findErr = errors.New("db down")
	service, _ := NewMoodService(repo)

	if _, err := service.ResolveForInteraction(context.Background(), "guild-1", "ch-1", "user-1", "hi"); err == nil {
		t.Fatal("expected repository find error")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.ResolveForInteraction(ctx, "guild-1", "ch-1", "user-1", "hi"); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestMoodServicePerUserCooldownProtectsCommunityMoodWhileUpdatingTemporary(t *testing.T) {
	repo := newInMemoryCommunityMoodRepo()
	service, err := NewMoodService(repo)
	if err != nil {
		t.Fatalf("NewMoodService() error = %v", err)
	}

	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	current := now
	if err := service.SetClock(nil); err == nil {
		t.Fatal("expected error for nil clock")
	}
	if err := service.SetClock(func() time.Time { return current }); err != nil {
		t.Fatalf("SetClock() error = %v", err)
	}
	if err := service.SetUserCooldown(-time.Second); err == nil {
		t.Fatal("expected error for negative cooldown")
	}
	if err := service.SetUserCooldown(2 * time.Minute); err != nil {
		t.Fatalf("SetUserCooldown() error = %v", err)
	}

	// 1. User A changes persistent community mood to cheerful(2).
	eff1, err := service.ResolveForInteraction(context.Background(), "guild-1", "ch-1", "user-a", "thanks awesome")
	if err != nil {
		t.Fatalf("first ResolveForInteraction() error = %v", err)
	}
	if eff1.Community.Category != core.CategoryCheerful || eff1.Community.Intensity != 2 {
		t.Fatalf("community = (%s, %d), want (cheerful, 2)", eff1.Community.Category, eff1.Community.Intensity)
	}

	// 2. User A sends hostile signal 30s later (inside 2m cooldown):
	//    persistent community mood MUST NOT change, while temporary channel mood updates to annoyed(3).
	current = now.Add(30 * time.Second)
	eff2, err := service.ResolveForInteraction(context.Background(), "guild-1", "ch-1", "user-a", "shut up stupid")
	if err != nil {
		t.Fatalf("second ResolveForInteraction() error = %v", err)
	}
	if eff2.Community.Category != core.CategoryCheerful || eff2.Community.Intensity != 2 {
		t.Errorf("community mood mutated during user cooldown: (%s, %d)", eff2.Community.Category, eff2.Community.Intensity)
	}
	if eff2.Temporary == nil || eff2.Temporary.Category != core.CategoryAnnoyed || eff2.Temporary.Intensity != 3 {
		t.Errorf("temporary mood should still update during cooldown, got %+v", eff2.Temporary)
	}

	// 3. Different user (User B) at 45s CAN change persistent community mood.
	current = now.Add(45 * time.Second)
	eff3, err := service.ResolveForInteraction(context.Background(), "guild-1", "ch-2", "user-b", "shut up")
	if err != nil {
		t.Fatalf("user-b ResolveForInteraction() error = %v", err)
	}
	if eff3.Community.Category != core.CategoryCheerful || eff3.Community.Intensity != 1 {
		t.Errorf("expected user-b to soften community mood to cheerful(1), got (%s, %d)", eff3.Community.Category, eff3.Community.Intensity)
	}

	// 4. User A after 2m cooldown elapses CAN update persistent community mood again.
	current = now.Add(2*time.Minute + time.Second)
	eff4, err := service.ResolveForInteraction(context.Background(), "guild-1", "ch-1", "user-a", "yeah right")
	if err != nil {
		t.Fatalf("post-cooldown ResolveForInteraction() error = %v", err)
	}
	if eff4.Community.Category != core.CategorySarcastic || eff4.Community.Intensity != 2 {
		t.Errorf("expected post-cooldown community mood sarcastic(2), got (%s, %d)", eff4.Community.Category, eff4.Community.Intensity)
	}
}

func TestMoodServiceEvictsExpiredTemporaryMoodsAndIdleGuildLocks(t *testing.T) {
	repo := newInMemoryCommunityMoodRepo()
	service, _ := NewMoodService(repo)

	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	current := now
	_ = service.SetClock(func() time.Time { return current })

	// Create temporary moods across two channels/guilds.
	_, _ = service.ResolveForInteraction(context.Background(), "guild-a", "ch-1", "u1", "thanks")
	_, _ = service.ResolveForInteraction(context.Background(), "guild-b", "ch-2", "u2", "haha")

	if got := service.temporaryMoodCount(); got != 2 {
		t.Fatalf("temporaryMoodCount() = %d, want 2", got)
	}
	if got := service.guildLockCount(); got != 0 {
		t.Fatalf("guildLockCount() when idle = %d, want 0", got)
	}

	// Advance past 15m TTL and interact in a third channel; expired entries must be evicted.
	current = now.Add(core.TemporaryMoodTTL + time.Minute)
	_, _ = service.ResolveForInteraction(context.Background(), "guild-c", "ch-3", "u3", "neutral question")

	if got := service.temporaryMoodCount(); got != 0 {
		t.Fatalf("temporaryMoodCount() after TTL eviction = %d, want 0", got)
	}
	if got := service.guildLockCount(); got != 0 {
		t.Fatalf("guildLockCount() after idle = %d, want 0", got)
	}
}
