package gateway

import (
	"fmt"

	"github.com/bwmarrin/discordgo"

	"the-bebot/internal/conversation/core"
)

func isBotMentioned(
	message *discordgo.Message,
	botUserID string,
) bool {
	if message == nil || botUserID == "" {
		return false
	}

	for _, user := range message.Mentions {
		if user != nil && user.ID == botUserID {
			return true
		}
	}

	return false
}

func toMentionEvent(
	message *discordgo.Message,
) (core.MentionEvent, error) {
	if message == nil {
		return core.MentionEvent{}, fmt.Errorf(
			"discord message is nil",
		)
	}

	if message.Author == nil {
		return core.MentionEvent{}, fmt.Errorf(
			"discord message author is nil",
		)
	}

	event := core.MentionEvent{
		GuildID:     message.GuildID,
		ChannelID:   message.ChannelID,
		MessageID:   message.ID,
		UserID:      message.Author.ID,
		Content:     message.Content,
		MentionedAt: message.Timestamp,
	}

	if err := event.Validate(); err != nil {
		return core.MentionEvent{}, fmt.Errorf(
			"invalid mention event: %w",
			err,
		)
	}

	return event, nil
}
