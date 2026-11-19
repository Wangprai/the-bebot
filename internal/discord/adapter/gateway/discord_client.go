package gateway

import (
	"context"
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"

	"the-bebot/internal/conversation/core"
)

type DiscordClient struct {
	session *discordgo.Session
}

func NewDiscordClient(
	session *discordgo.Session,
) (*DiscordClient, error) {
	if session == nil {
		return nil, fmt.Errorf("discord session is required")
	}

	return &DiscordClient{
		session: session,
	}, nil
}

// Compile-time interface checks.
var (
	_ core.ContextProvider = (*DiscordClient)(nil)
	_ core.MessageSender   = (*DiscordClient)(nil)
)

// FetchRecentMessages retrieves recent messages from one channel.
func (c *DiscordClient) FetchRecentMessages(
	ctx context.Context,
	guildID string,
	channelID string,
	limit int,
) ([]core.Message, error) {
	if guildID == "" || channelID == "" {
		return nil, fmt.Errorf("guild ID and channel ID are required")
	}

	if limit <= 0 {
		return nil, fmt.Errorf("message limit must be greater than zero")
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	discordMessages, err := c.session.ChannelMessages(
		channelID,
		limit,
		"",
		"",
		"",
	)
	if err != nil {
		return nil, fmt.Errorf("fetch discord messages: %w", err)
	}

	messages := make([]core.Message, 0, len(discordMessages))

	for _, discordMessage := range discordMessages {
		if discordMessage == nil || discordMessage.Author == nil {
			continue
		}

		// Only accept messages belonging to the requested scope.
		if discordMessage.GuildID != guildID ||
			discordMessage.ChannelID != channelID {
			continue
		}

		// MVP conversation context supports text messages.
		if strings.TrimSpace(discordMessage.Content) == "" {
			continue
		}

		messages = append(messages, core.Message{
			ID:        discordMessage.ID,
			GuildID:   discordMessage.GuildID,
			ChannelID: discordMessage.ChannelID,
			UserID:    discordMessage.Author.ID,
			Content:   discordMessage.Content,
			CreatedAt: discordMessage.Timestamp,
			IsBot:     discordMessage.Author.Bot,
		})
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return messages, nil
}

// Send sends a message to the specified Discord channel.
func (c *DiscordClient) Send(
	ctx context.Context,
	guildID string,
	channelID string,
	content string,
) (core.Message, error) {
	if guildID == "" || channelID == "" {
		return core.Message{}, fmt.Errorf(
			"guild ID and channel ID are required",
		)
	}

	if strings.TrimSpace(content) == "" {
		return core.Message{}, fmt.Errorf(
			"message content cannot be empty",
		)
	}

	if err := ctx.Err(); err != nil {
		return core.Message{}, err
	}

	discordMessage, err := c.session.ChannelMessageSend(
		channelID,
		content,
	)
	if err != nil {
		return core.Message{}, fmt.Errorf(
			"send discord message: %w",
			err,
		)
	}

	if discordMessage == nil || discordMessage.Author == nil {
		return core.Message{}, fmt.Errorf(
			"discord returned an invalid message",
		)
	}

	return core.Message{
		ID:        discordMessage.ID,
		GuildID:   discordMessage.GuildID,
		ChannelID: discordMessage.ChannelID,
		UserID:    discordMessage.Author.ID,
		Content:   discordMessage.Content,
		CreatedAt: discordMessage.Timestamp,
		IsBot:     discordMessage.Author.Bot,
	}, nil
}
