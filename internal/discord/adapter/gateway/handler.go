package gateway

import (
	"context"

	"the-bebot/internal/conversation/core"
)

type MentionHandler interface {
	HandleMention(
		ctx context.Context,
		event core.MentionEvent,
	) error
}
