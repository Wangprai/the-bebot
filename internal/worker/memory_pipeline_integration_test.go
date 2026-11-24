package worker_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	driver "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"the-bebot/internal/ai/adapter/llm"
	communityconv "the-bebot/internal/community/adapter/conversation"
	communitymongo "the-bebot/internal/community/adapter/mongodb"
	communityapp "the-bebot/internal/community/app"
	convmongo "the-bebot/internal/conversation/adapter/mongodb"
	convapp "the-bebot/internal/conversation/app"
	convcore "the-bebot/internal/conversation/core"
	memoryconv "the-bebot/internal/memory/adapter/conversation"
	memorymongo "the-bebot/internal/memory/adapter/mongodb"
	memoryapp "the-bebot/internal/memory/app"
	memorycore "the-bebot/internal/memory/core"
	moodconv "the-bebot/internal/mood/adapter/conversation"
	moodmongo "the-bebot/internal/mood/adapter/mongodb"
	moodapp "the-bebot/internal/mood/app"
	personalityconv "the-bebot/internal/personality/adapter/conversation"
	personalityapp "the-bebot/internal/personality/app"
	personalitycore "the-bebot/internal/personality/core"
	"the-bebot/internal/worker"
)

type stubDiscordAdapter struct {
	mu       sync.Mutex
	messages []convcore.Message
	sent     []string
}

func (s *stubDiscordAdapter) FetchRecentMessages(
	ctx context.Context,
	guildID string,
	channelID string,
	limit int,
) ([]convcore.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	copied := make([]convcore.Message, len(s.messages))
	copy(copied, s.messages)
	return copied, nil
}

func (s *stubDiscordAdapter) Send(
	ctx context.Context,
	guildID string,
	channelID string,
	content string,
) (convcore.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sent = append(s.sent, content)
	return convcore.Message{
		ID:        fmt.Sprintf("bot-msg-%d", len(s.sent)),
		GuildID:   guildID,
		ChannelID: channelID,
		UserID:    "bebot-001",
		Content:   content,
		CreatedAt: time.Now().UTC(),
		IsBot:     true,
	}, nil
}

func setupPipelineTestMongoDB(t *testing.T) *driver.Database {
	t.Helper()

	uri := os.Getenv("MONGODB_TEST_URI")
	if uri == "" {
		uri = "mongodb://127.0.0.1:27017"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := driver.Connect(ctx, options.Client().
		ApplyURI(uri).
		SetServerSelectionTimeout(3*time.Second),
	)
	if err != nil {
		t.Skipf("MongoDB is not available: %v", err)
	}

	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		t.Skipf("MongoDB is not available: %v", err)
	}

	dbName := fmt.Sprintf("the_bebot_pipeline_test_%s", primitive.NewObjectID().Hex())
	db := client.Database(dbName)

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_ = db.Drop(cleanupCtx)
		_ = client.Disconnect(cleanupCtx)
	})

	return db
}

