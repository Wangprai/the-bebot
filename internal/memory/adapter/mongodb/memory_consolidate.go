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

	"the-bebot/internal/memory/core"
)

// ConsolidationReport summarizes a near-duplicate memory consolidation pass.
type ConsolidationReport struct {
	Scanned      int
	GroupsMerged int
	MergedCount  int
	Threshold    float64
	DryRun       bool
}

// ConsolidateNearDuplicates scans memories (for one guildID, or all guilds when empty)
// and merges near-duplicate memories within the same (guild_id, scope, type, user_id)
// bucket whose NearDuplicateSimilarity is at least threshold.
// Merged metadata retains max importance/confidence/updated_at, min created_at,
// summed usage_count, max last_used_at, and unioned tags and source_user_ids.
func (r *MemoryRepository) ConsolidateNearDuplicates(
	ctx context.Context,
	guildID string,
	threshold float64,
	dryRun bool,
) (ConsolidationReport, error) {
	if r == nil || r.collection == nil {
		return ConsolidationReport{}, errors.New("memory repository is not initialized")
	}

	if ctx == nil {
		return ConsolidationReport{}, errors.New("context is required")
	}

	if threshold <= 0 {
		threshold = core.DefaultNearDuplicateThreshold
	}
	if threshold > 1 {
		return ConsolidationReport{}, errors.New("similarity threshold must be between 0 and 1")
	}

	report := ConsolidationReport{
		Threshold: threshold,
		DryRun:    dryRun,
	}

	filter := bson.M{}
	if trimmed := strings.TrimSpace(guildID); trimmed != "" {
		filter["guild_id"] = trimmed
	}

	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		return report, fmt.Errorf("scan memories for consolidation: %w", err)
	}
	defer cursor.Close(ctx)

	type bucketItem struct {
		doc    memoryDocument
		domain core.Memory
	}

	buckets := make(map[string][]bucketItem)
	var bucketKeys []string

	for cursor.Next(ctx) {
		var doc memoryDocument
		if err := cursor.Decode(&doc); err != nil {
			return report, fmt.Errorf("decode memory document: %w", err)
		}

		domain, err := doc.toDomain()
		if err != nil {
			continue
		}

		report.Scanned++
		userKey := ""
		if domain.Scope == core.MemoryScopeMember {
			userKey = domain.UserID
		}
		bKey := strings.Join([]string{domain.GuildID, string(domain.Scope), string(domain.Type), userKey}, "\x00")
		if _, exists := buckets[bKey]; !exists {
			bucketKeys = append(bucketKeys, bKey)
		}
		buckets[bKey] = append(buckets[bKey], bucketItem{doc: doc, domain: domain})
	}

	if err := cursor.Err(); err != nil {
		return report, fmt.Errorf("iterate memories for consolidation: %w", err)
	}

	sort.Strings(bucketKeys)

	for _, bKey := range bucketKeys {
		items := buckets[bKey]
		if len(items) < 2 {
			continue
		}

		// Sort deterministically so survivor choice is stable across runs:
		// highest importance, highest usage_count, longest content, oldest created_at, smallest ID.
		sort.SliceStable(items, func(i, j int) bool {
			if items[i].doc.Importance != items[j].doc.Importance {
				return items[i].doc.Importance > items[j].doc.Importance
			}
			if items[i].doc.UsageCount != items[j].doc.UsageCount {
				return items[i].doc.UsageCount > items[j].doc.UsageCount
			}
			if len(items[i].doc.Content) != len(items[j].doc.Content) {
				return len(items[i].doc.Content) > len(items[j].doc.Content)
			}
			if !items[i].doc.CreatedAt.Equal(items[j].doc.CreatedAt) {
				return items[i].doc.CreatedAt.Before(items[j].doc.CreatedAt)
			}
			return items[i].doc.ID.Hex() < items[j].doc.ID.Hex()
		})

		consumed := make([]bool, len(items))
		for i := 0; i < len(items); i++ {
			if consumed[i] {
				continue
			}

			survivor := items[i].doc
			var losers []memoryDocument

			for j := i + 1; j < len(items); j++ {
				if consumed[j] {
					continue
				}
				if core.NearDuplicateSimilarity(items[i].domain, items[j].domain) >= threshold {
					consumed[j] = true
					losers = append(losers, items[j].doc)
				}
			}

			if len(losers) == 0 {
				continue
			}

			report.GroupsMerged++
			report.MergedCount += len(losers)

			if dryRun {
				continue
			}

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

			if _, err := r.collection.DeleteMany(ctx, bson.M{
				"_id": bson.M{"$in": loserIDs},
			}); err != nil {
				return report, fmt.Errorf("delete consolidated memories: %w", err)
			}

			setFields := bson.M{
				"importance":        mergedImportance,
				"confidence":        mergedConfidence,
				"usage_count":       mergedUsage,
				"created_at":        mergedCreatedAt,
				"updated_at":        mergedUpdatedAt,
				"last_used_at":      mergedLastUsedAt,
				"tags":              mergedTags,
				"content_dedup_key": newMemoryDocumentContentDedupKey(survivor),
			}
			if len(mergedSourceUsers) > 0 {
				setFields["source_user_ids"] = mergedSourceUsers
			}

			update := bson.M{"$set": setFields}
			if mergedUsage > 0 || (r.unusedTTL > 0 && mergedImportance > r.unusedTTLMaxImportance) {
				update["$unset"] = bson.M{"expires_at": ""}
			}

			if _, err := r.collection.UpdateOne(
				ctx,
				bson.M{"_id": survivor.ID},
				update,
			); err != nil {
				return report, fmt.Errorf("update consolidated memory %s: %w", survivor.ID.Hex(), err)
			}
		}
	}

	return report, nil
}
