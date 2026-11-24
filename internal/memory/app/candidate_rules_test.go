package app

import (
	"testing"

	"the-bebot/internal/memory/core"
)

func newTestSnapshot(messages ...core.ConversationMessage) core.ConversationSnapshot {
	return core.ConversationSnapshot{
		GuildID:          "guild-001",
		ChannelID:        "channel-001",
		TriggerMessageID: "message-trigger",
		Messages: append(messages, core.ConversationMessage{
			MessageID: "message-trigger",
			UserID:    "user-001",
			Content:   "@TheBebot",
		}),
	}
}

func TestCandidateRulesShouldCreateCandidate(t *testing.T) {
	rules := NewCandidateRules()

	tests := []struct {
		name     string
		messages []core.ConversationMessage
		expected bool
	}{
		{
			name: "Thai preference",
			messages: []core.ConversationMessage{
				{
					MessageID: "message-001",
					UserID:    "user-001",
					Content:   "ผมชอบเขียนโปรแกรมด้วยภาษา Go มากกว่า Java",
				},
			},
			expected: true,
		},
		{
			name: "Thai personal fact",
			messages: []core.ConversationMessage{
				{
					MessageID: "message-001",
					UserID:    "user-001",
					Content:   "ผมทำงานเป็น Software Engineer อยู่ครับ",
				},
			},
			expected: true,
		},
		{
			name: "Thai promise",
			messages: []core.ConversationMessage{
				{
					MessageID: "message-001",
					UserID:    "user-001",
					Content:   "พรุ่งนี้จะเอาโปรเจกต์มาให้ดูนะ",
				},
			},
			expected: true,
		},
		{
			name: "English preference",
			messages: []core.ConversationMessage{
				{
					MessageID: "message-001",
					UserID:    "user-001",
					Content:   "I like programming with Go",
				},
			},
			expected: true,
		},
		{
			name: "No memory signal",
			messages: []core.ConversationMessage{
				{
					MessageID: "message-001",
					UserID:    "user-001",
					Content:   "วันนี้กินข้าวหรือยัง ไปกินด้วยกันไหม",
				},
			},
			expected: false,
		},
		{
			name: "Content too short",
			messages: []core.ConversationMessage{
				{
					MessageID: "message-001",
					UserID:    "user-001",
					Content:   "ชอบ Go",
				},
			},
			expected: false,
		},
		{
			name: "Bot message should be ignored",
			messages: []core.ConversationMessage{
				{
					MessageID: "message-001",
					UserID:    "bot-001",
					Content:   "ฉันชอบเขียนโปรแกรมด้วยภาษา Go",
					IsBot:     true,
				},
			},
			expected: false,
		},
		{
			name: "Another user has memory signal should be ignored",
			messages: []core.ConversationMessage{
				{
					MessageID: "message-001",
					UserID:    "user-002",
					Content:   "ฉันชอบเขียนโปรแกรมด้วยภาษา Go",
				},
			},
			expected: false,
		},
		{
			name: "English word boundary prevents Miami matching i am",
			messages: []core.ConversationMessage{
				{
					MessageID: "message-001",
					UserID:    "user-001",
					Content:   "Miami is a warm city to visit next year",
				},
			},
			expected: false,
		},
		{
			name: "English word boundary prevents hi william matching i will",
			messages: []core.ConversationMessage{
				{
					MessageID: "message-001",
					UserID:    "user-001",
					Content:   "Hi William, how is the project going?",
				},
			},
			expected: false,
		},
		{
			name: "Mention tokens are stripped before length and signal checks",
			messages: []core.ConversationMessage{
				{
					MessageID: "message-001",
					UserID:    "user-001",
					Content:   "<@123456789012345678> ชอบ Go",
				},
			},
			expected: false,
		},
		{
			name: "Valid signal still matches after stripping mention token",
			messages: []core.ConversationMessage{
				{
					MessageID: "message-001",
					UserID:    "user-001",
					Content:   "<@!123456789012345678> I'm building a Discord bot in Go",
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := newTestSnapshot(tt.messages...)

			result, err := rules.ShouldCreateCandidate(snapshot)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestCandidateRulesRejectsInvalidSnapshot(t *testing.T) {
	rules := NewCandidateRules()

	snapshot := core.ConversationSnapshot{
		GuildID:   "",
		ChannelID: "channel-001",
		Messages: []core.ConversationMessage{
			{
				MessageID: "message-001",
				UserID:    "user-001",
				Content:   "ผมชอบเขียนโปรแกรมด้วยภาษา Go",
			},
		},
	}

	_, err := rules.ShouldCreateCandidate(snapshot)

	if err == nil {
		t.Fatal("expected error for invalid snapshot")
	}
}

func TestCandidateRulesRejectsEmptySnapshot(t *testing.T) {
	rules := NewCandidateRules()

	snapshot := core.ConversationSnapshot{
		GuildID:          "guild-001",
		ChannelID:        "channel-001",
		TriggerMessageID: "message-trigger",
		Messages:         []core.ConversationMessage{},
	}

	_, err := rules.ShouldCreateCandidate(snapshot)

	if err == nil {
		t.Fatal("expected error for empty snapshot")
	}
}
