package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/bwmarrin/discordgo"
)

type Gateway struct {
	session *discordgo.Session
	logger  *slog.Logger

	mentionHandler MentionHandler

	mu        sync.RWMutex
	botUserID string
}

func NewGateway(
	token string,
	logger *slog.Logger,
	mentionHandler MentionHandler,
) (*Gateway, error) {
	if token == "" {
		return nil, fmt.Errorf("discord bot token is required")
	}

	if mentionHandler == nil {
		return nil, fmt.Errorf("mention handler is required")
	}

	session, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("create discord session: %w", err)
	}

	session.Identify.Intents =
		discordgo.IntentsGuilds |
			discordgo.IntentsGuildMessages |
			discordgo.IntentsMessageContent

	gateway := &Gateway{
		session:        session,
		logger:         logger,
		mentionHandler: mentionHandler,
	}

	session.AddHandler(gateway.handleReady)
	session.AddHandler(gateway.handleMessageCreate)

	return gateway, nil
}

func (g *Gateway) Start() error {
	if err := g.session.Open(); err != nil {
		return fmt.Errorf("open discord gateway: %w", err)
	}

	g.logger.Info("Discord gateway connected")

	return nil
}

func (g *Gateway) Close() error {
	if g.session == nil {
		return nil
	}

	if err := g.session.Close(); err != nil {
		return fmt.Errorf("close discord gateway: %w", err)
	}

	g.logger.Info("Discord gateway disconnected")

	return nil
}

func (g *Gateway) Session() *discordgo.Session {
	return g.session
}

func (g *Gateway) handleReady(
	s *discordgo.Session,
	event *discordgo.Ready,
) {
	if event.User == nil {
		g.logger.Error("Discord ready event has no bot user")
		return
	}

	g.mu.Lock()
	g.botUserID = event.User.ID
	g.mu.Unlock()

	g.logger.Info(
		"Discord bot is ready",
		"username", event.User.Username,
		"bot_user_id", event.User.ID,
	)
}

func (g *Gateway) handleMessageCreate(
	s *discordgo.Session,
	event *discordgo.MessageCreate,
) {
	if event == nil || event.Message == nil {
		return
	}

	message := event.Message

	// Ignore Direct Messages in MVP.
	if message.GuildID == "" {
		return
	}

	if message.Author == nil {
		return
	}

	g.mu.RLock()
	botUserID := g.botUserID
	g.mu.RUnlock()

	if botUserID == "" {
		g.logger.Warn("Ignoring message before bot identity is ready")
		return
	}

	// Ignore messages sent by The Bebot itself.
	if message.Author.ID == botUserID {
		return
	}

	if !isBotMentioned(message, botUserID) {
		return
	}

	mentionEvent, err := toMentionEvent(message)
	if err != nil {
		g.logger.Error(
			"Failed to convert Discord message",
			"error", err,
		)
		return
	}

	g.logger.Info(
		"Bot mentioned",
		"guild_id", mentionEvent.GuildID,
		"channel_id", mentionEvent.ChannelID,
		"message_id", mentionEvent.MessageID,
		"user_id", mentionEvent.UserID,
	)

	if err := g.mentionHandler.HandleMention(
		context.Background(),
		mentionEvent,
	); err != nil {
		g.logger.Error(
			"Failed to handle mention",
			"message_id", mentionEvent.MessageID,
			"error", err,
		)
	}
}
