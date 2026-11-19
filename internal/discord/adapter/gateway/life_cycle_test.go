package gateway

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"the-bebot/internal/conversation/core"
)

type testMentionHandler struct{}

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
