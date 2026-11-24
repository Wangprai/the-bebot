package core

import (
	"testing"
	"time"
)

func TestRankMemoriesPrefersTopicMatchAndConfidenceAndRecency(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)

	candidates := []ScoredMemory{
		{
			Memory: Memory{
				ID:         "unrelated-high-imp",
				GuildID:    "g1",
				UserID:     "u1",
				Scope:      MemoryScopeMember,
				Type:       MemoryTypePersonalFact,
				Content:    "Works as a backend engineer",
				Importance: 5,
				Confidence: 0.95,
				Tags:       []string{"career"},
				UpdatedAt:  now,
			},
		},
		{
			Memory: Memory{
				ID:         "topic-match-med-imp",
				GuildID:    "g1",
				UserID:     "u1",
				Scope:      MemoryScopeMember,
				Type:       MemoryTypePreference,
				Content:    "Loves drinking dark roast espresso every morning",
				Importance: 3,
				Confidence: 0.85,
				Tags:       []string{"coffee", "espresso"},
				UpdatedAt:  now.Add(-48 * time.Hour),
			},
			TextScore: 3.0,
		},
		{
			Memory: Memory{
				ID:         "low-confidence",
				GuildID:    "g1",
				UserID:     "u1",
				Scope:      MemoryScopeMember,
				Type:       MemoryTypePreference,
				Content:    "Maybe likes espresso sometimes",
				Importance: 5,
				Confidence: 0.30,
				Tags:       []string{"espresso"},
				UpdatedAt:  now,
			},
			TextScore: 4.0,
		},
	}

	ranked := RankMemories(candidates, RetrievalQuery{
		GuildID:                   "g1",
		UserID:                    "u1",
		QueryText:                 "<@12345> what espresso should I brew today?",
		Limit:                     5,
		MinConfidence:             0.5,
		MinCommunityCorroborators: 2,
		Now:                       now,
	})

	if len(ranked) != 2 {
		t.Fatalf("ranked count = %d, want 2 (low-confidence excluded)", len(ranked))
	}
	if ranked[0].ID != "topic-match-med-imp" {
		t.Errorf("ranked[0].ID = %q, want topic-match-med-imp", ranked[0].ID)
	}
	if ranked[1].ID != "unrelated-high-imp" {
		t.Errorf("ranked[1].ID = %q, want unrelated-high-imp", ranked[1].ID)
	}
}

func TestRankMemoriesFallbackToImportanceConfidenceRecencyWithoutTextMatch(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)

	candidates := []ScoredMemory{
		{
			Memory: Memory{
				ID:         "older-low-conf",
				Scope:      MemoryScopeMember,
				UserID:     "u1",
				Content:    "Likes hiking",
				Importance: 4,
				Confidence: 0.6,
				UpdatedAt:  now.Add(-30 * 24 * time.Hour),
			},
		},
		{
			Memory: Memory{
				ID:         "newer-high-conf",
				Scope:      MemoryScopeMember,
				UserID:     "u1",
				Content:    "Likes Go",
				Importance: 4,
				Confidence: 0.95,
				UpdatedAt:  now,
			},
		},
		{
			Memory: Memory{
				ID:         "highest-importance",
				Scope:      MemoryScopeMember,
				UserID:     "u1",
				Content:    "Allergic to peanuts",
				Importance: 5,
				Confidence: 0.9,
				UpdatedAt:  now.Add(-2 * 24 * time.Hour),
			},
		},
	}

	ranked := RankMemories(candidates, RetrievalQuery{
		GuildID:       "g1",
		UserID:        "u1",
		QueryText:     "completely unrelated topic xyz",
		Limit:         3,
		MinConfidence: 0.5,
		Now:           now,
	})

	if len(ranked) != 3 {
		t.Fatalf("ranked count = %d, want 3", len(ranked))
	}
	if ranked[0].ID != "highest-importance" || ranked[1].ID != "newer-high-conf" || ranked[2].ID != "older-low-conf" {
		t.Errorf("unexpected fallback order: %q, %q, %q", ranked[0].ID, ranked[1].ID, ranked[2].ID)
	}
}

