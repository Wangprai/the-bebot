package app

import (
	"errors"
	"strings"
	"testing"
	"time"

	"the-bebot/internal/memory/core"
)

func newTestMemoryValidator() *MemoryValidator {
	now := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)

	return &MemoryValidator{
		clock: func() time.Time {
			return now
		},
	}
}

func validExtractedMemory() core.ExtractedMemory {
	return core.ExtractedMemory{
		Scope:      core.MemoryScopeMember,
		Type:       core.MemoryTypePreference,
		Content:    "Likes Go programming",
		Importance: 3,
		Confidence: 0.95,
		Tags:       []string{"Go", "Programming"},
	}
}

func validExtractionSource() core.ExtractionSource {
	return core.ExtractionSource{
		GuildID:   "guild-001",
		ChannelID: "channel-001",
		MessageID: "message-001",
		UserID:    "user-001",
	}
}

func TestMemoryValidatorMemberMemory(t *testing.T) {
	validator := newTestMemoryValidator()

	memory, err := validator.Validate(
		validExtractedMemory(),
		validExtractionSource(),
	)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	if memory.GuildID != "guild-001" {
		t.Errorf("GuildID = %s", memory.GuildID)
	}

	if memory.UserID != "user-001" {
		t.Errorf("UserID = %s, want user-001", memory.UserID)
	}

	if memory.Scope != core.MemoryScopeMember {
		t.Errorf("Scope = %s", memory.Scope)
	}

	if memory.SourceMessageID != "message-001" {
		t.Errorf("SourceMessageID = %s", memory.SourceMessageID)
	}

	if memory.Content != "Likes Go programming" {
		t.Errorf("Content = %s", memory.Content)
	}

	if memory.Importance != 3 {
		t.Errorf("Importance = %d", memory.Importance)
	}

	if memory.Confidence != 0.95 {
		t.Errorf("Confidence = %f", memory.Confidence)
	}

	if len(memory.Tags) != 2 {
		t.Fatalf("Tags count = %d, want 2", len(memory.Tags))
	}

	if memory.Tags[0] != "go" || memory.Tags[1] != "programming" {
		t.Errorf("Tags = %v, want normalized tags", memory.Tags)
	}

	if memory.CreatedAt.IsZero() || memory.UpdatedAt.IsZero() {
		t.Error("timestamps should not be zero")
	}

	if memory.ID == "" {
		t.Error("memory ID should not be empty")
	}
}

func TestMemoryValidatorUsesSourceUserID(t *testing.T) {
	validator := newTestMemoryValidator()
	input := validExtractedMemory()

	// LLM does not provide a UserID.
	input.UserID = ""

	memory, err := validator.Validate(input, validExtractionSource())
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	if memory.UserID != "user-001" {
		t.Errorf("UserID = %s, want source user-001", memory.UserID)
	}
}

func TestMemoryValidatorIgnoresLLMUserID(t *testing.T) {
	validator := newTestMemoryValidator()
	input := validExtractedMemory()

	// Simulate incorrect UserID returned by LLM.
	input.UserID = "user-999"

	memory, err := validator.Validate(
		input,
		validExtractionSource(),
	)

	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	if memory.UserID != "user-001" {
		t.Errorf(
			"UserID = %s, want user-001",
			memory.UserID,
		)
	}
}

func TestMemoryValidatorCommunityMemory(t *testing.T) {
	validator := newTestMemoryValidator()
	input := validExtractedMemory()

	input.Scope = core.MemoryScopeCommunity
	input.UserID = "user-999"

	memory, err := validator.Validate(input, validExtractionSource())
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	if memory.Scope != core.MemoryScopeCommunity {
		t.Errorf("Scope = %s", memory.Scope)
	}

	if memory.UserID != "" {
		t.Errorf("community memory UserID = %s, want empty", memory.UserID)
	}

	if len(memory.SourceUserIDs) != 1 || memory.SourceUserIDs[0] != "user-001" {
		t.Errorf("community memory SourceUserIDs = %v, want [user-001]", memory.SourceUserIDs)
	}
}

func TestMemoryValidatorRejectsInvalidScope(t *testing.T) {
	validator := newTestMemoryValidator()
	input := validExtractedMemory()

	input.Scope = core.MemoryScope("unknown")

	_, err := validator.Validate(input, validExtractionSource())

	if !errors.Is(err, core.ErrInvalidExtractedMemory) {
		t.Errorf("error = %v, want ErrInvalidExtractedMemory", err)
	}
}

func TestMemoryValidatorRejectsInvalidType(t *testing.T) {
	validator := newTestMemoryValidator()
	input := validExtractedMemory()

	input.Type = core.MemoryType("unknown")

	_, err := validator.Validate(input, validExtractionSource())

	if !errors.Is(err, core.ErrInvalidExtractedMemory) {
		t.Errorf("error = %v, want ErrInvalidExtractedMemory", err)
	}
}

func TestMemoryValidatorRejectsInvalidImportance(t *testing.T) {
	tests := []int{0, 6, -1}

	for _, importance := range tests {
		t.Run(strings.ReplaceAll(
			time.Duration(importance).String(),
			"-",
			"negative-",
		), func(t *testing.T) {
			validator := newTestMemoryValidator()
			input := validExtractedMemory()
			input.Importance = importance

			_, err := validator.Validate(input, validExtractionSource())

			if !errors.Is(err, core.ErrInvalidImportance) {
				t.Errorf("error = %v, want ErrInvalidImportance", err)
			}
		})
	}
}

