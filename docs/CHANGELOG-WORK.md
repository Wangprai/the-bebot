# Engineering Work Changelog

This file records the work completed across each feature/fix branch, including commits, changes, verification executed, and known gaps.

---

## A1 — `fix/memory-dedup-and-trigger-only-rules`

- **Depends on:** `main`
- **Commits:**
  - `6476dc7` `fix(memory): evaluate only trigger message in candidate rules`
  - `f1851d2` `fix(memory): deduplicate memories by normalized content`
  - `(this commit)` `docs(memory): document content deduplication and changelog`
- **What changed:**
  - Restricted `CandidateRules.ShouldCreateCandidate` to evaluate only the trigger message while preserving earlier messages in the snapshot as extraction context only.
  - Normalized Unicode curly apostrophes (`U+2019`/`U+2018`) to ASCII `'` before matching English signal patterns (e.g., `I'm`) and when normalizing memory content for deduplication.
  - Added `content_dedup_key` (SHA-256 of `guild_id`, `scope`, `type`, member `user_id`, and normalized content) and migration-safe unique partial index `idx_memories_content_dedup`.
  - Updated `MemoryRepository.Save` to preserve candidate-level retry idempotency (`guild_id`, `candidate_id`, `candidate_index`) while deduplicating across distinct candidates by `content_dedup_key` and advancing `importance` and `updated_at` to their maximum values.
  - Added idempotent one-off cleanup helper `MemoryRepository.DeduplicateMemories` and manual CLI `cmd/dedup-memories` (defaults to `--apply=false` dry-run; never auto-run by `cmd/bot`).
  - Updated `README.md` with trigger-only rules, new index, and cleanup helper usage.
- **Tests run:**
  - `gofmt -l .` (clean)
  - `go vet ./...` (pass)
  - `go test ./... -count=1 -race` against real local MongoDB on `127.0.0.1:27017` (all packages pass, including new unit tests for `CandidateRules` and `newMemoryContentDedupKey`, and MongoDB integration tests `TestMemoryRepositoryContentDedupAcrossDistinctCandidates` and `TestMemoryRepositoryDeduplicateMemoriesCleanupHelper`)
  - `go build` of `./cmd/bot` and `./cmd/dedup-memories` (pass)
- **Known gaps:**
  - Near-duplicate / semantic-rephrase merging (beyond whitespace/case/apostrophe normalization) is addressed separately in A3.
  - Live Discord Gateway and live external LLM extraction E2E remain unverified in automated runs.

---

## A2 — `fix/sensitive-data-filter`

- **Depends on:** `main`
- **Commits:**
  - `e0cf0a4` `fix(memory): harden sensitive data filter with checksums and bilingual categories`
  - `8977119` `fix(ai): align extraction prompt and personality boundaries with sensitive filter`
  - `(this commit)` `docs(memory): document sensitive data filter and A2 changelog`
- **What changed:**
  - Fixed digit-run extraction in `internal/memory/app/validator.go` to count ASCII and Thai digits across common separators (space, dash, dot, non-breaking space, en/em dash) with sliding-window checks.
  - Added Luhn checksum detection (`isValidLuhn`, 13–19 digits), Thai national ID checksum detection (`isValidThaiNationalID`, 13 digits), and 13+ separated digit-run rejection (catching regressions `"4111 1111 1111 1111"` and `"1-2345-67890-12-3"`).
  - Added conservative, best-effort bilingual (Thai/English) keyword and regex coverage for health/medical conditions, religion, politics, sexual orientation, precise locations (4+ decimal GPS coords, Google Maps pins, postal codes/unit numbers), and minors' data.
  - Updated `memoryExtractionSystemPrompt` (`internal/ai/adapter/llm/memory_extractor.go`) and `DefaultPersonality().Boundaries` (`internal/personality/core/domain.go`) to stay consistent with the filter.
  - Documented the best-effort filter coverage in `validator.go` and `README.md`.
- **Tests run:**
  - `gofmt -l .` (clean)
  - `go vet ./...` (pass)
  - `go test ./... -count=1 -race` against real local MongoDB on `127.0.0.1:27017` (all packages pass, including table-driven true positives, false-negative regressions, checksum unit tests, and 10 benign Thai/English sentences)
  - `go build ./cmd/bot` (pass)
- **Known gaps:**
  - Keyword/regex filtering is best-effort and cannot guarantee detection of heavily obfuscated or indirect sensitive disclosures.

---

## A3 — `fix/memory-retrieval-quality-and-poisoning`

- **Depends on:** `fix/memory-dedup-and-trigger-only-rules`
- **Commits:**
  - `1968d83` `feat(memory): add relevance ranking, corroboration fields, and similarity scoring`
  - `c58f397` `feat(memory): add text-indexed retrieval, corroboration, TTL, and consolidation in MongoDB`
  - `9424d9c` `feat(memory): wire topic query, corroboration thresholds, and TTL config`
  - `(this commit)` `docs(memory): document retrieval ranking, corroboration, TTL, and A3 changelog`
