package mongodb

import (
	"testing"

	"the-bebot/internal/memory/core"
)

func TestNewMemoryContentDedupKey(t *testing.T) {
	base := core.Memory{
		GuildID: "guild-1",
		UserID:  "user-1",
		Scope:   core.MemoryScopeMember,
		Type:    core.MemoryTypePreference,
		Content: "I'm a big fan of Go programming",
	}

	sameNormalized := base
	sameNormalized.Content = "  i\u2019m   a big\tfan of go programming "
	sameNormalized.CandidateID = "cand-2"
	sameNormalized.SourceMessageID = "msg-2"

	if got, want := newMemoryContentDedupKey(sameNormalized), newMemoryContentDedupKey(base); got != want {
		t.Errorf("expected same content dedup key for normalized/curly-apostrophe content: got %q, want %q", got, want)
	}

	diffUser := base
	diffUser.UserID = "user-2"
	if newMemoryContentDedupKey(diffUser) == newMemoryContentDedupKey(base) {
		t.Error("expected different content dedup key for different member user")
	}

	diffGuild := base
	diffGuild.GuildID = "guild-2"
	if newMemoryContentDedupKey(diffGuild) == newMemoryContentDedupKey(base) {
		t.Error("expected different content dedup key for different guild")
	}

	diffType := base
	diffType.Type = core.MemoryTypePersonalFact
	if newMemoryContentDedupKey(diffType) == newMemoryContentDedupKey(base) {
		t.Error("expected different content dedup key for different memory type")
	}

	communityA := core.Memory{
		GuildID: "guild-1",
		Scope:   core.MemoryScopeCommunity,
		Type:    core.MemoryTypeRunningJoke,
		Content: "Everyone loves coffee",
	}
	communityB := core.Memory{
		GuildID: "guild-1",
		UserID:  "ignored-for-community",
		Scope:   core.MemoryScopeCommunity,
		Type:    core.MemoryTypeRunningJoke,
		Content: "  EVERYONE   LOVES COFFEE ",
	}
	if newMemoryContentDedupKey(communityA) != newMemoryContentDedupKey(communityB) {
		t.Error("expected same content dedup key for community memories with same normalized content")
	}
}
