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

var ErrEmptyResponse = errors.New("response generator returned empty response")

type HandleMentionUseCase struct {
	contextProvider     core.ContextProvider
	conversationService core.ConversationService
	responseGenerator   core.ResponseGenerator
	messageSender       core.MessageSender
	candidateCreator    core.CandidateCreator

	maxMessages int
	contextTTL  time.Duration
}

func NewHandleMentionUseCase(
	contextProvider core.ContextProvider,
	conversationService core.ConversationService,
	responseGenerator core.ResponseGenerator,
	messageSender core.MessageSender,
	candidateCreator core.CandidateCreator,
	maxMessages int,
	contextTTL time.Duration,
) *HandleMentionUseCase {
	return &HandleMentionUseCase{
		contextProvider:     contextProvider,
		conversationService: conversationService,
		responseGenerator:   responseGenerator,
		messageSender:       messageSender,
		candidateCreator:    candidateCreator,
		maxMessages:         maxMessages,
		contextTTL:          contextTTL,
	}
}

func (uc *HandleMentionUseCase) HandleMention(
	ctx context.Context,
	event core.MentionEvent,
) error {
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

	// Fetch recent messages from the same channel.
	messages, err := uc.contextProvider.FetchRecentMessages(
		ctx,
		event.GuildID,
		event.ChannelID,
		uc.maxMessages,
	)
	if err != nil {
		return fmt.Errorf("fetch recent messages: %w", err)
	}

	// Ensure the trigger message is included.
	messages, err = ensureTriggerMessage(
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

	// Create memory candidate for background processing.
	if err := uc.candidateCreator.CreateCandidate(
		ctx,
		conversationContext,
	); err != nil {
		return fmt.Errorf("create memory candidate: %w", err)
	}

	return nil
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
