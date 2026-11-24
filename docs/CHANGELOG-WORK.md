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
