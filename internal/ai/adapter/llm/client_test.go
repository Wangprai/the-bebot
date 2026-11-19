package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"the-bebot/internal/conversation/core"
)

func newTestClient(t *testing.T, endpoint string) *Client {
	t.Helper()

	client, err := NewClient(Config{
		Endpoint: endpoint,
		APIKey:   "test-api-key",
		Model:    "test-model",
		Timeout:  5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	return client
}

func TestNewClientRejectsInvalidConfig(t *testing.T) {
	tests := []struct {
		name   string
		config Config
	}{
		{
			name: "missing endpoint",
			config: Config{
				APIKey: "test-key",
				Model:  "test-model",
			},
		},
		{
			name: "invalid endpoint",
			config: Config{
				Endpoint: "not-a-url",
				APIKey:   "test-key",
				Model:    "test-model",
			},
		},
		{
			name: "missing API key",
			config: Config{
				Endpoint: "https://example.com/v1/chat/completions",
				Model:    "test-model",
			},
		},
		{
			name: "missing model",
			config: Config{
				Endpoint: "https://example.com/v1/chat/completions",
				APIKey:   "test-key",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewClient(tt.config)

			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestGenerateSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("method = %s, want POST", r.Method)
			}

			if r.Header.Get("Authorization") != "Bearer test-api-key" {
				t.Errorf("unexpected Authorization header")
			}

			if r.Header.Get("Content-Type") != "application/json" {
				t.Errorf("unexpected Content-Type")
			}

			var request chatRequest

			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode request: %v", err)
				return
			}

			if request.Model != "test-model" {
				t.Errorf("model = %s, want test-model", request.Model)
			}

			if len(request.Messages) != 3 {
				t.Fatalf("messages length = %d, want 3", len(request.Messages))
			}

			if request.Messages[0].Role != "system" {
				t.Errorf("first role = %s, want system", request.Messages[0].Role)
			}

			if request.Messages[1].Content != "Hello Bebot" {
				t.Errorf("unexpected first conversation message")
			}

			if request.Messages[2].Content != "[Another bot said]: Hello" {
				t.Errorf("unexpected bot message: %s", request.Messages[2].Content)
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)

			_, _ = w.Write([]byte(`{
				"choices": [
					{
						"message": {
							"role": "assistant",
							"content": "  เอ้า เรียกฉันทำไมเนี่ย  "
						}
					}
				]
			}`))
		},
	))
	defer server.Close()

	client := newTestClient(t, server.URL)

	conversationContext := core.ConversationContext{
		Messages: []core.Message{
			{
				Content: "Hello Bebot",
			},
			{
				Content: "Hello",
				IsBot:   true,
			},
		},
	}

	got, err := client.Generate(context.Background(), conversationContext)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	want := "เอ้า เรียกฉันทำไมเนี่ย"

	if got != want {
		t.Errorf("Generate() = %q, want %q", got, want)
	}
}

func TestGenerateReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"message":"Invalid API key"}}`))
		},
	))
	defer server.Close()

	client := newTestClient(t, server.URL)

	_, err := client.Generate(
		context.Background(),
		core.ConversationContext{
			Messages: []core.Message{
				{Content: "Hello"},
			},
		},
	)

	if err == nil {
		t.Fatal("expected API error, got nil")
	}

	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected status 401, got %v", err)
	}
}

func TestGenerateRejectsEmptyChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[]}`))
		},
	))
	defer server.Close()

	client := newTestClient(t, server.URL)

	_, err := client.Generate(
		context.Background(),
		core.ConversationContext{
			Messages: []core.Message{
				{Content: "Hello"},
			},
		},
	)

	if err == nil {
		t.Fatal("expected error for empty choices")
	}
}

func TestGenerateRejectsEmptyConversation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			t.Fatal("HTTP request should not be sent")
		},
	))
	defer server.Close()

	client := newTestClient(t, server.URL)

	_, err := client.Generate(
		context.Background(),
		core.ConversationContext{},
	)

	if err == nil {
		t.Fatal("expected error for empty conversation")
	}
}

func TestGenerateRespectsCanceledContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			t.Fatal("HTTP request should not be sent")
		},
	))
	defer server.Close()

	client := newTestClient(t, server.URL)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.Generate(
		ctx,
		core.ConversationContext{
			Messages: []core.Message{
				{Content: "Hello"},
			},
		},
	)

	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}
