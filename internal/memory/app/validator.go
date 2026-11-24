package app

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"the-bebot/internal/memory/core"
)

var (
	thaiPhonePattern            = regexp.MustCompile(`(?:\b0[689]\d[\s-]?\d{3}[\s-]?\d{4}\b|\b0[2-7]\d?[\s-]?\d{3}[\s-]?\d{3,4}\b)`)
	intlThaiPhonePattern        = regexp.MustCompile(`\+66[\s-]?\d{1,2}[\s-]?\d{3}[\s-]?\d{3,4}\b`)
	emailAddressPattern         = regexp.MustCompile(`(?i)\b[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}\b`)
	englishStreetAddressPattern = regexp.MustCompile(`(?i)\b\d{1,5}(?:/\d{1,4})?\s+[a-z0-9][a-z0-9\s.\-]{1,40}\b(?:street|st|road|rd|avenue|ave|boulevard|blvd|lane|ln|drive|dr|soi|alley|way|court|ct|place|pl)\b\.?`)
	thaiStreetAddressPattern    = regexp.MustCompile(`(?:บ้านเลขที่\s*\d+|\d+/\d+\s*(?:หมู่|ม\.|ซอย|ซ\.|ถนน|ถ\.)|(?:ซอย|ถนน)\s*\S+.*(?:แขวง|เขต|ตำบล|อำเภอ|จังหวัด)|(?:แขวง|ตำบล)\s*\S+\s*(?:เขต|อำเภอ)\s*\S+)`)

	// Best-effort bilingual patterns for special sensitive categories
	// (health, religion, politics, sexual orientation, precise location, minors).
	sensitiveCategoryPatterns = []*regexp.Regexp{
		// Health / medical
		regexp.MustCompile(`(?i)\b(?:diagnos(?:ed|is)|medical\s+(?:record|history|condition)|prescription|antidepressant|chemotherapy|hiv|aids|cancer|diabetes|depression|bipolar|schizophrenia|anxiety\s+disorder|adhd|autism|ptsd|chronic\s+illness|mental\s+health|therapy\s+session|psychiatrist|hospitalized|surgery)\b`),
		// Religion / religious belief
		regexp.MustCompile(`(?i)\b(?:christian(?:ity)?|muslim|islam(?:ic)?|buddhist|buddhism|hindu(?:ism)?|jewish|judaism|catholic|protestant|atheist|agnostic|religion|religious\s+belief)\b`),
		// Politics / political affiliation
		regexp.MustCompile(`(?i)\b(?:political\s+(?:party|view|belief|affiliation|opinion)|voted\s+for|democrat(?:ic)?\s+party|republican\s+party|conservative\s+party|liberal\s+party|socialist|communist|right-wing|left-wing|election\s+vote)\b`),
		// Sexual orientation / gender identity
		regexp.MustCompile(`(?i)\b(?:sexual\s+orientation|sexuality|homosexual|heterosexual|bisexual|pansexual|asexual|lgbtq\+?|transgender|non-binary|came\s+out\s+as\s+(?:gay|lesbian|bi|trans|queer)|identifies\s+as\s+(?:gay|lesbian|bisexual|transgender|queer|non-binary))\b`),
		// Precise location (GPS coordinates with >=4 decimals, map pins, postal/unit numbers)
		regexp.MustCompile(`(?i)(?:^|[^\d])[-+]?(?:[1-8]?\d\.\d{4,}|90\.0{4,})\s*,\s*[-+]?(?:1[0-7]\d\.\d{4,}|[1-9]?\d\.\d{4,}|180\.0{4,})(?:$|[^\d])`),
		regexp.MustCompile(`(?i)(?:maps\.google\.com|goo\.gl/maps|maps\.app\.goo\.gl|\bpin\s+location\b|\bgps\s+coordinates?\b|\blive\s+location\b)`),
		regexp.MustCompile(`(?i)\b(?:home\s+address|exact\s+address|current\s+location|zip\s+code|postal\s+code|apartment\s+(?:number|#|no\.?)\s*\d+|room\s+(?:number|#|no\.?)\s*\d+)\b`),
		// Minors' data (under-18 ages, K-12 school info, children's personal details)
		regexp.MustCompile(`(?i)\b(?:is|am|i['\x{2019}]m|aged?|turns?|turning)\s+(?:[1-9]|1[0-7])\s*(?:years?\s+old|yo|y/o)\b`),
		regexp.MustCompile(`(?i)\b(?:minor|underage|under\s+18|elementary\s+school|middle\s+school|high\s+school\s+student|kindergarten|preschool|grade\s+[1-9]|grade\s+1[0-2]|my\s+(?:kid|child|son|daughter)(?:['\x{2019}]s)?\s+(?:name|school|age|goes\s+to))\b`),
		regexp.MustCompile(`(?:อายุ\s*(?:[1-9]|1[0-7])\s*(?:ปี|ขวบ)|นักเรียนชั้น\s*(?:ป\.|ม\.)|อยู่ชั้น\s*(?:ป\.|ม\.)|เรียนอยู่\s*(?:ป\.|ม\.)|โรงเรียนประถม|โรงเรียนมัธยม|อนุบาล|ผู้เยาว์|ยังไม่บรรลุนิติภาวะ|ลูก(?:ชาย|สาว)?อายุ)`),
	}
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

// containsSensitivePattern is a conservative, best-effort filter that rejects
// credentials, financial/national IDs, contact details, precise locations, and
// special sensitive categories (health, religion, politics, sexual orientation,
// and minors' personal data) in English and Thai. It is not a substitute for
// comprehensive DLP or human moderation.
func containsSensitivePattern(content string) bool {
	lower := strings.ToLower(content)

	sensitiveKeywords := []string{
		// Credentials & financial
		"password",
		"passwd",
		"secret key",
		"api key",
		"access token",
		"refresh token",
		"private key",
		"credit card",
		"bank account",
		"บัตรประชาชน",
		"เลขบัตรประชาชน",
		"รหัสผ่าน",
		"เลขบัญชี",
		"หมายเลขบัตรเครดิต",
		"บัตรเครดิต",
		"รหัส pin",

		// Thai health / medical
		"โรคซึมเศร้า",
		"ไบโพลาร์",
		"โรคเบาหวาน",
		"โรคมะเร็ง",
		"โรคหัวใจ",
		"โรคประจำตัว",
		"ประวัติการรักษา",
		"พบจิตแพทย์",
		"กินยาต้านเศร้า",
		"ติดเชื้อ hiv",
		"เอดส์",
		"สมาธิสั้น",
		"ออทิสติก",
		"แพ้ยา",
		"ผ่าตัด",

		// Thai religion
		"นับถือศาสนา",
		"ศาสนาพุทธ",
		"ศาสนาอิสลาม",
		"ศาสนาคริสต์",
		"ศาสนาฮินดู",
		"เป็นชาวพุทธ",
		"เป็นมุสลิม",
		"เป็นคริสเตียน",
		"ไม่นับถือศาสนา",

		// Thai politics
		"พรรคการเมือง",
		"เลือกตั้งพรรค",
		"จุดยืนทางการเมือง",
		"อุดมการณ์ทางการเมือง",
		"โหวตพรรค",
		"สนับสนุนพรรค",
		"ฝักใฝ่การเมือง",

		// Thai sexual orientation / gender identity
		"รสนิยมทางเพศ",
		"อัตลักษณ์ทางเพศ",
		"เป็นเกย์",
		"เป็นเลสเบี้ยน",
		"เป็นไบเซ็กชวล",
		"เป็นทรานส์",
		"คนข้ามเพศ",

		// Thai precise location
		"พิกัดบ้าน",
		"พิกัด gps",
		"โลเคชั่นบ้าน",
		"ที่อยู่บ้าน",
		"ที่อยู่ปัจจุบัน",
		"รหัสไปรษณีย์",
	}

	for _, pattern := range sensitiveKeywords {
		if strings.Contains(lower, pattern) {
			return true
		}
	}

	if thaiPhonePattern.MatchString(content) ||
		intlThaiPhonePattern.MatchString(content) ||
		emailAddressPattern.MatchString(content) ||
		englishStreetAddressPattern.MatchString(content) ||
		thaiStreetAddressPattern.MatchString(content) {
		return true
	}

	for _, pattern := range sensitiveCategoryPatterns {
		if pattern.MatchString(content) {
			return true
		}
	}

	return containsSensitiveDigitSequence(content)
}

// containsSensitiveDigitSequence extracts digit runs across common separators
// (space, dash, dot) and checks sliding windows for Thai national ID checksums,
// Luhn-valid card-like sequences (13-19 digits), and long identifier runs (>=13 digits).
func containsSensitiveDigitSequence(content string) bool {
	runs := extractSeparatedDigitRuns(content)
	for _, digits := range runs {
		if len(digits) < 13 {
			continue
		}

		if hasValidThaiNationalIDWindow(digits) {
			return true
		}

		if hasValidLuhnWindow(digits) {
			return true
		}

		// Conservative fallback: reject any 13+ digit sequence across common
		// separators (covers bank/account/ID numbers even if formatted or synthetic).
		return true
	}

	return false
}

func extractSeparatedDigitRuns(content string) [][]int {
	var runs [][]int
	var current []int
	pendingSep := false

	flush := func() {
		if len(current) > 0 {
			runs = append(runs, current)
			current = nil
		}
		pendingSep = false
	}

	for _, r := range content {
		if d, ok := digitValue(r); ok {
			current = append(current, d)
			pendingSep = false
			continue
		}

		if len(current) > 0 && isDigitSeparator(r) && !pendingSep {
			pendingSep = true
			continue
		}

		flush()
	}

	flush()
	return runs
}

func digitValue(r rune) (int, bool) {
	if r >= '0' && r <= '9' {
		return int(r - '0'), true
	}
	if r >= '๐' && r <= '๙' {
		return int(r - '๐'), true
	}
	return 0, false
}

func isDigitSeparator(r rune) bool {
	switch r {
	case ' ', '-', '.', '\u00a0', '\u2013', '\u2014':
		return true
	default:
		return false
	}
}

func hasValidThaiNationalIDWindow(digits []int) bool {
	if len(digits) < 13 {
		return false
	}

	for i := 0; i+13 <= len(digits); i++ {
		if isValidThaiNationalID(digits[i : i+13]) {
			return true
		}
	}

	return false
}

// isValidThaiNationalID verifies the 13-digit Thai national identification number checksum.
func isValidThaiNationalID(digits []int) bool {
	if len(digits) != 13 {
		return false
	}
	if digits[0] < 1 || digits[0] > 8 {
		return false
	}

	sum := 0
	for i := 0; i < 12; i++ {
		sum += digits[i] * (13 - i)
	}

	checkDigit := (11 - (sum % 11)) % 10
	return checkDigit == digits[12]
}

func hasValidLuhnWindow(digits []int) bool {
	for windowLen := 13; windowLen <= 19; windowLen++ {
		if len(digits) < windowLen {
			break
		}
		for i := 0; i+windowLen <= len(digits); i++ {
			if isValidLuhn(digits[i : i+windowLen]) {
				return true
			}
		}
	}
	return false
}

// isValidLuhn verifies whether a 13-19 digit sequence satisfies the Luhn checksum.
func isValidLuhn(digits []int) bool {
	if len(digits) < 13 || len(digits) > 19 {
		return false
	}

	sum := 0
	double := false
	for i := len(digits) - 1; i >= 0; i-- {
		d := digits[i]
		if double {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		double = !double
	}

	return sum%10 == 0
}
