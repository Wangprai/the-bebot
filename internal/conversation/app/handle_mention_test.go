package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"the-bebot/internal/conversation/core"
)

// ContextProvider Mock
type mockContextProvider struct {
	messages []core.Message
	err      error
	called   bool

	receivedGuildID   string
	receivedChannelID string
	receivedLimit     int
}

func (m *mockContextProvider) FetchRecentMessages(
	ctx context.Context,
	guildID string,
	channelID string,
	limit int,
) ([]core.Message, error) {
	m.called = true
	m.receivedGuildID = guildID
	m.receivedChannelID = channelID
	m.receivedLimit = limit

	return m.messages, m.err
}

// ConversationService Mock
type mockConversationService struct {
	result core.ConversationContext
	err    error
	called bool
}

func (m *mockConversationService) BuildContext(
	event core.MentionEvent,
	messages []core.Message,
	maxMessages int,
	ttl time.Duration,
) (core.ConversationContext, error) {
	m.called = true

	return m.result, m.err
}

func (m *mockConversationService) ValidateContext(
	conversationContext core.ConversationContext,
	now time.Time,
) error {
	return nil
}

// ResponseGenerator Mock
type mockResponseGenerator struct {
	response string
	err      error
	called   bool

	receivedContext core.ConversationContext
}

func (m *mockResponseGenerator) Generate(
	ctx context.Context,
	conversationContext core.ConversationContext,
) (string, error) {
	m.called = true
	m.receivedContext = conversationContext

	return m.response, m.err
}

// MessageSender Mock
type mockMessageSender struct {
	err     error
	called  bool
	content string

	receivedGuildID   string
	receivedChannelID string
}

func (m *mockMessageSender) Send(
	ctx context.Context,
	guildID string,
	channelID string,
	content string,
) (core.Message, error) {
	m.called = true
	m.content = content
	m.receivedGuildID = guildID
	m.receivedChannelID = channelID

	return core.Message{}, m.err
}

func newTestUseCase() (
	*HandleMentionUseCase,
	*mockContextProvider,
	*mockConversationService,
	*mockResponseGenerator,
	*mockMessageSender,
	core.MentionEvent,
) {
	now := time.Date(
		2026, 11, 19, 10, 0, 0, 0, time.UTC,
	)

	event := core.MentionEvent{
		GuildID:     "guild-001",
		ChannelID:   "channel-001",
		MessageID:   "message-002",
		UserID:      "user-001",
		Content:     "@TheBebot hello",
		MentionedAt: now,
	}

	contextProvider := &mockContextProvider{
		messages: []core.Message{
			{
				ID:        "message-001",
				GuildID:   "guild-001",
				ChannelID: "channel-001",
				UserID:    "user-001",
				Content:   "Hello",
				CreatedAt: now.Add(-time.Minute),
			},
			{
				ID:        "message-002",
				GuildID:   "guild-001",
				ChannelID: "channel-001",
				UserID:    "user-001",
				Content:   "@TheBebot hello",
				CreatedAt: now,
			},
		},
	}

	conversationService := &mockConversationService{
		result: core.ConversationContext{
			GuildID:          "guild-001",
			ChannelID:        "channel-001",
			TriggerMessageID: "message-002",
			Messages:         contextProvider.messages,
			CreatedAt:        now,
			ExpiresAt:        now.Add(24 * time.Hour),
		},
	}

	responseGenerator := &mockResponseGenerator{
		response: "Hello! What's up?",
	}

	messageSender := &mockMessageSender{}

	candidateCreator := &mockCandidateCreator{}

	useCase := NewHandleMentionUseCase(
		contextProvider,
		conversationService,
		responseGenerator,
		messageSender,
		candidateCreator,
		nil,
		15,
		24*time.Hour,
	)

	return useCase,
		contextProvider,
		conversationService,
		responseGenerator,
		messageSender,
		event
}

// Tests

