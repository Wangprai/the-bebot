package app

import (
	"context"
	"errors"
	"testing"

	"the-bebot/internal/personality/core"
)

func TestGetPersonalityUseCaseDefaultAndDefensiveCopy(t *testing.T) {
	uc, err := NewGetPersonalityUseCase(core.Overrides{})
	if err != nil {
		t.Fatalf("NewGetPersonalityUseCase() error = %v", err)
	}

	first, err := uc.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if first.Name != "The Bebot" {
		t.Errorf("Name = %q, want The Bebot", first.Name)
	}

	// Mutating returned slice must not affect future calls.
	first.Traits[0] = "mutated"

	second, err := uc.Execute(context.Background())
	if err != nil {
		t.Fatalf("second Execute() error = %v", err)
	}

	if second.Traits[0] == "mutated" {
		t.Fatal("returned Personality slice was not defensively copied")
	}
}

func TestGetPersonalityUseCaseOverrides(t *testing.T) {
	uc, err := NewGetPersonalityUseCase(core.Overrides{
		Name:           "The Bebot Custom",
		ExtraTraits:    []string{"observant"},
		LanguagePolicy: "Use natural Thai.",
	})
	if err != nil {
		t.Fatalf("NewGetPersonalityUseCase() error = %v", err)
	}

	profile, err := uc.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if profile.Name != "The Bebot Custom" {
		t.Errorf("Name = %q, want The Bebot Custom", profile.Name)
	}

	if profile.LanguagePolicy != "Use natural Thai." {
		t.Errorf("LanguagePolicy = %q", profile.LanguagePolicy)
	}
}

func TestGetPersonalityUseCaseValidationErrors(t *testing.T) {
	var nilUC *GetPersonalityUseCase
	if _, err := nilUC.Execute(context.Background()); err == nil {
		t.Fatal("expected error for nil use case")
	}

	uc, err := NewGetPersonalityUseCase(core.Overrides{})
	if err != nil {
		t.Fatalf("NewGetPersonalityUseCase() error = %v", err)
	}

	if _, err := uc.Execute(context.Context(nil)); err == nil {
		t.Fatal("expected error for nil context")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := uc.Execute(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}
