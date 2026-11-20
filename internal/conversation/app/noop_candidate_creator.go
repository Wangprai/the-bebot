package app

import (
	"context"

	"the-bebot/internal/conversation/core"
)

type NoopCandidateCreator struct{}

func (NoopCandidateCreator) CreateCandidate(
	ctx context.Context,
	conversationContext core.ConversationContext,
) error {
	return nil
}

var _ core.CandidateCreator = NoopCandidateCreator{}