func TestRankMemoriesEnforcesCommunityCorroborationAndExpiry(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour)
	future := now.Add(24 * time.Hour)

	candidates := []ScoredMemory{
		{
			Memory: Memory{
				ID:            "single-user-community",
				Scope:         MemoryScopeCommunity,
				Content:       "Unverified guild rumor",
				Importance:    5,
				Confidence:    0.9,
				SourceUserIDs: []string{"user-1"},
				UpdatedAt:     now,
			},
		},
		{
			Memory: Memory{
				ID:            "corroborated-community",
				Scope:         MemoryScopeCommunity,
				Content:       "Friday night game night",
				Importance:    4,
				Confidence:    0.85,
				SourceUserIDs: []string{"user-1", "user-2"},
				UpdatedAt:     now,
			},
		},
		{
			Memory: Memory{
				ID:         "expired-member",
				Scope:      MemoryScopeMember,
				UserID:     "user-1",
				Content:    "Old unused note",
				Importance: 1,
				Confidence: 0.8,
				ExpiresAt:  &past,
				UpdatedAt:  now,
			},
		},
		{
			Memory: Memory{
				ID:         "active-member",
				Scope:      MemoryScopeMember,
				UserID:     "user-1",
				Content:    "Still valid note",
				Importance: 2,
				Confidence: 0.8,
				ExpiresAt:  &future,
				UpdatedAt:  now,
			},
		},
	}

	ranked := RankMemories(candidates, RetrievalQuery{
		GuildID:                   "g1",
		UserID:                    "user-1",
		Limit:                     10,
		MinConfidence:             0.5,
		MinCommunityCorroborators: 2,
		Now:                       now,
	})

	if len(ranked) != 2 {
		t.Fatalf("ranked count = %d, want 2 (%+v)", len(ranked), ranked)
	}
	if ranked[0].ID != "corroborated-community" || ranked[1].ID != "active-member" {
		t.Errorf("unexpected filtered memories: %q, %q", ranked[0].ID, ranked[1].ID)
	}

	// Explicit high-confidence bypass allows single-user community memory when configured.
	rankedWithBypass := RankMemories(candidates, RetrievalQuery{
		GuildID:                   "g1",
		UserID:                    "user-1",
		Limit:                     10,
		MinConfidence:             0.5,
		MinCommunityCorroborators: 2,
		CommunityHighConfidence:   0.9,
		Now:                       now,
	})
	if len(rankedWithBypass) != 3 {
		t.Fatalf("rankedWithBypass count = %d, want 3", len(rankedWithBypass))
	}
}

func TestNearDuplicateSimilarityBilingual(t *testing.T) {
	enA := Memory{Content: "Likes Go programming for backend services"}
	enB := Memory{Content: "Likes Go programming for backend service"}
	enDiff := Memory{Content: "Prefers hiking in northern mountains"}

	if sim := NearDuplicateSimilarity(enA, enB); sim < 0.70 {
		t.Errorf("English near-duplicate similarity = %.3f, want >= 0.70", sim)
	}
	if sim := NearDuplicateSimilarity(enA, enDiff); sim >= 0.50 {
		t.Errorf("English unrelated similarity = %.3f, want < 0.50", sim)
	}

	thA := Memory{Content: "ชอบเขียนโปรแกรมด้วยภาษา Go มาก"}
	thB := Memory{Content: "ชอบเขียนโปรแกรมภาษา Go มาก"}
	thDiff := Memory{Content: "ชอบกินข้าวซอยไก่ตอนเที่ยง"}

	if sim := NearDuplicateSimilarity(thA, thB); sim < 0.70 {
		t.Errorf("Thai near-duplicate similarity = %.3f, want >= 0.70", sim)
	}
	if sim := NearDuplicateSimilarity(thA, thDiff); sim >= 0.50 {
		t.Errorf("Thai unrelated similarity = %.3f, want < 0.50", sim)
	}
}
