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
	"the-bebot/internal/conversation/app"
	"the-bebot/internal/conversation/core"
	"the-bebot/internal/discord/adapter/gateway"
	"the-bebot/internal/platform/config"
	"the-bebot/internal/platform/database"
	"the-bebot/internal/platform/logger"
	"the-bebot/internal/worker"

	memoryadapter "the-bebot/internal/memory/adapter/conversation"
	memorymongodb "the-bebot/internal/memory/adapter/mongodb"
	memoryapp "the-bebot/internal/memory/app"
	memorycore "the-bebot/internal/memory/core"
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

		appLogger.Info(
			"Memory worker started",
			"workers", cfg.Memory.WorkerCount,
		)

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
