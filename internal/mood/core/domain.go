package core

import (
	"errors"
	"regexp"
	"strings"
	"time"
)

var (
	ErrInvalidGuildID      = errors.New("invalid guild ID")
	ErrInvalidChannelID    = errors.New("invalid channel ID")
	ErrInvalidMoodCategory = errors.New("invalid mood category")
	ErrInvalidIntensity    = errors.New("invalid mood intensity")
	ErrInvalidTimestamp    = errors.New("invalid mood timestamp")
)

const (
	MinIntensity = 1
	MaxIntensity = 5

	DefaultCategory  = CategoryPlayful
	DefaultIntensity = 2

	DecayStepDuration = 6 * time.Hour
	TemporaryMoodTTL  = 15 * time.Minute
)

type Category string

const (
	CategoryCheerful  Category = "cheerful"
	CategoryPlayful   Category = "playful"
	CategoryNeutral   Category = "neutral"
	CategorySarcastic Category = "sarcastic"
	CategoryAnnoyed   Category = "annoyed"
	CategoryTired     Category = "tired"
)

func (c Category) IsValid() bool {
	switch c {
	case CategoryCheerful,
		CategoryPlayful,
		CategoryNeutral,
		CategorySarcastic,
		CategoryAnnoyed,
		CategoryTired:
		return true
	default:
		return false
	}
}

func IntensityLabel(intensity int) string {
	switch intensity {
	case 1:
		return "mild"
	case 2:
		return "noticeable"
	case 3:
		return "moderate"
	case 4:
		return "strong"
	case 5:
		return "very strong"
	default:
		return "unknown"
	}
}

// CommunityMood is the persistent guild-level mood stored in MongoDB.
type CommunityMood struct {
	GuildID   string
	Category  Category
	Intensity int
	CreatedAt time.Time
	UpdatedAt time.Time
}

func DefaultCommunityMood(guildID string, now time.Time) (CommunityMood, error) {
	nowUTC := now.UTC()
	return NewCommunityMood(CommunityMood{
		GuildID:   guildID,
		Category:  DefaultCategory,
		Intensity: DefaultIntensity,
		CreatedAt: nowUTC,
		UpdatedAt: nowUTC,
	})
}

func NewCommunityMood(m CommunityMood) (CommunityMood, error) {
	m.GuildID = strings.TrimSpace(m.GuildID)

	if m.GuildID == "" {
		return CommunityMood{}, ErrInvalidGuildID
	}

	if !m.Category.IsValid() {
		return CommunityMood{}, ErrInvalidMoodCategory
	}

	if m.Intensity < MinIntensity || m.Intensity > MaxIntensity {
		return CommunityMood{}, ErrInvalidIntensity
	}

	if m.CreatedAt.IsZero() || m.UpdatedAt.IsZero() {
		return CommunityMood{}, ErrInvalidTimestamp
	}

	m.CreatedAt = m.CreatedAt.UTC()
	m.UpdatedAt = m.UpdatedAt.UTC()

	return m, nil
}

// TemporaryMood is the short-lived in-memory interaction mood scoped to (guild_id, channel_id).
type TemporaryMood struct {
	GuildID   string
	ChannelID string
	Category  Category
	Intensity int
	UpdatedAt time.Time
	ExpiresAt time.Time
}

func NewTemporaryMood(m TemporaryMood) (TemporaryMood, error) {
	m.GuildID = strings.TrimSpace(m.GuildID)
	m.ChannelID = strings.TrimSpace(m.ChannelID)

	if m.GuildID == "" {
		return TemporaryMood{}, ErrInvalidGuildID
	}

	if m.ChannelID == "" {
		return TemporaryMood{}, ErrInvalidChannelID
	}

	if !m.Category.IsValid() {
		return TemporaryMood{}, ErrInvalidMoodCategory
	}

	if m.Intensity < MinIntensity || m.Intensity > MaxIntensity {
		return TemporaryMood{}, ErrInvalidIntensity
	}

	if m.UpdatedAt.IsZero() || m.ExpiresAt.IsZero() || !m.ExpiresAt.After(m.UpdatedAt) {
		return TemporaryMood{}, ErrInvalidTimestamp
	}

	m.UpdatedAt = m.UpdatedAt.UTC()
	m.ExpiresAt = m.ExpiresAt.UTC()

	return m, nil
}

func (m TemporaryMood) IsExpired(now time.Time) bool {
	return !now.UTC().Before(m.ExpiresAt.UTC())
}

