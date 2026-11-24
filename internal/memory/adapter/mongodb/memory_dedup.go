package mongodb

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// DeduplicationReport summarizes a one-off memory deduplication pass.
type DeduplicationReport struct {
	Scanned           int
	GroupsMerged      int
	DuplicatesDeleted int
	Backfilled        int
	DryRun            bool
}

// DeduplicateMemories scans memories (for one guildID, or all guilds when guildID
// is empty), merges duplicate records sharing the same normalized content dedup key
// into a deterministic survivor (keeping max importance/confidence/updated_at, min
// created_at, summed usage_count, and unioned tags), and backfills content_dedup_key.
// It is idempotent and intended for manual one-off operator cleanup, never run
// automatically at startup.
func (r *MemoryRepository) DeduplicateMemories(
	ctx context.Context,
	guildID string,
	dryRun bool,
) (DeduplicationReport, error) {
	if r == nil || r.collection == nil {
		return DeduplicationReport{}, errors.New("memory repository is not initialized")
	}

	if ctx == nil {
		return DeduplicationReport{}, errors.New("context is required")
	}

	report := DeduplicationReport{DryRun: dryRun}

	filter := bson.M{}
	if trimmed := strings.TrimSpace(guildID); trimmed != "" {
		filter["guild_id"] = trimmed
	}

	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		return report, fmt.Errorf("scan memories for deduplication: %w", err)
	}
	defer cursor.Close(ctx)

	groups := make(map[string][]memoryDocument)
	var groupOrder []string

	for cursor.Next(ctx) {
		var doc memoryDocument
		if err := cursor.Decode(&doc); err != nil {
			return report, fmt.Errorf("decode memory document: %w", err)
		}

		report.Scanned++
		dedupKey := newMemoryDocumentContentDedupKey(doc)
		groupKey := doc.GuildID + "\x00" + dedupKey
		if _, exists := groups[groupKey]; !exists {
			groupOrder = append(groupOrder, groupKey)
		}
		groups[groupKey] = append(groups[groupKey], doc)
	}

	if err := cursor.Err(); err != nil {
		return report, fmt.Errorf("iterate memories for deduplication: %w", err)
	}

	sort.Strings(groupOrder)

	for _, groupKey := range groupOrder {
		docs := groups[groupKey]
		if len(docs) == 0 {
			continue
		}

		dedupKey := newMemoryDocumentContentDedupKey(docs[0])

		if len(docs) == 1 {
			if docs[0].ContentDedupKey != dedupKey {
				report.Backfilled++
				if !dryRun {
					if _, err := r.collection.UpdateOne(
						ctx,
						bson.M{"_id": docs[0].ID},
						bson.M{"$set": bson.M{"content_dedup_key": dedupKey}},
					); err != nil {
						return report, fmt.Errorf("backfill content_dedup_key for %s: %w", docs[0].ID.Hex(), err)
					}
				}
			}
			continue
		}

		// Pick survivor deterministically: highest importance, highest usage_count,
		// oldest created_at, then lexicographically smallest ObjectID.
		sort.SliceStable(docs, func(i, j int) bool {
			if docs[i].Importance != docs[j].Importance {
				return docs[i].Importance > docs[j].Importance
			}
			if docs[i].UsageCount != docs[j].UsageCount {
				return docs[i].UsageCount > docs[j].UsageCount
			}
			if !docs[i].CreatedAt.Equal(docs[j].CreatedAt) {
				return docs[i].CreatedAt.Before(docs[j].CreatedAt)
			}
			return docs[i].ID.Hex() < docs[j].ID.Hex()
		})

		survivor := docs[0]
		losers := docs[1:]

		mergedImportance := survivor.Importance
		mergedConfidence := survivor.Confidence
		mergedUsage := survivor.UsageCount
		mergedCreatedAt := survivor.CreatedAt
		mergedUpdatedAt := survivor.UpdatedAt
		var mergedLastUsedAt *time.Time
		if survivor.LastUsedAt != nil {
			ts := *survivor.LastUsedAt
			mergedLastUsedAt = &ts
		}

		tagSet := make(map[string]struct{})
		for _, tag := range survivor.Tags {
			if t := strings.ToLower(strings.TrimSpace(tag)); t != "" {
				tagSet[t] = struct{}{}
			}
		}

		sourceUserSet := make(map[string]struct{})
		for _, uid := range survivor.SourceUserIDs {
			if u := strings.TrimSpace(uid); u != "" {
				sourceUserSet[u] = struct{}{}
			}
		}

		loserIDs := make([]primitive.ObjectID, 0, len(losers))
		for _, loser := range losers {
			loserIDs = append(loserIDs, loser.ID)

			if loser.Importance > mergedImportance {
				mergedImportance = loser.Importance
			}
			if loser.Confidence > mergedConfidence {
				mergedConfidence = loser.Confidence
			}
			mergedUsage += loser.UsageCount
			if !loser.CreatedAt.IsZero() && (mergedCreatedAt.IsZero() || loser.CreatedAt.Before(mergedCreatedAt)) {
				mergedCreatedAt = loser.CreatedAt
			}
			if loser.UpdatedAt.After(mergedUpdatedAt) {
				mergedUpdatedAt = loser.UpdatedAt
			}
			if loser.LastUsedAt != nil {
				if mergedLastUsedAt == nil || loser.LastUsedAt.After(*mergedLastUsedAt) {
					ts := *loser.LastUsedAt
					mergedLastUsedAt = &ts
				}
			}
			for _, tag := range loser.Tags {
				if t := strings.ToLower(strings.TrimSpace(tag)); t != "" {
					tagSet[t] = struct{}{}
				}
			}
			for _, uid := range loser.SourceUserIDs {
				if u := strings.TrimSpace(uid); u != "" {
					sourceUserSet[u] = struct{}{}
				}
			}
		}

		mergedTags := make([]string, 0, len(tagSet))
		for tag := range tagSet {
			mergedTags = append(mergedTags, tag)
		}
		sort.Strings(mergedTags)

		mergedSourceUsers := make([]string, 0, len(sourceUserSet))
		for uid := range sourceUserSet {
			mergedSourceUsers = append(mergedSourceUsers, uid)
		}
		sort.Strings(mergedSourceUsers)

		report.GroupsMerged++
		report.DuplicatesDeleted += len(loserIDs)

		if dryRun {
			continue
		}

		// Delete duplicates before setting content_dedup_key on the survivor to avoid
		// unique index collisions if any loser already carried content_dedup_key.
		if _, err := r.collection.DeleteMany(ctx, bson.M{
			"_id": bson.M{"$in": loserIDs},
		}); err != nil {
			return report, fmt.Errorf("delete duplicate memories: %w", err)
		}

		setFields := bson.M{
			"importance":        mergedImportance,
			"confidence":        mergedConfidence,
			"usage_count":       mergedUsage,
			"created_at":        mergedCreatedAt,
			"updated_at":        mergedUpdatedAt,
			"last_used_at":      mergedLastUsedAt,
			"tags":              mergedTags,
			"content_dedup_key": dedupKey,
		}
		if len(mergedSourceUsers) > 0 {
			setFields["source_user_ids"] = mergedSourceUsers
		}

		if _, err := r.collection.UpdateOne(
			ctx,
			bson.M{"_id": survivor.ID},
			bson.M{"$set": setFields},
		); err != nil {
			return report, fmt.Errorf("update surviving memory %s: %w", survivor.ID.Hex(), err)
		}
	}

	return report, nil
}
