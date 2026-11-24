package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"the-bebot/internal/mood/core"
)

const defaultMoodUserCooldown = 2 * time.Minute

type channelKey struct {
	guildID   string
	channelID string
}

type userGuildKey struct {
	guildID string
	userID  string
}

type guildLockEntry struct {
	mu   sync.Mutex
	refs int
}

// MoodService manages persistent CommunityMood (MongoDB) and in-memory
// TemporaryMood scoped per (guild_id, channel_id).
type MoodService struct {
	repository   core.CommunityMoodRepository
	clock        func() time.Time
	userCooldown time.Duration

	guildLocksMu sync.Mutex
	guildLocks   map[string]*guildLockEntry

	userCooldownMu          sync.Mutex
	userLastCommunityUpdate map[userGuildKey]time.Time
	lastCooldownSweep       time.Time

	tempMu        sync.RWMutex
	temporaryMood map[channelKey]core.TemporaryMood
}

func NewMoodService(
	repository core.CommunityMoodRepository,
) (*MoodService, error) {
	if repository == nil {
		return nil, errors.New("community mood repository is required")
	}

	return &MoodService{
		repository:              repository,
		clock:                   time.Now,
		userCooldown:            defaultMoodUserCooldown,
		guildLocks:              make(map[string]*guildLockEntry),
		userLastCommunityUpdate: make(map[userGuildKey]time.Time),
		temporaryMood:           make(map[channelKey]core.TemporaryMood),
	}, nil
}

func (s *MoodService) SetClock(clock func() time.Time) error {
	if s == nil {
		return errors.New("mood service is not initialized")
	}

	if clock == nil {
		return errors.New("clock is required")
	}

	s.clock = clock
	return nil
}

func (s *MoodService) SetUserCooldown(cooldown time.Duration) error {
	if s == nil {
		return errors.New("mood service is not initialized")
	}

	if cooldown < 0 {
		return errors.New("mood user cooldown must be zero or greater")
	}

	s.userCooldownMu.Lock()
	s.userCooldown = cooldown
	s.userCooldownMu.Unlock()

	return nil
}

// ResolveForInteraction loads & decays the guild's CommunityMood, evaluates deterministic
// interaction signals from triggerContent, updates persistent and temporary mood if signaled,
// and returns the combined EffectiveMood.
func (s *MoodService) ResolveForInteraction(
	ctx context.Context,
	guildID string,
	channelID string,
	userID string,
	triggerContent string,
) (core.EffectiveMood, error) {
	if s == nil || s.repository == nil {
		return core.EffectiveMood{}, errors.New("mood service is not initialized")
	}

	if ctx == nil {
		return core.EffectiveMood{}, errors.New("context is required")
	}

	if err := ctx.Err(); err != nil {
		return core.EffectiveMood{}, err
	}

	guildID = strings.TrimSpace(guildID)
	channelID = strings.TrimSpace(channelID)
	userID = strings.TrimSpace(userID)

	if guildID == "" {
		return core.EffectiveMood{}, core.ErrInvalidGuildID
	}

	if channelID == "" {
		return core.EffectiveMood{}, core.ErrInvalidChannelID
	}

	now := s.now().UTC()

	// Serialize per-guild mood load -> decay -> transition -> save and release idle locks.
	unlockGuild := s.acquireGuildLock(guildID)
	defer unlockGuild()

	community, needsPersist, err := s.loadAndDecayCommunityMood(ctx, guildID, now)
	if err != nil {
		return core.EffectiveMood{}, err
	}

	signal := core.DetectSignal(triggerContent)
	communityMutatedByUser := false

	if signal != core.SignalNone && s.canUserUpdateCommunityMood(guildID, userID, now) {
		if nextCommunity, changed := core.ApplySignalToCommunity(community, signal, now); changed {
			community = nextCommunity
			needsPersist = true
			communityMutatedByUser = true
		}
	}

	if needsPersist {
		if err := s.repository.Save(ctx, community); err != nil {
			return core.EffectiveMood{}, fmt.Errorf("persist community mood: %w", err)
		}

		if communityMutatedByUser {
			s.recordUserCommunityMoodUpdate(guildID, userID, now)
		}
	}

	// Temporary channel mood may still update even when persistent community mood is on cooldown.
	tempMood := s.updateTemporaryMood(guildID, channelID, signal, now)

	return core.CombineMoods(community, tempMood, now), nil
}