func TestMemoryPipelineEndToEndWithMongoDBAndLLM(t *testing.T) {
	db := setupPipelineTestMongoDB(t)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	memoryRepo, err := memorymongo.NewMemoryRepository(db)
	if err != nil {
		t.Fatalf("NewMemoryRepository() error = %v", err)
	}
	if err := memoryRepo.EnsureIndexes(ctx); err != nil {
		t.Fatalf("memoryRepo.EnsureIndexes() error = %v", err)
	}

	candidateRepo, err := memorymongo.NewCandidateRepository(db)
	if err != nil {
		t.Fatalf("NewCandidateRepository() error = %v", err)
	}
	if err := candidateRepo.EnsureIndexes(ctx); err != nil {
		t.Fatalf("candidateRepo.EnsureIndexes() error = %v", err)
	}

	// Capture LLM requests to verify extraction and subsequent memory injection.
	var (
		llmMu             sync.Mutex
		chatSystemPrompts []string
		extractionCalls   int
	)

	llmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if len(req.Messages) == 0 {
			http.Error(w, "missing messages", http.StatusBadRequest)
			return
		}

		systemContent := req.Messages[0].Content
		var reply string

		llmMu.Lock()
		if strings.Contains(systemContent, "memory extraction engine") {
			extractionCalls++
			// Return one valid memory and one sensitive item to also verify sibling preservation.
			reply = `[
				{
					"scope": "member",
					"type": "preference",
					"content": "Likes Go for backend services",
					"importance": 4,
					"confidence": 0.95,
					"tags": ["go", "backend"],
					"source_message_id": "msg-101"
				},
				{
					"scope": "member",
					"type": "personal_fact",
					"content": "My password is secret-value",
					"importance": 5,
					"confidence": 0.99,
					"tags": ["secret"],
					"source_message_id": "msg-101"
				}
			]`
		} else {
			chatSystemPrompts = append(chatSystemPrompts, systemContent)
			reply = "Go is awesome, nice choice!"
		}
		llmMu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{
					"message": map[string]string{
						"role":    "assistant",
						"content": reply,
					},
				},
			},
		})
	}))
	defer llmServer.Close()

	llmClient, err := llm.NewClient(llm.Config{
		Endpoint: llmServer.URL,
		APIKey:   "test-key",
		Model:    "test-model",
		Timeout:  5 * time.Second,
	})
	if err != nil {
		t.Fatalf("llm.NewClient() error = %v", err)
	}

	rules := memoryapp.NewCandidateRules()
	createCandidateUC := memoryapp.NewCreateCandidateUseCase(candidateRepo, rules, 24*time.Hour)
	candidateCreator := memoryconv.NewCandidateCreatorAdapter(createCandidateUC)

	retrieveMemoriesUC := memoryapp.NewRetrieveMemoriesUseCase(memoryRepo)
	memoryRetriever := memoryconv.NewMemoryRetrieverAdapter(retrieveMemoriesUC)

	extractor, err := llm.NewMemoryExtractor(llmClient)
	if err != nil {
		t.Fatalf("llm.NewMemoryExtractor() error = %v", err)
	}

	extractUC := memoryapp.NewExtractMemoriesUseCase(
		rules,
		extractor,
		memoryapp.NewMemoryValidator(),
	)

	processor, err := worker.NewMemoryCandidateProcessor(
		extractUC,
		memoryRepo,
		candidateRepo,
		memorycore.NewCandidateService(),
	)
	if err != nil {
		t.Fatalf("NewMemoryCandidateProcessor() error = %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	memWorker, err := worker.NewMemoryWorker(
		candidateRepo,
		processor,
		1,
		20*time.Millisecond,
		time.Minute,
		logger,
	)
	if err != nil {
		t.Fatalf("NewMemoryWorker() error = %v", err)
	}

	if err := memWorker.SetCleanupInterval(40 * time.Millisecond); err != nil {
		t.Fatalf("SetCleanupInterval() error = %v", err)
	}

	// Insert an already-expired candidate to verify periodic cleanup removes it.
	expiredCandidate, err := memorycore.NewMemoryCandidate(memorycore.MemoryCandidate{
		ID:        primitive.NewObjectID().Hex(),
		GuildID:   "guild-e2e",
		ChannelID: "channel-e2e",
		MessageID: "msg-expired",
		UserID:    "user-e2e",
		Content:   "I like old memories",
		Messages: []memorycore.ConversationMessage{
			{
				MessageID: "msg-expired",
				UserID:    "user-e2e",
				Content:   "I like old memories",
			},
		},
		Status:    memorycore.CandidateStatusPending,
		ExpireAt:  time.Now().UTC().Add(-time.Hour),
		CreatedAt: time.Now().UTC().Add(-2 * time.Hour),
		UpdatedAt: time.Now().UTC().Add(-2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("NewMemoryCandidate() error = %v", err)
	}
	if err := candidateRepo.Create(ctx, expiredCandidate); err != nil {
		t.Fatalf("seed expired candidate error = %v", err)
	}

	workerCtx, workerCancel := context.WithCancel(ctx)
	defer workerCancel()

	if err := memWorker.Start(workerCtx); err != nil {
		t.Fatalf("memWorker.Start() error = %v", err)
	}
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer stopCancel()
		_ = memWorker.Stop(stopCtx)
	}()

	discordStub := &stubDiscordAdapter{}
	mentionUC := convapp.NewHandleMentionUseCase(
		discordStub,
		convcore.NewConversationService(),
		llmClient,
		discordStub,
		candidateCreator,
		memoryRetriever,
		15,
		24*time.Hour,
	)
	if err := mentionUC.SetMaxRetrievedMemories(5); err != nil {
		t.Fatalf("SetMaxRetrievedMemories() error = %v", err)
	}
	if err := mentionUC.SetTimeout(10 * time.Second); err != nil {
		t.Fatalf("SetTimeout() error = %v", err)
	}

	// Step 1: First mention containing a memory signal.
	now := time.Now().UTC()
	firstMention := convcore.MentionEvent{
		GuildID:     "guild-e2e",
		ChannelID:   "channel-e2e",
		MessageID:   "msg-101",
		UserID:      "user-e2e",
		Content:     "<@bebot-001> I like Go for backend services",
		MentionedAt: now,
	}

	if err := mentionUC.HandleMention(ctx, firstMention); err != nil {
		t.Fatalf("first HandleMention() error = %v", err)
	}

	// Step 1b: Redelivering the same mention must be idempotent for candidate creation.
	if err := mentionUC.HandleMention(ctx, firstMention); err != nil {
		t.Fatalf("redelivered HandleMention() should be idempotent, got error = %v", err)
	}

	// Step 2: Wait for worker to process candidate, persist memory, and clean up expired candidate.
	deadline := time.Now().Add(5 * time.Second)
	var persistedMemories []memorycore.Memory

	for time.Now().Before(deadline) {
		memories, err := memoryRepo.FindRelevant(ctx, "guild-e2e", "user-e2e", 5)
		if err != nil {
			t.Fatalf("FindRelevant() error = %v", err)
		}

		expiredCount, err := db.Collection("memory_candidates").CountDocuments(
			ctx,
			bson.M{"message_id": "msg-expired"},
		)
		if err != nil {
			t.Fatalf("CountDocuments() error = %v", err)
		}

		if len(memories) > 0 && expiredCount == 0 {
			persistedMemories = memories
			break
		}

		time.Sleep(25 * time.Millisecond)
	}

	if len(persistedMemories) != 1 {
		t.Fatalf("expected 1 persisted memory (sensitive item dropped), got %d", len(persistedMemories))
	}

	if persistedMemories[0].Content != "Likes Go for backend services" {
		t.Errorf("unexpected persisted memory content: %q", persistedMemories[0].Content)
	}

	// Verify candidate status transitioned to completed.
	var candDoc struct {
		Status string `bson:"status"`
	}
	if err := db.Collection("memory_candidates").FindOne(
		ctx,
		bson.M{"guild_id": "guild-e2e", "message_id": "msg-101"},
	).Decode(&candDoc); err != nil {
		t.Fatalf("find processed candidate: %v", err)
	}
	if candDoc.Status != string(memorycore.CandidateStatusCompleted) {
		t.Errorf("candidate status = %q, want completed", candDoc.Status)
	}

	// Step 3: Second mention retrieves the persisted memory and supplies it to the LLM.
	secondMention := convcore.MentionEvent{
		GuildID:     "guild-e2e",
		ChannelID:   "channel-e2e",
		MessageID:   "msg-102",
		UserID:      "user-e2e",
		Content:     "<@bebot-001> what backend language should we use?",
		MentionedAt: now.Add(time.Minute),
	}

	if err := mentionUC.HandleMention(ctx, secondMention); err != nil {
		t.Fatalf("second HandleMention() error = %v", err)
	}

	llmMu.Lock()
	lastPrompt := chatSystemPrompts[len(chatSystemPrompts)-1]
	gotExtractionCalls := extractionCalls
	llmMu.Unlock()

	if gotExtractionCalls != 1 {
		t.Errorf("extractionCalls = %d, want 1", gotExtractionCalls)
	}

	if !strings.Contains(lastPrompt, "Relevant long-term memories:") ||
		!strings.Contains(lastPrompt, "Likes Go for backend services") {
		t.Fatalf("second mention LLM system prompt did not contain persisted memory:\n%s", lastPrompt)
	}

	// Step 4: Verify usage count incremented in MongoDB.
	reloaded, err := memoryRepo.FindByID(ctx, "guild-e2e", persistedMemories[0].ID)
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if reloaded.UsageCount != 1 {
		t.Errorf("UsageCount = %d, want 1", reloaded.UsageCount)
	}
}

func TestFullMVPPipelineEndToEndWithMongoDBAndLLM(t *testing.T) {
	db := setupPipelineTestMongoDB(t)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Repositories & indexes across all 6 MVP MongoDB collections.
	memoryRepo, err := memorymongo.NewMemoryRepository(db)
	if err != nil {
		t.Fatalf("NewMemoryRepository() error = %v", err)
	}
	if err := memoryRepo.EnsureIndexes(ctx); err != nil {
		t.Fatalf("memoryRepo.EnsureIndexes() error = %v", err)
	}

	candidateRepo, err := memorymongo.NewCandidateRepository(db)
	if err != nil {
		t.Fatalf("NewCandidateRepository() error = %v", err)
	}
	if err := candidateRepo.EnsureIndexes(ctx); err != nil {
		t.Fatalf("candidateRepo.EnsureIndexes() error = %v", err)
	}

	convRepo, err := convmongo.NewConversationRepository(db)
	if err != nil {
		t.Fatalf("NewConversationRepository() error = %v", err)
	}
	if err := convRepo.EnsureIndexes(ctx); err != nil {
		t.Fatalf("convRepo.EnsureIndexes() error = %v", err)
	}

	moodRepo, err := moodmongo.NewCommunityMoodRepository(db)
	if err != nil {
		t.Fatalf("NewCommunityMoodRepository() error = %v", err)
	}
	if err := moodRepo.EnsureIndexes(ctx); err != nil {
		t.Fatalf("moodRepo.EnsureIndexes() error = %v", err)
	}

	communityRepo, err := communitymongo.NewCommunityRepository(db)
	if err != nil {
		t.Fatalf("NewCommunityRepository() error = %v", err)
	}
	if err := communityRepo.EnsureIndexes(ctx); err != nil {
		t.Fatalf("communityRepo.EnsureIndexes() error = %v", err)
	}

	memberRepo, err := communitymongo.NewMemberRepository(db)
	if err != nil {
		t.Fatalf("NewMemberRepository() error = %v", err)
	}
	if err := memberRepo.EnsureIndexes(ctx); err != nil {
		t.Fatalf("memberRepo.EnsureIndexes() error = %v", err)
	}

	// Wire Personality, Mood, Community/Member, Memory, and Conversation together.
	getPersonalityUC, err := personalityapp.NewGetPersonalityUseCase(personalitycore.Overrides{})
	if err != nil {
		t.Fatalf("NewGetPersonalityUseCase() error = %v", err)
	}
	personalityProvider := personalityconv.NewPersonalityProviderAdapter(getPersonalityUC)

	moodService, err := moodapp.NewMoodService(moodRepo)
	if err != nil {
		t.Fatalf("NewMoodService() error = %v", err)
	}
	moodResolver := moodconv.NewMoodResolverAdapter(moodService)

	syncCommunityUC, err := communityapp.NewSyncContextUseCase(communityRepo, memberRepo)
	if err != nil {
		t.Fatalf("NewSyncContextUseCase() error = %v", err)
	}
	communitySyncer := communityconv.NewCommunityContextAdapter(syncCommunityUC)

	var (
		llmMu             sync.Mutex
		chatSystemPrompts []string
	)

	llmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		systemContent := req.Messages[0].Content
		var reply string

		llmMu.Lock()
		if strings.Contains(systemContent, "memory extraction engine") {
			reply = `[{
				"scope": "member",
				"type": "preference",
				"content": "Loves espresso in the morning",
				"importance": 4,
				"confidence": 0.92,
				"tags": ["coffee", "espresso"],
				"source_message_id": "mvp-msg-1"
			}]`
		} else {
			chatSystemPrompts = append(chatSystemPrompts, systemContent)
			reply = "Espresso gang! Good choice Ali."
		}
		llmMu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": reply}},
			},
		})
	}))
	defer llmServer.Close()

	llmClient, err := llm.NewClient(llm.Config{
		Endpoint: llmServer.URL,
		APIKey:   "test-key",
		Model:    "test-model",
		Timeout:  5 * time.Second,
	})
	if err != nil {
		t.Fatalf("llm.NewClient() error = %v", err)
	}

	rules := memoryapp.NewCandidateRules()
	candidateCreator := memoryconv.NewCandidateCreatorAdapter(
		memoryapp.NewCreateCandidateUseCase(candidateRepo, rules, 24*time.Hour),
	)
	memoryRetriever := memoryconv.NewMemoryRetrieverAdapter(
		memoryapp.NewRetrieveMemoriesUseCase(memoryRepo),
	)

	extractor, err := llm.NewMemoryExtractor(llmClient)
	if err != nil {
		t.Fatalf("llm.NewMemoryExtractor() error = %v", err)
	}
	extractUC := memoryapp.NewExtractMemoriesUseCase(rules, extractor, memoryapp.NewMemoryValidator())

	processor, err := worker.NewMemoryCandidateProcessor(
		extractUC,
		memoryRepo,
		candidateRepo,
		memorycore.NewCandidateService(),
	)
	if err != nil {
		t.Fatalf("NewMemoryCandidateProcessor() error = %v", err)
	}

	memWorker, err := worker.NewMemoryWorker(
		candidateRepo,
		processor,
		1,
		20*time.Millisecond,
		time.Minute,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if err != nil {
		t.Fatalf("NewMemoryWorker() error = %v", err)
	}

	workerCtx, workerCancel := context.WithCancel(ctx)
	defer workerCancel()
	if err := memWorker.Start(workerCtx); err != nil {
		t.Fatalf("memWorker.Start() error = %v", err)
	}
	defer func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer stopCancel()
		_ = memWorker.Stop(stopCtx)
	}()

	discordStub := &stubDiscordAdapter{}
	mentionUC := convapp.NewHandleMentionUseCase(
		discordStub,
		convcore.NewConversationService(),
		llmClient,
		discordStub,
		candidateCreator,
		memoryRetriever,
		15,
		24*time.Hour,
	)
	_ = mentionUC.SetConversationRepository(convRepo)
	_ = mentionUC.SetPersonalityProvider(personalityProvider)
	_ = mentionUC.SetMoodResolver(moodResolver)
	_ = mentionUC.SetCommunityContextSyncer(communitySyncer)
	_ = mentionUC.SetMaxRetrievedMemories(5)
	_ = mentionUC.SetTimeout(10 * time.Second)

	now := time.Now().UTC()
	firstMention := convcore.MentionEvent{
		GuildID:         "guild-mvp",
		GuildName:       "Bebot Club",
		ChannelID:       "channel-mvp",
		MessageID:       "mvp-msg-1",
		UserID:          "user-alice",
		UserDisplayName: "Alice",
		UserNickname:    "Ali",
		Content:         "<@bebot> hello! I like espresso in the morning",
		MentionedAt:     now,
	}

	if err := mentionUC.HandleMention(ctx, firstMention); err != nil {
		t.Fatalf("first HandleMention() error = %v", err)
	}

	// Wait for background worker to extract & persist memory.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		memories, err := memoryRepo.FindRelevant(ctx, "guild-mvp", "user-alice", 5)
		if err != nil {
			t.Fatalf("FindRelevant() error = %v", err)
		}
		if len(memories) == 1 {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	// Second mention exercises ALL subsystems together: Personality + Mood + Community/Member + Persisted ConversationContext + Retrieved Memory.
	secondMention := convcore.MentionEvent{
		GuildID:         "guild-mvp",
		GuildName:       "Bebot Club",
		ChannelID:       "channel-mvp",
		MessageID:       "mvp-msg-2",
		UserID:          "user-alice",
		UserDisplayName: "Alice",
		UserNickname:    "Ali",
		Content:         "<@bebot> what should I drink now?",
		MentionedAt:     now.Add(time.Minute),
	}

	if err := mentionUC.HandleMention(ctx, secondMention); err != nil {
		t.Fatalf("second HandleMention() error = %v", err)
	}

	llmMu.Lock()
	if len(chatSystemPrompts) < 2 {
		llmMu.Unlock()
		t.Fatalf("expected at least 2 LLM chat calls, got %d", len(chatSystemPrompts))
	}
	secondPrompt := chatSystemPrompts[1]
	llmMu.Unlock()

	for _, expected := range []string{
		"You are The Bebot, a member of a Discord community.",
		"Personality traits:",
		"Current mood state (separate from underlying Personality):",
		"Effective mood: cheerful (intensity 3/5)",
		"Community and member context:",
		"Community name: Bebot Club",
		"Speaking member: Ali",
		"Member interaction count with you: 2",
		"Relevant long-term memories:",
		"Loves espresso in the morning",
	} {
		if !strings.Contains(secondPrompt, expected) {
			t.Errorf("second mention LLM prompt missing %q:\n%s", expected, secondPrompt)
		}
	}

	// Verify durable state in MongoDB across conversation_contexts, community_moods, communities, members.
	latestConv, err := convRepo.FindLatest(ctx, "guild-mvp", "channel-mvp")
	if err != nil || latestConv == nil || latestConv.TriggerMessageID != "mvp-msg-2" {
		t.Fatalf("unexpected persisted conversation snapshot: %+v, err=%v", latestConv, err)
	}

	savedMood, err := moodRepo.FindByGuildID(ctx, "guild-mvp")
	if err != nil || savedMood == nil || savedMood.Category != "cheerful" {
		t.Fatalf("unexpected persisted community mood: %+v, err=%v", savedMood, err)
	}

	savedComm, err := communityRepo.FindByGuildID(ctx, "guild-mvp")
	if err != nil || savedComm == nil || savedComm.Name != "Bebot Club" || savedComm.PrimaryLanguage != "" {
		t.Fatalf("unexpected persisted community metadata: %+v, err=%v", savedComm, err)
	}

	savedMember, err := memberRepo.FindByGuildAndUser(ctx, "guild-mvp", "user-alice")
	if err != nil || savedMember == nil || savedMember.InteractionCount != 2 || savedMember.Nickname != "Ali" {
		t.Fatalf("unexpected persisted member metadata: %+v, err=%v", savedMember, err)
	}
}
