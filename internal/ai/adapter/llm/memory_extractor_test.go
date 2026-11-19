package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"the-bebot/internal/memory/core"
)

func newTestMemoryExtractor(
	t *testing.T,
	handler http.HandlerFunc,
) *MemoryExtractor {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	client, err := NewClient(Config{
		Endpoint: server.URL,
		APIKey:   "test-api-key",
		Model:    "test-model",
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	extractor, err := NewMemoryExtractor(client)
	if err != nil {
		t.Fatalf("NewMemoryExtractor() error = %v", err)
	}

	return extractor
}

func validExtractionInput() core.ExtractionInput {
	return core.ExtractionInput{
		GuildID:   "guild-001",
		ChannelID: "channel-001",
		UserID:    "user-001",
		Messages: []core.ConversationMessage{
			{
				MessageID: "message-001",
				UserID:    "user-001",
				Content:   "ผมชอบเขียนโปรแกรมด้วย Go",
			},
			{
				MessageID: "message-002",
				UserID:    "user-002",
				Content:   "ภาษา Go ก็ดีนะ",
			},
		},
	}
}

func writeLLMResponse(
	w http.ResponseWriter,
	content string,
) {
	w.Header().Set("Content-Type", "application/json")

	response := chatResponse{
		Choices: []chatChoice{
			{
				Message: chatMessage{
					Role:    "assistant",
					Content: content,
				},
			},
		},
	}

	_ = json.NewEncoder(w).Encode(response)
}

func TestMemoryExtractorSuccess(t *testing.T) {
	expectedResponse := `[
		{
			"scope": "member",
			"type": "preference",
			"content": "ชอบเขียนโปรแกรมด้วย Go",
			"importance": 3,
			"confidence": 0.95,
			"tags": ["go", "programming"]
		}
	]`

	extractor := newTestMemoryExtractor(t, func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}

		if r.Header.Get("Authorization") != "Bearer test-api-key" {
			t.Errorf("unexpected authorization header")
		}

		var request chatRequest

		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		if request.Model != "test-model" {
			t.Errorf("model = %s, want test-model", request.Model)
		}

		if len(request.Messages) != 2 {
			t.Fatalf("messages count = %d, want 2", len(request.Messages))
		}

		if request.Messages[0].Role != "system" {
			t.Errorf("first role = %s, want system", request.Messages[0].Role)
		}

		if request.Messages[1].Role != "user" {
			t.Errorf("second role = %s, want user", request.Messages[1].Role)
		}

		if !strings.Contains(
			request.Messages[1].Content,
			"Target User ID: user-001",
		) {
			t.Error("request does not contain target user ID")
		}

		if !strings.Contains(
			request.Messages[1].Content,
			"ผมชอบเขียนโปรแกรมด้วย Go",
		) {
			t.Error("request does not contain conversation content")
		}

		writeLLMResponse(w, expectedResponse)
	})

	memories, err := extractor.Extract(
		context.Background(),
		validExtractionInput(),
	)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	if len(memories) != 1 {
		t.Fatalf("memories count = %d, want 1", len(memories))
	}

	memory := memories[0]

	if memory.Scope != core.MemoryScope("member") {
		t.Errorf("scope = %s, want member", memory.Scope)
	}

	if memory.Type != core.MemoryType("preference") {
		t.Errorf("type = %s, want preference", memory.Type)
	}

	if memory.Content != "ชอบเขียนโปรแกรมด้วย Go" {
		t.Errorf("content = %s", memory.Content)
	}

	if memory.Importance != 3 {
		t.Errorf("importance = %d, want 3", memory.Importance)
	}

	if memory.Confidence != 0.95 {
		t.Errorf("confidence = %f, want 0.95", memory.Confidence)
	}

	if len(memory.Tags) != 2 {
		t.Errorf("tags count = %d, want 2", len(memory.Tags))
	}
}

func TestMemoryExtractorEmptyResult(t *testing.T) {
	extractor := newTestMemoryExtractor(t, func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		writeLLMResponse(w, `[]`)
	})

	memories, err := extractor.Extract(
		context.Background(),
		validExtractionInput(),
	)
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	if len(memories) != 0 {
		t.Errorf("memories count = %d, want 0", len(memories))
	}
}

func TestMemoryExtractorInvalidJSON(t *testing.T) {
	extractor := newTestMemoryExtractor(t, func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		writeLLMResponse(w, `[{"scope": "member"`)
	})

	_, err := extractor.Extract(
		context.Background(),
		validExtractionInput(),
	)

	if err == nil {
		t.Fatal("expected JSON decoding error")
	}
}

func TestMemoryExtractorRejectsNonArrayResponse(t *testing.T) {
	extractor := newTestMemoryExtractor(t, func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		writeLLMResponse(w, `{"scope":"member"}`)
	})

	_, err := extractor.Extract(
		context.Background(),
		validExtractionInput(),
	)

	if err == nil {
		t.Fatal("expected non-array response error")
	}
}

func TestMemoryExtractorRejectsNullResponse(t *testing.T) {
	extractor := newTestMemoryExtractor(t, func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		writeLLMResponse(w, `null`)
	})

	_, err := extractor.Extract(
		context.Background(),
		validExtractionInput(),
	)

	if err == nil {
		t.Fatal("expected null response error")
	}
}

func TestMemoryExtractorAPIError(t *testing.T) {
	extractor := newTestMemoryExtractor(t, func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		http.Error(
			w,
			`{"error":{"message":"service unavailable"}}`,
			http.StatusServiceUnavailable,
		)
	})

	_, err := extractor.Extract(
		context.Background(),
		validExtractionInput(),
	)

	if err == nil {
		t.Fatal("expected API error")
	}
}

func TestMemoryExtractorCanceledContext(t *testing.T) {
	extractor := newTestMemoryExtractor(t, func(
		w http.ResponseWriter,
		r *http.Request,
	) {
		t.Error("API should not be called")
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := extractor.Extract(ctx, validExtractionInput())

	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestNewMemoryExtractorRejectsNilClient(t *testing.T) {
	extractor, err := NewMemoryExtractor(nil)

	if err == nil {
		t.Fatal("expected error for nil client")
	}

	if extractor != nil {
		t.Error("extractor should be nil")
	}
}
