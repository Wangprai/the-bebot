package core

import (
	"errors"
	"strings"
)

var (
	ErrInvalidExtractionInput      = errors.New("invalid extraction input")
	ErrInvalidExtractionSource     = errors.New("invalid extraction source")
	ErrInvalidExtractedMemory      = errors.New("invalid extracted memory")
	ErrAllExtractedMemoriesInvalid = errors.New("all extracted memories failed validation")
)

type ExtractionInput struct {
	GuildID   string
	ChannelID string
	UserID    string
	Messages  []ConversationMessage
}

type ConversationSnapshot struct {
	GuildID          string
	ChannelID        string
	TriggerMessageID string
	TargetUserID     string
	Messages         []ConversationMessage
}

type ConversationMessage struct {
	MessageID string
	UserID    string
	Content   string
	IsBot     bool
}

type ExtractionSource struct {
	GuildID   string
	ChannelID string
	MessageID string
	UserID    string
}

type ExtractedMemory struct {
	Scope           MemoryScope `json:"scope"`
	Type            MemoryType  `json:"type"`
	UserID          string      `json:"user_id"`
	Content         string      `json:"content"`
	Importance      int         `json:"importance"`
	Confidence      float64     `json:"confidence"`
	Tags            []string    `json:"tags"`
	SourceMessageID string      `json:"source_message_id"`
}

func NewExtractionInput(input ExtractionInput) (ExtractionInput, error) {
	input.GuildID = strings.TrimSpace(input.GuildID)
	input.ChannelID = strings.TrimSpace(input.ChannelID)
	input.UserID = strings.TrimSpace(input.UserID)

	if input.GuildID == "" ||
		input.ChannelID == "" ||
		input.UserID == "" ||
		len(input.Messages) == 0 {
		return ExtractionInput{}, ErrInvalidExtractionInput
	}

	for i := range input.Messages {
		input.Messages[i].MessageID = strings.TrimSpace(input.Messages[i].MessageID)
		input.Messages[i].UserID = strings.TrimSpace(input.Messages[i].UserID)
		input.Messages[i].Content = strings.TrimSpace(input.Messages[i].Content)

		if input.Messages[i].MessageID == "" ||
			input.Messages[i].UserID == "" ||
			input.Messages[i].Content == "" {
			return ExtractionInput{}, ErrInvalidExtractionInput
		}
	}

	return input, nil
}

func NewConversationSnapshot(snapshot ConversationSnapshot) (ConversationSnapshot, error) {
	snapshot.GuildID = strings.TrimSpace(snapshot.GuildID)
	snapshot.ChannelID = strings.TrimSpace(snapshot.ChannelID)
	snapshot.TriggerMessageID = strings.TrimSpace(snapshot.TriggerMessageID)

	if snapshot.GuildID == "" ||
		snapshot.ChannelID == "" ||
		snapshot.TriggerMessageID == "" ||
		len(snapshot.Messages) == 0 {
		return ConversationSnapshot{}, ErrInvalidExtractionInput
	}

	triggerFound := false

	for i := range snapshot.Messages {
		message := &snapshot.Messages[i]

		message.MessageID = strings.TrimSpace(message.MessageID)
		message.UserID = strings.TrimSpace(message.UserID)
		message.Content = strings.TrimSpace(message.Content)

		if message.MessageID == "" ||
			message.UserID == "" ||
			message.Content == "" {
			return ConversationSnapshot{}, ErrInvalidExtractionInput
		}

		if message.MessageID == snapshot.TriggerMessageID {
			triggerFound = true
		}
	}

	if !triggerFound {
		return ConversationSnapshot{}, ErrInvalidExtractionInput
	}

	return snapshot, nil
}
