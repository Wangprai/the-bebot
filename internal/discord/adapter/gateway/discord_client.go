package gateway

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"unicode"

	"github.com/bwmarrin/discordgo"

	"the-bebot/internal/conversation/core"
)

const maxDiscordMessageRunes = 2000

type DiscordClient struct {
	session *discordgo.Session

	mu        sync.RWMutex
	botUserID string
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

// SetBotUserID sets a fallback bot user ID used if session.State.User is unavailable.
func (c *DiscordClient) SetBotUserID(userID string) {
	if c == nil {
		return
	}

	c.mu.Lock()
	c.botUserID = strings.TrimSpace(userID)
	c.mu.Unlock()
}

// BotUserID returns the bot's own Discord user ID from session state when
// available, falling back safely to the explicitly configured ID.
func (c *DiscordClient) BotUserID() string {
	if c == nil {
		return ""
	}

	if c.session != nil && c.session.State != nil {
		c.session.State.RLock()
		var stateUserID string
		if c.session.State.User != nil {
			stateUserID = c.session.State.User.ID
		}
		c.session.State.RUnlock()

		if stateUserID != "" {
			return stateUserID
		}
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.botUserID
}

// Compile-time interface checks.
var (
	_ core.ContextProvider = (*DiscordClient)(nil)
	_ core.MessageSender   = (*DiscordClient)(nil)
	_ core.TypingIndicator = (*DiscordClient)(nil)
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
	botUserID := c.BotUserID()

	for _, discordMessage := range discordMessages {
		if discordMessage == nil || discordMessage.Author == nil {
			continue
		}

		// Only accept messages belonging to the requested scope.
		// Discord REST ChannelMessages omits guild_id on returned messages,
		// so accept empty GuildID and assign the known guildID while still
		// rejecting any non-empty mismatched guild ID.
		if (discordMessage.GuildID != "" && discordMessage.GuildID != guildID) ||
			discordMessage.ChannelID != channelID {
			continue
		}

		// MVP conversation context supports text messages.
		if strings.TrimSpace(discordMessage.Content) == "" {
			continue
		}

		messageGuildID := discordMessage.GuildID
		if messageGuildID == "" {
			messageGuildID = guildID
		}

		authorName := resolveDiscordAuthorName(
			c.session,
			messageGuildID,
			discordMessage.Author,
			discordMessage.Member,
		)

		content := discordMessage.Content
		if mentionNames := extractMentionNames(c.session, messageGuildID, discordMessage.Mentions); len(mentionNames) > 0 {
			content = replaceMentionTokens(content, mentionNames)
		}

		messages = append(messages, core.Message{
			ID:         discordMessage.ID,
			GuildID:    messageGuildID,
			ChannelID:  discordMessage.ChannelID,
			UserID:     discordMessage.Author.ID,
			AuthorName: authorName,
			Content:    content,
			CreatedAt:  discordMessage.Timestamp,
			IsBot:      discordMessage.Author.Bot,
			IsSelf:     botUserID != "" && discordMessage.Author.ID == botUserID,
		})
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return messages, nil
}

// StartTyping broadcasts a typing indicator to the specified Discord channel.
func (c *DiscordClient) StartTyping(
	ctx context.Context,
	channelID string,
) error {
	if channelID == "" {
		return fmt.Errorf("channel ID is required")
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if err := c.session.ChannelTyping(
		channelID,
		discordgo.WithContext(ctx),
	); err != nil {
		return fmt.Errorf("send discord typing indicator: %w", err)
	}

	return nil
}

// Send sends a message to the specified Discord channel, splitting long
// responses to respect Discord's 2000-character limit and disabling mention
// parsing so LLM output can never ping users, roles, or @everyone/@here.
func (c *DiscordClient) Send(
	ctx context.Context,
	guildID string,
	channelID string,
	replyToMessageID string,
	content string,
) (core.Message, error) {
	if guildID == "" || channelID == "" {
		return core.Message{}, fmt.Errorf(
			"guild ID and channel ID are required",
		)
	}

	chunks := splitDiscordMessage(content, maxDiscordMessageRunes)
	if len(chunks) == 0 {
		return core.Message{}, fmt.Errorf(
			"message content cannot be empty",
		)
	}

	if err := ctx.Err(); err != nil {
		return core.Message{}, err
	}

	var firstSent core.Message

	for i, chunk := range chunks {
		if err := ctx.Err(); err != nil {
			return core.Message{}, err
		}

		msgSend := &discordgo.MessageSend{
			Content: chunk,
			AllowedMentions: &discordgo.MessageAllowedMentions{
				Parse: []discordgo.AllowedMentionType{},
			},
		}

		if i == 0 && strings.TrimSpace(replyToMessageID) != "" {
			failIfNotExists := false
			msgSend.Reference = &discordgo.MessageReference{
				MessageID:       replyToMessageID,
				ChannelID:       channelID,
				GuildID:         guildID,
				FailIfNotExists: &failIfNotExists,
			}
		}

		discordMessage, err := c.session.ChannelMessageSendComplex(
			channelID,
			msgSend,
			discordgo.WithContext(ctx),
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

		if i == 0 {
			messageGuildID := discordMessage.GuildID
			if messageGuildID == "" {
				messageGuildID = guildID
			}

			messageChannelID := discordMessage.ChannelID
			if messageChannelID == "" {
				messageChannelID = channelID
			}

			firstSent = core.Message{
				ID:        discordMessage.ID,
				GuildID:   messageGuildID,
				ChannelID: messageChannelID,
				UserID:    discordMessage.Author.ID,
				AuthorName: resolveDiscordAuthorName(
					c.session,
					messageGuildID,
					discordMessage.Author,
					discordMessage.Member,
				),
				Content:   discordMessage.Content,
				CreatedAt: discordMessage.Timestamp,
				IsBot:     discordMessage.Author.Bot,
				IsSelf:    true,
			}
		}
	}

	return firstSent, nil
}

func splitDiscordMessage(content string, maxRunes int) []string {
	if maxRunes <= 0 {
		maxRunes = maxDiscordMessageRunes
	}

	remaining := strings.TrimSpace(content)
	if remaining == "" {
		return nil
	}

	runes := []rune(remaining)
	if len(runes) <= maxRunes {
		return []string{remaining}
	}

	var chunks []string

	for len(runes) > 0 {
		if len(runes) <= maxRunes {
			chunk := strings.TrimSpace(string(runes))
			if chunk != "" {
				chunks = append(chunks, chunk)
			}
			break
		}

		cut, skip := findSplitPoint(runes, maxRunes)
		chunk := strings.TrimRightFunc(string(runes[:cut]), unicode.IsSpace)
		if chunk != "" {
			chunks = append(chunks, chunk)
		}

		nextStart := cut + skip
		if nextStart <= 0 {
			nextStart = maxRunes
		}
		if nextStart > len(runes) {
			nextStart = len(runes)
		}

		runes = runes[nextStart:]
		if skip > 0 {
			for len(runes) > 0 && unicode.IsSpace(runes[0]) {
				runes = runes[1:]
			}
		}
	}

	return chunks
}

func findSplitPoint(runes []rune, maxRunes int) (int, int) {
	// 1. Paragraph boundary (\n\n or \r\n\r\n).
	for i := maxRunes; i >= 1; i-- {
		if runes[i] == '\n' && i+1 < len(runes) && runes[i+1] == '\n' && hasNonSpace(runes[:i]) {
			return i, 2
		}
		if runes[i] == '\r' && i+3 < len(runes) &&
			runes[i+1] == '\n' && runes[i+2] == '\r' && runes[i+3] == '\n' &&
			hasNonSpace(runes[:i]) {
			return i, 4
		}
	}

	// 2. Line boundary (\n).
	for i := maxRunes; i >= 1; i-- {
		if runes[i] == '\n' && hasNonSpace(runes[:i]) {
			return i, 1
		}
	}

	// 3. Word boundary (any whitespace).
	for i := maxRunes; i >= 1; i-- {
		if unicode.IsSpace(runes[i]) && hasNonSpace(runes[:i]) {
			return i, 1
		}
	}

	// 4. Hard rune cut, backing up past combining marks (e.g. Thai vowels/tones).
	cut := maxRunes
	if cut < len(runes) && unicode.Is(unicode.Mn, runes[cut]) {
		adjusted := cut
		for adjusted > 0 && unicode.Is(unicode.Mn, runes[adjusted]) {
			adjusted--
		}
		if adjusted > 0 {
			cut = adjusted
		}
	}

	return cut, 0
}

func hasNonSpace(runes []rune) bool {
	for _, r := range runes {
		if !unicode.IsSpace(r) {
			return true
		}
	}
	return false
}
