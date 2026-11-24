package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"the-bebot/internal/memory/core"
)

const memoryExtractionSystemPrompt = `
You are a memory extraction engine for The Bebot,
a playful Discord community chatbot.

Extract useful long-term memories from the provided conversation.

Rules:
- Extract only information explicitly supported by messages.
- Never invent facts or infer unsupported personal details.
- Ignore temporary small talk and meaningless conversation.
- Focus on preferences, personal facts, events, promises,
  running jokes, catchphrases, and interaction patterns.
- Use only the provided memory types.
- Do not extract sensitive personal information.
- Member memories must belong to the target user.
- Community memories must genuinely apply to the community.
- Importance must be an integer from 1 to 5.
- Confidence must be between 0 and 1.
- Return valid JSON only.
- Do not include Markdown code fences.
- Return an empty array [] if no useful memory is found.

Expected JSON format:
[
  {
    "scope": "member",
    "type": "preference",
    "content": "Likes Go programming",
    "importance": 3,
    "confidence": 0.95,
    "tags": ["go", "programming"],
    "source_message_id": "message-001"
  }
]

For every extracted memory:

- Include source_message_id.
- source_message_id must be an existing message ID
  from the provided conversation.
- Do not invent message IDs.
- The source message must support the extracted memory.
- For member scope, the source message author must
  match the target user ID.
- For community scope, the source message may belong
  to any community member.

Return JSON with these fields:
scope, type, content, importance, confidence,
tags, source_message_id.

Do not return user_id.
`

type MemoryExtractor struct {
	client *Client
}

var _ core.MemoryExtractor = (*MemoryExtractor)(nil)

func NewMemoryExtractor(client *Client) (*MemoryExtractor, error) {
	if client == nil {
		return nil, fmt.Errorf("LLM client is required")
	}

	return &MemoryExtractor{client: client}, nil
}

func (e *MemoryExtractor) Extract(
	ctx context.Context,
	input core.ExtractionInput,
) ([]core.ExtractedMemory, error) {
	if e == nil || e.client == nil {
		return nil, fmt.Errorf("memory extractor is not initialized")
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	input, err := core.NewExtractionInput(input)
	if err != nil {
		return nil, fmt.Errorf("validate extraction input: %w", err)
	}

	type extractionMessage struct {
		MessageID string `json:"message_id"`
		UserID    string `json:"user_id"`
		Content   string `json:"content"`
		IsBot     bool   `json:"is_bot"`
	}

	messages := make([]extractionMessage, 0, len(input.Messages))

	for _, message := range input.Messages {
		messages = append(messages, extractionMessage{
			MessageID: message.MessageID,
			UserID:    message.UserID,
			Content:   message.Content,
			IsBot:     message.IsBot,
		})
	}

	messageJSON, err := json.Marshal(messages)
	if err != nil {
		return nil, fmt.Errorf("marshal extraction messages: %w", err)
	}

	userPrompt := fmt.Sprintf(
		"Community ID: %s\nChannel ID: %s\nTarget User ID: %s\n\n"+
			"Extract useful memories from these messages:\n%s",
		input.GuildID,
		input.ChannelID,
		input.UserID,
		string(messageJSON),
	)

	response, err := e.client.GenerateText(
		ctx,
		memoryExtractionSystemPrompt,
		userPrompt,
	)
	if err != nil {
		return nil, fmt.Errorf("generate memory extraction: %w", err)
	}

	response = strings.TrimSpace(response)

	// Reject non-array responses before unmarshalling.
	if !strings.HasPrefix(response, "[") {
		return nil, fmt.Errorf("memory extraction response must be a JSON array")
	}

	var memories []core.ExtractedMemory

	if err := json.Unmarshal([]byte(response), &memories); err != nil {
		return nil, fmt.Errorf("decode extracted memories: %w", err)
	}

	if memories == nil {
		return nil, fmt.Errorf("memory extraction returned null instead of an array")
	}

	return memories, nil
}
