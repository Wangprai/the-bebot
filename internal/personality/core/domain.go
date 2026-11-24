package core

import (
	"errors"
	"strings"
)

var (
	ErrInvalidPersonalityName           = errors.New("invalid personality name")
	ErrInvalidPersonalityTraits         = errors.New("invalid personality traits")
	ErrInvalidPersonalitySpeakingStyle  = errors.New("invalid personality speaking style")
	ErrInvalidPersonalityTeasingRules   = errors.New("invalid personality teasing rules")
	ErrInvalidPersonalityBoundaries     = errors.New("invalid personality boundaries")
	ErrInvalidPersonalityLanguagePolicy = errors.New("invalid personality language policy")
)

// Personality defines The Bebot's underlying behavioral identity,
// separate from transient or community Mood.
type Personality struct {
	Name           string
	Traits         []string
	SpeakingStyle  []string
	TeasingRules   []string
	Boundaries     []string
	LanguagePolicy string
}

// Overrides allows optional configuration-level customization on top of the
// default Bebot personality without replacing required safety boundaries.
type Overrides struct {
	Name           string
	ExtraTraits    []string
	LanguagePolicy string
}

// DefaultPersonality returns the established default behavioral identity for The Bebot.
func DefaultPersonality() Personality {
	return Personality{
		Name: "The Bebot",
		Traits: []string{
			"casual and informal",
			"playful and witty",
			"lively and entertaining",
			"mischievous",
			"opinionated with its own point of view",
		},
		SpeakingStyle: []string{
			"Talk like a lively friend and community member, never like a formal AI assistant.",
			"Use natural humor, banter, and light sarcasm where it fits.",
			"Share genuine personal takes instead of neutral boilerplate.",
			"Keep replies conversational rather than lecturing or providing customer support.",
		},
		TeasingRules: []string{
			"Light-hearted teasing and playful banter are encouraged when appropriate.",
			"Keep jokes clearly friendly and good-natured.",
		},
		Boundaries: []string{
			"Never humiliate, insult, bully, or intentionally hurt community members.",
			"Never invent memories or claim to remember events absent from provided context.",
			"Never expose or solicit private or sensitive personal information (credentials, national IDs, financial/card numbers, contact details, precise locations, health conditions, religious beliefs, political views, sexual orientation, or minors' data).",
		},
		LanguagePolicy: "Communicate naturally in the language used by the community.",
	}
}

// NewPersonality validates and normalizes a Personality domain value.
func NewPersonality(p Personality) (Personality, error) {
	p.Name = strings.TrimSpace(p.Name)
	p.LanguagePolicy = strings.TrimSpace(p.LanguagePolicy)
	p.Traits = normalizeLines(p.Traits)
	p.SpeakingStyle = normalizeLines(p.SpeakingStyle)
	p.TeasingRules = normalizeLines(p.TeasingRules)
	p.Boundaries = normalizeLines(p.Boundaries)

	if p.Name == "" {
		return Personality{}, ErrInvalidPersonalityName
	}

	if len(p.Traits) == 0 {
		return Personality{}, ErrInvalidPersonalityTraits
	}

	if len(p.SpeakingStyle) == 0 {
		return Personality{}, ErrInvalidPersonalitySpeakingStyle
	}

	if len(p.TeasingRules) == 0 {
		return Personality{}, ErrInvalidPersonalityTeasingRules
	}

	if len(p.Boundaries) == 0 {
		return Personality{}, ErrInvalidPersonalityBoundaries
	}

	if p.LanguagePolicy == "" {
		return Personality{}, ErrInvalidPersonalityLanguagePolicy
	}

	return p, nil
}

// ApplyOverrides returns a validated Personality derived from the base profile
// with any non-empty override fields applied.
func (p Personality) ApplyOverrides(overrides Overrides) (Personality, error) {
	updated := Personality{
		Name:           p.Name,
		Traits:         append([]string(nil), p.Traits...),
		SpeakingStyle:  append([]string(nil), p.SpeakingStyle...),
		TeasingRules:   append([]string(nil), p.TeasingRules...),
		Boundaries:     append([]string(nil), p.Boundaries...),
		LanguagePolicy: p.LanguagePolicy,
	}

	if name := strings.TrimSpace(overrides.Name); name != "" {
		updated.Name = name
	}

	if policy := strings.TrimSpace(overrides.LanguagePolicy); policy != "" {
		updated.LanguagePolicy = policy
	}

	if len(overrides.ExtraTraits) > 0 {
		updated.Traits = append(updated.Traits, overrides.ExtraTraits...)
	}

	return NewPersonality(updated)
}

func normalizeLines(items []string) []string {
	result := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))

	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}

		key := strings.ToLower(trimmed)
		if _, exists := seen[key]; exists {
			continue
		}

		seen[key] = struct{}{}
		result = append(result, trimmed)
	}

	return result
}
