package app

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"the-bebot/internal/memory/core"
)

var (
	candidateMentionPattern = regexp.MustCompile(`<@!?[0-9A-Za-z_-]+>`)
	englishSignalPatterns   = []*regexp.Regexp{
		regexp.MustCompile(`\bi\s+like\b`),
		regexp.MustCompile(`\bi\s+dislike\b`),
		regexp.MustCompile(`\bi\s+hate\b`),
		regexp.MustCompile(`\bi\s+am\b`),
		regexp.MustCompile(`\bi'm\b`),
		regexp.MustCompile(`\bi\s+work\b`),
		regexp.MustCompile(`\bi\s+study\b`),
		regexp.MustCompile(`\bi\s+will\b`),
		regexp.MustCompile(`\bi\s+promise\b`),
		regexp.MustCompile(`\bmy\s+favorite\b`),
		regexp.MustCompile(`\bmy\s+hobby\b`),
	}
)

type CandidateRules struct {
	minContentLength int
	thaiSignals      []string
	englishPatterns  []*regexp.Regexp
}

func NewCandidateRules() *CandidateRules {
	return &CandidateRules{
		minContentLength: 12,
		thaiSignals: []string{
			// Thai: Preferences
			"ชอบ",
			"ไม่ชอบ",
			"เกลียด",
			"ไม่ค่อยชอบ",

			// Thai: Personal facts
			"ผมเป็น",
			"ฉันเป็น",
			"เราเป็น",
			"ผมทำงาน",
			"ฉันทำงาน",
			"เราเรียน",

			// Thai: Plans and promises
			"พรุ่งนี้จะ",
			"เดี๋ยวจะ",
			"สัญญาว่า",
			"ไว้จะ",
			"อาทิตย์หน้าจะ",
		},
		englishPatterns: englishSignalPatterns,
	}
}

func (r *CandidateRules) ShouldCreateCandidate(
	snapshot core.ConversationSnapshot,
) (bool, error) {
	if r == nil {
		return false, core.ErrInvalidExtractionInput
	}

	validated, err := core.NewConversationSnapshot(snapshot)
	if err != nil {
		return false, err
	}

	var triggerMessage *core.ConversationMessage
	for i := range validated.Messages {
		if validated.Messages[i].MessageID == validated.TriggerMessageID {
			triggerMessage = &validated.Messages[i]
			break
		}
	}

	if triggerMessage == nil || triggerMessage.IsBot {
		return false, nil
	}

	targetUserID := strings.TrimSpace(validated.TargetUserID)
	if targetUserID != "" && triggerMessage.UserID != targetUserID {
		return false, nil
	}

	cleaned := stripCandidateMentions(triggerMessage.Content)
	if utf8.RuneCountInString(cleaned) < r.minContentLength {
		return false, nil
	}

	normalized := normalizeSignalContent(cleaned)

	for _, signal := range r.thaiSignals {
		if strings.Contains(normalized, signal) {
			return true, nil
		}
	}

	for _, pattern := range r.englishPatterns {
		if pattern.MatchString(normalized) {
			return true, nil
		}
	}

	return false, nil
}

func normalizeSignalContent(content string) string {
	normalized := strings.ReplaceAll(content, "\u2019", "'")
	normalized = strings.ReplaceAll(normalized, "\u2018", "'")
	return strings.ToLower(normalized)
}

func stripCandidateMentions(content string) string {
	stripped := candidateMentionPattern.ReplaceAllString(content, " ")
	return strings.TrimSpace(strings.Join(strings.Fields(stripped), " "))
}
