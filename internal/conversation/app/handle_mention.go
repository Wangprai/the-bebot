package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"the-bebot/internal/conversation/core"
)

const (
	defaultMaxRetrievedMemories = 5
	defaultTypingInterval       = 8 * time.Second
)

var ErrEmptyResponse = errors.New("response generator returned empty response")

type HandleMentionUseCase struct {
	contextProvider        core.ContextProvider
	conversationService    core.ConversationService
	responseGenerator      core.ResponseGenerator
	messageSender          core.MessageSender
	typingIndicator        core.TypingIndicator
	candidateCreator       core.CandidateCreator
	memoryRetriever        core.MemoryRetriever
	conversationRepository core.ConversationRepository
	personalityProvider    core.PersonalityProvider
	moodResolver           core.MoodResolver
	communitySyncer        core.CommunityContextSyncer
	logger                 *slog.Logger

	maxMessages          int
	maxRetrievedMemories int
	contextTTL           time.Duration
	mentionTimeout       time.Duration
	typingInterval       time.Duration

	clock               func() time.Time
	userCooldown        time.Duration
	cooldownMu          sync.Mutex
	userLastMention     map[string]time.Time
	lastCooldownCleanup time.Time
	mentionSem          chan struct{}
}

func NewHandleMentionUseCase(
	contextProvider core.ContextProvider,
	conversationService core.ConversationService,
	responseGenerator core.ResponseGenerator,
	messageSender core.MessageSender,
	candidateCreator core.CandidateCreator,
	memoryRetriever core.MemoryRetriever,
	maxMessages int,
	contextTTL time.Duration,
) *HandleMentionUseCase {
	return &HandleMentionUseCase{
		contextProvider:      contextProvider,
		conversationService:  conversationService,
		responseGenerator:    responseGenerator,
		messageSender:        messageSender,
		candidateCreator:     candidateCreator,
		memoryRetriever:      memoryRetriever,
		logger:               slog.New(slog.NewTextHandler(io.Discard, nil)),
		maxMessages:          maxMessages,
		maxRetrievedMemories: defaultMaxRetrievedMemories,
		contextTTL:           contextTTL,
		typingInterval:       defaultTypingInterval,
		clock:                time.Now,
		userLastMention:      make(map[string]time.Time),
	}
}

func (uc *HandleMentionUseCase) SetMaxRetrievedMemories(limit int) error {
	if uc == nil {
		return fmt.Errorf("handle mention use case is nil")
	}

	if limit <= 0 {
		return fmt.Errorf("max retrieved memories must be greater than zero")
	}

	uc.maxRetrievedMemories = limit
	return nil
}

func (uc *HandleMentionUseCase) SetTimeout(timeout time.Duration) error {
	if uc == nil {
		return fmt.Errorf("handle mention use case is nil")
	}

	if timeout <= 0 {
		return fmt.Errorf("mention timeout must be greater than zero")
	}

	uc.mentionTimeout = timeout
	return nil
}

func (uc *HandleMentionUseCase) SetConversationRepository(
	repository core.ConversationRepository,
) error {
	if uc == nil {
		return fmt.Errorf("handle mention use case is nil")
	}

	if repository == nil {
		return fmt.Errorf("conversation repository is required")
	}

	uc.conversationRepository = repository
	return nil
}

func (uc *HandleMentionUseCase) SetPersonalityProvider(
	provider core.PersonalityProvider,
) error {
	if uc == nil {
		return fmt.Errorf("handle mention use case is nil")
	}

	if provider == nil {
		return fmt.Errorf("personality provider is required")
	}

	uc.personalityProvider = provider
	return nil
}

func (uc *HandleMentionUseCase) SetMoodResolver(
	resolver core.MoodResolver,
) error {
	if uc == nil {
		return fmt.Errorf("handle mention use case is nil")
	}

	if resolver == nil {
		return fmt.Errorf("mood resolver is required")
	}

	uc.moodResolver = resolver
	return nil
}

func (uc *HandleMentionUseCase) SetCommunityContextSyncer(
	syncer core.CommunityContextSyncer,
) error {
	if uc == nil {
		return fmt.Errorf("handle mention use case is nil")
	}

	if syncer == nil {
		return fmt.Errorf("community context syncer is required")
	}

	uc.communitySyncer = syncer
	return nil
}

func (uc *HandleMentionUseCase) SetTypingIndicator(
	indicator core.TypingIndicator,
) error {
	if uc == nil {
		return fmt.Errorf("handle mention use case is nil")
	}

	if indicator == nil {
		return fmt.Errorf("typing indicator is required")
	}

	uc.typingIndicator = indicator
	return nil
}

