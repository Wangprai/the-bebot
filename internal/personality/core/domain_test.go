package core

import (
	"errors"
	"testing"
)

func TestDefaultPersonalityIsValid(t *testing.T) {
	defaultProfile := DefaultPersonality()

	validated, err := NewPersonality(defaultProfile)
	if err != nil {
		t.Fatalf("DefaultPersonality() failed validation: %v", err)
	}

	if validated.Name != "The Bebot" {
		t.Errorf("Name = %q, want The Bebot", validated.Name)
	}

	if len(validated.Traits) == 0 ||
		len(validated.SpeakingStyle) == 0 ||
		len(validated.TeasingRules) == 0 ||
		len(validated.Boundaries) == 0 ||
		validated.LanguagePolicy == "" {
		t.Fatalf("default personality is missing required sections: %+v", validated)
	}
}

func TestNewPersonalityRejectsInvalidFields(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Personality)
		wantErr error
	}{
		{
			name:    "empty name",
			modify:  func(p *Personality) { p.Name = "   " },
			wantErr: ErrInvalidPersonalityName,
		},
		{
			name:    "empty traits",
			modify:  func(p *Personality) { p.Traits = []string{" ", ""} },
			wantErr: ErrInvalidPersonalityTraits,
		},
		{
			name:    "empty speaking style",
			modify:  func(p *Personality) { p.SpeakingStyle = nil },
			wantErr: ErrInvalidPersonalitySpeakingStyle,
		},
		{
			name:    "empty teasing rules",
			modify:  func(p *Personality) { p.TeasingRules = nil },
			wantErr: ErrInvalidPersonalityTeasingRules,
		},
		{
			name:    "empty boundaries",
			modify:  func(p *Personality) { p.Boundaries = nil },
			wantErr: ErrInvalidPersonalityBoundaries,
		},
		{
			name:    "empty language policy",
			modify:  func(p *Personality) { p.LanguagePolicy = " " },
			wantErr: ErrInvalidPersonalityLanguagePolicy,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := DefaultPersonality()
			tt.modify(&p)

			_, err := NewPersonality(p)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestPersonalityApplyOverrides(t *testing.T) {
	base := DefaultPersonality()

	customized, err := base.ApplyOverrides(Overrides{
		Name:           "Bebot Jr.",
		ExtraTraits:    []string{"  curious  ", "Playful and Witty", "energetic"},
		LanguagePolicy: "Prefer Thai unless the user speaks English.",
	})
	if err != nil {
		t.Fatalf("ApplyOverrides() error = %v", err)
	}

	if customized.Name != "Bebot Jr." {
		t.Errorf("Name = %q, want Bebot Jr.", customized.Name)
	}

	if customized.LanguagePolicy != "Prefer Thai unless the user speaks English." {
		t.Errorf("unexpected LanguagePolicy: %q", customized.LanguagePolicy)
	}

	// Duplicate trait ("Playful and Witty") should be deduplicated case-insensitively.
	expectedCount := len(base.Traits) + 2
	if len(customized.Traits) != expectedCount {
		t.Errorf("len(Traits) = %d, want %d (%v)", len(customized.Traits), expectedCount, customized.Traits)
	}

	// Base boundaries must remain intact.
	if len(customized.Boundaries) != len(base.Boundaries) {
		t.Error("overrides must preserve base boundaries")
	}

	// Base personality must not be mutated.
	if base.Name != "The Bebot" {
		t.Error("base personality was mutated")
	}
}
