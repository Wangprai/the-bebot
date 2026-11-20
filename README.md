# The Bebot

The Bebot is a Discord bot project written in Go. The project aims to create a bot that behaves like a playful member of a Discord community rather than a generic assistant.

The bot uses an LLM to generate natural-language responses, while application behavior, business rules, conversation context, memory processing, and other state-related responsibilities are handled by the Go application.

> **Project status:** The core implementation has progressed through the main development phases. End-to-end testing and final testing/hardening remain. The status below distinguishes implemented components from planned requirements and items that still need verification.

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
- [Database and Persistence](#database-and-persistence)
- [Background Processing](#background-processing)
- [Memory Types](#memory-types)
- [Development Scope](#development-scope)
- [Current Implementation Status](#current-implementation-status)
- [Testing and Verification](#testing-and-verification)
- [Outstanding Work](#outstanding-work)
- [Requirements Not Yet Finalized](#requirements-not-yet-finalized)

---

## Project Overview

The Bebot is designed to participate in Discord community conversations.

Its intended behavior is different from a conventional AI assistant:

- It should feel like a member of the community.
- It should communicate casually and naturally.
- It should have a recognizable personality.
- It should be able to retain relevant information from previous interactions.
- It should use community and member context when generating responses.
- Its mood should influence how it responds.
- It should respond when explicitly mentioned.

The project separates application responsibilities from language generation.

Go is responsible for application logic, state, memory, context, and business rules. The LLM is responsible for generating natural-language responses using the context provided by the application.

---

## Project Goals

The main goals are:

1. Build a Discord bot using Go.
2. Use an LLM to generate conversational responses.
3. Make the bot behave like a playful and recognizable community member.
4. Provide a Personality system.
5. Provide a Mood system.
6. Provide long-term Memory.
7. Support community-level and member-level context.
8. Process memory candidates asynchronously.
9. Persist relevant information in MongoDB.
10. Keep application logic and infrastructure separated through Hexagonal Architecture.

---

## Technology Stack

| Technology | Responsibility |
|---|---|
| Go | Main application language |
| DiscordGo | Discord Gateway and Discord API integration |
| MongoDB | Persistent data storage |
| MongoDB Go Driver | MongoDB integration |
| LLM | Natural-language response generation |
| Goroutines | Background worker execution |

The project uses Hexagonal Architecture to separate application logic from external infrastructure.

---

## Architecture

The project follows **Hexagonal Architecture**.

The main architectural responsibilities are:

- **Core:** Domain concepts, interfaces, and business rules.
- **Application:** Use cases and application workflows.
- **Adapters:** Implementations that connect the application to external systems.
- **Infrastructure:** External services and their integrations.

The application should not depend directly on infrastructure implementations. Interfaces define the boundaries between application logic and adapters.

### High-level interaction flow

```text
Discord Message
      |
      v
Discord Gateway
      |
      v
Mention Handler
      |
      v
Conversation Context
      |
      +--------------------------+
      |                          |
      v                          v
Memory Retrieval            Context Processing
      |                          |
      +------------+-------------+
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
                   v
        Memory Candidate Creation
                   |
                   v
             MongoDB Queue
                   |
                   v
           Memory Background Worker
                   |
                   v
          Extract and Validate Memory
                   |
                   v
           Persist Long-term Memory
```

The flow describes the intended application workflow. Individual components may have different implementation and verification status, as described in the implementation section.

---

## Discord Interaction

The bot integrates with Discord through DiscordGo.

Expected interaction behavior:

- Connect to the Discord Gateway.
- Receive Discord messages.
- Respond when the bot is explicitly mentioned.
- Retrieve recent channel messages to provide conversation context.
- Generate a response using the LLM.
- Send the response back to Discord.

The bot is intended to respond to mentions rather than act as an automatic responder to every message.

---

## Personality

Personality defines the bot's recognizable behavior and communication style.

The intended personality direction is:

- Casual and informal.
- Playful and witty.
- Lively and entertaining.
- Mischievous.
- Able to tease lightly.
- Has its own opinions.
- Communicates naturally using the community's language.
- Avoids sounding like a formal AI assistant.

Teasing must remain light-hearted. The bot should not humiliate, insult, or intentionally hurt community members.

Personality and Mood are separate concepts:

- **Personality** represents the bot's underlying behavioral identity.
- **Mood** represents its current emotional state and can influence the tone of an individual response.

The current implementation contains a static `defaultSystemPrompt` in the LLM adapter. A separate configurable Personality domain/system has not been established as complete.

---

## Mood System

Mood is intended to influence the bot's conversational behavior.

Mood is separate from Personality.

The requirements established for Mood include:

- Persistent mood at the community level.
- Temporary mood influenced by interactions.
- Community mood should survive application restarts.
- Mood should be included in the context provided to the LLM.
- Mood should influence the bot's response behavior.

The intended distinction is:

| Concept | Responsibility |
|---|---|
| Personality | Defines the bot's underlying identity and behavior |
| Community Mood | Represents the community's persistent mood |
| Temporary Interaction Mood | Represents temporary mood changes caused by interactions |

The exact mood categories, transition formulas, intensity levels, decay rules, and persistence schema have not yet been finalized.

A complete Mood system is not currently confirmed as implemented.

---

## Memory System

Memory allows the bot to retain relevant information from conversations and use it in future interactions.

The intended memory workflow is:

1. Receive a Discord mention.
2. Build conversation context.
3. Identify whether the conversation contains information worth remembering.
4. Create a memory candidate when the conversation satisfies the candidate rules.
5. Store the candidate in MongoDB.
6. Process candidates asynchronously.
7. Extract and validate memory information.
8. Save accepted memories.
9. Retrieve relevant memories during future conversations.
10. Include retrieved memories in the LLM context.

Not every conversation should become a memory. Candidate creation is subject to rules and conditions.

### Memory Candidate

A Memory Candidate is information identified as potentially useful for long-term memory.

Candidate creation and memory extraction are separate responsibilities:

- Candidate creation identifies conversations that may contain useful information.
- Candidate processing extracts structured memory information.
- Validation determines whether extracted information should be stored.
- Memory persistence stores accepted information.

The candidate workflow uses MongoDB as persistent storage.

### Memory Retrieval

When generating a response, the application retrieves relevant memories and includes them in the conversation context.

The current implementation retrieves up to five memories for context.

Memory usage is updated when retrieved memories are used.

---

## Conversation Context

Conversation Context provides recent messages to the LLM so that responses can take the surrounding conversation into account.

The established requirements include:

- Retrieve recent Discord channel messages.
- Use recent messages as conversational context.
- Limit the number of messages through `maxMessages`.
- Use a 24-hour TTL for conversation context.

Recent Discord messages are retrieved for context. Durable storage of complete conversation history has not been established as an implemented feature.

A `ConversationRepository` port was defined with responsibilities for:

- Saving conversation context.
- Finding the latest conversation context.
- Deleting expired conversation context.

A MongoDB implementation and complete application wiring for this repository are not confirmed as complete.

---

## Database and Persistence

MongoDB is used for persistent storage.

The memory implementation includes the following collections:

| Collection | Responsibility |
|---|---|
| `memory_candidates` | Stores memory candidates awaiting processing or in processing states |
| `memories` | Stores accepted long-term memories |

Memory persistence includes repository operations and candidate-processing support.

### Intended persistence areas

The broader project requirements also include persistent community and member information, mood state, and conversation context.

The following areas have been identified as intended persistence responsibilities:

| Data | Intended responsibility | Status |
|---|---|---|
| Memories | Long-term memory storage | Implemented |
| Memory Candidates | Candidate queue and processing state | Implemented |
| Communities | Community-level information | Not confirmed complete |
| Members | Member-level information | Not confirmed complete |
| Mood | Persistent community mood | Not confirmed complete |
| Conversation Context | Recent conversation persistence and expiration | Not confirmed complete |

Collection names and schemas for the remaining persistence areas have not been finalized in the available requirements.

---

## Background Processing

Memory candidate processing is performed asynchronously using Go goroutines.

MongoDB acts as the durable storage and queue for memory candidates.

The intended processing responsibilities are:

1. Claim an available candidate.
2. Process the candidate.
3. Extract memory information using the LLM.
4. Validate the extracted information.
5. Save accepted memory.
6. Update candidate processing status.
7. Handle processing errors and retry where applicable.

The worker implementation includes lifecycle management and candidate-processing components.

The project does not require Redis, Kafka, Kubernetes, or a Vector Database as part of the established MVP scope.

---

## Memory Types

The memory types identified in the project requirements are:

| Memory Type | Description |
|---|---|
| `running_joke` | Running jokes shared within the community |
| `personal_fact` | Personal facts about a member |
| `preference` | Preferences expressed by a member |
| `event` | Events and relevant community occurrences |
| `promise` | Promises made during interactions |
| `catchphrase` | Common phrases or expressions |
| `interaction_pattern` | Patterns observed from interactions |

These types describe the intended categories of information that the bot may remember.

---

## Development Scope

### MVP scope

The established MVP includes:

- Discord Gateway integration.
- Mention-based interaction.
- LLM response generation.
- Conversation context.
- Personality.
- Mood.
- Long-term Memory.
- Community-level and member-level context.
- MongoDB persistence.
- Asynchronous memory candidate processing.
- Integration of relevant context into LLM requests.

### Explicit exclusions

The MVP does not include:

- Redis.
- Kafka.
- Kubernetes.
- Vector Database.

DM support and complete Discord message-history persistence are not established as MVP requirements.

---

## Current Implementation Status

The following table separates implementation that exists from requirements that are not yet confirmed complete.

| Component | Current status |
|---|---|
| Discord Gateway | Implemented |
| Mention handling | Implemented |
| LLM integration | Implemented |
| Recent message context retrieval | Implemented |
| Static system prompt | Implemented |
| Separate Personality system | Not confirmed complete |
| Mood system | Not confirmed complete |
| MongoDB connection | Implemented |
| Memory Candidate creation | Implemented |
| Memory Candidate repository | Implemented |
| Memory background processing | Implemented |
| Memory validation and persistence | Implemented |
| Memory retrieval | Implemented |
| Community persistence | Not confirmed complete |
| Member persistence | Not confirmed complete |
| Conversation persistence | Not confirmed complete |
| End-to-end Memory workflow verification | Remaining verification |
| Final testing and hardening | Remaining work |

The implementation status should not be interpreted as proof that every component has passed production-level verification.

---

## Testing and Verification

The following commands have been reported as passing during development:

```bash
go test ./... -count=1
```

```bash
go vet ./...
```

```bash
gofmt -l .
```

A project-wide race test was also reported as passing:

```bash
go test -race ./... -count=1
```

Passing automated tests does not establish that the complete bot has been verified in a real production Discord server and MongoDB environment.

### End-to-end verification

The remaining end-to-end workflow to verify is:

```text
Discord Mention
      |
      v
Conversation
      |
      v
LLM Response
      |
      v
Memory Candidate
      |
      v
Background Worker
      |
      v
MongoDB Persistence
      |
      v
Memory Retrieval
      |
      v
LLM Context
```

The complete flow must be verified to confirm that a candidate can be created, processed, stored, retrieved, and used in a subsequent response.

---

## Outstanding Work

The remaining work identified for the MVP is:

1. Complete end-to-end verification of the Memory workflow.
2. Verify the integration between Discord, Conversation, LLM, Candidate creation, Worker, MongoDB, and Memory retrieval.
3. Complete the remaining final testing.
4. Perform final hardening.
5. Verify that the implemented behavior matches the established Personality, Mood, Memory, and community-context requirements.

Items whose implementation is not confirmed complete must be reviewed against the actual source code before being marked as finished.

---

## Requirements Not Yet Finalized

The following details have not been fully specified in the available project requirements:

### Personality

- Exact Personality configuration schema.
- Whether Personality is global or configurable per community.
- How Personality configuration is stored and managed.

### Mood

- Exact mood categories.
- Mood intensity representation.
- Mood transition rules and formulas.
- Mood decay rules.
- Exact temporary interaction mood behavior.
- Final persistence schema.

### Persistence

- Final schemas and collection names for Communities.
- Final schemas and collection names for Members.
- Final schema and collection name for Mood.
- Final MongoDB implementation for Conversation Context.

These details should not be treated as implemented requirements or filled in without an explicit project decision.

---

## Project Principles

The following principles guide the project:

- Go owns application behavior, state, business rules, and memory management.
- The LLM generates language rather than owning the application's business logic.
- Personality and Mood are separate concepts.
- Memory should be processed asynchronously.
- MongoDB provides persistent storage for long-term memory and memory candidates.
- Application logic should remain separated from infrastructure through Hexagonal Architecture.
- The bot should behave like a playful community member, not a generic assistant.
- Features outside the agreed MVP scope should not be added without an explicit decision.