func (uc *HandleMentionUseCase) SetLogger(logger *slog.Logger) error {
	if uc == nil {
		return fmt.Errorf("handle mention use case is nil")
	}

	if logger == nil {
		return fmt.Errorf("logger is required")
	}

	uc.logger = logger
	return nil
}

func (uc *HandleMentionUseCase) SetClock(clock func() time.Time) error {
	if uc == nil {
		return fmt.Errorf("handle mention use case is nil")
	}

	if clock == nil {
		return fmt.Errorf("clock is required")
	}

	uc.clock = clock
	return nil
}

func (uc *HandleMentionUseCase) SetUserCooldown(cooldown time.Duration) error {
	if uc == nil {
		return fmt.Errorf("handle mention use case is nil")
	}

	if cooldown < 0 {
		return fmt.Errorf("user cooldown must be zero or greater")
	}

	uc.cooldownMu.Lock()
	uc.userCooldown = cooldown
	uc.cooldownMu.Unlock()

	return nil
}

func (uc *HandleMentionUseCase) SetMaxConcurrentMentions(limit int) error {
	if uc == nil {
		return fmt.Errorf("handle mention use case is nil")
	}

	if limit <= 0 {
		return fmt.Errorf("max concurrent mentions must be greater than zero")
	}

	uc.mentionSem = make(chan struct{}, limit)
	return nil
}

func (uc *HandleMentionUseCase) logWarn(msg string, args ...any) {
	if uc != nil && uc.logger != nil {
		uc.logger.Warn(msg, args...)
	}
}

func (uc *HandleMentionUseCase) logDebug(msg string, args ...any) {
	if uc != nil && uc.logger != nil {
		uc.logger.Debug(msg, args...)
	}
}

func (uc *HandleMentionUseCase) now() time.Time {
	if uc != nil && uc.clock != nil {
		return uc.clock()
	}
	return time.Now()
}

func (uc *HandleMentionUseCase) allowUserMention(userID string) bool {
	uc.cooldownMu.Lock()
	defer uc.cooldownMu.Unlock()

	if uc.userCooldown <= 0 {
		return true
	}

	now := uc.now()

	if uc.userLastMention == nil {
		uc.userLastMention = make(map[string]time.Time)
	}

	if uc.lastCooldownCleanup.IsZero() || now.Sub(uc.lastCooldownCleanup) >= uc.userCooldown {
		for uid, ts := range uc.userLastMention {
			if now.Sub(ts) >= uc.userCooldown {
				delete(uc.userLastMention, uid)
			}
		}
		uc.lastCooldownCleanup = now
	}

	if last, exists := uc.userLastMention[userID]; exists && now.Sub(last) < uc.userCooldown {
		return false
	}

	uc.userLastMention[userID] = now
	return true
}

func (uc *HandleMentionUseCase) cooldownEntryCount() int {
	uc.cooldownMu.Lock()
	defer uc.cooldownMu.Unlock()
	return len(uc.userLastMention)
}