func TestMemoryValidatorRejectsInvalidConfidence(t *testing.T) {
	tests := []float64{-0.1, 1.1}

	for _, confidence := range tests {
		t.Run(time.Duration(confidence*100).String(), func(t *testing.T) {
			validator := newTestMemoryValidator()
			input := validExtractedMemory()
			input.Confidence = confidence

			_, err := validator.Validate(input, validExtractionSource())

			if !errors.Is(err, core.ErrInvalidConfidence) {
				t.Errorf("error = %v, want ErrInvalidConfidence", err)
			}
		})
	}
}

func TestMemoryValidatorRejectsEmptyContent(t *testing.T) {
	validator := newTestMemoryValidator()
	input := validExtractedMemory()

	input.Content = "   "

	_, err := validator.Validate(input, validExtractionSource())

	if !errors.Is(err, core.ErrInvalidExtractedMemory) {
		t.Errorf("error = %v, want ErrInvalidExtractedMemory", err)
	}
}

func TestMemoryValidatorRejectsLongContent(t *testing.T) {
	validator := newTestMemoryValidator()
	input := validExtractedMemory()

	input.Content = strings.Repeat("a", 2001)

	_, err := validator.Validate(input, validExtractionSource())

	if !errors.Is(err, core.ErrInvalidExtractedMemory) {
		t.Errorf("error = %v, want ErrInvalidExtractedMemory", err)
	}
}

func TestMemoryValidatorRejectsSensitiveContent(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "password",
			content: "My password is example123",
		},
		{
			name:    "API key",
			content: "My API key is abc123",
		},
		{
			name:    "Thai password",
			content: "รหัสผ่านของฉันคือ example",
		},
		{
			name:    "long digit sequence",
			content: "My account number is 1234567890123",
		},
		{
			name:    "Thai mobile phone with dashes",
			content: "Call me at 081-234-5678 anytime",
		},
		{
			name:    "Thai mobile phone compact",
			content: "Phone is 0812345678",
		},
		{
			name:    "Thai landline phone",
			content: "Office number 02-123-4567",
		},
		{
			name:    "International +66 phone",
			content: "WhatsApp +66 81-234-5678",
		},
		{
			name:    "International +66 compact phone",
			content: "Reach me at +66812345678",
		},
		{
			name:    "Email address",
			content: "Email me at alice.dev+bot@example.co.th",
		},
		{
			name:    "English street address",
			content: "Lives at 123 Main Street downtown",
		},
		{
			name:    "Thai/English road address",
			content: "Address is 45/2 Sukhumvit Rd.",
		},
		{
			name:    "Thai house number address",
			content: "บ้านเลขที่ 99/12 ถนนสุขุมวิท",
		},
		{
			name:    "Thai soi house number address",
			content: "อยู่ 123/45 ซอยลาดพร้าว 101",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := newTestMemoryValidator()
			input := validExtractedMemory()
			input.Content = tt.content

			_, err := validator.Validate(input, validExtractionSource())

			if !errors.Is(err, core.ErrInvalidExtractedMemory) {
				t.Errorf("error = %v, want ErrInvalidExtractedMemory", err)
			}
		})
	}
}

func TestMemoryValidatorAllowsNonSensitiveContent(t *testing.T) {
	allowed := []string{
		"Likes Go 1.27 and joined the server in 2026",
		"ชอบกินข้าวซอยไก่มาก",
		"Talked with @Alice about coffee shops",
		"Prefers running on the road in the morning",
	}

	for _, content := range allowed {
		t.Run(content, func(t *testing.T) {
			validator := newTestMemoryValidator()
			input := validExtractedMemory()
			input.Content = content

			if _, err := validator.Validate(input, validExtractionSource()); err != nil {
				t.Errorf("expected %q to be allowed, got error: %v", content, err)
			}
		})
	}
}

func TestMemoryValidatorRejectsInvalidSource(t *testing.T) {
	tests := []struct {
		name   string
		source core.ExtractionSource
	}{
		{
			name: "empty guild ID",
			source: core.ExtractionSource{
				ChannelID: "channel-001",
				MessageID: "message-001",
				UserID:    "user-001",
			},
		},
		{
			name: "empty channel ID",
			source: core.ExtractionSource{
				GuildID:   "guild-001",
				MessageID: "message-001",
				UserID:    "user-001",
			},
		},
		{
			name: "empty message ID",
			source: core.ExtractionSource{
				GuildID:   "guild-001",
				ChannelID: "channel-001",
				UserID:    "user-001",
			},
		},
		{
			name: "empty user ID",
			source: core.ExtractionSource{
				GuildID:   "guild-001",
				ChannelID: "channel-001",
				MessageID: "message-001",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validator := newTestMemoryValidator()

			_, err := validator.Validate(
				validExtractedMemory(),
				tt.source,
			)

			if !errors.Is(err, core.ErrInvalidExtractionSource) {
				t.Errorf("error = %v, want ErrInvalidExtractionSource", err)
			}
		})
	}
}

func TestMemoryValidatorRejectsNilReceiver(t *testing.T) {
	var validator *MemoryValidator

	_, err := validator.Validate(
		validExtractedMemory(),
		validExtractionSource(),
	)

	if err == nil {
		t.Fatal("expected error for nil validator")
	}
}