func (s *MoodService) now() time.Time {
	if s != nil && s.clock != nil {
		return s.clock()
	}
	return time.Now()
}

func (s *MoodService) canUserUpdateCommunityMood(
	guildID string,
	userID string,
	now time.Time,
) bool {
	if userID == "" {
		return true
	}

	s.userCooldownMu.Lock()
	defer s.userCooldownMu.Unlock()

	if s.userCooldown <= 0 {
		return true
	}

	s.evictExpiredCooldownsLocked(now)

	last, exists := s.userLastCommunityUpdate[userGuildKey{guildID: guildID, userID: userID}]
	if exists && now.Sub(last) < s.userCooldown {
		return false
	}

	return true
}

func (s *MoodService) recordUserCommunityMoodUpdate(
	guildID string,
	userID string,
	now time.Time,
) {
	if userID == "" {
		return
	}

	s.userCooldownMu.Lock()
	defer s.userCooldownMu.Unlock()

	if s.userCooldown <= 0 {
		return
	}

	if s.userLastCommunityUpdate == nil {
		s.userLastCommunityUpdate = make(map[userGuildKey]time.Time)
	}

	s.evictExpiredCooldownsLocked(now)
	s.userLastCommunityUpdate[userGuildKey{guildID: guildID, userID: userID}] = now
}

func (s *MoodService) evictExpiredCooldownsLocked(now time.Time) {
	if s.userCooldown <= 0 {
		return
	}

	if !s.lastCooldownSweep.IsZero() && now.Sub(s.lastCooldownSweep) < s.userCooldown {
		return
	}

	for key, ts := range s.userLastCommunityUpdate {
		if now.Sub(ts) >= s.userCooldown {
			delete(s.userLastCommunityUpdate, key)
		}
	}

	s.lastCooldownSweep = now
}

func (s *MoodService) loadAndDecayCommunityMood(
	ctx context.Context,
	guildID string,
	now time.Time,
) (core.CommunityMood, bool, error) {
	stored, err := s.repository.FindByGuildID(ctx, guildID)
	if err != nil {
		return core.CommunityMood{}, false, fmt.Errorf("load community mood: %w", err)
	}

	if stored == nil {
		defaultMood, err := core.DefaultCommunityMood(guildID, now)
		if err != nil {
			return core.CommunityMood{}, false, err
		}
		return defaultMood, true, nil
	}

	decayed := core.ApplyDecay(*stored, now)
	if decayed.Category != stored.Category || decayed.Intensity != stored.Intensity {
		decayed.UpdatedAt = now
		return decayed, true, nil
	}

	return decayed, false, nil
}

func (s *MoodService) updateTemporaryMood(
	guildID string,
	channelID string,
	signal core.Signal,
	now time.Time,
) *core.TemporaryMood {
	key := channelKey{guildID: guildID, channelID: channelID}

	s.tempMu.Lock()
	defer s.tempMu.Unlock()

	// Evict all expired temporary moods so the map cannot grow unboundedly.
	for existingKey, entry := range s.temporaryMood {
		if entry.IsExpired(now) {
			delete(s.temporaryMood, existingKey)
		}
	}

	var current *core.TemporaryMood
	if entry, exists := s.temporaryMood[key]; exists {
		copied := entry
		current = &copied
	}

	next, changed := core.ApplySignalToTemporary(guildID, channelID, current, signal, now)
	if changed && next != nil {
		s.temporaryMood[key] = *next
		copied := *next
		return &copied
	}

	return current
}

func (s *MoodService) acquireGuildLock(guildID string) func() {
	s.guildLocksMu.Lock()
	entry, exists := s.guildLocks[guildID]
	if !exists {
		entry = &guildLockEntry{}
		s.guildLocks[guildID] = entry
	}
	entry.refs++
	s.guildLocksMu.Unlock()

	entry.mu.Lock()

	return func() {
		entry.mu.Unlock()

		s.guildLocksMu.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(s.guildLocks, guildID)
		}
		s.guildLocksMu.Unlock()
	}
}

func (s *MoodService) temporaryMoodCount() int {
	s.tempMu.RLock()
	defer s.tempMu.RUnlock()
	return len(s.temporaryMood)
}

func (s *MoodService) guildLockCount() int {
	s.guildLocksMu.Lock()
	defer s.guildLocksMu.Unlock()
	return len(s.guildLocks)
}
