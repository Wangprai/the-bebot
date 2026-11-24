package core

import (
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

const (
	DefaultMinConfidence             = 0.5
	DefaultMinCommunityCorroborators = 2
	DefaultUnusedTTLMaxImportance    = 2
	DefaultNearDuplicateThreshold    = 0.70
	defaultRecencyHalfLifeDays       = 30.0
	textScoreSaturationConstant      = 2.0
)

var queryMentionPattern = regexp.MustCompile(`<@!?[0-9A-Za-z_-]+>`)

// RetrievalQuery captures ranking, confidence, corroboration, and expiry parameters
// for retrieving relevant long-term memories.
type RetrievalQuery struct {
	GuildID                   string
	UserID                    string
	QueryText                 string
	Limit                     int
	MinConfidence             float64
	MinCommunityCorroborators int
	CommunityHighConfidence   float64
	Now                       time.Time
}

// ScoredMemory pairs a Memory with its optional MongoDB $text score prior to combined ranking.
type ScoredMemory struct {
	Memory    Memory
	TextScore float64
}

// CleanRetrievalQuery strips Discord mention tokens and collapses whitespace.
func CleanRetrievalQuery(raw string) string {
	stripped := queryMentionPattern.ReplaceAllString(raw, " ")
	return strings.TrimSpace(strings.Join(strings.Fields(stripped), " "))
}

// IsCorroboratedCommunityMemory reports whether a community-scope memory has enough
// distinct source users or satisfies an explicit high-confidence bypass threshold.
func IsCorroboratedCommunityMemory(
	memory Memory,
	minCorroborators int,
	highConfidence float64,
) bool {
	if memory.Scope != MemoryScopeCommunity {
		return true
	}

	if minCorroborators <= 1 {
		return true
	}

	if len(memory.SourceUserIDs) >= minCorroborators {
		return true
	}

	if highConfidence > 0 && memory.Confidence >= highConfidence {
		return true
	}

	return false
}

// IsMemoryExpired reports whether a memory has a non-nil ExpiresAt at or before now.
func IsMemoryExpired(memory Memory, now time.Time) bool {
	if memory.ExpiresAt == nil {
		return false
	}
	return !now.UTC().Before(memory.ExpiresAt.UTC())
}

// ScoreMemory computes a monotonic combined relevance score in [0, 1] combining:
//   - text/lexical relevance to queryTokens (0.50)
//   - importance 1..5 (0.25)
//   - confidence 0..1 (0.15)
//   - recency based on UpdatedAt relative to now (0.10)
func ScoreMemory(
	memory Memory,
	mongoTextScore float64,
	queryTokens []string,
	now time.Time,
) float64 {
	importanceNorm := float64(memory.Importance) / 5.0
	if importanceNorm < 0 {
		importanceNorm = 0
	}
	if importanceNorm > 1 {
		importanceNorm = 1
	}

	confidenceNorm := memory.Confidence
	if confidenceNorm < 0 {
		confidenceNorm = 0
	}
	if confidenceNorm > 1 {
		confidenceNorm = 1
	}

	ageDays := now.UTC().Sub(memory.UpdatedAt.UTC()).Hours() / 24.0
	if ageDays < 0 {
		ageDays = 0
	}
	recencyNorm := 1.0 / (1.0 + ageDays/defaultRecencyHalfLifeDays)

	var textNorm float64
	if mongoTextScore > 0 {
		textNorm = mongoTextScore / (mongoTextScore + textScoreSaturationConstant)
	}

	lexicalNorm := lexicalRelevance(memory, queryTokens)
	relevanceNorm := textNorm
	if lexicalNorm > relevanceNorm {
		relevanceNorm = lexicalNorm
	}

	return 0.50*relevanceNorm +
		0.25*importanceNorm +
		0.15*confidenceNorm +
		0.10*recencyNorm
}

// RankMemories filters candidates by confidence, corroboration, and expiry, ranks them
// by combined relevance score (with deterministic tie-breakers), and returns up to limit items.
func RankMemories(candidates []ScoredMemory, query RetrievalQuery) []Memory {
	if len(candidates) == 0 || query.Limit <= 0 {
		return []Memory{}
	}

	now := query.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	queryTokens := extractQueryTokens(CleanRetrievalQuery(query.QueryText))

	type ranked struct {
		memory Memory
		score  float64
	}

	items := make([]ranked, 0, len(candidates))
	for _, candidate := range candidates {
		mem := candidate.Memory
		if query.MinConfidence > 0 && mem.Confidence < query.MinConfidence {
			continue
		}
		if IsMemoryExpired(mem, now) {
			continue
		}
		if !IsCorroboratedCommunityMemory(mem, query.MinCommunityCorroborators, query.CommunityHighConfidence) {
			continue
		}

		items = append(items, ranked{
			memory: mem,
			score:  ScoreMemory(mem, candidate.TextScore, queryTokens, now),
		})
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].score != items[j].score {
			return items[i].score > items[j].score
		}
		if items[i].memory.Importance != items[j].memory.Importance {
			return items[i].memory.Importance > items[j].memory.Importance
		}
		if !items[i].memory.UpdatedAt.Equal(items[j].memory.UpdatedAt) {
			return items[i].memory.UpdatedAt.After(items[j].memory.UpdatedAt)
		}
		return items[i].memory.ID < items[j].memory.ID
	})

	if len(items) > query.Limit {
		items = items[:query.Limit]
	}

	result := make([]Memory, len(items))
	for i, item := range items {
		result[i] = item.memory
	}

	return result
}

