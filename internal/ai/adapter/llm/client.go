package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"the-bebot/internal/conversation/core"
)

const (
	defaultTimeout  = 30 * time.Second
	maxResponseSize = 1 << 20 // 1 MB
)

const defaultSystemPrompt = `
You are The Bebot, a playful and mischievous member of a Discord community.

Personality:
- Be casual, witty, lively, and entertaining.
- Tease people lightly without humiliating or hurting them.
- Have your own opinions and personality.
- Use sarcasm and humor naturally.
- Avoid sounding like a formal assistant.
- Respond in the language used by the community.
- Do not invent memories or claim to remember things that are not present in the provided context.

You are participating in a community conversation, not providing formal customer support.
`

type Config struct {
	Endpoint     string
	APIKey       string
	Model        string
	SystemPrompt string
	Timeout      time.Duration
}

type Client struct {
	endpoint     string
	apiKey       string
	model        string
	systemPrompt string
	httpClient   *http.Client
}

var _ core.ResponseGenerator = (*Client)(nil)

func NewClient(cfg Config) (*Client, error) {
	cfg.Endpoint = strings.TrimSpace(cfg.Endpoint)
	cfg.APIKey = strings.TrimSpace(cfg.APIKey)
	cfg.Model = strings.TrimSpace(cfg.Model)

	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("LLM endpoint is required")
	}

	parsedURL, err := url.ParseRequestURI(cfg.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid LLM endpoint: %w", err)
	}

	if (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") ||
		parsedURL.Host == "" {
		return nil, fmt.Errorf("LLM endpoint must be a valid HTTP or HTTPS URL")
	}

	if cfg.APIKey == "" {
		return nil, fmt.Errorf("LLM API key is required")
	}

	if cfg.Model == "" {
		return nil, fmt.Errorf("LLM model is required")
	}

	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultTimeout
	}

	if strings.TrimSpace(cfg.SystemPrompt) == "" {
		cfg.SystemPrompt = defaultSystemPrompt
	}

	return &Client{
		endpoint:     cfg.Endpoint,
		apiKey:       cfg.APIKey,
		model:        cfg.Model,
		systemPrompt: strings.TrimSpace(cfg.SystemPrompt),
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
	}, nil
}

func (c *Client) Generate(
	ctx context.Context,
	conversationContext core.ConversationContext,
) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	messages, err := buildChatMessages(
		c.systemPrompt,
		conversationContext.Messages,
	)
	if err != nil {
		return "", err
	}

	return c.complete(ctx, messages)
}

func buildChatMessages(
	systemPrompt string,
	contextMessages []core.Message,
) ([]chatMessage, error) {
	if len(contextMessages) == 0 {
		return nil, fmt.Errorf("conversation context is empty")
	}

	messages := make([]chatMessage, 0, len(contextMessages)+1)

	messages = append(messages, chatMessage{
		Role:    "system",
		Content: systemPrompt,
	})

	for _, message := range contextMessages {
		content := strings.TrimSpace(message.Content)

		if content == "" {
			continue
		}

		// Other bots are context, not The Bebot's own responses.
		if message.IsBot {
			content = "[Another bot said]: " + content
		}

		messages = append(messages, chatMessage{
			Role:    "user",
			Content: content,
		})
	}

	if len(messages) == 1 {
		return nil, fmt.Errorf("conversation context contains no usable messages")
	}

	return messages, nil
}

func (c *Client) GenerateText(
	ctx context.Context,
	systemPrompt string,
	userPrompt string,
) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	systemPrompt = strings.TrimSpace(systemPrompt)
	userPrompt = strings.TrimSpace(userPrompt)

	if systemPrompt == "" || userPrompt == "" {
		return "", fmt.Errorf("system prompt and user prompt are required")
	}

	messages := []chatMessage{
		{
			Role:    "system",
			Content: systemPrompt,
		},
		{
			Role:    "user",
			Content: userPrompt,
		},
	}

	return c.complete(ctx, messages)
}

func (c *Client) complete(
	ctx context.Context,
	messages []chatMessage,
) (string, error) {
	requestBody := chatRequest{
		Model:    c.model,
		Messages: messages,
	}

	payload, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("marshal LLM request: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.endpoint,
		bytes.NewReader(payload),
	)
	if err != nil {
		return "", fmt.Errorf("create LLM request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("send LLM request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(
		io.LimitReader(resp.Body, maxResponseSize),
	)
	if err != nil {
		return "", fmt.Errorf("read LLM response: %w", err)
	}

	if resp.StatusCode < http.StatusOK ||
		resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf(
			"LLM API returned status %d: %s",
			resp.StatusCode,
			strings.TrimSpace(string(body)),
		)
	}

	var result chatResponse

	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("decode LLM response: %w", err)
	}

	if result.Error != nil {
		return "", fmt.Errorf("LLM API error: %s", result.Error.Message)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("LLM response contains no choices")
	}

	content := strings.TrimSpace(result.Choices[0].Message.Content)

	if content == "" {
		return "", fmt.Errorf("LLM response content is empty")
	}

	return content, nil
}