package gateway

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/bwmarrin/discordgo"

	"the-bebot/internal/conversation/core"
)

var discordMentionTokenPattern = regexp.MustCompile(`<@!?([0-9A-Za-z_-]+)>`)

func resolveDiscordAuthorName(
	session *discordgo.Session,
	guildID string,
	author *discordgo.User,
	member *discordgo.Member,
) string {
	if member != nil {
		if nick := strings.TrimSpace(member.Nick); nick != "" {
			return nick
		}
	}

	if author != nil && guildID != "" && session != nil && session.State != nil {
		if stateMember, err := session.State.Member(guildID, author.ID); err == nil && stateMember != nil {
			if nick := strings.TrimSpace(stateMember.Nick); nick != "" {
				return nick
			}
		}
	}

	if author == nil {
		return ""
	}

	if globalName := strings.TrimSpace(author.GlobalName); globalName != "" {
		return globalName
	}

	return strings.TrimSpace(author.Username)
}

func extractMentionNames(
	session *discordgo.Session,
	guildID string,
	mentions []*discordgo.User,
) map[string]string {
	if len(mentions) == 0 {
		return nil
	}

	names := make(map[string]string, len(mentions))
	for _, user := range mentions {
		if user == nil || user.ID == "" {
			continue
		}
		if name := resolveDiscordAuthorName(session, guildID, user, nil); name != "" {
			names[user.ID] = name
		}
	}

	if len(names) == 0 {
		return nil
	}

	return names
}

func replaceMentionTokens(content string, names map[string]string) string {
	if len(names) == 0 || !strings.Contains(content, "<@") {
		return content
	}

	return discordMentionTokenPattern.ReplaceAllStringFunc(content, func(token string) string {
		matches := discordMentionTokenPattern.FindStringSubmatch(token)
		if len(matches) < 2 {
			return token
		}
		if name := strings.TrimSpace(names[matches[1]]); name != "" {
			return "@" + name
		}
		return token
	})
}

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

	displayName := strings.TrimSpace(message.Author.GlobalName)
	if displayName == "" {
		displayName = strings.TrimSpace(message.Author.Username)
	}

	var nickname string
	if message.Member != nil {
		nickname = strings.TrimSpace(message.Member.Nick)
	}

	event := core.MentionEvent{
		GuildID:         message.GuildID,
		ChannelID:       message.ChannelID,
		MessageID:       message.ID,
		UserID:          message.Author.ID,
		UserDisplayName: displayName,
		UserNickname:    nickname,
		MentionNames:    extractMentionNames(nil, message.GuildID, message.Mentions),
		Content:         message.Content,
		MentionedAt:     message.Timestamp,
	}

	if err := event.Validate(); err != nil {
		return core.MentionEvent{}, fmt.Errorf(
			"invalid mention event: %w",
			err,
		)
	}

	return event, nil
}
