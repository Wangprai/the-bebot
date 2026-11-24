package app

import (
	"context"
	"errors"

	"the-bebot/internal/personality/core"
)

// GetPersonalityUseCase provides The Bebot's global Go-owned Personality profile.
type GetPersonalityUseCase struct {
	personality core.Personality
}

func NewGetPersonalityUseCase(
	overrides core.Overrides,
) (*GetPersonalityUseCase, error) {
	profile, err := core.DefaultPersonality().ApplyOverrides(overrides)
	if err != nil {
		return nil, err
	}

	return &GetPersonalityUseCase{
		personality: profile,
	}, nil
}

func (uc *GetPersonalityUseCase) Execute(
	ctx context.Context,
) (core.Personality, error) {
	if uc == nil {
		return core.Personality{}, errors.New("get personality use case is nil")
	}

	if ctx == nil {
		return core.Personality{}, errors.New("context is required")
	}

	if err := ctx.Err(); err != nil {
		return core.Personality{}, err
	}

	// Return defensive copies of slices so callers cannot mutate shared state.
	return core.Personality{
		Name:           uc.personality.Name,
		Traits:         append([]string(nil), uc.personality.Traits...),
		SpeakingStyle:  append([]string(nil), uc.personality.SpeakingStyle...),
		TeasingRules:   append([]string(nil), uc.personality.TeasingRules...),
		Boundaries:     append([]string(nil), uc.personality.Boundaries...),
		LanguagePolicy: uc.personality.LanguagePolicy,
	}, nil
}
