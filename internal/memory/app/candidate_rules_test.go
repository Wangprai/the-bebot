package app

import (
	"testing"

	"the-bebot/internal/memory/core"
)

func newTriggerTestSnapshot(trigger core.ConversationMessage, contextMessages ...core.ConversationMessage) core.ConversationSnapshot {
	if trigger.MessageID == "" {
		trigger.MessageID = "message-trigger"
	}
	if trigger.UserID == "" {
		trigger.UserID = "user-001"
	}
	messages := append([]core.ConversationMessage(nil), contextMessages...)
	messages = append(messages, trigger)
	return core.ConversationSnapshot{
		GuildID:          "guild-001",
		ChannelID:        "channel-001",
		TriggerMessageID: trigger.MessageID,
		TargetUserID:     "user-001",
		Messages:         messages,
	}
}

func TestCandidateRulesShouldCreateCandidate(t *testing.T) {
	rules := NewCandidateRules()

	tests := []struct {
		name     string
		trigger  core.ConversationMessage
		context  []core.ConversationMessage
		expected bool
	}{
		{
			name: "Thai preference on trigger message",
			trigger: core.ConversationMessage{
				MessageID: "message-trigger",
				UserID:    "user-001",
				Content:   "ผมชอบเขียนโปรแกรมด้วยภาษา Go มากกว่า Java",
			},
			expected: true,
		},
		{
			name: "Thai personal fact on trigger message",
			trigger: core.ConversationMessage{
				MessageID: "message-trigger",
				UserID:    "user-001",
				Content:   "ผมทำงานเป็น Software Engineer อยู่ครับ",
			},
			expected: true,
		},
		{
			name: "Thai promise on trigger message",
			trigger: core.ConversationMessage{
				MessageID: "message-trigger",
				UserID:    "user-001",
				Content:   "พรุ่งนี้จะเอาโปรเจกต์มาให้ดูนะ",
			},
			expected: true,
		},
		{
			name: "English preference on trigger message",
			trigger: core.ConversationMessage{
				MessageID: "message-trigger",
				UserID:    "user-001",
				Content:   "I like programming with Go",
			},
			expected: true,
		},
		{
			name: "English curly apostrophe U+2019 matches I'm",
			trigger: core.ConversationMessage{
				MessageID: "message-trigger",
				UserID:    "user-001",
				Content:   "<@!123456789012345678> I\u2019m building a Discord bot in Go",
			},
			expected: true,
		},
		{
			name: "Older message by same user with signal must NOT trigger candidate",
			trigger: core.ConversationMessage{
				MessageID: "message-trigger",
				UserID:    "user-001",
				Content:   "<@123456789012345678> what do you think about dinner tonight?",
			},
			context: []core.ConversationMessage{
				{
					MessageID: "message-older",
					UserID:    "user-001",
					Content:   "I like programming with Go very much",
				},
			},
			expected: false,
		},
		{
			name: "No memory signal",
			trigger: core.ConversationMessage{
				MessageID: "message-trigger",
				UserID:    "user-001",
				Content:   "วันนี้กินข้าวหรือยัง ไปกินด้วยกันไหม",
			},
			expected: false,
		},
		{
			name: "Content too short",
			trigger: core.ConversationMessage{
				MessageID: "message-trigger",
				UserID:    "user-001",
				Content:   "ชอบ Go",
			},
			expected: false,
		},
		{
			name: "Bot trigger message should be ignored",
			trigger: core.ConversationMessage{
				MessageID: "message-trigger",
				UserID:    "bot-001",
				Content:   "ฉันชอบเขียนโปรแกรมด้วยภาษา Go",
				IsBot:     true,
			},
			expected: false,
		},
		{
			name: "Another user has memory signal in window should be ignored",
			trigger: core.ConversationMessage{
				MessageID: "message-trigger",
				UserID:    "user-001",
				Content:   "@TheBebot hello there!",
			},
			context: []core.ConversationMessage{
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
			trigger: core.ConversationMessage{
				MessageID: "message-trigger",
				UserID:    "user-001",
				Content:   "Miami is a warm city to visit next year",
			},
			expected: false,
		},
		{
			name: "English word boundary prevents hi william matching i will",
			trigger: core.ConversationMessage{
				MessageID: "message-trigger",
				UserID:    "user-001",
				Content:   "Hi William, how is the project going?",
			},
			expected: false,
		},
		{
			name: "Mention tokens are stripped before length and signal checks",
			trigger: core.ConversationMessage{
				MessageID: "message-trigger",
				UserID:    "user-001",
				Content:   "<@123456789012345678> ชอบ Go",
			},
			expected: false,
		},
		{
			name: "Valid signal still matches after stripping mention token",
			trigger: core.ConversationMessage{
				MessageID: "message-trigger",
				UserID:    "user-001",
				Content:   "<@!123456789012345678> I'm building a Discord bot in Go",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshot := newTriggerTestSnapshot(tt.trigger, tt.context...)

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
