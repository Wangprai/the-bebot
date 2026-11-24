package gateway

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

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
		"trigger-001",
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

	if _, err := client.Send(context.Background(), "", "channel-001", "trigger-001", "hello"); err == nil {
		t.Error("expected error for empty guild ID")
	}

	if _, err := client.Send(context.Background(), "guild-001", "", "trigger-001", "hello"); err == nil {
		t.Error("expected error for empty channel ID")
	}

	if _, err := client.Send(context.Background(), "guild-001", "channel-001", "trigger-001", "   "); err == nil {
		t.Error("expected error for empty content")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := client.Send(ctx, "guild-001", "channel-001", "trigger-001", "hello"); err == nil {
		t.Error("expected error for canceled context")
	}
}

func TestSplitDiscordMessage(t *testing.T) {
	t.Run("empty and whitespace", func(t *testing.T) {
		if chunks := splitDiscordMessage("", maxDiscordMessageRunes); len(chunks) != 0 {
			t.Errorf("expected 0 chunks for empty string, got %d", len(chunks))
		}
		if chunks := splitDiscordMessage("   \n\t  ", maxDiscordMessageRunes); len(chunks) != 0 {
			t.Errorf("expected 0 chunks for whitespace string, got %d", len(chunks))
		}
	})

	t.Run("exactly 2000 runes", func(t *testing.T) {
		input := strings.Repeat("a", 2000)
		chunks := splitDiscordMessage(input, maxDiscordMessageRunes)
		if len(chunks) != 1 {
			t.Fatalf("expected 1 chunk, got %d", len(chunks))
		}
		if chunks[0] != input {
			t.Errorf("chunk mismatch: len=%d", utf8.RuneCountInString(chunks[0]))
		}
	})

	t.Run("2001 runes no whitespace", func(t *testing.T) {
		input := strings.Repeat("b", 2001)
		chunks := splitDiscordMessage(input, maxDiscordMessageRunes)
		if len(chunks) != 2 {
			t.Fatalf("expected 2 chunks, got %d", len(chunks))
		}
		if utf8.RuneCountInString(chunks[0]) != 2000 || utf8.RuneCountInString(chunks[1]) != 1 {
			t.Errorf("unexpected chunk rune lengths: %d, %d",
				utf8.RuneCountInString(chunks[0]),
				utf8.RuneCountInString(chunks[1]),
			)
		}
		if strings.Join(chunks, "") != input {
			t.Error("rejoined chunks do not match original input")
		}
	})

	t.Run("multi-byte Thai without whitespace stays valid UTF-8 and preserves combining marks", func(t *testing.T) {
		// "พี่" has 3 runes: พ (consonant), ี (Mn), ่ (Mn) -> 9 bytes per cluster.
		cluster := "พี่"
		input := strings.Repeat(cluster, 700) // 2100 runes, 6300 bytes
		chunks := splitDiscordMessage(input, maxDiscordMessageRunes)
		if len(chunks) != 2 {
			t.Fatalf("expected 2 chunks, got %d", len(chunks))
		}

		for i, chunk := range chunks {
			if !utf8.ValidString(chunk) {
				t.Fatalf("chunk %d is not valid UTF-8", i)
			}
			rc := utf8.RuneCountInString(chunk)
			if rc == 0 || rc > maxDiscordMessageRunes {
				t.Errorf("chunk %d rune count = %d, want 1..%d", i, rc, maxDiscordMessageRunes)
			}
			firstRune, _ := utf8.DecodeRuneInString(chunk)
			if unicode.Is(unicode.Mn, firstRune) {
				t.Errorf("chunk %d starts with orphaned Thai combining mark %U", i, firstRune)
			}
		}

		if strings.Join(chunks, "") != input {
			t.Error("rejoined Thai chunks do not match original input")
		}
	})

	t.Run("prefers paragraph then line then word boundaries", func(t *testing.T) {
		// Paragraph boundary
		p1 := strings.Repeat("a", 1200)
		p2 := strings.Repeat("b", 1200)
		chunks := splitDiscordMessage(p1+"\n\n"+p2, maxDiscordMessageRunes)
		if len(chunks) != 2 || chunks[0] != p1 || chunks[1] != p2 {
			t.Fatalf("paragraph split failed: got %d chunks", len(chunks))
		}

		// Line boundary
		l1 := strings.Repeat("c", 1200)
		l2 := strings.Repeat("d", 1200)
		chunks = splitDiscordMessage(l1+"\n"+l2, maxDiscordMessageRunes)
		if len(chunks) != 2 || chunks[0] != l1 || chunks[1] != l2 {
			t.Fatalf("line split failed: got %d chunks", len(chunks))
		}

		// Word boundary
		w1 := strings.Repeat("e", 1200)
		w2 := strings.Repeat("f", 1200)
		chunks = splitDiscordMessage(w1+" "+w2, maxDiscordMessageRunes)
		if len(chunks) != 2 || chunks[0] != w1 || chunks[1] != w2 {
			t.Fatalf("word split failed: got %d chunks", len(chunks))
		}
	})
}