// EffectiveMood represents the combined mood state supplied to the LLM.
type EffectiveMood struct {
	EffectiveCategory  Category
	EffectiveIntensity int
	Community          CommunityMood
	Temporary          *TemporaryMood
}

// ApplyDecay reduces community mood intensity by 1 per 6 hours of inactivity.
// When intensity falls below 1, the mood resets to default (playful, intensity 2).
func ApplyDecay(mood CommunityMood, now time.Time) CommunityMood {
	nowUTC := now.UTC()
	elapsed := nowUTC.Sub(mood.UpdatedAt.UTC())
	if elapsed < DecayStepDuration {
		return mood
	}

	steps := int(elapsed / DecayStepDuration)
	decayedIntensity := mood.Intensity - steps

	if decayedIntensity < MinIntensity {
		mood.Category = DefaultCategory
		mood.Intensity = DefaultIntensity
		return mood
	}

	mood.Intensity = decayedIntensity
	return mood
}

// Signal represents a deterministic lightweight interaction signal.
type Signal string

const (
	SignalNone      Signal = ""
	SignalGreeting  Signal = "greeting_or_compliment"
	SignalPlayful   Signal = "playful_teasing_or_joke"
	SignalSarcastic Signal = "sarcastic"
	SignalHostile   Signal = "hostile_or_rude"
	SignalTired     Signal = "tired"
)

type signalRule struct {
	signal       Signal
	thaiCues     []string
	latinPattern *regexp.Regexp
}

var (
	urlPattern          = regexp.MustCompile(`https?://\S+`)
	mentionTokenPattern = regexp.MustCompile(`<@!?[0-9A-Za-z_-]+>`)
)

var interactionSignalRules = []signalRule{
	{
		signal: SignalHostile,
		thaiCues: []string{
			"หุบปาก",
			"น่ารำคาญ",
			"ไอ้โง่",
		},
		latinPattern: regexp.MustCompile(`\b(?:shut\s+up|stupid|idiot|annoying|go\s+away|useless)\b`),
	},
	{
		signal: SignalSarcastic,
		thaiCues: []string{
			"เก่งจังเลยนะ",
			"ฉลาดมากมั้ง",
			"สุดยอดเลยมั้ง",
		},
		latinPattern: regexp.MustCompile(`\b(?:yeah\s+right|sure\s+buddy|oh\s+really|totally\s+not)\b`),
	},
	{
		signal: SignalTired,
		thaiCues: []string{
			"ง่วง",
			"เหนื่อย",
			"หมดแรง",
			"เพลีย",
		},
		latinPattern: regexp.MustCompile(`\b(?:sleepy|tired|exhausted|burnt\s+out)\b`),
	},
	{
		signal: SignalGreeting,
		thaiCues: []string{
			"สวัสดี",
			"หวัดดี",
			"ขอบคุณ",
			"เก่งมาก",
			"น่ารัก",
			"สุดยอด",
		},
		latinPattern: regexp.MustCompile(`\b(?:good\s+morning|good\s+bot|thank\s+you|thanks|awesome|great\s+job|hello)\b`),
	},
	{
		signal: SignalPlayful,
		thaiCues: []string{
			"ฮ่าๆ",
			"ขำ",
			"ตลก",
			"แกล้ง",
			"หยอก",
		},
		// Match Latin playful words on word boundaries and "555+" only as a standalone token.
		latinPattern: regexp.MustCompile(`(?:\b(?:haha|lmao|lol|rofl|prank|tease)\b|(?:^|[^0-9A-Za-z_\-/])5{3,}(?:$|[^0-9A-Za-z_\-/]))`),
	},
}

// DetectSignal deterministically identifies a clear interaction signal from message text.
// Neutral or ambiguous messages return SignalNone.
func DetectSignal(content string) Signal {
	cleaned := urlPattern.ReplaceAllString(content, " ")
	cleaned = mentionTokenPattern.ReplaceAllString(cleaned, " ")
	normalized := strings.ToLower(strings.TrimSpace(cleaned))
	if normalized == "" {
		return SignalNone
	}

	for _, rule := range interactionSignalRules {
		for _, cue := range rule.thaiCues {
			if strings.Contains(normalized, cue) {
				return rule.signal
			}
		}

		if rule.latinPattern != nil && rule.latinPattern.MatchString(normalized) {
			return rule.signal
		}
	}

	return SignalNone
}