func (uc *HandleMentionUseCase) HandleMention(
	ctx context.Context,
	event core.MentionEvent,
) error {
	if ctx == nil {
		return fmt.Errorf("context is required")
	}

	// Validate incoming event.
	if err := event.Validate(); err != nil {
		return fmt.Errorf("validate mention event: %w", err)
	}

	// Enforce global concurrency cap without piling up blocked goroutines.
	if uc.mentionSem != nil {
		select {
		case uc.mentionSem <- struct{}{}:
			defer func() { <-uc.mentionSem }()
		default:
			uc.logDebug(
				"Dropping mention: max concurrent mentions reached",
				"guild_id", event.GuildID,
				"channel_id", event.ChannelID,
				"user_id", event.UserID,
				"message_id", event.MessageID,
			)
			return nil
		}
	}

	// Enforce per-user cooldown silently.
	if !uc.allowUserMention(event.UserID) {
		uc.logDebug(
			"Ignoring mention inside per-user cooldown",
			"guild_id", event.GuildID,
			"channel_id", event.ChannelID,
			"user_id", event.UserID,
			"message_id", event.MessageID,
		)
		return nil
	}

	if uc.mentionTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, uc.mentionTimeout)
		defer cancel()
	}

	if uc.maxMessages <= 0 {
		return fmt.Errorf("max messages must be greater than zero")
	}

	if uc.contextTTL <= 0 {
		return fmt.Errorf("context TTL must be greater than zero")
	}

	// Fetch recent messages from Discord as the primary source.
	messages, fetchErr := uc.contextProvider.FetchRecentMessages(
		ctx,
		event.GuildID,
		event.ChannelID,
		uc.maxMessages,
	)

	if fetchErr != nil {
		// Fall back to the latest non-expired persisted snapshot if available.
		fallback, ok := uc.loadLatestPersistedMessages(ctx, event)
		if !ok {
			return fmt.Errorf("fetch recent messages: %w", fetchErr)
		}
		messages = fallback
	} else if len(messages) < uc.maxMessages {
		// Supplement live messages with non-duplicate messages from the latest snapshot.
		if persisted, ok := uc.loadLatestPersistedMessages(ctx, event); ok {
			messages = mergeUniqueMessages(messages, persisted)
		}
	}

	// Ensure the trigger message is included.
	messages, err := ensureTriggerMessage(
		event,
		messages,
		uc.maxMessages,
	)
	if err != nil {
		return fmt.Errorf("ensure trigger message: %w", err)
	}

	// Build conversation context.
	conversationContext, err := uc.conversationService.BuildContext(
		event,
		messages,
		uc.maxMessages,
		uc.contextTTL,
	)
	if err != nil {
		return fmt.Errorf("build conversation context: %w", err)
	}

	if conversationContext.BotUserID == "" && event.BotUserID != "" {
		conversationContext.BotUserID = event.BotUserID
	}

	if len(conversationContext.MentionNames) == 0 && len(event.MentionNames) > 0 {
		conversationContext.MentionNames = make(map[string]string, len(event.MentionNames))
		for id, name := range event.MentionNames {
			conversationContext.MentionNames[id] = name
		}
	}

	if uc.personalityProvider != nil {
		personalityCtx, err := uc.personalityProvider.GetPersonality(ctx)
		if err != nil {
			return fmt.Errorf("get personality: %w", err)
		}

		conversationContext.Personality = &personalityCtx
	}

	if uc.moodResolver != nil {
		moodCtx, err := uc.moodResolver.ResolveMood(
			ctx,
			event.GuildID,
			event.ChannelID,
			event.Content,
		)
		if err != nil {
			uc.logWarn(
				"Failed to resolve mood; continuing without mood context",
				"guild_id", event.GuildID,
				"channel_id", event.ChannelID,
				"error", err,
			)
		} else {
			conversationContext.Mood = &moodCtx
		}
	}

	if uc.communitySyncer != nil {
		communityCtx, err := uc.communitySyncer.SyncMentionContext(ctx, event)
		if err != nil {
			uc.logWarn(
				"Failed to sync community context; continuing without community context",
				"guild_id", event.GuildID,
				"user_id", event.UserID,
				"error", err,
			)
		} else {
			conversationContext.Community = &communityCtx
		}
	}

	if uc.memoryRetriever != nil {
		limit := uc.maxRetrievedMemories
		if limit <= 0 {
			limit = defaultMaxRetrievedMemories
		}

		memories, err := uc.memoryRetriever.RetrieveMemories(
			ctx,
			event.GuildID,
			event.UserID,
			limit,
		)
		if err != nil {
			uc.logWarn(
				"Failed to retrieve memories; continuing without memory context",
				"guild_id", event.GuildID,
				"user_id", event.UserID,
				"error", err,
			)
		} else {
			conversationContext.Memories = memories
		}
	}

	// Generate AI response while showing typing indicator.
	stopTyping := uc.startTyping(ctx, event.ChannelID)
	defer stopTyping()

	response, err := uc.responseGenerator.Generate(
		ctx,
		conversationContext,
	)
	stopTyping()
	if err != nil {
		return fmt.Errorf("generate response: %w", err)
	}

	response = strings.TrimSpace(response)
	if response == "" {
		return ErrEmptyResponse
	}

	// Send response to Discord as a reply to the trigger message.
	_, err = uc.messageSender.Send(
		ctx,
		event.GuildID,
		event.ChannelID,
		event.MessageID,
		response,
	)
	if err != nil {
		return fmt.Errorf("send response: %w", err)
	}

	// Post-send steps are best-effort: create memory candidate first, then save snapshot.
	if uc.candidateCreator != nil {
		if err := uc.candidateCreator.CreateCandidate(
			ctx,
			conversationContext,
		); err != nil {
			uc.logWarn(
				"Failed to create memory candidate after sending reply",
				"guild_id", event.GuildID,
				"channel_id", event.ChannelID,
				"message_id", event.MessageID,
				"error", err,
			)
		}
	}

	if uc.conversationRepository != nil {
		if err := uc.conversationRepository.Save(
			ctx,
			conversationContext,
		); err != nil {
			uc.logWarn(
				"Failed to save conversation context after sending reply",
				"guild_id", event.GuildID,
				"channel_id", event.ChannelID,
				"message_id", event.MessageID,
				"error", err,
			)
		}
	}

	return nil
}

