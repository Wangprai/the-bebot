package gateway

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"the-bebot/internal/conversation/core"
	"time"

	"github.com/bwmarrin/discordgo"
)

type testMentionHandler struct{}

type blockingMentionHandler struct {
	started chan struct{}
	release chan struct{}
}

func (h *blockingMentionHandler) HandleMention(
	ctx context.Context,
	event core.MentionEvent,
) error {
	h.started <- struct{}{}
	<-h.release
	return nil
}

func (h *testMentionHandler) HandleMention(
	ctx context.Context,
	event core.MentionEvent,
) error {
	return nil
}

func TestSetMentionHandlerRejectsNil(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	g, err := NewGateway("test-token", logger, nil)
	if err != nil {
		t.Fatalf("NewGateway() error = %v", err)
	}
	defer g.Close()

	err = g.SetMentionHandler(nil)
	if err == nil {
		t.Fatal("expected error for nil handler")
	}
}

func TestStartRejectsNilHandler(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	g, err := NewGateway("test-token", logger, nil)
	if err != nil {
		t.Fatalf("NewGateway() error = %v", err)
	}
	defer g.Close()

	err = g.Start(context.Background())
	if err == nil {
		t.Fatal("expected error for missing handler")
	}
}

func TestSetMentionHandlerAfterClose(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	g, err := NewGateway("test-token", logger, nil)
	if err != nil {
		t.Fatalf("NewGateway() error = %v", err)
	}

	if err := g.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	err = g.SetMentionHandler(&testMentionHandler{})
	if err == nil {
		t.Fatal("expected error after gateway closed")
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	g, err := NewGateway("test-token", logger, nil)
	if err != nil {
		t.Fatalf("NewGateway() error = %v", err)
	}

	if err := g.Close(); err != nil {
		t.Fatalf("first Close() error = %v", err)
	}

	if err := g.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

func TestCloseWaitsForInFlightHandler(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	handler := &blockingMentionHandler{
		started: make(chan struct{}, 2),
		release: make(chan struct{}),
	}

	g, err := NewGateway("test-token", logger, handler)
	if err != nil {
		t.Fatalf("NewGateway() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	g.mu.Lock()
	g.ctx = ctx
	g.cancel = cancel
	g.started = true
	g.botUserID = "bot"
	g.mu.Unlock()

	event := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ID:        "message-1",
			GuildID:   "guild-1",
			ChannelID: "channel-1",
			Content:   "<@bot>",
			Timestamp: time.Now(),
			Author: &discordgo.User{
				ID: "user-1",
			},
			Mentions: []*discordgo.User{
				{ID: "bot"},
			},
		},
	}

	go g.handleMessageCreate(nil, event)

	select {
	case <-handler.started:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not start")
	}

	closeDone := make(chan error, 1)

	go func() {
		closeDone <- g.Close()
	}()

	select {
	case err := <-closeDone:
		t.Fatalf("Close() returned before handler completed: %v", err)
	case <-time.After(100 * time.Millisecond):
		// Close is correctly waiting for the in-flight handler.
	}

	close(handler.release)

	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Close() did not return after handler completed")
	}

	// Verify that no new handler starts after shutdown.
	g.handleMessageCreate(nil, event)

	select {
	case <-handler.started:
		t.Fatal("handler should not start after gateway closed")
	default:
	}
}

type recordingMentionHandler struct {
	called int
}

func (h *recordingMentionHandler) HandleMention(
	ctx context.Context,
	event core.MentionEvent,
) error {
	h.called++
	return nil
}

func TestHandleMessageCreateIgnoresBotAuthors(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := &recordingMentionHandler{}

	g, err := NewGateway("test-token", logger, handler)
	if err != nil {
		t.Fatalf("NewGateway() error = %v", err)
	}
	defer g.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	g.mu.Lock()
	g.ctx = ctx
	g.cancel = cancel
	g.started = true
	g.botUserID = "bebot-001"
	g.mu.Unlock()

	tests := []struct {
		name           string
		author         *discordgo.User
		expectedCalled int
	}{
		{
			name: "ignores another bot mentioning The Bebot",
			author: &discordgo.User{
				ID:  "other-bot-999",
				Bot: true,
			},
			expectedCalled: 0,
		},
		{
			name: "ignores The Bebot self message even if Bot flag false",
			author: &discordgo.User{
				ID:  "bebot-001",
				Bot: false,
			},
			expectedCalled: 0,
		},
		{
			name: "handles normal human mention",
			author: &discordgo.User{
				ID:  "human-001",
				Bot: false,
			},
			expectedCalled: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler.called = 0

			event := &discordgo.MessageCreate{
				Message: &discordgo.Message{
					ID:        "message-1",
					GuildID:   "guild-1",
					ChannelID: "channel-1",
					Content:   "<@bebot-001> hello",
					Timestamp: time.Now(),
					Author:    tt.author,
					Mentions: []*discordgo.User{
						{ID: "bebot-001"},
					},
				},
			}

			g.handleMessageCreate(nil, event)

			if handler.called != tt.expectedCalled {
				t.Errorf("handler.called = %d, want %d", handler.called, tt.expectedCalled)
			}
		})
	}
}