func TestHandleMentionSuccess(t *testing.T) {
	useCase, provider, service, generator, sender, event :=
		newTestUseCase()

	err := useCase.HandleMention(context.Background(), event)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !provider.called {
		t.Error("expected ContextProvider to be called")
	}

	if !service.called {
		t.Error("expected ConversationService to be called")
	}

	if !generator.called {
		t.Error("expected ResponseGenerator to be called")
	}

	if !sender.called {
		t.Error("expected MessageSender to be called")
	}

	if sender.content != "Hello! What's up?" {
		t.Errorf("unexpected sent content: %s", sender.content)
	}
}

func TestHandleMentionFetchError(t *testing.T) {
	useCase, provider, service, generator, sender, event :=
		newTestUseCase()

	provider.err = errors.New("discord fetch failed")

	err := useCase.HandleMention(context.Background(), event)

	if err == nil {
		t.Fatal("expected fetch error")
	}

	if service.called || generator.called || sender.called {
		t.Error("downstream dependencies should not be called")
	}
}

func TestHandleMentionGenerateError(t *testing.T) {
	useCase, _, _, generator, sender, event :=
		newTestUseCase()

	generator.err = errors.New("llm failed")

	err := useCase.HandleMention(context.Background(), event)

	if err == nil {
		t.Fatal("expected generation error")
	}

	if sender.called {
		t.Error("MessageSender should not be called")
	}
}

func TestHandleMentionEmptyResponse(t *testing.T) {
	useCase, _, _, generator, sender, event :=
		newTestUseCase()

	generator.response = "   "

	err := useCase.HandleMention(context.Background(), event)

	if !errors.Is(err, ErrEmptyResponse) {
		t.Fatalf("expected ErrEmptyResponse, got %v", err)
	}

	if sender.called {
		t.Error("MessageSender should not be called")
	}
}

func TestHandleMentionSendError(t *testing.T) {
	useCase, _, _, _, sender, event :=
		newTestUseCase()

	sender.err = errors.New("discord send failed")

	err := useCase.HandleMention(context.Background(), event)

	if err == nil {
		t.Fatal("expected send error")
	}
}

func TestEnsureTriggerMessageRespectsLimit(t *testing.T) {
	now := time.Now()

	event := core.MentionEvent{
		GuildID:     "guild-001",
		ChannelID:   "channel-001",
		MessageID:   "message-004",
		UserID:      "user-001",
		Content:     "@TheBebot hello",
		MentionedAt: now,
	}

	messages := []core.Message{
		{
			ID:        "message-001",
			GuildID:   "guild-001",
			ChannelID: "channel-001",
			UserID:    "user-001",
			Content:   "First",
			CreatedAt: now.Add(-4 * time.Minute),
		},
		{
			ID:        "message-002",
			GuildID:   "guild-001",
			ChannelID: "channel-001",
			UserID:    "user-001",
			Content:   "Second",
			CreatedAt: now.Add(-3 * time.Minute),
		},
		{
			ID:        "message-003",
			GuildID:   "guild-001",
			ChannelID: "channel-001",
			UserID:    "user-001",
			Content:   "Third",
			CreatedAt: now.Add(-2 * time.Minute),
		},
	}

	result, err := ensureTriggerMessage(event, messages, 3)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}

	if result[len(result)-1].ID != event.MessageID {
		t.Error("trigger message should be included and ordered last")
	}

	if result[0].ID != "message-002" {
		t.Errorf("expected oldest retained message to be message-002, got %s", result[0].ID)
	}
}

type mockCandidateCreator struct {
	called bool
	err    error
}

func (m *mockCandidateCreator) CreateCandidate(
	ctx context.Context,
	conversationContext core.ConversationContext,
) error {
	m.called = true
	return m.err
}

type mockMemoryRetriever struct {
	memories []core.MemoryContext
	err      error
	called   bool

	receivedGuildID string
	receivedUserID  string
	receivedLimit   int
}

