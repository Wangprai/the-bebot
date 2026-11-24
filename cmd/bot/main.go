package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"the-bebot/internal/ai/adapter/llm"
	convmongodb "the-bebot/internal/conversation/adapter/mongodb"
	"the-bebot/internal/conversation/app"
	"the-bebot/internal/conversation/core"
	"the-bebot/internal/discord/adapter/gateway"
	"the-bebot/internal/platform/config"
	"the-bebot/internal/platform/database"
	"the-bebot/internal/platform/logger"
	"the-bebot/internal/worker"

	communityadapter "the-bebot/internal/community/adapter/conversation"
	communitymongodb "the-bebot/internal/community/adapter/mongodb"
	communityapp "the-bebot/internal/community/app"
	memoryadapter "the-bebot/internal/memory/adapter/conversation"
	memorymongodb "the-bebot/internal/memory/adapter/mongodb"
	memoryapp "the-bebot/internal/memory/app"
	memorycore "the-bebot/internal/memory/core"
	moodadapter "the-bebot/internal/mood/adapter/conversation"
	moodmongodb "the-bebot/internal/mood/adapter/mongodb"
	moodapp "the-bebot/internal/mood/app"
	personalityadapter "the-bebot/internal/personality/adapter/conversation"
	personalityapp "the-bebot/internal/personality/app"
	personalitycore "the-bebot/internal/personality/core"
)

const (
	conversationContextTTL = 24 * time.Hour
	memoryCandidateTTL     = 24 * time.Hour
	memoryPollInterval     = 5 * time.Second
	memoryLockDuration     = 2 * time.Minute
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("application error: %v", err)
	}
}

