package app

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"the-bebot/internal/memory/core"
)

type MemoryValidator struct {
	clock func() time.Time
}

var _ core.MemoryValidator = (*MemoryValidator)(nil)

func NewMemoryValidator() *MemoryValidator {
	return &MemoryValidator{
		clock: time.Now,
	}
}

func (v *MemoryValidator) Validate(
	input core.ExtractedMemory,
	source core.ExtractionSource,
) (core.Memory, error) {
	if v == nil {
		return core.Memory{}, fmt.Errorf(
			"%w: validator is not initialized",
			core.ErrInvalidExtractedMemory,
		)
	}

	source.GuildID = strings.TrimSpace(source.GuildID)
	source.ChannelID = strings.TrimSpace(source.ChannelID)
	source.MessageID = strings.TrimSpace(source.MessageID)
	source.UserID = strings.TrimSpace(source.UserID)

	if source.GuildID == "" ||
		source.ChannelID == "" ||
		source.MessageID == "" ||
		source.UserID == "" {
		return core.Memory{}, core.ErrInvalidExtractionSource
	}

	input.Content = strings.TrimSpace(input.Content)
	input.UserID = strings.TrimSpace(input.UserID)

	if input.Content == "" || len(input.Content) > 2000 {
		return core.Memory{}, core.ErrInvalidExtractedMemory
	}

	if containsSensitivePattern(input.Content) {
		return core.Memory{}, fmt.Errorf(
			"%w: potentially sensitive content",
			core.ErrInvalidExtractedMemory,
		)
	}

	if !input.Scope.IsValid() || !input.Type.IsValid() {
		return core.Memory{}, core.ErrInvalidExtractedMemory
	}

	switch input.Scope {
	case core.MemoryScopeMember:
		// 	// The source user is authoritative.
		// 	// Never trust the user ID returned by an LLM.
		input.UserID = source.UserID

	case core.MemoryScopeCommunity:
		// Community memory must not belong to an individual.
		input.UserID = ""
	}

	now := v.clock()

	memory := core.Memory{
		ID:              fmt.Sprintf("%s-%d", source.MessageID, now.UnixNano()),
		GuildID:         source.GuildID,
		UserID:          input.UserID,
		Scope:           input.Scope,
		Type:            input.Type,
		Content:         input.Content,
		Importance:      input.Importance,
		Confidence:      input.Confidence,
		Tags:            input.Tags,
		SourceMessageID: source.MessageID,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	validated, err := core.NewMemory(memory)
	if err != nil {
		return core.Memory{}, fmt.Errorf("validate memory domain: %w", err)
	}

	return validated, nil
}

// This is a conservative first-pass filter, not a complete
// sensitive-information detector.
func containsSensitivePattern(content string) bool {
	lower := strings.ToLower(content)

	sensitivePatterns := []string{
		"password",
		"passwd",
		"secret key",
		"api key",
		"access token",
		"refresh token",
		"private key",
		"บัตรประชาชน",
		"เลขบัตรประชาชน",
		"รหัสผ่าน",
		"เลขบัญชี",
		"หมายเลขบัตรเครดิต",
	}

	for _, pattern := range sensitivePatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}

	// Detect long digit sequences that may represent
	// account numbers or other identifiers.
	var digitCount int

	for _, r := range content {
		if unicode.IsDigit(r) {
			digitCount++
			if digitCount >= 13 {
				return true
			}
		} else {
			digitCount = 0
		}
	}

	return false
}