- **What changed:**
  - Added MongoDB text index `idx_memories_text_search` (`content` weight 2, `tags` weight 3, `default_language: "none"` for Thai/English compatibility) and partial TTL index `idx_memories_expires_at` (`expires_at` date field, `expireAfterSeconds: 0`).
  - Added `FindRelevantByQuery` and domain `ScoreMemory`/`RankMemories` combining text/lexical relevance (0.50), importance (0.25), confidence (0.15), and recency (0.10), falling back cleanly to importance/confidence/recency when there is no text match.
  - Added configurable `MEMORY_MIN_CONFIDENCE` (default `0.5`).
  - Tracked distinct `source_user_ids` in `MemoryValidator` and `MemoryRepository.Save` (`$addToSet`); community-scope memories now require corroboration from at least `MEMORY_COMMUNITY_MIN_CORROBORATORS` distinct users (default `2`) or optional explicit `MEMORY_COMMUNITY_HIGH_CONFIDENCE` (default `0` disabled) before becoming retrievable.
  - Added optional off-safe expiration (`MEMORY_UNUSED_TTL`, default `0s`; `MEMORY_UNUSED_TTL_MAX_IMPORTANCE`, default `2`) for unused low-importance memories; usage updates (`UpdateUsage`/`UpdateUsageMany`) or importance upgrades clear `expires_at`.
  - Added bilingual `NearDuplicateSimilarity` and `MemoryRepository.ConsolidateNearDuplicates` (also exposed via `cmd/dedup-memories --consolidate`).
  - Extended `conversation/core.MemoryRetriever` via `ContextualMemoryRetriever` so `HandleMentionUseCase` forwards trigger message content for topic-relevant ranking.
  - Updated `.env.example` and `README.md`.
- **Tests run:**
  - `gofmt -l .` (clean)
  - `go vet ./...` (pass)
  - `go test ./... -count=1 -race` against real local MongoDB on `127.0.0.1:27017` (all packages pass, including unit ranking/similarity/config tests and MongoDB integration tests `TestMemoryRepositoryTextSearchRankingAndMinConfidence`, `TestMemoryRepositoryCommunityCorroborationPreventsSingleUserPoisoning`, `TestMemoryRepositoryUnusedTTLAndExpiry`, and `TestMemoryRepositoryConsolidateNearDuplicates`)
  - `go build` of `./cmd/bot` and `./cmd/dedup-memories` (pass)
- **Known gaps:**
  - MongoDB `$text` index tokenizes on whitespace, so unspaced Thai queries rely on the Go-side lexical/tag overlap fallback over the fetched pool.
  - Pre-existing community memories without `source_user_ids` require either corroboration or setting `MEMORY_COMMUNITY_HIGH_CONFIDENCE` / `MEMORY_COMMUNITY_MIN_CORROBORATORS=1` during migration.

---

## Checkpoint Status (Stopped by Owner)

- **Current branch:** `fix/memory-retrieval-quality-and-poisoning` (A3)
- **Completed items:**
  - **A1** `fix/memory-dedup-and-trigger-only-rules` (branched from `main`) — complete, all gates verified green.
  - **A2** `fix/sensitive-data-filter` (branched from `main`) — complete, all gates verified green.
  - **A3** `fix/memory-retrieval-quality-and-poisoning` (stacked on A1 `fix/memory-dedup-and-trigger-only-rules`) — complete, all gates verified green prior to stop.
- **Remaining items (not started):**
  - **Part A:** A4 (`fix/conversation-context-privacy`), A5 (`fix/llm-client-robustness`), A6 (`fix/graceful-failure-and-cooldown`), A7 (`fix/reply-mentions-and-reply-triggers`), A8 (`fix/mood-scope-and-signal-accuracy`), A9 (`fix/prompt-injection-hardening`), A10 (`fix/ops-resilience`), A11 (`ci/security-and-quality`), A12 (`feat/user-controls-slash-commands`).
  - **Part B:** B0 (`feat/voice-dave-spike`), B1 (`feat/voice-ports-and-providers`), B2 (`feat/voice-messages`), B3 (`feat/voice-channel-session`), B4 (`feat/voice-listening-and-consent`), B5 (`feat/voice-conversation-loop`), B6 (`docs/voice-and-final-hardening`).
- **Test status:**
  - On A1, A2, and A3 (after final Go code commit `9424d9c`): `gofmt -l .` (clean), `go vet ./...` (pass), `go test ./... -count=1 -race` against real local MongoDB on `127.0.0.1:27017` (all 20 test packages passed, 0 failures, 0 skips), and `go build` of `./cmd/bot` (and `./cmd/dedup-memories` on A1/A3) passed.
  - Live Discord Gateway, live external LLM, and all remaining items (A4–A12, B0–B6) are **UNVERIFIED / not started**.