func (m *mockMemoryRetriever) RetrieveMemories(
	ctx context.Context,
	guildID string,
	userID string,
	limit int,
) ([]core.MemoryContext, error) {
	m.called = true
	m.receivedGuildID = guildID
	m.receivedUserID = userID
	m.receivedLimit = limit
	return m.memories, m.err
}

func TestHandleMentionInjectsRetrievedMemories(t *testing.T) {
	useCase, _, _, generator, sender, event := newTestUseCase()

	retriever := &mockMemoryRetriever{
		memories: []core.MemoryContext{
			{
				Scope:      "member",
				Type:       "preference",
				Content:    "Likes Go",
				Importance: 4,
			},
		},
	}

	useCase.memoryRetriever = retriever

	err := useCase.HandleMention(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !retriever.called {
		t.Fatal("expected MemoryRetriever to be called")
	}

	if retriever.receivedGuildID != event.GuildID ||
		retriever.receivedUserID != event.UserID ||
		retriever.receivedLimit != 5 {
		t.Errorf(
			"unexpected retriever params: guild=%s user=%s limit=%d",
			retriever.receivedGuildID,
			retriever.receivedUserID,
			retriever.receivedLimit,
		)
	}

	if len(generator.receivedContext.Memories) != 1 ||
		generator.receivedContext.Memories[0].Content != "Likes Go" {
		t.Fatalf("expected retrieved memories in LLM context, got %+v", generator.receivedContext.Memories)
	}

	if !sender.called {
		t.Error("expected MessageSender to be called")
	}
}

func TestHandleMentionMemoryRetrievalErrorStopsPipeline(t *testing.T) {
	useCase, _, _, generator, sender, event := newTestUseCase()

	expectedErr := errors.New("memory store unavailable")
	useCase.memoryRetriever = &mockMemoryRetriever{
		err: expectedErr,
	}

	err := useCase.HandleMention(context.Background(), event)
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}

	if generator.called {
		t.Error("ResponseGenerator should not be called when memory retrieval fails")
	}

	if sender.called {
		t.Error("MessageSender should not be called when memory retrieval fails")
	}
}

func TestHandleMentionUsesConfiguredMaxRetrievedMemories(t *testing.T) {
	useCase, _, _, _, _, event := newTestUseCase()

	retriever := &mockMemoryRetriever{}
	useCase.memoryRetriever = retriever

	if err := useCase.SetMaxRetrievedMemories(0); err == nil {
		t.Fatal("expected error for non-positive retrieval limit")
	}

	if err := useCase.SetMaxRetrievedMemories(8); err != nil {
		t.Fatalf("SetMaxRetrievedMemories() error = %v", err)
	}

	if err := useCase.HandleMention(context.Background(), event); err != nil {
		t.Fatalf("HandleMention() error = %v", err)
	}

	if retriever.receivedLimit != 8 {
		t.Errorf("receivedLimit = %d, want 8", retriever.receivedLimit)
	}
}

type blockingResponseGenerator struct {
	delay time.Duration
}

func (g *blockingResponseGenerator) Generate(
	ctx context.Context,
	conversationContext core.ConversationContext,
) (string, error) {
	select {
	case <-time.After(g.delay):
		return "too late", nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func TestHandleMentionEnforcesTimeout(t *testing.T) {
	useCase, _, _, _, sender, event := newTestUseCase()
	useCase.responseGenerator = &blockingResponseGenerator{delay: 500 * time.Millisecond}

	if err := useCase.SetTimeout(0); err == nil {
		t.Fatal("expected error for non-positive timeout")
	}

	if err := useCase.SetTimeout(20 * time.Millisecond); err != nil {
		t.Fatalf("SetTimeout() error = %v", err)
	}

	err := useCase.HandleMention(context.Background(), event)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}

	if sender.called {
		t.Error("MessageSender should not be called after timeout")
	}
}

type mockConversationRepository struct {
	saved      []core.ConversationContext
	latest     *core.ConversationContext
	saveErr    error
	findErr    error
	findCalled bool
}

func (m *mockConversationRepository) Save(
	ctx context.Context,
	conversationContext core.ConversationContext,
) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.saved = append(m.saved, conversationContext)
	return nil
}

