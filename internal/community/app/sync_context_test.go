package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"the-bebot/internal/community/core"
)

type mockCommunityRepo struct {
	community core.Community
	err       error
	lastInput core.CommunityUpsertInput
}

func (m *mockCommunityRepo) UpsertInteraction(
	ctx context.Context,
	input core.CommunityUpsertInput,
) (core.Community, error) {
	m.lastInput = input
	if m.err != nil {
		return core.Community{}, m.err
	}
	return m.community, nil
}

func (m *mockCommunityRepo) FindByGuildID(ctx context.Context, guildID string) (*core.Community, error) {
	return nil, nil
}

type mockMemberRepo struct {
	member    core.Member
	err       error
	lastInput core.MemberUpsertInput
}

func (m *mockMemberRepo) UpsertInteraction(
	ctx context.Context,
	input core.MemberUpsertInput,
) (core.Member, error) {
	m.lastInput = input
	if m.err != nil {
		return core.Member{}, m.err
	}
	return m.member, nil
}

func (m *mockMemberRepo) FindByGuildAndUser(ctx context.Context, guildID, userID string) (*core.Member, error) {
	return nil, nil
}

func TestSyncContextUseCaseSuccess(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)

	commRepo := &mockCommunityRepo{
		community: core.Community{
			GuildID:     "guild-1",
			Name:        "Go Hangout",
			ActiveSince: now,
			UpdatedAt:   now,
		},
	}

	memRepo := &mockMemberRepo{
		member: core.Member{
			GuildID:          "guild-1",
			UserID:           "user-1",
			DisplayName:      "Bob",
			Nickname:         "Bobby",
			InteractionCount: 4,
			LastSeenAt:       now,
		},
	}

	uc, err := NewSyncContextUseCase(commRepo, memRepo)
	if err != nil {
		t.Fatalf("NewSyncContextUseCase() error = %v", err)
	}

	result, err := uc.Execute(context.Background(), SyncInteractionInput{
		GuildID:     "guild-1",
		GuildName:   "Go Hangout",
		UserID:      "user-1",
		DisplayName: "Bob",
		Nickname:    "Bobby",
		SeenAt:      now,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if result.Community.Name != "Go Hangout" || result.Member.InteractionCount != 4 {
		t.Fatalf("unexpected synced context: %+v", result)
	}
}

func TestSyncContextUseCaseErrorPropagation(t *testing.T) {
	if _, err := NewSyncContextUseCase(nil, &mockMemberRepo{}); err == nil {
		t.Fatal("expected error for nil community repo")
	}
	if _, err := NewSyncContextUseCase(&mockCommunityRepo{}, nil); err == nil {
		t.Fatal("expected error for nil member repo")
	}

	expectedErr := errors.New("db error")
	uc, _ := NewSyncContextUseCase(&mockCommunityRepo{err: expectedErr}, &mockMemberRepo{})

	if _, err := uc.Execute(context.Background(), SyncInteractionInput{GuildID: "g1", UserID: "u1"}); !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := uc.Execute(ctx, SyncInteractionInput{GuildID: "g1", UserID: "u1"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}