func TestDiscordClientSendUsesEmptyAllowedMentionsAndReplyReference(t *testing.T) {
	now := time.Date(2026, 11, 24, 10, 5, 0, 0, time.UTC)

	var capturedBody map[string]any

	client := newTestDiscordClient(t, func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", req.Method)
		}

		bodyBytes, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}

		if err := json.Unmarshal(bodyBytes, &capturedBody); err != nil {
			t.Fatalf("unmarshal request body: %v", err)
		}

		return jsonResponse(http.StatusOK, &discordgo.Message{
			ID:        "sent-001",
			GuildID:   "guild-001",
			ChannelID: "channel-001",
			Content:   "@everyone @here <@123> <@&456>",
			Timestamp: now,
			Author: &discordgo.User{
				ID:  "bot-001",
				Bot: true,
			},
		}), nil
	})

	_, err := client.Send(
		context.Background(),
		"guild-001",
		"channel-001",
		"trigger-123",
		"@everyone @here <@123> <@&456>",
	)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	amRaw, ok := capturedBody["allowed_mentions"].(map[string]any)
	if !ok {
		t.Fatalf("expected allowed_mentions object in payload, got %#v", capturedBody["allowed_mentions"])
	}

	parseRaw, exists := amRaw["parse"]
	if !exists {
		t.Fatal("expected allowed_mentions.parse key in payload")
	}

	parseSlice, ok := parseRaw.([]any)
	if !ok {
		t.Fatalf("expected allowed_mentions.parse to be JSON array [], got %T (%#v)", parseRaw, parseRaw)
	}

	if len(parseSlice) != 0 {
		t.Errorf("expected empty allowed_mentions.parse, got %#v", parseSlice)
	}

	refRaw, ok := capturedBody["message_reference"].(map[string]any)
	if !ok {
		t.Fatalf("expected message_reference object in payload, got %#v", capturedBody["message_reference"])
	}

	if refRaw["message_id"] != "trigger-123" {
		t.Errorf("message_reference.message_id = %v, want trigger-123", refRaw["message_id"])
	}

	if refRaw["channel_id"] != "channel-001" {
		t.Errorf("message_reference.channel_id = %v, want channel-001", refRaw["channel_id"])
	}

	if refRaw["guild_id"] != "guild-001" {
		t.Errorf("message_reference.guild_id = %v, want guild-001", refRaw["guild_id"])
	}

	failIfNotExists, ok := refRaw["fail_if_not_exists"].(bool)
	if !ok || failIfNotExists {
		t.Errorf("message_reference.fail_if_not_exists = %#v, want false", refRaw["fail_if_not_exists"])
	}
}

func TestDiscordClientSendSplitsLongResponseAndRepliesOnlyOnFirstChunk(t *testing.T) {
	now := time.Date(2026, 11, 24, 10, 5, 0, 0, time.UTC)

	var payloads []map[string]any

	client := newTestDiscordClient(t, func(req *http.Request) (*http.Response, error) {
		bodyBytes, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}

		var body map[string]any
		if err := json.Unmarshal(bodyBytes, &body); err != nil {
			t.Fatalf("unmarshal request body: %v", err)
		}
		payloads = append(payloads, body)

		content, _ := body["content"].(string)
		return jsonResponse(http.StatusOK, &discordgo.Message{
			ID:        "sent-chunk",
			GuildID:   "guild-001",
			ChannelID: "channel-001",
			Content:   content,
			Timestamp: now,
			Author: &discordgo.User{
				ID:  "bot-001",
				Bot: true,
			},
		}), nil
	})

	longText := strings.Repeat("สวัสดี", 250) + "\n\n" + strings.Repeat("ทดสอบ", 250)
	_, err := client.Send(
		context.Background(),
		"guild-001",
		"channel-001",
		"trigger-999",
		longText,
	)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if len(payloads) != 2 {
		t.Fatalf("expected 2 chunks sent, got %d", len(payloads))
	}

	for i, payload := range payloads {
		content, _ := payload["content"].(string)
		if utf8.RuneCountInString(content) > maxDiscordMessageRunes {
			t.Errorf("chunk %d exceeds 2000 runes: %d", i, utf8.RuneCountInString(content))
		}

		am, ok := payload["allowed_mentions"].(map[string]any)
		if !ok {
			t.Fatalf("chunk %d missing allowed_mentions", i)
		}
		parse, ok := am["parse"].([]any)
		if !ok || len(parse) != 0 {
			t.Errorf("chunk %d allowed_mentions.parse = %#v, want []", i, am["parse"])
		}

		_, hasRef := payload["message_reference"]
		if i == 0 && !hasRef {
			t.Error("first chunk must include message_reference")
		}
		if i > 0 && hasRef {
			t.Errorf("chunk %d must not include message_reference", i)
		}
	}
}

func TestDiscordClientStartTyping(t *testing.T) {
	called := false

	client := newTestDiscordClient(t, func(req *http.Request) (*http.Response, error) {
		called = true
		if req.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", req.Method)
		}
		if !strings.Contains(req.URL.Path, "/channels/channel-001/typing") {
			t.Errorf("unexpected path: %s", req.URL.Path)
		}
		return &http.Response{
			StatusCode: http.StatusNoContent,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})

	if err := client.StartTyping(context.Background(), "channel-001"); err != nil {
		t.Fatalf("StartTyping() error = %v", err)
	}

	if !called {
		t.Fatal("expected typing endpoint to be called")
	}

	if err := client.StartTyping(context.Background(), ""); err == nil {
		t.Error("expected error for empty channel ID")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := client.StartTyping(ctx, "channel-001"); err == nil {
		t.Error("expected error for canceled context")
	}
}
