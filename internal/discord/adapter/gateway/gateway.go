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

	ctx    context.Context
	cancel context.CancelFunc

	mu        sync.RWMutex
	botUserID string

	started bool
	closed  bool

	closeOnce sync.Once
	closeErr  error
}

func NewGateway(
	token string,
	logger *slog.Logger,
	mentionHandler MentionHandler,
) (*Gateway, error) {
	if token == "" {
		return nil, fmt.Errorf("discord bot token is required")
	}

	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	session, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("create discord session: %w", err)
	}

	session.Identify.Intents =
		discordgo.IntentsGuilds |
			discordgo.IntentsGuildMessages |
			discordgo.IntentsMessageContent

	g := &Gateway{
		session:        session,
		logger:         logger,
		mentionHandler: mentionHandler,
	}

	session.AddHandler(g.handleReady)
	session.AddHandler(g.handleMessageCreate)

	return g, nil
}

func (g *Gateway) SetMentionHandler(handler MentionHandler) error {
	if handler == nil {
		return fmt.Errorf("mention handler is required")
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	if g.started {
		return fmt.Errorf("cannot set mention handler after gateway started")
	}

	if g.closed {
		return fmt.Errorf("cannot set mention handler after gateway closed")
	}

	g.mentionHandler = handler

	return nil
}

func (g *Gateway) Start(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("gateway context is required")
	}

	g.mu.Lock()

	if g.started {
		g.mu.Unlock()
		return fmt.Errorf("discord gateway already started")
	}

	if g.closed {
		g.mu.Unlock()
		return fmt.Errorf("discord gateway is already closed")
	}

	if g.mentionHandler == nil {
		g.mu.Unlock()
		return fmt.Errorf("mention handler is required")
	}

	g.ctx, g.cancel = context.WithCancel(ctx)
	g.started = true

	g.mu.Unlock()

	if err := g.session.Open(); err != nil {
		g.mu.Lock()
		g.cancel()
		g.ctx = nil
		g.cancel = nil
		g.started = false
		g.mu.Unlock()

		return fmt.Errorf("open discord gateway: %w", err)
	}

	g.logger.Info("Discord gateway connected")

	return nil
}

func (g *Gateway) Close() error {
	g.closeOnce.Do(func() {
		g.mu.Lock()

		g.closed = true

		cancel := g.cancel
		g.cancel = nil
		g.ctx = nil

		g.mu.Unlock()

		if cancel != nil {
			cancel()
		}

		if g.session != nil {
			if err := g.session.Close(); err != nil {
				g.closeErr = fmt.Errorf(
					"close discord gateway: %w",
					err,
				)
				return
			}
		}

		g.logger.Info("Discord gateway disconnected")
	})

	return g.closeErr
}

func (g *Gateway) Session() *discordgo.Session {
	return g.session
}

func (g *Gateway) handleReady(
	s *discordgo.Session,
	event *discordgo.Ready,
) {
	if event == nil || event.User == nil {
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
	ctx := g.ctx
	handler := g.mentionHandler
	g.mu.RUnlock()

	// Ignore messages before the gateway is started.
	if ctx == nil {
		return
	}

	// Ignore messages while shutting down.
	if ctx.Err() != nil {
		return
	}

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

	if handler == nil {
		g.logger.Error("Mention handler is not configured")
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

	if err := handler.HandleMention(ctx, mentionEvent); err != nil {
		g.logger.Error(
			"Failed to handle mention",
			"message_id", mentionEvent.MessageID,
			"error", err,
		)
	}
}
