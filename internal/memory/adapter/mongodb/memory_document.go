package mongodb

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"

	"the-bebot/internal/memory/core"
)

type memoryDocument struct {
	ID              primitive.ObjectID `bson:"_id,omitempty"`
	GuildID         string             `bson:"guild_id"`
	UserID          string             `bson:"user_id,omitempty"`
	Scope           string             `bson:"scope"`
	Type            string             `bson:"type"`
	Content         string             `bson:"content"`
	Importance      int                `bson:"importance"`
	Confidence      float64            `bson:"confidence"`
	Tags            []string           `bson:"tags"`
	SourceMessageID string             `bson:"source_message_id"`
	UsageCount      int                `bson:"usage_count"`
	LastUsedAt      *time.Time         `bson:"last_used_at"`
	CreatedAt       time.Time          `bson:"created_at"`
	UpdatedAt       time.Time          `bson:"updated_at"`
}

func toMemoryDocument(memory core.Memory) memoryDocument {
	document := memoryDocument{
		GuildID:         memory.GuildID,
		UserID:          memory.UserID,
		Scope:           string(memory.Scope),
		Type:            string(memory.Type),
		Content:         memory.Content,
		Importance:      memory.Importance,
		Confidence:      memory.Confidence,
		Tags:            memory.Tags,
		SourceMessageID: memory.SourceMessageID,
		UsageCount:      memory.UsageCount,
		LastUsedAt:      memory.LastUsedAt,
		CreatedAt:       memory.CreatedAt,
		UpdatedAt:       memory.UpdatedAt,
	}

	if memory.ID != "" {
		if objectID, err := primitive.ObjectIDFromHex(memory.ID); err == nil {
			document.ID = objectID
		}
	}

	return document
}

func (d memoryDocument) toDomain() (core.Memory, error) {
	return core.NewMemory(core.Memory{
		ID:              d.ID.Hex(),
		GuildID:         d.GuildID,
		UserID:          d.UserID,
		Scope:           core.MemoryScope(d.Scope),
		Type:            core.MemoryType(d.Type),
		Content:         d.Content,
		Importance:      d.Importance,
		Confidence:      d.Confidence,
		Tags:            d.Tags,
		SourceMessageID: d.SourceMessageID,
		UsageCount:      d.UsageCount,
		LastUsedAt:      d.LastUsedAt,
		CreatedAt:       d.CreatedAt,
		UpdatedAt:       d.UpdatedAt,
	})
}
