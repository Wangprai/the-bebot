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
		{
			name:    "Card number separated by spaces (Luhn valid)",
			content: "My card is 4111 1111 1111 1111",
		},
		{
			name:    "Card number separated by dashes (Luhn valid)",
			content: "Card 4111-1111-1111-1111",
		},
		{
			name:    "Card number separated by dots (Luhn valid)",
			content: "Use 4111.1111.1111.1111 for payment",
		},
		{
			name:    "Thai national ID formatted with dashes (regression)",
			content: "ID is 1-2345-67890-12-3",
		},
		{
			name:    "Thai national ID with valid checksum and dashes",
			content: "Citizen ID 1-2345-67890-12-1",
		},
		{
			name:    "Health condition English",
			content: "Was diagnosed with diabetes last year",
		},
		{
			name:    "Health condition Thai",
			content: "เป็นโรคซึมเศร้าและต้องพบจิตแพทย์ทุกเดือน",
		},
		{
			name:    "Religion English",
			content: "Identifies as a devout Buddhist",
		},
		{
			name:    "Religion Thai",
			content: "นับถือศาสนาอิสลามมาตั้งแต่เด็ก",
		},
		{
			name:    "Politics English",
			content: "Voted for the Democratic Party in the last election",
		},
		{
			name:    "Politics Thai",
			content: "สนับสนุนพรรคการเมืองฝ่ายค้านมาตลอด",
		},
		{
			name:    "Sexual orientation English",
			content: "Came out as bisexual to friends last year",
		},
		{
			name:    "Sexual orientation Thai",
			content: "เล่าให้เพื่อนฟังว่าเป็นเกย์",
		},
		{
			name:    "Precise GPS coordinates",
			content: "My home is at 13.7563, 100.5018",
		},
		{
			name:    "Google Maps precise location link",
			content: "Pinned location at https://maps.google.com/?q=13.7563,100.5018",
		},
		{
			name:    "Thai precise location and postal code",
			content: "พิกัดบ้านอยู่ที่รหัสไปรษณีย์ 10110",
		},
		{
			name:    "Minor age English",
			content: "I am 14 years old and in middle school",
		},
		{
			name:    "Minor age Thai",
			content: "อายุ 15 ปี เรียนอยู่ ม.3",
		},
		{
			name:    "Minor child school info English",
			content: "My daughter's school is Riverside Elementary",
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
		"Lives in Bangkok and loves late-night street food",
		"I am 25 years old and work as a backend developer",
		"อายุ 28 ปี ชอบเล่นเกมหลังเลิกงาน",
		"The latte costs 95.50 baht and the croissant is 85.00",
		"ชอบไปเที่ยวเชียงใหม่ช่วงหน้าหนาว",
		"Working on release 2.4.12 for the Discord bot",
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

func TestSensitiveChecksumsAndDigitWindows(t *testing.T) {
	if !isValidLuhn([]int{4, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}) {
		t.Error("expected 4111111111111111 to pass Luhn check")
	}
	if isValidLuhn([]int{4, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 2}) {
		t.Error("expected 4111111111111112 to fail Luhn check")
	}

	if !isValidThaiNationalID([]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1, 2, 1}) {
		t.Error("expected 1-2345-67890-12-1 to pass Thai national ID checksum")
	}
	if isValidThaiNationalID([]int{1, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1, 2, 3}) {
		t.Error("expected 1-2345-67890-12-3 to fail Thai national ID checksum")
	}
	if isValidThaiNationalID([]int{0, 2, 3, 4, 5, 6, 7, 8, 9, 0, 1, 2, 1}) {
		t.Error("expected leading 0 to fail Thai national ID checksum")
	}

	runs := extractSeparatedDigitRuns("Card 4111 1111-1111.1111 and Go 1.27")
	if len(runs) != 2 || len(runs[0]) != 16 || len(runs[1]) != 3 {
		t.Fatalf("unexpected digit runs: %+v", runs)
	}
}