func extractQueryTokens(cleanedQuery string) []string {
	if cleanedQuery == "" {
		return nil
	}

	lower := strings.ToLower(cleanedQuery)
	fields := strings.FieldsFunc(lower, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && !unicode.Is(unicode.Mn, r)
	})

	seen := make(map[string]struct{}, len(fields))
	tokens := make([]string, 0, len(fields))
	for _, token := range fields {
		if len([]rune(token)) < 2 {
			continue
		}
		if _, exists := seen[token]; exists {
			continue
		}
		seen[token] = struct{}{}
		tokens = append(tokens, token)
	}

	return tokens
}

func lexicalRelevance(memory Memory, queryTokens []string) float64 {
	if len(queryTokens) == 0 {
		return 0
	}

	contentLower := strings.ToLower(memory.Content)
	tagSet := make(map[string]struct{}, len(memory.Tags))
	for _, tag := range memory.Tags {
		tagSet[strings.ToLower(tag)] = struct{}{}
	}

	matchedWeight := 0.0
	for _, token := range queryTokens {
		if _, exactTag := tagSet[token]; exactTag {
			matchedWeight += 1.0
			continue
		}
		if strings.Contains(contentLower, token) {
			matchedWeight += 0.85
			continue
		}
		for tag := range tagSet {
			if len([]rune(tag)) >= 2 && (strings.Contains(token, tag) || strings.Contains(tag, token)) {
				matchedWeight += 0.75
				break
			}
		}
	}

	if matchedWeight <= 0 {
		return 0
	}

	ratio := matchedWeight / float64(len(queryTokens))
	if ratio > 1 {
		ratio = 1
	}

	// Keep lexical overlap smoothly scaled into (0, 0.90] so strong matches outrank
	// unrelated high-importance memories while preserving room for tie-breaking.
	return 0.45 + 0.45*ratio
}

// NearDuplicateSimilarity computes a bilingual similarity score in [0, 1] between
// two memories using word/tag token Jaccard similarity combined with character 3-gram
// Jaccard similarity (supporting both English and unspaced Thai text).
func NearDuplicateSimilarity(a, b Memory) float64 {
	normA := normalizeForSimilarity(a.Content)
	normB := normalizeForSimilarity(b.Content)
	if normA == "" || normB == "" {
		return 0
	}
	if normA == normB {
		return 1.0
	}

	wordSim := jaccardSets(tokenSet(normA), tokenSet(normB))
	gramSim := jaccardSets(charNgramSet(normA, 3), charNgramSet(normB, 3))

	if gramSim > wordSim {
		return gramSim
	}
	return wordSim
}

func normalizeForSimilarity(content string) string {
	s := strings.ReplaceAll(content, "\u2019", "'")
	s = strings.ReplaceAll(s, "\u2018", "'")
	s = strings.ToLower(s)

	var b strings.Builder
	prevSpace := true
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.Is(unicode.Mn, r) {
			b.WriteRune(r)
			prevSpace = false
		} else if !prevSpace {
			b.WriteByte(' ')
			prevSpace = true
		}
	}

	return strings.TrimSpace(b.String())
}

func tokenSet(normalized string) map[string]struct{} {
	fields := strings.Fields(normalized)
	set := make(map[string]struct{}, len(fields))
	for _, f := range fields {
		set[f] = struct{}{}
	}
	return set
}

func charNgramSet(normalized string, n int) map[string]struct{} {
	compact := strings.ReplaceAll(normalized, " ", "")
	runes := []rune(compact)
	set := make(map[string]struct{})
	if len(runes) < n {
		if len(runes) > 0 {
			set[compact] = struct{}{}
		}
		return set
	}

	for i := 0; i+n <= len(runes); i++ {
		set[string(runes[i:i+n])] = struct{}{}
	}
	return set
}

func jaccardSets(a, b map[string]struct{}) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}

	intersection := 0
	for k := range a {
		if _, ok := b[k]; ok {
			intersection++
		}
	}

	union := len(a) + len(b) - intersection
	if union == 0 {
		return 0
	}

	return float64(intersection) / float64(union)
}
