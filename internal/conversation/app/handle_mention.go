package app

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"the-bebot/internal/conversation/core"
)

const defaultMaxRetrievedMemories = 5

var ErrEmptyResponse = errors.New("response generator returned empty response")

type HandleMentionUseCase struct {
	contextProvider        core.ContextProvider
	conversationService    core.ConversationService
	responseGenerator      core.ResponseGenerator
	messageSender          core.MessageSender
	candidateCreator       core.CandidateCreator
	memoryRetriever        core.MemoryRetriever
	conversationRepository core.ConversationRepository
	personalityProvider    core.PersonalityProvider
	moodResolver           core.MoodResolver

	maxMessages          int
	maxRetrievedMemories int
	contextTTL           time.Duration
	mentionTimeout       time.Duration
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
		maxMessages:          maxMessages,
		maxRetrievedMemories: defaultMaxRetrievedMemories,
		contextTTL:           contextTTL,
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

func (uc *HandleMentionUseCase) HandleMention(
	ctx context.Context,
	event core.MentionEvent,
) error {
	if ctx == nil {
		return fmt.Errorf("context is required")
	}

	if uc.mentionTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, uc.mentionTimeout)
		defer cancel()
	}

	// Validate incoming event.
	if err := event.Validate(); err != nil {
		return fmt.Errorf("validate mention event: %w", err)
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
			return fmt.Errorf("resolve mood: %w", err)
		}

		conversationContext.Mood = &moodCtx
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
			return fmt.Errorf("retrieve memories: %w", err)
		}

		conversationContext.Memories = memories
	}

	// Generate AI response.
	response, err := uc.responseGenerator.Generate(
		ctx,
		conversationContext,
	)
	if err != nil {
		return fmt.Errorf("generate response: %w", err)
	}

	response = strings.TrimSpace(response)
	if response == "" {
		return ErrEmptyResponse
	}

	// Send response to Discord.
	_, err = uc.messageSender.Send(
		ctx,
		event.GuildID,
		event.ChannelID,
		response,
	)
	if err != nil {
		return fmt.Errorf("send response: %w", err)
	}

	// Persist latest conversation context snapshot (24h TTL).
	if uc.conversationRepository != nil {
		if err := uc.conversationRepository.Save(
			ctx,
			conversationContext,
		); err != nil {
			return fmt.Errorf("save conversation context: %w", err)
		}
	}

	// Create memory candidate for background processing.
	if err := uc.candidateCreator.CreateCandidate(
		ctx,
		conversationContext,
	); err != nil {
		return fmt.Errorf("create memory candidate: %w", err)
	}

	return nil
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

		messages[i].Content = strings.TrimSpace(event.Content)
		messages[i].CreatedAt = event.MentionedAt
		messages[i].IsBot = false

		triggerFound = true
		break
	}

	if !triggerFound {
		messages = append(messages, core.Message{
			ID:        event.MessageID,
			GuildID:   event.GuildID,
			ChannelID: event.ChannelID,
			UserID:    event.UserID,
			Content:   strings.TrimSpace(event.Content),
			CreatedAt: event.MentionedAt,
			IsBot:     false,
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