// ApplySignalToCommunity computes the next CommunityMood for a detected signal.
// Returns (mood, changed). SignalNone causes no change.
func ApplySignalToCommunity(
	current CommunityMood,
	signal Signal,
	now time.Time,
) (CommunityMood, bool) {
	if signal == SignalNone {
		return current, false
	}

	updated := current
	updated.UpdatedAt = now.UTC()

	switch signal {
	case SignalHostile:
		switch current.Category {
		case CategoryAnnoyed:
			updated.Intensity = clampIntensity(current.Intensity + 1)
		case CategoryCheerful, CategoryPlayful:
			if current.Intensity > MinIntensity {
				updated.Intensity = current.Intensity - 1
			} else {
				updated.Category = CategoryAnnoyed
				updated.Intensity = 1
			}
		default:
			updated.Category = CategoryAnnoyed
			updated.Intensity = 1
		}

	case SignalSarcastic:
		if current.Category == CategorySarcastic {
			updated.Intensity = clampIntensity(current.Intensity + 1)
		} else {
			updated.Category = CategorySarcastic
			updated.Intensity = 2
		}

	case SignalTired:
		if current.Category == CategoryTired {
			updated.Intensity = clampIntensity(current.Intensity + 1)
		} else {
			updated.Category = CategoryTired
			updated.Intensity = 2
		}

	case SignalGreeting:
		switch current.Category {
		case CategoryCheerful:
			updated.Intensity = clampIntensity(current.Intensity + 1)
		case CategoryAnnoyed:
			if current.Intensity > MinIntensity {
				updated.Intensity = current.Intensity - 1
			} else {
				updated.Category = CategoryNeutral
				updated.Intensity = 1
			}
		default:
			updated.Category = CategoryCheerful
			updated.Intensity = 2
		}

	case SignalPlayful:
		switch current.Category {
		case CategoryPlayful:
			updated.Intensity = clampIntensity(current.Intensity + 1)
		case CategoryAnnoyed:
			if current.Intensity > MinIntensity {
				updated.Intensity = current.Intensity - 1
			} else {
				updated.Category = CategoryNeutral
				updated.Intensity = 1
			}
		default:
			updated.Category = CategoryPlayful
			updated.Intensity = 2
		}

	default:
		return current, false
	}

	return updated, true
}

// ApplySignalToTemporary computes a new TemporaryMood for a detected signal.
// Returns (tempMood, true) if a signal triggered an update, or (nil, false) for SignalNone.
func ApplySignalToTemporary(
	guildID string,
	channelID string,
	existing *TemporaryMood,
	signal Signal,
	now time.Time,
) (*TemporaryMood, bool) {
	if signal == SignalNone {
		return existing, false
	}

	var targetCategory Category
	switch signal {
	case SignalHostile:
		targetCategory = CategoryAnnoyed
	case SignalSarcastic:
		targetCategory = CategorySarcastic
	case SignalTired:
		targetCategory = CategoryTired
	case SignalGreeting:
		targetCategory = CategoryCheerful
	case SignalPlayful:
		targetCategory = CategoryPlayful
	default:
		return existing, false
	}

	intensity := 3
	if existing != nil && !existing.IsExpired(now) && existing.Category == targetCategory {
		intensity = clampIntensity(existing.Intensity + 1)
	}

	nowUTC := now.UTC()
	next := TemporaryMood{
		GuildID:   strings.TrimSpace(guildID),
		ChannelID: strings.TrimSpace(channelID),
		Category:  targetCategory,
		Intensity: intensity,
		UpdatedAt: nowUTC,
		ExpiresAt: nowUTC.Add(TemporaryMoodTTL),
	}

	return &next, true
}

// CombineMoods produces the effective mood context from persistent CommunityMood
// and an optional active (non-expired) TemporaryMood.
func CombineMoods(
	community CommunityMood,
	temporary *TemporaryMood,
	now time.Time,
) EffectiveMood {
	if temporary == nil || temporary.IsExpired(now) {
		return EffectiveMood{
			EffectiveCategory:  community.Category,
			EffectiveIntensity: community.Intensity,
			Community:          community,
			Temporary:          nil,
		}
	}

	tempCopy := *temporary
	effectiveIntensity := tempCopy.Intensity

	if tempCopy.Category == community.Category && community.Intensity > effectiveIntensity {
		effectiveIntensity = community.Intensity
	}

	return EffectiveMood{
		EffectiveCategory:  tempCopy.Category,
		EffectiveIntensity: effectiveIntensity,
		Community:          community,
		Temporary:          &tempCopy,
	}
}

func clampIntensity(v int) int {
	if v < MinIntensity {
		return MinIntensity
	}
	if v > MaxIntensity {
		return MaxIntensity
	}
	return v
}
