package conversation

import (
	"context"
	"testing"
	"time"

	communityapp "the-bebot/internal/community/app"
	communitycore "the-bebot/internal/community/core"
	conversationcore "the-bebot/internal/conversation/core"
)

type stubCommunityRepo struct{}

func (stubCommunityRepo) UpsertInteraction(
	ctx context.Context,
	input communitycore.CommunityUpsertInput,
) (communitycore.Community, error) {
	return communitycore.Community{
		GuildID:     input.GuildID,
		Name:        input.Name,
		ActiveSince: input.SeenAt,
		UpdatedAt:   input.SeenAt,
	}, nil
}

func (stubCommunityRepo) FindByGuildID(ctx context.Context, guildID string) (*communitycore.Community, error) {
	return nil, nil
}

type stubMemberRepo struct {
	incrementCalls int
}

func (s *stubMemberRepo) UpsertInteraction(
	ctx context.Context,
	input communitycore.MemberUpsertInput,
) (communitycore.Member, error) {
	return communitycore.Member{
		GuildID:          input.GuildID,
		UserID:           input.UserID,
		DisplayName:      input.DisplayName,
		Nickname:         input.Nickname,
		InteractionCount: 2,
		LastSeenAt:       input.SeenAt,
	}, nil
}

func (s *stubMemberRepo) IncrementInteractionCount(
	ctx context.Context,
	guildID string,
	userID string,
	interactedAt time.Time,
) error {
	s.incrementCalls++
	return nil
}

func (s *stubMemberRepo) FindByGuildAndUser(ctx context.Context, guildID, userID string) (*communitycore.Member, error) {
	return nil, nil
}

func TestCommunityContextAdapterSuccess(t *testing.T) {
	memberRepo := &stubMemberRepo{}
	uc, err := communityapp.NewSyncContextUseCase(stubCommunityRepo{}, memberRepo)
	if err != nil {
		t.Fatalf("NewSyncContextUseCase() error = %v", err)
	}

	adapter := NewCommunityContextAdapter(uc)

	event := conversationcore.MentionEvent{
		GuildID:         "guild-1",
		GuildName:       "Dev Club",
		ChannelID:       "ch-1",
		MessageID:       "msg-1",
		UserID:          "user-1",
		UserDisplayName: "Alice",
		UserNickname:    "Ali",
		Content:         "hi",
		MentionedAt:     time.Now().UTC(),
	}

	got, err := adapter.SyncMentionContext(context.Background(), event)
	if err != nil {
		t.Fatalf("SyncMentionContext() error = %v", err)
	}

	if got.GuildName != "Dev Club" ||
		got.MemberName != "Alice" ||
		got.MemberNickname != "Ali" ||
		got.InteractionCount != 2 {
		t.Fatalf("unexpected CommunityContext: %+v", got)
	}

	if memberRepo.incrementCalls != 0 {
		t.Fatalf("SyncMentionContext must not increment count, got %d", memberRepo.incrementCalls)
	}

	if err := adapter.RecordInteraction(context.Background(), event); err != nil {
		t.Fatalf("RecordInteraction() error = %v", err)
	}

	if memberRepo.incrementCalls != 1 {
		t.Fatalf("RecordInteraction calls = %d, want 1", memberRepo.incrementCalls)
	}
}

func TestCommunityContextAdapterRejectsNil(t *testing.T) {
	var nilAdapter *CommunityContextAdapter
	if _, err := nilAdapter.SyncMentionContext(context.Background(), conversationcore.MentionEvent{}); err == nil {
		t.Fatal("expected error for nil adapter")
	}

	adapter := NewCommunityContextAdapter(nil)
	if _, err := adapter.SyncMentionContext(context.Background(), conversationcore.MentionEvent{}); err == nil {
		t.Fatal("expected error for nil use case")
	}
}
