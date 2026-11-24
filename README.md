# The Bebot

The Bebot is a Discord bot project written in Go. The project aims to create a bot that behaves like a playful member of a Discord community rather than a generic assistant.

The bot uses an LLM to generate natural-language responses, while application behavior, business rules, personality, mood state, conversation context, memory processing, and community/member metadata are handled by the Go application.

> **Project status:** All approved MVP components (Discord Gateway, Conversation Context persistence, Global Go Personality, Mood system, Long-term Memory pipeline, and Community/Member metadata) plus review hardening fixes are implemented and verified with automated unit tests, real MongoDB integration tests, race detection (`go test ./... -count=1 -race`), and full-MVP pipeline integration tests (`httptest` LLM + real MongoDB). Live production Discord Gateway + external LLM E2E verification has not been executed in automated runs.

---

## Table of Contents

- [Project Overview](#project-overview)
- [Project Goals](#project-goals)
- [Technology Stack](#technology-stack)
- [Architecture](#architecture)
- [Discord Interaction](#discord-interaction)
- [Personality](#personality)
- [Mood System](#mood-system)
- [Memory System](#memory-system)
- [Conversation Context](#conversation-context)
- [Community and Member Context](#community-and-member-context)
- [Database and Persistence](#database-and-persistence)
- [Background Processing](#background-processing)
- [Memory Types](#memory-types)
- [Configuration](#configuration)
- [Development Scope](#development-scope)
- [Current Implementation Status](#current-implementation-status)
- [Testing and Verification](#testing-and-verification)
- [Known Limitations and Remaining Verification](#known-limitations-and-remaining-verification)
- [Project Principles](#project-principles)

---

## Project Overview

The Bebot is designed to participate in Discord community conversations.

Its intended behavior is different from a conventional AI assistant:

- It feels like a member of the community.
- It communicates casually and naturally.
- It has a recognizable Go-owned personality.
- It retains relevant information from previous interactions.
- It uses community-level and member-level context when generating responses.
- Its mood influences how it responds.
- It responds when explicitly mentioned by human community members.

The project separates application responsibilities from language generation.

Go is responsible for application logic, state, personality, mood, memory, context, and business rules. The LLM is responsible for generating natural-language responses using the structured context provided by the application.

---

## Project Goals

The main goals are:

1. Build a Discord bot using Go.
2. Use an LLM to generate conversational responses.
3. Make the bot behave like a playful and recognizable community member.
4. Provide a Go-owned Personality system.
5. Provide a Mood system (persistent community mood + temporary interaction mood).
6. Provide long-term Memory.
7. Support community-level and member-level context without duplicating Memory records.
8. Process memory candidates asynchronously.
9. Persist relevant state in MongoDB.
10. Keep application logic and infrastructure separated through Hexagonal Architecture.

---

## Technology Stack

| Technology | Responsibility |
|---|---|
| Go | Main application language |
| DiscordGo | Discord Gateway and Discord API integration |
| MongoDB | Persistent data storage |
| MongoDB Go Driver | MongoDB integration |
| LLM | Natural-language response generation and memory extraction |
| Goroutines | Background worker execution and periodic cleanup |

The project uses Hexagonal Architecture to separate application logic from external infrastructure.

---

## Architecture

The project follows **Hexagonal Architecture**.

The main architectural responsibilities are:

- **Core:** Domain concepts, ports/interfaces, and business rules.
- **Application:** Use cases and application workflows.
- **Adapters:** Implementations that connect the application to external systems (MongoDB, Discord, LLM, and cross-domain conversation ports).
- **Infrastructure:** Configuration, database connection lifecycle, structured logging, and composition root (`cmd/bot/main.go`).

Application logic depends on core interfaces rather than concrete infrastructure or cross-domain implementations.

### High-level interaction flow

```text
Discord Message (human authors only; ignores bot authors and DMs)
      |
      v
Discord Gateway
      |
      v
Mention Handler (enforces MENTION_TIMEOUT)
      |
      v
Conversation Context (live Discord REST fetch + MongoDB 24h fallback/dedup)
      |
      +------------------+------------------+------------------+
      |                  |                  |                  |
      v                  v                  v                  v
Personality        Mood System        Community/Member   Memory Retrieval
(Go domain)     (MongoDB + 15m TTL)   (MongoDB upsert)  (top N memories)
      |                  |                  |                  |
      +------------------+--------+---------+------------------+
                                  |
                                  v
                            LLM Request
                                  |
                                  v
                            LLM Response
                                  |
                                  v
                           Discord Reply
                                  |
                  +---------------+---------------+
                  |                               |
                  v                               v
     Persist ConversationContext       Memory Candidate Creation
       (MongoDB upsert, 24h TTL)         (idempotent, rule-gated)
                                                  |
                                                  v
                                        MongoDB Candidate Queue
                                                  |
                                                  v
                                         Memory Background Worker
                                        (+ periodic expired cleanup)
                                                  |
                                                  v
                                        Extract & Validate Memory
                                        (sibling-preserving batch)
                                                  |
                                                  v
                                         Persist Long-term Memory
```

---

## Discord Interaction

The bot integrates with Discord through DiscordGo (`internal/discord/adapter/gateway/`).

### Required Discord Privileged Intent

In the **Discord Developer Portal** (Bot settings), you **must enable the `Message Content Intent`** privileged gateway intent. The bot requests `IntentsGuilds | IntentsGuildMessages | IntentsMessageContent` so it can read mention text and recent guild channel context.

Implemented interaction behavior:

- Connect to the Discord Gateway with graceful shutdown that stops accepting new events, closes the Discord session, waits up to 30 seconds for in-flight mention handlers with a non-canceled handler context, and only then cancels the handler context.
- Ignore Direct Messages (MVP scope is guild channels only).
- Ignore messages authored by any bot (`message.Author.Bot == true`) as well as The Bebot itself to prevent bot-to-bot response loops.
- Detect explicit bot mentions, enforce per-user cooldown (`MENTION_USER_COOLDOWN`) and global concurrency cap (`MAX_CONCURRENT_MENTIONS`), and extract real Discord identity metadata (`GuildName` from session state, `Member.Nick` / `GlobalName` / `Username`).
- Retrieve recent channel messages via Discord REST API (handling empty `guild_id` on REST message payloads while still rejecting mismatched non-empty guild IDs or mismatched channel IDs).
- Show Discord's typing indicator while generating the LLM response.
- Send replies via `ChannelMessageSendComplex` with empty `AllowedMentions.Parse` (preventing unintended `@everyone`, `@here`, role, or user pings), referencing the trigger message (`FailIfNotExists=false`) on the first chunk, and splitting responses over 2000 runes cleanly at paragraph/line/word boundaries without corrupting multi-byte Thai text.

---

## Personality

Personality defines the bot's underlying behavioral identity and is owned by Go in `internal/personality/` (separate from Mood and decoupled from the LLM adapter).

Implemented Personality model (`internal/personality/core`):

- `Name`
- `Traits`
- `SpeakingStyle`
- `TeasingRules`
- `Boundaries`
- `LanguagePolicy`

The default Bebot profile encodes the established direction:

- Casual and informal.
- Playful and witty.
- Lively and entertaining.
- Mischievous.
- Has its own opinions.
- Light-hearted teasing only; never humiliates, insults, or intentionally hurts community members.
- Communicates naturally using the community's language.
- Avoids sounding like a formal AI assistant.

Personality is global across communities for MVP, with optional configuration overrides (`PERSONALITY_NAME`, `PERSONALITY_EXTRA_TRAITS`, `PERSONALITY_LANGUAGE_POLICY`). The LLM adapter renders Go-supplied `PersonalityContext` into the system prompt and retains `defaultSystemPrompt` only as a fallback when no Personality context is provided.

---

## Mood System

Mood (`internal/mood/`) is separate from Personality and influences tone and energy without violating Personality boundaries.

### Categories and intensity

- **Categories (6 fixed):** `cheerful`, `playful`, `neutral`, `sarcastic`, `annoyed`, `tired`.
- **Intensity scale:** integer `1` (mild) to `5` (very strong).
- **Default community mood:** `playful`, intensity `2`.

### Persistent community mood and decay

- Persisted per `guild_id` in MongoDB collection `community_moods` (unique index on `guild_id`), surviving application restarts.
- **Deterministic decay:** reduces intensity by `1` for every `6` hours of inactivity since `updated_at`; when intensity falls below `1`, resets to default `playful` (`2`). Decay is computed deterministically from elapsed time without requiring a background timer.
- Per-guild locking serializes load → decay → transition → persist for concurrent mentions.

### Temporary interaction mood

- Maintained in memory only (not persisted in MongoDB), scoped by `(guild_id, channel_id)` with a `15`-minute TTL and `sync.RWMutex` concurrency safety.
- Combined with persistent Community Mood into an `EffectiveMood` supplied to the LLM in its own prompt section, distinct from Personality.

### Deterministic interaction signals

Mood transitions are computed deterministically in Go (never mutated by the LLM) from clear Thai/English interaction cues across 5 categories: greetings/compliments, playful teasing/jokes, sarcasm, hostile/rude cues, and explicit tiredness. Neutral or ambiguous messages cause no mood mutation, and rapid-fire mentions alone are not treated as annoyed behavior.

---

## Memory System

Memory (`internal/memory/`) retains useful long-term information and supplies it as untrusted reference data in future interactions.

Implemented workflow:

1. Receive a Discord mention and build conversation context.
2. Retrieve up to `MEMORY_MAX_RETRIEVED_MEMORIES` (default `5`) relevant memories (`community` scope + speaking `member` scope) via the `MemoryRetriever` port, updating usage tracking (`usage_count`, `last_used_at`).
3. Include retrieved memories as JSON reference data with prompt-injection defense guidelines in the LLM system prompt.
4. After replying, evaluate `CandidateRules` on the **trigger message only** (keeping earlier window messages as extraction context only; Thai/English preference, fact, and promise signals including Unicode curly apostrophes, minimum 12 runes, ignoring bot messages).
5. Create a `MemoryCandidate` in MongoDB (`memory_candidates`), treating duplicate `(guild_id, message_id)` creation idempotently on redelivered events.
6. Background `MemoryWorker` goroutines atomically claim pending, retry-ready, or lease-expired candidates via `ClaimNext`.
7. `MemoryCandidateProcessor` extracts structured memories via `llm.MemoryExtractor` (prompt schema includes `source_message_id`), validates each item with `MemoryValidator` (authoritative source user ID, non-bot source check, sensitive-pattern filtering), preserves valid sibling memories if another item in the batch is invalid, saves via candidate-level idempotency and content-based deduplication (`content_dedup_key`, advancing `importance` and `updated_at` to max on duplicates) to `memories`, and updates candidate status with ownership/lock verification (`UpdateProcessingResult`).
8. A dedicated cleanup goroutine in `MemoryWorker` periodically invokes `CandidateRepository.DeleteExpired` without blocking candidate processing.

### One-off deduplication cleanup for legacy data

For existing deployments created before `content_dedup_key` was introduced, a manual, idempotent cleanup helper is available (never executed automatically by `cmd/bot`):

```bash
# Dry-run (default, inspects without mutating):
go run ./cmd/dedup-memories

# Apply cleanup across all guilds (or add --guild <guild_id>):
go run ./cmd/dedup-memories --apply
```

---

## Conversation Context

Conversation Context (`internal/conversation/`) provides chronological recent messages to the LLM:

- Live Discord `FetchRecentMessages` is the primary source of recent channel messages.
- The trigger message is guaranteed to be included and sorted chronologically, bounded by `MAX_CONTEXT_MESSAGES` (default `15`) with a 24-hour TTL.
- The latest `ConversationContext` snapshot per `(guild_id, channel_id)` is upserted into MongoDB (`conversation_contexts`, unique `(guild_id, channel_id)` index and 24-hour TTL index on `expires_at`).
- When live Discord messages return fewer than `maxMessages`, non-duplicate messages from the latest non-expired snapshot supplement context (live messages take precedence). If live Discord fetch fails transiently, a non-expired persisted snapshot serves as fallback.
- `HandleMentionUseCase` enforces `MENTION_TIMEOUT` per mention, while `CONVERSATION_TIMEOUT` bounds individual LLM HTTP calls.

---

## Community and Member Context

Lightweight identity metadata (`internal/community/`) is persisted separately from long-term `Memory`:

- **Communities (`communities`):** `guild_id` (unique), `name` (real Discord guild name when available), `primary_language` (never guessed from a single message; remains unset unless explicitly stored), `active_since`, `updated_at`.
- **Members (`members`):** `(guild_id, user_id)` (unique), `display_name` (Discord `GlobalName` / `Username`), `nickname` (Discord guild `Nick`, `$unset` when cleared), `interaction_count` (atomically incremented only after a reply is sent successfully), `last_seen_at`.

Long-term facts, preferences, events, jokes, promises, catchphrases, and interaction patterns remain exclusively in `memories` (`community` and `member` scopes).

---

## Database and Persistence

MongoDB collections and indexes ensured at startup:

| Collection | Responsibility | Key Indexes | Status |
|---|---|---|---|
| `memories` | Long-term memories (`community` & `member` scopes) | `idx_memories_community_relevance`, `idx_memories_member_relevance`, unique partial `idx_memories_idempotency`, unique partial `idx_memories_candidate_item`, unique partial `idx_memories_content_dedup` | Implemented & Verified (MongoDB tests) |
| `memory_candidates` | Durable candidate queue and processing leases | unique `(guild_id, message_id)`, pending/retry index, processing lock index, expiration index | Implemented & Verified (MongoDB tests) |
| `conversation_contexts` | Latest conversation context snapshot per `(guild_id, channel_id)` | unique `(guild_id, channel_id)`, TTL index on `expires_at` (`expireAfterSeconds: 0`) | Implemented & Verified (MongoDB tests) |
| `community_moods` | Persistent guild-level mood state | unique `guild_id` | Implemented & Verified (MongoDB tests) |
| `communities` | Lightweight community identity metadata | unique `guild_id` | Implemented & Verified (MongoDB tests) |
| `members` | Lightweight member identity & interaction count metadata | unique `(guild_id, user_id)` | Implemented & Verified (MongoDB tests) |

---

## Background Processing

Memory candidate processing runs asynchronously via `MemoryWorker` goroutines (`internal/worker/`):

- Uses MongoDB (`memory_candidates`) as the durable candidate store and atomic lease queue (`ClaimNext`).
- Exponential backoff retries up to 5 attempts with lock-ownership verification (`UpdateProcessingResult`).
- Periodic non-blocking cleanup goroutine calling `DeleteExpired`.
- Graceful shutdown via context cancellation and `Stop(ctx)`.

The MVP intentionally excludes Redis, Kafka, Kubernetes, and Vector Databases.

---

## Memory Types

| Memory Type | Description |
|---|---|
| `running_joke` | Running jokes shared within the community |
| `personal_fact` | Personal facts about a member |
| `preference` | Preferences expressed by a member |
| `event` | Events and relevant community occurrences |
| `promise` | Promises made during interactions |
| `catchphrase` | Common phrases or expressions |
| `interaction_pattern` | Patterns observed from interactions |

---

## Configuration

Configuration is loaded via `internal/platform/config` from environment variables (and `.env` if present), validated on startup (`.env.example` lists all supported keys):

- **Discord:** `DISCORD_BOT_TOKEN`
- **MongoDB:** `MONGODB_URI`, `MONGODB_DATABASE`
- **LLM:** `LLM_BASE_URL`, `LLM_API_KEY`, `LLM_MODEL`, `LLM_MAX_TOKENS` (default `600`, must be `> 0`)
- **Conversation:** `CONVERSATION_TIMEOUT` (default `30s`, LLM HTTP client timeout, must be `> 0`), `MENTION_TIMEOUT` (default `60s`, whole mention pipeline timeout, must be `> CONVERSATION_TIMEOUT`), `MENTION_USER_COOLDOWN` (default `5s`, per-user mention cooldown, must be `>= 0`), `MAX_CONCURRENT_MENTIONS` (default `8`, global in-flight mention concurrency cap, must be `> 0`), `MAX_CONTEXT_MESSAGES` (default `15`, must be `> 0`)
- **Memory:** `MEMORY_ENABLED` (default `true`), `MEMORY_MAX_RETRIEVED_MEMORIES` (default `5`, must be `> 0`), `MEMORY_WORKER_COUNT` (default `2`, must be `> 0`)
- **Mood:** `MOOD_USER_COOLDOWN` (default `2m`, per-user cooldown on persistent community mood changes, must be `>= 0`)
- **Personality (optional overrides):** `PERSONALITY_NAME`, `PERSONALITY_EXTRA_TRAITS` (comma-separated), `PERSONALITY_LANGUAGE_POLICY`
- **App / Logging:** `APP_NAME`, `APP_ENV`, `LOG_LEVEL` (default `info`)

Unused legacy keys `MEMORY_MAX_CANDIDATES` and `MEMORY_QUEUE_SIZE` were removed in favor of `MEMORY_MAX_RETRIEVED_MEMORIES` and MongoDB-backed polling.

---

## Development Scope

### MVP scope

- Discord Gateway integration and mention-based interaction (ignoring bots and DMs).
- LLM response generation and memory extraction.
- Conversation context with live Discord fetch and 24-hour MongoDB snapshot persistence.
- Global Go-owned Personality system.
- Persistent Community Mood + in-memory Temporary Interaction Mood.
- Long-term Memory candidate creation, background processing, validation, persistence, and retrieval.
- Lightweight Community and Member metadata context.
- MongoDB persistence across all required collections.

### Explicit exclusions

- Redis, Kafka, Kubernetes, Vector Database.
- Direct Message (DM) conversations.
- Unlimited/complete Discord message-history archival.
- Per-community Personality persistence or social-graph/relationship analytics.

---

## Current Implementation Status

| Component | Status | Verification Evidence |
|---|---|---|
| Discord Gateway & Mention handling (incl. bot-author filter & REST guild handling) | **Implemented** | Unit tests pass (`gateway` package); live Discord Gateway unverified in automated run |
| Conversation Context (live fetch + 24h MongoDB snapshot upsert/fallback) | **Implemented** | Unit tests + real MongoDB integration tests pass |
| Global Go-owned Personality system | **Implemented** | Unit tests + LLM prompt tests + full-MVP E2E test pass |
| Mood system (persistent `community_moods`, 6h decay, 15m in-memory temporary mood) | **Implemented** | Unit tests + concurrency test + real MongoDB restart test + full-MVP E2E test pass |
| Memory pipeline (rules, idempotent candidates, worker, cleanup, sibling-preserving extraction, validation, persistence, retrieval) | **Implemented** | Unit tests + real MongoDB repository tests + automated MongoDB+`httptest` E2E tests pass |
| Community & Member lightweight metadata (`communities`, `members`) | **Implemented** | Unit tests + real MongoDB integration tests + full-MVP E2E test pass |
| Configuration loading & validation | **Implemented** | Unit tests pass (`internal/platform/config`) |
| Automated Full-MVP Pipeline Verification (MongoDB + `httptest` LLM) | **Implemented & Verified** | `TestMemoryPipelineEndToEndWithMongoDBAndLLM` and `TestFullMVPPipelineEndToEndWithMongoDBAndLLM` pass against real MongoDB |
| Race detector verification (`go test ./... -count=1 -race`) | **Verified** | Passes locally with `gcc` and in GitHub Actions CI |
| Live Production Discord + External LLM E2E Verification | **Unverified** | Not executed against live Discord/external LLM APIs in automated test run |

---

## Testing and Verification

### Running locally

1. Copy `.env.example` to `.env` and fill in `DISCORD_BOT_TOKEN`, `MONGODB_URI`, `LLM_BASE_URL`, `LLM_API_KEY`, and `LLM_MODEL`.
2. Ensure **Message Content Intent** is enabled for your bot in the Discord Developer Portal.
3. Start MongoDB locally (or via Docker: `docker run -d -p 27017:27017 mongo:7`).
4. Run the bot:

```bash
go run ./cmd/bot
```

### Running with Docker

Build and run the multi-stage, non-root container image:

```bash
docker build -t the-bebot .
docker run --rm --env-file .env the-bebot
```

### Verification and CI

GitHub Actions (`.github/workflows/ci.yml`) runs formatting checks, `go vet`, full race-enabled unit and MongoDB integration tests (using a `mongo:7` service container), and `go build ./cmd/bot` on every push to `main` and every pull request.

Local verification commands:

```bash
gofmt -l .
go vet ./...
go test ./... -count=1 -race
go build ./cmd/bot
```

> Note: `-race` requires a C compiler (`gcc`, e.g. MSYS2 MinGW/UCRT64 on Windows) on `PATH`.

MongoDB integration tests connect to `MONGODB_TEST_URI` (defaulting to `mongodb://127.0.0.1:27017`), create isolated temporary databases per test, and drop them automatically during test cleanup. They skip cleanly if MongoDB is unreachable.

---

## Known Limitations and Remaining Verification

1. **Live external E2E (`UNVERIFIED`):** End-to-end tests verify all real Go components and all 6 real MongoDB collections using `httptest` for OpenAI-compatible LLM endpoints. Live verification with a real Discord guild and external LLM provider should be performed manually with valid credentials in `.env` and Message Content Intent enabled.

---

## Project Principles

- Go owns application behavior, state, personality, mood, business rules, and memory management.
- The LLM generates language rather than owning application business logic.
- Personality and Mood are separate concepts.
- Memory is processed asynchronously via MongoDB-backed background workers.
- Application logic remains separated from infrastructure through Hexagonal Architecture.
- The bot behaves like a playful community member, not a generic assistant.
- Features outside the agreed MVP scope are not added without an explicit decision.
