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

type channelKey struct {
	guildID   string
	channelID string
}

// MoodService manages persistent CommunityMood (MongoDB) and in-memory
// TemporaryMood scoped per (guild_id, channel_id).
type MoodService struct {
	repository core.CommunityMoodRepository
	clock      func() time.Time

	guildLocksMu sync.Mutex
	guildLocks   map[string]*sync.Mutex

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
		repository:    repository,
		clock:         time.Now,
		guildLocks:    make(map[string]*sync.Mutex),
		temporaryMood: make(map[channelKey]core.TemporaryMood),
	}, nil
}

// ResolveForInteraction loads & decays the guild's CommunityMood, evaluates deterministic
// interaction signals from triggerContent, updates persistent and temporary mood if signaled,
// and returns the combined EffectiveMood.
func (s *MoodService) ResolveForInteraction(
	ctx context.Context,
	guildID string,
	channelID string,
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

	if guildID == "" {
		return core.EffectiveMood{}, core.ErrInvalidGuildID
	}

	if channelID == "" {
		return core.EffectiveMood{}, core.ErrInvalidChannelID
	}

	now := s.clock().UTC()

	// Serialize per-guild mood load -> decay -> transition -> save to prevent races.
	guildMu := s.lockForGuild(guildID)
	guildMu.Lock()
	defer guildMu.Unlock()

	community, needsPersist, err := s.loadAndDecayCommunityMood(ctx, guildID, now)
	if err != nil {
		return core.EffectiveMood{}, err
	}

	signal := core.DetectSignal(triggerContent)

	if nextCommunity, changed := core.ApplySignalToCommunity(community, signal, now); changed {
		community = nextCommunity
		needsPersist = true
	}

	if needsPersist {
		if err := s.repository.Save(ctx, community); err != nil {
			return core.EffectiveMood{}, fmt.Errorf("persist community mood: %w", err)
		}
	}

	tempMood := s.updateTemporaryMood(guildID, channelID, signal, now)

	return core.CombineMoods(community, tempMood, now), nil
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

	var current *core.TemporaryMood
	if entry, exists := s.temporaryMood[key]; exists {
		if entry.IsExpired(now) {
			delete(s.temporaryMood, key)
		} else {
			copied := entry
			current = &copied
		}
	}

	next, changed := core.ApplySignalToTemporary(guildID, channelID, current, signal, now)
	if changed && next != nil {
		s.temporaryMood[key] = *next
		copied := *next
		return &copied
	}

	return current
}

func (s *MoodService) lockForGuild(guildID string) *sync.Mutex {
	s.guildLocksMu.Lock()
	defer s.guildLocksMu.Unlock()

	mu, exists := s.guildLocks[guildID]
	if !exists {
		mu = &sync.Mutex{}
		s.guildLocks[guildID] = mu
	}

	return mu
}
