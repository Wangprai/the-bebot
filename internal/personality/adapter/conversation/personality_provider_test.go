package conversation

import (
	"context"
	"errors"
	"testing"

	personalityapp "the-bebot/internal/personality/app"
	personalitycore "the-bebot/internal/personality/core"
)

func TestPersonalityProviderAdapterSuccess(t *testing.T) {
	uc, err := personalityapp.NewGetPersonalityUseCase(personalitycore.Overrides{})
	if err != nil {
		t.Fatalf("NewGetPersonalityUseCase() error = %v", err)
	}

	adapter := NewPersonalityProviderAdapter(uc)

	got, err := adapter.GetPersonality(context.Background())
	if err != nil {
		t.Fatalf("GetPersonality() error = %v", err)
	}

	if got.Name != "The Bebot" {
		t.Errorf("Name = %q, want The Bebot", got.Name)
	}

	if len(got.Traits) == 0 ||
		len(got.SpeakingStyle) == 0 ||
		len(got.TeasingRules) == 0 ||
		len(got.Boundaries) == 0 ||
		got.LanguagePolicy == "" {
		t.Fatalf("incomplete PersonalityContext: %+v", got)
	}
}

func TestPersonalityProviderAdapterRejectsNilAndCanceledContext(t *testing.T) {
	var nilAdapter *PersonalityProviderAdapter
	if _, err := nilAdapter.GetPersonality(context.Background()); err == nil {
		t.Fatal("expected error for nil adapter")
	}

	emptyAdapter := NewPersonalityProviderAdapter(nil)
	if _, err := emptyAdapter.GetPersonality(context.Background()); err == nil {
		t.Fatal("expected error for nil use case")
	}

	uc, _ := personalityapp.NewGetPersonalityUseCase(personalitycore.Overrides{})
	adapter := NewPersonalityProviderAdapter(uc)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := adapter.GetPersonality(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}