func run() error {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	// Initialize logger
	appLogger := logger.New(cfg.LogLevel, cfg.AppEnv)
	appLogger.Info("Starting The Bebot")

	// Create application context
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	// Connect MongoDB
	connectCtx, connectCancel := context.WithTimeout(
		ctx,
		10*time.Second,
	)

	db, err := database.NewMongoDB(
		connectCtx,
		cfg.MongoDB.URI,
		cfg.MongoDB.Database,
	)

	connectCancel()

	if err != nil {
		return fmt.Errorf("connect MongoDB: %w", err)
	}

	appLogger.Info(
		"MongoDB connected",
		"database", cfg.MongoDB.Database,
	)

	// MongoDB cleanup
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(
			context.Background(),
			10*time.Second,
		)
		defer cancel()

		if err := db.Close(shutdownCtx); err != nil {
			appLogger.Error(
				"Failed to disconnect MongoDB",
				"error", err,
			)
		} else {
			appLogger.Info("MongoDB disconnected")
		}
	}()

	// Initialize LLM Client
	llmClient, err := llm.NewClient(llm.Config{
		Endpoint: cfg.LLM.BaseURL,
		APIKey:   cfg.LLM.APIKey,
		Model:    cfg.LLM.Model,
		Timeout:  cfg.Conversation.Timeout,
	})
	if err != nil {
		return fmt.Errorf("initialize LLM client: %w", err)
	}

	// Initialize Discord Gateway
	discordGateway, err := gateway.NewGateway(
		cfg.Discord.BotToken,
		appLogger,
		nil,
	)
	if err != nil {
		return fmt.Errorf("initialize Discord gateway: %w", err)
	}

	// Gateway cleanup
	defer func() {
		if err := discordGateway.Close(); err != nil {
			appLogger.Error(
				"Failed to close Discord gateway",
				"error", err,
			)
		}
	}()

	// Create Discord Client
	discordClient, err := gateway.NewDiscordClient(
		discordGateway.Session(),
	)
	if err != nil {
		return fmt.Errorf("initialize Discord client: %w", err)
	}

	// Initialize Candidate Creator and Memory Retriever
	var candidateCreator core.CandidateCreator = app.NoopCandidateCreator{}
	var memoryRetriever core.MemoryRetriever

	if cfg.Memory.Enabled {
		// Initialize Memory Repository
		memoryRepository, err := memorymongodb.NewMemoryRepository(
			db.Database,
		)
		if err != nil {
			return fmt.Errorf("initialize memory repository: %w", err)
		}

		// Initialize Retrieve Memories Use Case
		retrieveMemories := memoryapp.NewRetrieveMemoriesUseCase(
			memoryRepository,
		)
		memoryRetriever = memoryadapter.NewMemoryRetrieverAdapter(
			retrieveMemories,
		)

		// Initialize Candidate Repository
		candidateRepository, err := memorymongodb.NewCandidateRepository(
			db.Database,
		)
		if err != nil {
			return fmt.Errorf("initialize candidate repository: %w", err)
		}

		// Ensure Memory Indexes
		indexCtx, indexCancel := context.WithTimeout(
			ctx,
			10*time.Second,
		)

		err = memoryRepository.EnsureIndexes(indexCtx)
		indexCancel()

		if err != nil {
			return fmt.Errorf("ensure memory indexes: %w", err)
		}

		// Ensure Candidate Indexes
		indexCtx, indexCancel = context.WithTimeout(
			ctx,
			10*time.Second,
		)

		err = candidateRepository.EnsureIndexes(indexCtx)
		indexCancel()

		if err != nil {
			return fmt.Errorf("ensure candidate indexes: %w", err)
		}

		appLogger.Info("Memory indexes ensured")

		// Initialize Candidate Rules
		candidateRules := memoryapp.NewCandidateRules()

		// Initialize Candidate Creation Use Case
		createCandidateUseCase := memoryapp.NewCreateCandidateUseCase(
			candidateRepository,
			candidateRules,
			memoryCandidateTTL,
		)

		candidateCreator = memoryadapter.NewCandidateCreatorAdapter(
			createCandidateUseCase,
		)

		// Initialize Memory Extractor
		memoryExtractor, err := llm.NewMemoryExtractor(llmClient)
		if err != nil {
			return fmt.Errorf("initialize memory extractor: %w", err)
		}

		// Initialize Memory Validator
		memoryValidator := memoryapp.NewMemoryValidator()

		// Initialize Extraction Use Case
		extractionUseCase := memoryapp.NewExtractMemoriesUseCase(
			candidateRules,
			memoryExtractor,
			memoryValidator,
		)

		// Initialize Candidate Processor
		candidateProcessor, err := worker.NewMemoryCandidateProcessor(
			extractionUseCase,
			memoryRepository,
			candidateRepository,
			memorycore.NewCandidateService(),
		)
		if err != nil {
			return fmt.Errorf("initialize memory candidate processor: %w", err)
		}

		// Initialize Memory Worker
		memoryWorker, err := worker.NewMemoryWorker(
			candidateRepository,
			candidateProcessor,
			cfg.Memory.WorkerCount,
			memoryPollInterval,
			memoryLockDuration,
			appLogger,
		)
		if err != nil {
			return fmt.Errorf("initialize memory worker: %w", err)
		}

		// Start Memory Worker
		if err := memoryWorker.Start(ctx); err != nil {
			return fmt.Errorf("start memory worker: %w", err)
		}

		// Memory Worker cleanup
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(
				context.Background(),
				30*time.Second,
			)
			defer cancel()

			if err := memoryWorker.Stop(shutdownCtx); err != nil {
				appLogger.Error(
					"Failed to stop memory worker",
					"error", err,
				)
			}
		}()

		appLogger.Info("Memory candidate creation enabled")
	} else {
		appLogger.Info("Memory candidate creation disabled")
	}

	// Initialize Conversation Repository
	conversationRepository, err := convmongodb.NewConversationRepository(
		db.Database,
	)
	if err != nil {
		return fmt.Errorf("initialize conversation repository: %w", err)
	}

	convIndexCtx, convIndexCancel := context.WithTimeout(ctx, 10*time.Second)
	err = conversationRepository.EnsureIndexes(convIndexCtx)
	convIndexCancel()
	if err != nil {
		return fmt.Errorf("ensure conversation indexes: %w", err)
	}

	appLogger.Info("Conversation indexes ensured")

	// Initialize Personality System (Global Go-owned profile)
	getPersonalityUseCase, err := personalityapp.NewGetPersonalityUseCase(
		personalitycore.Overrides{
			Name:           cfg.Personality.Name,
			ExtraTraits:    cfg.Personality.ExtraTraits,
			LanguagePolicy: cfg.Personality.LanguagePolicy,
		},
	)
	if err != nil {
		return fmt.Errorf("initialize personality: %w", err)
	}

	personalityProvider := personalityadapter.NewPersonalityProviderAdapter(
		getPersonalityUseCase,
	)

	// Initialize Mood System (Persistent Community Mood + In-Memory Temporary Mood)
	communityMoodRepository, err := moodmongodb.NewCommunityMoodRepository(
		db.Database,
	)
	if err != nil {
		return fmt.Errorf("initialize community mood repository: %w", err)
	}

	moodIndexCtx, moodIndexCancel := context.WithTimeout(ctx, 10*time.Second)
	err = communityMoodRepository.EnsureIndexes(moodIndexCtx)
	moodIndexCancel()
	if err != nil {
		return fmt.Errorf("ensure community mood indexes: %w", err)
	}

	appLogger.Info("Community mood indexes ensured")

	moodService, err := moodapp.NewMoodService(communityMoodRepository)
	if err != nil {
		return fmt.Errorf("initialize mood service: %w", err)
	}

	moodResolver := moodadapter.NewMoodResolverAdapter(moodService)

	// Initialize Community and Member Metadata Repositories
	communityRepository, err := communitymongodb.NewCommunityRepository(db.Database)
	if err != nil {
		return fmt.Errorf("initialize community repository: %w", err)
	}

	memberRepository, err := communitymongodb.NewMemberRepository(db.Database)
	if err != nil {
		return fmt.Errorf("initialize member repository: %w", err)
	}

	commIndexCtx, commIndexCancel := context.WithTimeout(ctx, 10*time.Second)
	err = communityRepository.EnsureIndexes(commIndexCtx)
	commIndexCancel()
	if err != nil {
		return fmt.Errorf("ensure community indexes: %w", err)
	}

	memIndexCtx, memIndexCancel := context.WithTimeout(ctx, 10*time.Second)
	err = memberRepository.EnsureIndexes(memIndexCtx)
	memIndexCancel()
	if err != nil {
		return fmt.Errorf("ensure member indexes: %w", err)
	}

	appLogger.Info("Community and member indexes ensured")

	syncCommunityUseCase, err := communityapp.NewSyncContextUseCase(
		communityRepository,
		memberRepository,
	)
	if err != nil {
		return fmt.Errorf("initialize community sync use case: %w", err)
	}

	communitySyncer := communityadapter.NewCommunityContextAdapter(syncCommunityUseCase)

	// Initialize Conversation Service
	conversationService := core.NewConversationService()

	// Initialize Conversation Use Case
	mentionHandler := app.NewHandleMentionUseCase(
		discordClient,
		conversationService,
		llmClient,
		discordClient,
		candidateCreator,
		memoryRetriever,
		cfg.Conversation.MaxMessages,
		conversationContextTTL,
	)

	if err := mentionHandler.SetConversationRepository(
		conversationRepository,
	); err != nil {
		return fmt.Errorf("configure conversation repository: %w", err)
	}

	if err := mentionHandler.SetPersonalityProvider(
		personalityProvider,
	); err != nil {
		return fmt.Errorf("configure personality provider: %w", err)
	}

	if err := mentionHandler.SetMoodResolver(
		moodResolver,
	); err != nil {
		return fmt.Errorf("configure mood resolver: %w", err)
	}

	if err := mentionHandler.SetCommunityContextSyncer(
		communitySyncer,
	); err != nil {
		return fmt.Errorf("configure community context syncer: %w", err)
	}

	if err := mentionHandler.SetMaxRetrievedMemories(
		cfg.Memory.MaxRetrievedMemories,
	); err != nil {
		return fmt.Errorf("configure max retrieved memories: %w", err)
	}

	if err := mentionHandler.SetTimeout(cfg.Conversation.Timeout); err != nil {
		return fmt.Errorf("configure mention timeout: %w", err)
	}

	// Inject Mention Handler
	if err := discordGateway.SetMentionHandler(mentionHandler); err != nil {
		return fmt.Errorf("set mention handler: %w", err)
	}

	// Start Discord Gateway
	if err := discordGateway.Start(ctx); err != nil {
		return fmt.Errorf("start Discord gateway: %w", err)
	}

	// Wait for shutdown signal
	<-ctx.Done()

	appLogger.Info("Shutdown signal received")
	appLogger.Info("The Bebot stopped")

	return nil
}
