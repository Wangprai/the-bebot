package gateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
)

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newTestDiscordClient(t *testing.T, fn roundTripFunc) *DiscordClient {
	t.Helper()

	session, err := discordgo.New("Bot test-token")
	if err != nil {
		t.Fatalf("discordgo.New() error = %v", err)
	}

	session.Client = &http.Client{
		Transport: fn,
	}

	client, err := NewDiscordClient(session)
	if err != nil {
		t.Fatalf("NewDiscordClient() error = %v", err)
	}

	return client
}

func jsonResponse(statusCode int, payload any) *http.Response {
	body, _ := json.Marshal(payload)

	return &http.Response{
		StatusCode: statusCode,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
		},
		Body: io.NopCloser(strings.NewReader(string(body))),
	}
}

func TestNewDiscordClientRejectsNilSession(t *testing.T) {
	client, err := NewDiscordClient(nil)
	if err == nil {
		t.Fatal("expected error for nil session")
	}

	if client != nil {
		t.Fatal("expected nil client")
	}
}

func TestDiscordClientFetchRecentMessagesHandlesRESTGuildIDAndFiltersScope(t *testing.T) {
	now := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)

	client := newTestDiscordClient(t, func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", req.Method)
		}

		if !strings.Contains(req.URL.Path, "/channels/channel-001/messages") {
			t.Errorf("unexpected path: %s", req.URL.Path)
		}

		messages := []*discordgo.Message{
			{
				ID:        "msg-rest-no-guild",
				GuildID:   "", // Discord REST ChannelMessages omits guild_id
				ChannelID: "channel-001",
				Content:   "Recent channel message without guild_id",
				Timestamp: now.Add(-2 * time.Minute),
				Author: &discordgo.User{
					ID:  "user-001",
					Bot: false,
				},
			},
			{
				ID:        "msg-matching-guild",
				GuildID:   "guild-001",
				ChannelID: "channel-001",
				Content:   "Message with matching guild_id",
				Timestamp: now.Add(-time.Minute),
				Author: &discordgo.User{
					ID:  "user-002",
					Bot: true,
				},
			},
			{
				ID:        "msg-wrong-guild",
				GuildID:   "guild-other",
				ChannelID: "channel-001",
				Content:   "Should be rejected",
				Timestamp: now,
				Author: &discordgo.User{
					ID: "user-003",
				},
			},
			{
				ID:        "msg-wrong-channel",
				GuildID:   "",
				ChannelID: "channel-other",
				Content:   "Should be rejected",
				Timestamp: now,
				Author: &discordgo.User{
					ID: "user-004",
				},
			},
			{
				ID:        "msg-empty-content",
				GuildID:   "",
				ChannelID: "channel-001",
				Content:   "   ",
				Timestamp: now,
				Author: &discordgo.User{
					ID: "user-005",
				},
			},
			{
				ID:        "msg-nil-author",
				GuildID:   "",
				ChannelID: "channel-001",
				Content:   "No author",
				Timestamp: now,
				Author:    nil,
			},
		}

		return jsonResponse(http.StatusOK, messages), nil
	})

	messages, err := client.FetchRecentMessages(
		context.Background(),
		"guild-001",
		"channel-001",
		10,
	)
	if err != nil {
		t.Fatalf("FetchRecentMessages() error = %v", err)
	}

	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d: %+v", len(messages), messages)
	}

	if messages[0].ID != "msg-rest-no-guild" {
		t.Errorf("first ID = %s, want msg-rest-no-guild", messages[0].ID)
	}

	if messages[0].GuildID != "guild-001" {
		t.Errorf("first GuildID = %s, want guild-001", messages[0].GuildID)
	}

	if messages[0].UserID != "user-001" || messages[0].IsBot {
		t.Errorf("unexpected first message author metadata: %+v", messages[0])
	}

	if messages[1].ID != "msg-matching-guild" {
		t.Errorf("second ID = %s, want msg-matching-guild", messages[1].ID)
	}

	if messages[1].GuildID != "guild-001" || !messages[1].IsBot {
		t.Errorf("unexpected second message metadata: %+v", messages[1])
	}
}

func TestDiscordClientFetchRecentMessagesValidation(t *testing.T) {
	client := newTestDiscordClient(t, func(req *http.Request) (*http.Response, error) {
		t.Fatal("HTTP request should not be sent for invalid inputs")
		return nil, nil
	})

	if _, err := client.FetchRecentMessages(context.Background(), "", "channel-001", 5); err == nil {
		t.Error("expected error for empty guild ID")
	}

	if _, err := client.FetchRecentMessages(context.Background(), "guild-001", "", 5); err == nil {
		t.Error("expected error for empty channel ID")
	}

	if _, err := client.FetchRecentMessages(context.Background(), "guild-001", "channel-001", 0); err == nil {
		t.Error("expected error for non-positive limit")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := client.FetchRecentMessages(ctx, "guild-001", "channel-001", 5); err == nil {
		t.Error("expected error for canceled context")
	}
}

func TestDiscordClientSendPreservesGuildIDFallback(t *testing.T) {
	now := time.Date(2026, 11, 24, 10, 5, 0, 0, time.UTC)

	client := newTestDiscordClient(t, func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", req.Method)
		}

		return jsonResponse(http.StatusOK, &discordgo.Message{
			ID:        "sent-001",
			GuildID:   "", // Discord REST response may omit guild_id
			ChannelID: "channel-001",
			Content:   "Hello community!",
			Timestamp: now,
			Author: &discordgo.User{
				ID:  "bot-001",
				Bot: true,
			},
		}), nil
	})

	sent, err := client.Send(
		context.Background(),
		"guild-001",
		"channel-001",
		"Hello community!",
	)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if sent.ID != "sent-001" {
		t.Errorf("ID = %s, want sent-001", sent.ID)
	}

	if sent.GuildID != "guild-001" {
		t.Errorf("GuildID = %s, want guild-001", sent.GuildID)
	}

	if sent.ChannelID != "channel-001" {
		t.Errorf("ChannelID = %s, want channel-001", sent.ChannelID)
	}

	if !sent.IsBot {
		t.Error("expected sent message IsBot to be true")
	}
}

func TestDiscordClientSendValidation(t *testing.T) {
	client := newTestDiscordClient(t, func(req *http.Request) (*http.Response, error) {
		t.Fatal("HTTP request should not be sent for invalid inputs")
		return nil, nil
	})

	if _, err := client.Send(context.Background(), "", "channel-001", "hello"); err == nil {
		t.Error("expected error for empty guild ID")
	}

	if _, err := client.Send(context.Background(), "guild-001", "", "hello"); err == nil {
		t.Error("expected error for empty channel ID")
	}

	if _, err := client.Send(context.Background(), "guild-001", "channel-001", "   "); err == nil {
		t.Error("expected error for empty content")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := client.Send(ctx, "guild-001", "channel-001", "hello"); err == nil {
		t.Error("expected error for canceled context")
	}
}