func (m *mockConversationRepository) FindLatest(
	ctx context.Context,
	guildID string,
	channelID string,
) (*core.ConversationContext, error) {
	m.findCalled = true
	return m.latest, m.findErr
}

func (m *mockConversationRepository) DeleteExpired(
	ctx context.Context,
	before time.Time,
) error {
	return nil
}

func TestHandleMentionPersistsLatestSnapshotAndMergesWithoutDuplicates(t *testing.T) {
	now := time.Now().UTC()

	event := core.MentionEvent{
		GuildID:     "guild-001",
		ChannelID:   "channel-001",
		MessageID:   "message-003",
		UserID:      "user-001",
		Content:     "@TheBebot hi again",
		MentionedAt: now,
	}

	// Live Discord returns message-002 (which also exists in persisted snapshot).
	provider := &mockContextProvider{
		messages: []core.Message{
			{
				ID:        "message-002",
				GuildID:   event.GuildID,
				ChannelID: event.ChannelID,
				UserID:    "user-002",
				Content:   "Live version of message 2",
				CreatedAt: now.Add(-time.Minute),
			},
		},
	}

	repo := &mockConversationRepository{
		latest: &core.ConversationContext{
			GuildID:          event.GuildID,
			ChannelID:        event.ChannelID,
			TriggerMessageID: "message-002",
			Messages: []core.Message{
				{
					ID:        "message-001",
					GuildID:   event.GuildID,
					ChannelID: event.ChannelID,
					UserID:    "user-001",
					Content:   "Older persisted message",
					CreatedAt: now.Add(-2 * time.Minute),
				},
				{
					ID:        "message-002",
					GuildID:   event.GuildID,
					ChannelID: event.ChannelID,
					UserID:    "user-002",
					Content:   "Stale persisted copy of message 2",
					CreatedAt: now.Add(-time.Minute),
				},
			},
			CreatedAt: now.Add(-time.Minute),
			ExpiresAt: now.Add(23 * time.Hour),
		},
	}

	generator := &mockResponseGenerator{response: "Hey there!"}
	sender := &mockMessageSender{}
	candidateCreator := &mockCandidateCreator{}

	uc := NewHandleMentionUseCase(
		provider,
		core.NewConversationService(),
		generator,
		sender,
		candidateCreator,
		nil,
		15,
		24*time.Hour,
	)

	if err := uc.SetConversationRepository(repo); err != nil {
		t.Fatalf("SetConversationRepository() error = %v", err)
	}

	if err := uc.HandleMention(context.Background(), event); err != nil {
		t.Fatalf("HandleMention() error = %v", err)
	}

	if !repo.findCalled {
		t.Fatal("expected FindLatest to be called")
	}

	if len(generator.receivedContext.Messages) != 3 {
		t.Fatalf("expected 3 deduplicated messages, got %d", len(generator.receivedContext.Messages))
	}

	if generator.receivedContext.Messages[0].ID != "message-001" ||
		generator.receivedContext.Messages[1].ID != "message-002" ||
		generator.receivedContext.Messages[2].ID != "message-003" {
		t.Errorf("unexpected chronological order: %+v", generator.receivedContext.Messages)
	}

	if generator.receivedContext.Messages[1].Content != "Live version of message 2" {
		t.Errorf("live message must take precedence, got %q", generator.receivedContext.Messages[1].Content)
	}

	if len(repo.saved) != 1 {
		t.Fatalf("expected 1 saved snapshot, got %d", len(repo.saved))
	}

	if repo.saved[0].TriggerMessageID != event.MessageID {
		t.Errorf("saved TriggerMessageID = %q, want %q", repo.saved[0].TriggerMessageID, event.MessageID)
	}
}

