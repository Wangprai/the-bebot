package conversation

import (
	"context"
	"fmt"

	conversationcore "the-bebot/internal/conversation/core"
	personalityapp "the-bebot/internal/personality/app"
)

type PersonalityProviderAdapter struct {
	useCase *personalityapp.GetPersonalityUseCase
}

func NewPersonalityProviderAdapter(
	useCase *personalityapp.GetPersonalityUseCase,
) *PersonalityProviderAdapter {
	return &PersonalityProviderAdapter{
		useCase: useCase,
	}
}

func (a *PersonalityProviderAdapter) GetPersonality(
	ctx context.Context,
) (conversationcore.PersonalityContext, error) {
	if a == nil || a.useCase == nil {
		return conversationcore.PersonalityContext{}, fmt.Errorf("personality use case is nil")
	}

	profile, err := a.useCase.Execute(ctx)
	if err != nil {
		return conversationcore.PersonalityContext{}, fmt.Errorf("get personality: %w", err)
	}

	return conversationcore.PersonalityContext{
		Name:           profile.Name,
		Traits:         profile.Traits,
		SpeakingStyle:  profile.SpeakingStyle,
		TeasingRules:   profile.TeasingRules,
		Boundaries:     profile.Boundaries,
		LanguagePolicy: profile.LanguagePolicy,
	}, nil
}

var _ conversationcore.PersonalityProvider = (*PersonalityProviderAdapter)(nil)