func (uc *HandleMentionUseCase) startTyping(
	ctx context.Context,
	channelID string,
) func() {
	indicator := uc.typingIndicator
	if indicator == nil {
		if ti, ok := uc.messageSender.(core.TypingIndicator); ok {
			indicator = ti
		}
	}

	if indicator == nil {
		return func() {}
	}

	// Send an initial typing signal synchronously (best-effort).
	_ = indicator.StartTyping(ctx, channelID)

	interval := uc.typingInterval
	if interval <= 0 {
		interval = defaultTypingInterval
	}

	typingCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})

	go func() {
		defer close(done)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-typingCtx.Done():
				return
			case <-ticker.C:
				_ = indicator.StartTyping(typingCtx, channelID)
			}
		}
	}()

	var once sync.Once
	return func() {
		once.Do(func() {
			cancel()
			<-done
		})
	}
}

func (uc *HandleMentionUseCase) loadLatestPersistedMessages(
	ctx context.Context,
	event core.MentionEvent,
) ([]core.Message, bool) {
	if uc.conversationRepository == nil {
		return nil, false
	}

	latest, err := uc.conversationRepository.FindLatest(
		ctx,
		event.GuildID,
		event.ChannelID,
	)
	if err != nil || latest == nil {
		return nil, false
	}

	if err := uc.conversationService.ValidateContext(*latest, time.Now().UTC()); err != nil {
		return nil, false
	}

	if len(latest.Messages) == 0 {
		return nil, false
	}

	return latest.Messages, true
}

func mergeUniqueMessages(
	live []core.Message,
	persisted []core.Message,
) []core.Message {
	seen := make(map[string]struct{}, len(live)+len(persisted))
	merged := make([]core.Message, 0, len(live)+len(persisted))

	// Live Discord messages take precedence over persisted copies.
	for _, message := range live {
		if _, exists := seen[message.ID]; exists {
			continue
		}
		seen[message.ID] = struct{}{}
		merged = append(merged, message)
	}

	for _, message := range persisted {
		if _, exists := seen[message.ID]; exists {
			continue
		}
		seen[message.ID] = struct{}{}
		merged = append(merged, message)
	}

	return merged
}

func ensureTriggerMessage(
	event core.MentionEvent,
	messages []core.Message,
	maxMessages int,
) ([]core.Message, error) {
	if maxMessages <= 0 {
		return nil, fmt.Errorf("max messages must be greater than zero")
	}

	triggerAuthorName := strings.TrimSpace(event.UserNickname)
	if triggerAuthorName == "" {
		triggerAuthorName = strings.TrimSpace(event.UserDisplayName)
	}

	triggerFound := false

	for i, message := range messages {
		if message.ID != event.MessageID {
			continue
		}

		if message.GuildID != event.GuildID ||
			message.ChannelID != event.ChannelID ||
			message.UserID != event.UserID {
			return nil, core.ErrInvalidMessage
		}

		if triggerAuthorName != "" {
			messages[i].AuthorName = triggerAuthorName
		}
		messages[i].Content = strings.TrimSpace(event.Content)
		messages[i].CreatedAt = event.MentionedAt
		messages[i].IsBot = false
		messages[i].IsSelf = false

		triggerFound = true
		break
	}

	if !triggerFound {
		messages = append(messages, core.Message{
			ID:         event.MessageID,
			GuildID:    event.GuildID,
			ChannelID:  event.ChannelID,
			UserID:     event.UserID,
			AuthorName: triggerAuthorName,
			Content:    strings.TrimSpace(event.Content),
			CreatedAt:  event.MentionedAt,
			IsBot:      false,
			IsSelf:     false,
		})
	}

	// Sort messages from oldest to newest.
	sort.SliceStable(messages, func(i, j int) bool {
		return messages[i].CreatedAt.Before(messages[j].CreatedAt)
	})

	// Keep the trigger message and the newest messages.
	if len(messages) > maxMessages {
		triggerIndex := -1

		for i, message := range messages {
			if message.ID == event.MessageID {
				triggerIndex = i
				break
			}
		}

		if triggerIndex == -1 {
			return nil, core.ErrTriggerMessageNotFound
		}

		selected := make([]core.Message, 0, maxMessages)
		selected = append(selected, messages[triggerIndex])

		for i := len(messages) - 1; i >= 0 && len(selected) < maxMessages; i-- {
			if i == triggerIndex {
				continue
			}

			selected = append(selected, messages[i])
		}

		messages = selected

		sort.SliceStable(messages, func(i, j int) bool {
			return messages[i].CreatedAt.Before(messages[j].CreatedAt)
		})
	}

	return messages, nil
}
