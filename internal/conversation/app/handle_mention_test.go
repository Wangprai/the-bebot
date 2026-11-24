package app

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
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

	receivedGuildID          string
	receivedChannelID        string
	receivedReplyToMessageID string
}

func (m *mockMessageSender) Send(
	ctx context.Context,
	guildID string,
	channelID string,
	replyToMessageID string,
	content string,
) (core.Message, error) {
	m.called = true
	m.content = content
	m.receivedGuildID = guildID
	m.receivedChannelID = channelID
	m.receivedReplyToMessageID = replyToMessageID

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

	if sender.receivedReplyToMessageID != event.MessageID {
		t.Errorf("replyToMessageID = %q, want %q", sender.receivedReplyToMessageID, event.MessageID)
	}
}

type mockTypingIndicator struct {
	called    int
	channelID string
	err       error
}

func (m *mockTypingIndicator) StartTyping(
	ctx context.Context,
	channelID string,
) error {
	m.called++
	m.channelID = channelID
	return m.err
}

func TestHandleMentionShowsTypingWhileGenerating(t *testing.T) {
	useCase, _, _, _, sender, event := newTestUseCase()

	typing := &mockTypingIndicator{
		err: errors.New("transient typing failure should not block reply"),
	}

	if err := useCase.SetTypingIndicator(nil); err == nil {
		t.Fatal("expected error for nil typing indicator")
	}

	if err := useCase.SetTypingIndicator(typing); err != nil {
		t.Fatalf("SetTypingIndicator() error = %v", err)
	}

	if err := useCase.HandleMention(context.Background(), event); err != nil {
		t.Fatalf("HandleMention() error = %v", err)
	}

	if typing.called == 0 {
		t.Fatal("expected StartTyping to be called while generating response")
	}

	if typing.channelID != event.ChannelID {
		t.Errorf("typing channelID = %q, want %q", typing.channelID, event.ChannelID)
	}

	if !sender.called {
		t.Error("expected MessageSender to be called even if typing indicator errors")
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

func TestHandleMentionContinuesOnMemoryRetrievalError(t *testing.T) {
	useCase, _, _, generator, sender, event := newTestUseCase()

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	if err := useCase.SetLogger(logger); err != nil {
		t.Fatalf("SetLogger() error = %v", err)
	}

	useCase.memoryRetriever = &mockMemoryRetriever{
		err: errors.New("memory store unavailable"),
	}

	if err := useCase.HandleMention(context.Background(), event); err != nil {
		t.Fatalf("expected reply to succeed despite memory retrieval error, got %v", err)
	}

	if !generator.called {
		t.Error("ResponseGenerator should still be called when memory retrieval fails")
	}

	if len(generator.receivedContext.Memories) != 0 {
		t.Errorf("expected no memories on retrieval error, got %+v", generator.receivedContext.Memories)
	}

	if !sender.called {
		t.Error("MessageSender should still be called when memory retrieval fails")
	}

	if !strings.Contains(logBuf.String(), "memory store unavailable") {
		t.Errorf("expected warning log for memory retrieval failure, got %q", logBuf.String())
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

func TestHandleMentionSaveConversationContextErrorIsLoggedAndNonFatal(t *testing.T) {
	useCase, _, _, _, sender, event := newTestUseCase()

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	if err := useCase.SetLogger(logger); err != nil {
		t.Fatalf("SetLogger() error = %v", err)
	}

	candidateCreator := &mockCandidateCreator{}
	useCase.candidateCreator = candidateCreator

	_ = useCase.SetConversationRepository(&mockConversationRepository{
		saveErr: errors.New("mongodb write error"),
	})

	if err := useCase.HandleMention(context.Background(), event); err != nil {
		t.Fatalf("expected nil error when Save fails after send, got %v", err)
	}

	if !sender.called {
		t.Error("expected reply to be sent")
	}

	if !candidateCreator.called {
		t.Error("expected CreateCandidate to still be called when Save fails")
	}

	if !strings.Contains(logBuf.String(), "mongodb write error") {
		t.Errorf("expected warning log for Save failure, got %q", logBuf.String())
	}
}

type orderedCandidateCreator struct {
	order *[]string
	err   error
}

func (c *orderedCandidateCreator) CreateCandidate(
	ctx context.Context,
	conversationContext core.ConversationContext,
) error {
	*c.order = append(*c.order, "create_candidate")
	return c.err
}

type orderedConversationRepository struct {
	order   *[]string
	saveErr error
}

func (r *orderedConversationRepository) Save(
	ctx context.Context,
	conversationContext core.ConversationContext,
) error {
	*r.order = append(*r.order, "save_context")
	return r.saveErr
}

func (r *orderedConversationRepository) FindLatest(
	ctx context.Context,
	guildID string,
	channelID string,
) (*core.ConversationContext, error) {
	return nil, nil
}

func (r *orderedConversationRepository) DeleteExpired(
	ctx context.Context,
	before time.Time,
) error {
	return nil
}

func TestHandleMentionRunsCreateCandidateBeforeSaveAndContinuesOnPostSendErrors(t *testing.T) {
	useCase, _, _, _, sender, event := newTestUseCase()

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	if err := useCase.SetLogger(nil); err == nil {
		t.Fatal("expected error for nil logger")
	}
	if err := useCase.SetLogger(logger); err != nil {
		t.Fatalf("SetLogger() error = %v", err)
	}

	var order []string
	useCase.candidateCreator = &orderedCandidateCreator{
		order: &order,
		err:   errors.New("candidate write failed"),
	}
	_ = useCase.SetConversationRepository(&orderedConversationRepository{
		order:   &order,
		saveErr: errors.New("snapshot save failed"),
	})

	if err := useCase.HandleMention(context.Background(), event); err != nil {
		t.Fatalf("expected nil error when post-send steps fail, got %v", err)
	}

	if !sender.called {
		t.Error("expected reply to be sent")
	}

	if len(order) != 2 || order[0] != "create_candidate" || order[1] != "save_context" {
		t.Fatalf("post-send execution order = %v, want [create_candidate save_context]", order)
	}

	logs := logBuf.String()
	if !strings.Contains(logs, "candidate write failed") || !strings.Contains(logs, "snapshot save failed") {
		t.Errorf("expected both post-send warnings logged, got %q", logs)
	}
}

type mockPersonalityProvider struct {
	personality core.PersonalityContext
	err         error
	called      bool
}

func (m *mockPersonalityProvider) GetPersonality(
	ctx context.Context,
) (core.PersonalityContext, error) {
	m.called = true
	return m.personality, m.err
}

func TestHandleMentionInjectsPersonalityContext(t *testing.T) {
	useCase, _, _, generator, sender, event := newTestUseCase()

	provider := &mockPersonalityProvider{
		personality: core.PersonalityContext{
			Name:           "The Bebot",
			Traits:         []string{"playful and witty"},
			SpeakingStyle:  []string{"casual banter"},
			TeasingRules:   []string{"light teasing only"},
			Boundaries:     []string{"never hurt members"},
			LanguagePolicy: "match community language",
		},
	}

	if err := useCase.SetPersonalityProvider(nil); err == nil {
		t.Fatal("expected error for nil personality provider")
	}

	if err := useCase.SetPersonalityProvider(provider); err != nil {
		t.Fatalf("SetPersonalityProvider() error = %v", err)
	}

	if err := useCase.HandleMention(context.Background(), event); err != nil {
		t.Fatalf("HandleMention() error = %v", err)
	}

	if !provider.called {
		t.Fatal("expected PersonalityProvider to be called")
	}

	if generator.receivedContext.Personality == nil ||
		generator.receivedContext.Personality.Name != "The Bebot" {
		t.Fatalf("expected Personality in LLM context, got %+v", generator.receivedContext.Personality)
	}

	if !sender.called {
		t.Error("expected MessageSender to be called")
	}
}

func TestHandleMentionPersonalityErrorStopsPipeline(t *testing.T) {
	useCase, _, _, generator, sender, event := newTestUseCase()

	expectedErr := errors.New("personality failed")
	_ = useCase.SetPersonalityProvider(&mockPersonalityProvider{err: expectedErr})

	err := useCase.HandleMention(context.Background(), event)
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}

	if generator.called || sender.called {
		t.Error("downstream dependencies should not be called on personality error")
	}
}

type mockMoodResolver struct {
	mood   core.MoodContext
	err    error
	called bool

	receivedGuildID   string
	receivedChannelID string
	receivedContent   string
}

func (m *mockMoodResolver) ResolveMood(
	ctx context.Context,
	guildID string,
	channelID string,
	triggerContent string,
) (core.MoodContext, error) {
	m.called = true
	m.receivedGuildID = guildID
	m.receivedChannelID = channelID
	m.receivedContent = triggerContent
	return m.mood, m.err
}

func TestHandleMentionInjectsMoodContext(t *testing.T) {
	useCase, _, _, generator, sender, event := newTestUseCase()

	resolver := &mockMoodResolver{
		mood: core.MoodContext{
			EffectiveCategory:  "cheerful",
			EffectiveIntensity: 3,
			CommunityCategory:  "playful",
			CommunityIntensity: 2,
			TemporaryCategory:  "cheerful",
			TemporaryIntensity: 3,
		},
	}

	if err := useCase.SetMoodResolver(nil); err == nil {
		t.Fatal("expected error for nil mood resolver")
	}

	if err := useCase.SetMoodResolver(resolver); err != nil {
		t.Fatalf("SetMoodResolver() error = %v", err)
	}

	if err := useCase.HandleMention(context.Background(), event); err != nil {
		t.Fatalf("HandleMention() error = %v", err)
	}

	if !resolver.called {
		t.Fatal("expected MoodResolver to be called")
	}

	if resolver.receivedGuildID != event.GuildID ||
		resolver.receivedChannelID != event.ChannelID ||
		resolver.receivedContent != event.Content {
		t.Errorf("unexpected mood resolver inputs: %+v", resolver)
	}

	if generator.receivedContext.Mood == nil ||
		generator.receivedContext.Mood.EffectiveCategory != "cheerful" ||
		generator.receivedContext.Mood.EffectiveIntensity != 3 {
		t.Fatalf("expected Mood in LLM context, got %+v", generator.receivedContext.Mood)
	}

	if !sender.called {
		t.Error("expected MessageSender to be called")
	}
}

func TestHandleMentionContinuesOnMoodError(t *testing.T) {
	useCase, _, _, generator, sender, event := newTestUseCase()

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	_ = useCase.SetLogger(logger)

	_ = useCase.SetMoodResolver(&mockMoodResolver{err: errors.New("mood store unavailable")})

	if err := useCase.HandleMention(context.Background(), event); err != nil {
		t.Fatalf("expected reply to succeed despite mood error, got %v", err)
	}

	if !generator.called || !sender.called {
		t.Error("downstream dependencies should still be called on mood error")
	}

	if generator.receivedContext.Mood != nil {
		t.Errorf("expected nil mood on mood error, got %+v", generator.receivedContext.Mood)
	}

	if !strings.Contains(logBuf.String(), "mood store unavailable") {
		t.Errorf("expected warning log for mood failure, got %q", logBuf.String())
	}
}

type mockCommunitySyncer struct {
	community core.CommunityContext
	err       error
	called    bool
	event     core.MentionEvent
}

func (m *mockCommunitySyncer) SyncMentionContext(
	ctx context.Context,
	event core.MentionEvent,
) (core.CommunityContext, error) {
	m.called = true
	m.event = event
	return m.community, m.err
}

func TestHandleMentionInjectsCommunityContext(t *testing.T) {
	useCase, _, _, generator, sender, event := newTestUseCase()
	event.GuildName = "Go Guild"
	event.UserDisplayName = "Alice"
	event.UserNickname = "Ali"

	syncer := &mockCommunitySyncer{
		community: core.CommunityContext{
			GuildID:          event.GuildID,
			GuildName:        "Go Guild",
			MemberUserID:     event.UserID,
			MemberName:       "Alice",
			MemberNickname:   "Ali",
			InteractionCount: 5,
		},
	}

	if err := useCase.SetCommunityContextSyncer(nil); err == nil {
		t.Fatal("expected error for nil community context syncer")
	}

	if err := useCase.SetCommunityContextSyncer(syncer); err != nil {
		t.Fatalf("SetCommunityContextSyncer() error = %v", err)
	}

	if err := useCase.HandleMention(context.Background(), event); err != nil {
		t.Fatalf("HandleMention() error = %v", err)
	}

	if !syncer.called {
		t.Fatal("expected CommunityContextSyncer to be called")
	}

	if generator.receivedContext.Community == nil ||
		generator.receivedContext.Community.GuildName != "Go Guild" ||
		generator.receivedContext.Community.MemberNickname != "Ali" ||
		generator.receivedContext.Community.InteractionCount != 5 {
		t.Fatalf("expected Community context in LLM call, got %+v", generator.receivedContext.Community)
	}

	if !sender.called {
		t.Error("expected MessageSender to be called")
	}
}

func TestHandleMentionContinuesOnCommunitySyncError(t *testing.T) {
	useCase, _, _, generator, sender, event := newTestUseCase()

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	_ = useCase.SetLogger(logger)

	_ = useCase.SetCommunityContextSyncer(&mockCommunitySyncer{err: errors.New("community store unavailable")})

	if err := useCase.HandleMention(context.Background(), event); err != nil {
		t.Fatalf("expected reply to succeed despite community sync error, got %v", err)
	}

	if !generator.called || !sender.called {
		t.Error("downstream dependencies should still be called on community sync error")
	}

	if generator.receivedContext.Community != nil {
		t.Errorf("expected nil community context on sync error, got %+v", generator.receivedContext.Community)
	}

	if !strings.Contains(logBuf.String(), "community store unavailable") {
		t.Errorf("expected warning log for community sync failure, got %q", logBuf.String())
	}
}

func TestHandleMentionPerUserCooldown(t *testing.T) {
	useCase, _, _, generator, sender, event := newTestUseCase()

	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	currentTime := now

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	_ = useCase.SetLogger(logger)

	if err := useCase.SetClock(nil); err == nil {
		t.Fatal("expected error for nil clock")
	}
	if err := useCase.SetClock(func() time.Time { return currentTime }); err != nil {
		t.Fatalf("SetClock() error = %v", err)
	}
	if err := useCase.SetUserCooldown(-time.Second); err == nil {
		t.Fatal("expected error for negative cooldown")
	}
	if err := useCase.SetUserCooldown(5 * time.Second); err != nil {
		t.Fatalf("SetUserCooldown() error = %v", err)
	}

	// 1. First mention by user-001 succeeds.
	if err := useCase.HandleMention(context.Background(), event); err != nil {
		t.Fatalf("first HandleMention() error = %v", err)
	}
	if !sender.called {
		t.Fatal("expected first mention to send reply")
	}

	// 2. Second mention by same user within cooldown (3s < 5s) is silently ignored.
	generator.called = false
	sender.called = false
	currentTime = now.Add(3 * time.Second)

	if err := useCase.HandleMention(context.Background(), event); err != nil {
		t.Fatalf("cooldown-ignored HandleMention() returned error = %v", err)
	}
	if generator.called || sender.called {
		t.Error("expected mention inside cooldown to be ignored without calling LLM or sender")
	}
	if !strings.Contains(logBuf.String(), "cooldown") {
		t.Errorf("expected debug log for cooldown drop, got %q", logBuf.String())
	}

	// 3. Another user is unaffected by user-001's cooldown.
	otherEvent := event
	otherEvent.UserID = "user-002"
	otherEvent.MessageID = "message-other"
	if err := useCase.HandleMention(context.Background(), otherEvent); err != nil {
		t.Fatalf("other user HandleMention() error = %v", err)
	}
	if !sender.called {
		t.Error("expected other user's mention to be processed")
	}

	// 4. After 5s cooldown expires, user-001 can mention again.
	sender.called = false
	currentTime = now.Add(5 * time.Second)
	if err := useCase.HandleMention(context.Background(), event); err != nil {
		t.Fatalf("post-cooldown HandleMention() error = %v", err)
	}
	if !sender.called {
		t.Error("expected mention after cooldown expiry to be processed")
	}
}

func TestHandleMentionCooldownCleanupEvictsStaleEntries(t *testing.T) {
	useCase, _, _, _, _, event := newTestUseCase()

	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	currentTime := now
	_ = useCase.SetClock(func() time.Time { return currentTime })
	_ = useCase.SetUserCooldown(5 * time.Second)

	// Record mentions from 5 distinct users at t=0.
	for i := 0; i < 5; i++ {
		ev := event
		ev.UserID = "stale-user-" + string(rune('a'+i))
		ev.MessageID = "msg-" + string(rune('a'+i))
		if err := useCase.HandleMention(context.Background(), ev); err != nil {
			t.Fatalf("HandleMention(%s) error = %v", ev.UserID, err)
		}
	}

	if got := useCase.cooldownEntryCount(); got != 5 {
		t.Fatalf("cooldownEntryCount() = %d, want 5", got)
	}

	// Advance clock past cooldown and trigger a mention from a fresh user.
	currentTime = now.Add(10 * time.Second)
	freshEvent := event
	freshEvent.UserID = "fresh-user"
	freshEvent.MessageID = "msg-fresh"
	if err := useCase.HandleMention(context.Background(), freshEvent); err != nil {
		t.Fatalf("HandleMention(fresh) error = %v", err)
	}

	// All 5 stale entries must have been cleaned up, leaving only fresh-user.
	if got := useCase.cooldownEntryCount(); got != 1 {
		t.Fatalf("cooldownEntryCount() after cleanup = %d, want 1", got)
	}
}

type gatedResponseGenerator struct {
	started chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

func (g *gatedResponseGenerator) Generate(
	ctx context.Context,
	conversationContext core.ConversationContext,
) (string, error) {
	g.calls.Add(1)
	g.started <- struct{}{}
	select {
	case <-g.release:
		return "ok", nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

type threadSafeMessageSender struct {
	count atomic.Int32
}

func (s *threadSafeMessageSender) Send(
	ctx context.Context,
	guildID string,
	channelID string,
	replyToMessageID string,
	content string,
) (core.Message, error) {
	s.count.Add(1)
	return core.Message{}, nil
}

type threadSafeContextProvider struct {
	messages []core.Message
}

func (p *threadSafeContextProvider) FetchRecentMessages(
	ctx context.Context,
	guildID string,
	channelID string,
	limit int,
) ([]core.Message, error) {
	copied := make([]core.Message, len(p.messages))
	copy(copied, p.messages)
	return copied, nil
}

type threadSafeCandidateCreator struct{}

func (c *threadSafeCandidateCreator) CreateCandidate(
	ctx context.Context,
	conversationContext core.ConversationContext,
) error {
	return nil
}

func TestHandleMentionMaxConcurrentMentionsCap(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)

	gatedGen := &gatedResponseGenerator{
		started: make(chan struct{}, 4),
		release: make(chan struct{}),
	}
	sender := &threadSafeMessageSender{}
	provider := &threadSafeContextProvider{}

	useCase := NewHandleMentionUseCase(
		provider,
		core.NewConversationService(),
		gatedGen,
		sender,
		&threadSafeCandidateCreator{},
		nil,
		15,
		24*time.Hour,
	)

	var (
		logMu  sync.Mutex
		logBuf bytes.Buffer
	)
	logger := slog.New(slog.NewTextHandler(&lockedBuffer{mu: &logMu, buf: &logBuf}, &slog.HandlerOptions{Level: slog.LevelDebug}))
	_ = useCase.SetLogger(logger)

	if err := useCase.SetMaxConcurrentMentions(0); err == nil {
		t.Fatal("expected error for zero concurrency cap")
	}
	if err := useCase.SetMaxConcurrentMentions(2); err != nil {
		t.Fatalf("SetMaxConcurrentMentions() error = %v", err)
	}

	makeEvent := func(id string) core.MentionEvent {
		return core.MentionEvent{
			GuildID:     "guild-001",
			ChannelID:   "channel-001",
			MessageID:   "msg-" + id,
			UserID:      "user-" + id,
			Content:     "@TheBebot hi " + id,
			MentionedAt: now,
		}
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 2)

	// Start 2 mentions that fill the concurrency cap.
	for _, id := range []string{"1", "2"} {
		wg.Add(1)
		go func(ev core.MentionEvent) {
			defer wg.Done()
			errCh <- useCase.HandleMention(context.Background(), ev)
		}(makeEvent(id))
	}

	// Wait until both are in-flight inside Generate.
	for i := 0; i < 2; i++ {
		select {
		case <-gatedGen.started:
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for in-flight mentions to start")
		}
	}

	// 3rd mention must drop immediately without blocking or calling Generate.
	if err := useCase.HandleMention(context.Background(), makeEvent("3")); err != nil {
		t.Fatalf("dropped mention returned error = %v", err)
	}

	if gatedGen.calls.Load() != 2 {
		t.Errorf("generator calls = %d, want 2 (3rd mention should be dropped)", gatedGen.calls.Load())
	}

	logMu.Lock()
	droppedLog := logBuf.String()
	logMu.Unlock()
	if !strings.Contains(droppedLog, "concurrent") {
		t.Errorf("expected debug log for concurrency drop, got %q", droppedLog)
	}

	// Release in-flight mentions.
	close(gatedGen.release)
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("in-flight HandleMention() error = %v", err)
		}
	}

	if sender.count.Load() != 2 {
		t.Errorf("sender count = %d, want 2", sender.count.Load())
	}

	// After slots free up, a 4th mention succeeds.
	if err := useCase.HandleMention(context.Background(), makeEvent("4")); err != nil {
		t.Fatalf("4th HandleMention() error = %v", err)
	}
	if sender.count.Load() != 3 {
		t.Errorf("sender count after 4th mention = %d, want 3", sender.count.Load())
	}
}

type lockedBuffer struct {
	mu  *sync.Mutex
	buf *bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}
