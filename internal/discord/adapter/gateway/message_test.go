package gateway

import (
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
)

func TestIsBotMentioned(t *testing.T) {
	bot := &discordgo.User{
		ID: "bot-001",
	}

	otherBot := &discordgo.User{
		ID: "bot-002",
	}

	tests := []struct {
		name     string
		message  *discordgo.Message
		botID    string
		expected bool
	}{
		{
			name: "bot is mentioned",
			message: &discordgo.Message{
				Mentions: []*discordgo.User{bot},
			},
			botID:    "bot-001",
			expected: true,
		},
		{
			name: "another bot is mentioned",
			message: &discordgo.Message{
				Mentions: []*discordgo.User{otherBot},
			},
			botID:    "bot-001",
			expected: false,
		},
		{
			name: "no mentions",
			message: &discordgo.Message{
				Mentions: []*discordgo.User{},
			},
			botID:    "bot-001",
			expected: false,
		},
		{
			name:     "nil message",
			message:  nil,
			botID:    "bot-001",
			expected: false,
		},
		{
			name: "empty bot ID",
			message: &discordgo.Message{
				Mentions: []*discordgo.User{bot},
			},
			botID:    "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isBotMentioned(
				tt.message,
				tt.botID,
			)

			if result != tt.expected {
				t.Errorf(
					"expected %v, got %v",
					tt.expected,
					result,
				)
			}
		})
	}
}

func TestToMentionEvent(t *testing.T) {
	now := time.Now()

	message := &discordgo.Message{
		ID:        "message-001",
		GuildID:   "guild-001",
		ChannelID: "channel-001",
		Content:   "@TheBebot hello",
		Timestamp: now,
		Author: &discordgo.User{
			ID: "user-001",
		},
	}

	event, err := toMentionEvent(message)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if event.GuildID != "guild-001" {
		t.Errorf("unexpected guild ID: %s", event.GuildID)
	}

	if event.ChannelID != "channel-001" {
		t.Errorf("unexpected channel ID: %s", event.ChannelID)
	}

	if event.MessageID != "message-001" {
		t.Errorf("unexpected message ID: %s", event.MessageID)
	}

	if event.UserID != "user-001" {
		t.Errorf("unexpected user ID: %s", event.UserID)
	}
}

func TestToMentionEventRejectsNilMessage(t *testing.T) {
	_, err := toMentionEvent(nil)

	if err == nil {
		t.Fatal("expected error for nil message")
	}
}

func TestToMentionEventExtractsRealDiscordIdentityMetadata(t *testing.T) {
	now := time.Now().UTC()

	msgWithGlobalAndNick := &discordgo.Message{
		ID:        "msg-1",
		GuildID:   "guild-1",
		ChannelID: "ch-1",
		Content:   "<@bot> hi",
		Timestamp: now,
		Author: &discordgo.User{
			ID:         "user-1",
			Username:   "alice_dev",
			GlobalName: "Alice",
		},
		Member: &discordgo.Member{
			Nick: "Ali",
		},
	}

	event, err := toMentionEvent(msgWithGlobalAndNick)
	if err != nil {
		t.Fatalf("toMentionEvent() error = %v", err)
	}

	if event.UserDisplayName != "Alice" || event.UserNickname != "Ali" {
		t.Errorf("unexpected identity metadata: display=%q nick=%q", event.UserDisplayName, event.UserNickname)
	}

	// Falls back to Username when GlobalName is empty, and leaves Nickname empty if absent.
	msgUsernameOnly := &discordgo.Message{
		ID:        "msg-2",
		GuildID:   "guild-1",
		ChannelID: "ch-1",
		Content:   "<@bot> hi",
		Timestamp: now,
		Author: &discordgo.User{
			ID:       "user-2",
			Username: "bob_dev",
		},
	}

	event2, err := toMentionEvent(msgUsernameOnly)
	if err != nil {
		t.Fatalf("toMentionEvent() error = %v", err)
	}

	if event2.UserDisplayName != "bob_dev" || event2.UserNickname != "" {
		t.Errorf("unexpected fallback metadata: display=%q nick=%q", event2.UserDisplayName, event2.UserNickname)
	}
}