func TestHandleMentionFallsBackToPersistedSnapshotOnFetchError(t *testing.T) {
	now := time.Now().UTC()

	event := core.MentionEvent{
		GuildID:     "guild-001",
		ChannelID:   "channel-001",
		MessageID:   "message-002",
		UserID:      "user-001",
		Content:     "@TheBebot fallback test",
		MentionedAt: now,
	}

	provider := &mockContextProvider{
		err: errors.New("discord REST temporarily unavailable"),
	}

	repo := &mockConversationRepository{
		latest: &core.ConversationContext{
			GuildID:          event.GuildID,
			ChannelID:        event.ChannelID,
			TriggerMessageID: "message-001",
			Messages: []core.Message{
				{
					ID:        "message-001",
					GuildID:   event.GuildID,
					ChannelID: event.ChannelID,
					UserID:    "user-001",
					Content:   "Earlier message",
					CreatedAt: now.Add(-time.Minute),
				},
			},
			CreatedAt: now.Add(-time.Minute),
			ExpiresAt: now.Add(23 * time.Hour),
		},
	}

	generator := &mockResponseGenerator{response: "Still here!"}
	sender := &mockMessageSender{}

	uc := NewHandleMentionUseCase(
		provider,
		core.NewConversationService(),
		generator,
		sender,
		&mockCandidateCreator{},
		nil,
		15,
		24*time.Hour,
	)

	if err := uc.SetConversationRepository(repo); err != nil {
		t.Fatalf("SetConversationRepository() error = %v", err)
	}

	if err := uc.HandleMention(context.Background(), event); err != nil {
		t.Fatalf("expected fallback to succeed, got error: %v", err)
	}

	if len(generator.receivedContext.Messages) != 2 {
		t.Fatalf("expected 2 messages (persisted + trigger), got %d", len(generator.receivedContext.Messages))
	}

	if !sender.called {
		t.Error("expected MessageSender to be called")
	}

	if len(repo.saved) != 1 {
		t.Fatalf("expected updated snapshot to be saved, got %d", len(repo.saved))
	}
}

func TestHandleMentionIgnoresExpiredFallbackAndReturnsFetchError(t *testing.T) {
	now := time.Now().UTC()

	event := core.MentionEvent{
		GuildID:     "guild-001",
		ChannelID:   "channel-001",
		MessageID:   "message-002",
		UserID:      "user-001",
		Content:     "@TheBebot expired fallback",
		MentionedAt: now,
	}

	fetchErr := errors.New("discord REST unavailable")
	provider := &mockContextProvider{err: fetchErr}

	repo := &mockConversationRepository{
		latest: &core.ConversationContext{
			GuildID:          event.GuildID,
			ChannelID:        event.ChannelID,
			TriggerMessageID: "message-001",
			Messages: []core.Message{
				{
					ID:        "message-001",
					GuildID:   event.GuildID,
					ChannelID: event.ChannelID,
					UserID:    "user-001",
					Content:   "Stale message",
					CreatedAt: now.Add(-25 * time.Hour),
				},
			},
			CreatedAt: now.Add(-25 * time.Hour),
			ExpiresAt: now.Add(-time.Hour),
		},
	}

	uc := NewHandleMentionUseCase(
		provider,
		core.NewConversationService(),
		&mockResponseGenerator{response: "nope"},
		&mockMessageSender{},
		&mockCandidateCreator{},
		nil,
		15,
		24*time.Hour,
	)
	_ = uc.SetConversationRepository(repo)

	err := uc.HandleMention(context.Background(), event)
	if !errors.Is(err, fetchErr) {
		t.Fatalf("expected fetch error when fallback is expired, got %v", err)
	}
}

func TestHandleMentionSaveConversationContextError(t *testing.T) {
	useCase, _, _, _, _, event := newTestUseCase()

	expectedErr := errors.New("mongodb write error")
	_ = useCase.SetConversationRepository(&mockConversationRepository{
		saveErr: expectedErr,
	})

	err := useCase.HandleMention(context.Background(), event)
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected save error %v, got %v", expectedErr, err)
	}
}
