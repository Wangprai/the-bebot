package app

import (
	"strings"
	"unicode/utf8"

	"the-bebot/internal/memory/core"
)

type CandidateRules struct {
	minContentLength int
	signals          []string
}

func NewCandidateRules() *CandidateRules {
	return &CandidateRules{
		minContentLength: 12,
		signals: []string{
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

			// English
			"i like",
			"i dislike",
			"i hate",
			"i am",
			"i'm",
			"i work",
			"i study",
			"i will",
			"i promise",
			"my favorite",
			"my hobby",
		},
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

	for _, message := range validated.Messages {
		if message.IsBot {
			continue
		}

		content := strings.TrimSpace(message.Content)

		if utf8.RuneCountInString(content) < r.minContentLength {
			continue
		}

		normalized := strings.ToLower(content)

		for _, signal := range r.signals {
			if strings.Contains(normalized, signal) {
				return true, nil
			}
		}
	}

	return false, nil
}
