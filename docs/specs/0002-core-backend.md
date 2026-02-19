# 0002: Core Backend

## Overview

Implement the three foundational backend subsystems: the Wyoming protocol TCP server, the database persistence layer (SQLite + PostgreSQL), and the TTS proxy with voice resolution and custom input parsing. After this spec, Meadowlark can accept Wyoming `synthesize` requests, resolve voices, call OpenAI-compatible TTS endpoints, and stream audio back to clients.

## Background

The Wyoming protocol is a JSONL + binary framing protocol over TCP used by Home Assistant for voice services. Meadowlark acts as a TTS server: it receives `synthesize` events, proxies them to OpenAI-compatible HTTP APIs, and returns raw PCM audio.

Configuration (endpoints, voice aliases) is persisted in a database. Voice resolution follows a priority chain: alias -> canonical -> fallback. Input text supports custom tag parsing and raw JSON for parameter overrides.

See `docs/meadowlark.md` sections 2-6 for full requirements.

## Goals

- Wyoming protocol event reader/writer (JSONL + binary framing)
- TCP server accepting concurrent connections (one goroutine per connection)
- Event handler for `describe`, `synthesize`, `ping`, and `error` events
- Zeroconf/mDNS service registration (`_wyoming._tcp.local.`)
- `Info` response builder that aggregates voices from all endpoints and aliases
- Database `Store` interface with SQLite implementation
- PostgreSQL implementation of the `Store` interface
- Auto-migration on startup
- Domain models: `Endpoint`, `VoiceAlias`, `ResolvedVoice`
- OpenAI-compatible TTS HTTP client (streaming response)
- WAV header parser (extract rate/width/channels, strip header)
- Voice resolver (alias -> canonical -> fallback, parameter priority chain)
- Custom input parser (tag format `[key: value]` and raw JSON)
- Full synthesis proxy orchestration: receive synthesize -> resolve voice -> parse input -> call API -> stream audio events

## Non-Goals

- HTTP REST API for CRUD (spec 0003)
- Frontend (spec 0004)
- Streaming synthesis (`synthesize-start/chunk/stop`)
- STT (speech-to-text)
- Audio resampling or format conversion

## Design

### Wyoming Protocol (`internal/wyoming/`)

#### Event Types

```go
type Event struct {
    Type    string
    Data    map[string]any
    Payload []byte
}
```

#### Wire Format

`ReadEvent(reader)` reads: JSON header line `\n` -> optional data bytes -> optional payload bytes.
`WriteEvent(writer, event)` writes the reverse. Protocol version: `"1.8.0"`.

See `docs/meadowlark.md` section 2.1 for the exact format.

#### TCP Server

`Server` listens on a configurable TCP address. Each connection spawns a goroutine running an event loop:

1. Read event
2. Dispatch by type (`describe` -> send `info`, `synthesize` -> proxy TTS, `ping` -> send `pong`)
3. On error -> send `error` event, log, continue
4. On connection close -> clean up

The server accepts a `Handler` interface so the TTS proxy logic is injected, not hardcoded.

#### Info Builder

Builds the `info` event from the current state of endpoints and aliases in the database. Voices are exposed as:
- Canonical: `"<voice> (<endpoint>, <model>)"` for each endpoint x model x voice
- Aliases: alias name directly

#### Zeroconf

Registers the service via `github.com/grandcat/zeroconf` with service type `_wyoming._tcp.local.`. Configurable name (default: hostname). Disabled via `--no-zeroconf`.

### Database Layer (`internal/store/`)

#### Store Interface

```go
type Store interface {
    ListEndpoints(ctx context.Context) ([]model.Endpoint, error)
    GetEndpoint(ctx context.Context, id string) (*model.Endpoint, error)
    CreateEndpoint(ctx context.Context, e *model.Endpoint) error
    UpdateEndpoint(ctx context.Context, e *model.Endpoint) error
    DeleteEndpoint(ctx context.Context, id string) error

    ListVoiceAliases(ctx context.Context) ([]model.VoiceAlias, error)
    GetVoiceAlias(ctx context.Context, id string) (*model.VoiceAlias, error)
    CreateVoiceAlias(ctx context.Context, a *model.VoiceAlias) error
    UpdateVoiceAlias(ctx context.Context, a *model.VoiceAlias) error
    DeleteVoiceAlias(ctx context.Context, id string) error

    Migrate(ctx context.Context) error
    Close() error
}
```

#### SQLite Implementation

Uses `modernc.org/sqlite` (pure Go, no CGO). Single connection with mutex for write serialization. Database file path from `--db-dsn` (default: `meadowlark.db`).

#### PostgreSQL Implementation

Uses `github.com/jackc/pgx/v5` with connection pooling. Connection string from `--db-dsn`.

#### Schema

Two tables: `endpoints` and `voice_aliases`. See `docs/meadowlark.md` section 6.2 for full column definitions. JSON arrays stored as TEXT. Migrations embedded in Go source and run on startup.

### Domain Models (`internal/model/`)

```go
type Endpoint struct {
    ID                    string
    Name                  string
    BaseURL               string
    APIKey                string
    Models                []string
    DefaultSpeed          *float64
    DefaultInstructions   *string
    DefaultResponseFormat string
    Enabled               bool
    CreatedAt             time.Time
    UpdatedAt             time.Time
}

type VoiceAlias struct {
    ID           string
    Name         string
    EndpointID   string
    Model        string
    Voice        string
    Speed        *float64
    Instructions *string
    Languages    []string
    Enabled      bool
    CreatedAt    time.Time
    UpdatedAt    time.Time
}

type ResolvedVoice struct {
    Name         string  // What Wyoming clients see
    EndpointID   string
    Model        string
    Voice        string  // Actual voice name on the endpoint
    Speed        *float64
    Instructions *string
    Languages    []string
    IsAlias      bool
}
```

### TTS Proxy (`internal/tts/`)

#### OpenAI Client

Makes `POST {base_url}/audio/speech` requests with streaming response. Uses `net/http` directly (no SDK). Sends `Authorization: Bearer {api_key}` header. Request body per `docs/meadowlark.md` section 3.2.

#### WAV Parser

Parses the 44-byte RIFF/WAV header from the first chunk of the streaming response to extract `rate`, `width`, `channels`. Strips the header and passes through raw PCM data.

Must handle edge cases:
- WAV header split across chunk boundaries (buffer until 44 bytes accumulated)
- Non-standard WAV headers (extra chunks before `data`)

#### Synthesis Orchestration

```
synthesize event received
  -> voice resolver: resolve voice name to ResolvedVoice
  -> input parser: extract overrides from text (tags/JSON/plain)
  -> parameter merge: input overrides > alias defaults > endpoint defaults
  -> OpenAI client: POST /audio/speech with merged params
  -> WAV parser: strip header, extract audio format
  -> Wyoming writer: send audio-start, audio-chunk (2048 byte chunks), audio-stop
```

### Voice Resolution (`internal/voice/`)

#### Resolver

Takes a voice name string and returns a `ResolvedVoice`:

1. Look up in voice aliases (by `name`, must be `enabled`)
2. Look up in canonical voices (parse `"<voice> (<endpoint>, <model>)"` format)
3. Fall back to first enabled endpoint's first voice

#### Input Parser

Parses the `text` field from a `synthesize` event:

1. If starts with `{` -> try JSON parse -> extract `input`, `instructions`, `speed`, `voice`, `model`
2. If starts with `[` -> parse bracket tags -> extract key-value pairs, remainder is the spoken text
3. Otherwise -> entire text is the spoken input

Tag parsing rules per `docs/meadowlark.md` section 5.2.

### Integration with main.go

The scaffold from spec 0001 is extended:

1. Parse CLI flags (already done)
2. Initialize database store (SQLite or PostgreSQL based on `--db-driver`)
3. Run migrations
4. Start Wyoming TCP server on `--wyoming-host:--wyoming-port`
5. Register Zeroconf service (unless `--no-zeroconf`)
6. Block until SIGTERM/SIGINT
7. Graceful shutdown: stop accepting connections, drain active ones, close DB

## Testing

### Unit Tests

- `internal/wyoming/`: Event read/write round-trip, header parsing edge cases
- `internal/store/`: CRUD operations for both SQLite and PostgreSQL (SQLite in-memory, Postgres via testcontainers)
- `internal/tts/`: WAV header parsing with various sample rates/widths, streaming response mock
- `internal/voice/`: Voice resolution priority chain, input parser for tags/JSON/plain text, edge cases (malformed tags, nested brackets, empty input)
- `internal/model/`: JSON array serialization/deserialization

### Integration Tests

- Full synthesis flow: mock HTTP server returning WAV audio -> verify Wyoming events sent back correctly
- Wyoming protocol conformance: raw TCP client sends `describe` -> verify `info` response structure
- Database migration: verify clean migration on empty database, verify idempotent re-migration

### Coverage Target

> 80% across all `internal/` packages.

## Tasks

- [ ] Implement Wyoming protocol event reader/writer
  - [ ] Define `Event` struct (Type, Data, Payload)
  - [ ] Implement `ReadEvent(reader)` -- JSON header line + optional data bytes + optional payload bytes
  - [ ] Implement `WriteEvent(writer, event)` -- reverse of read
  - [ ] Write round-trip tests (serialize -> deserialize -> compare)
  - [ ] Test edge cases: empty data, large payloads, missing optional fields
- [ ] Define Wyoming TTS event types
  - [ ] `Synthesize` (text, voice name, speaker, language)
  - [ ] `AudioStart` (rate, width, channels)
  - [ ] `AudioChunk` (rate, width, channels, payload)
  - [ ] `AudioStop`
  - [ ] `Describe` / `Info` (with TtsProgram, TtsVoice, TtsVoiceSpeaker)
  - [ ] `Ping` / `Pong`
  - [ ] `Error` (text, code)
  - [ ] Conversion methods: typed struct <-> generic Event
- [x] Implement domain models
  - [x] `model.Endpoint` with JSON array fields (Models)
  - [x] `model.VoiceAlias` with JSON array fields (Languages)
  - [x] `model.ResolvedVoice` (Name, EndpointID, Model, Voice, Speed, Instructions, Languages, IsAlias)
  - [x] JSON serialization/deserialization tests for array fields
- [x] Implement database Store interface and SQLite backend
  - [x] Define `Store` interface in `internal/store/store.go`
  - [x] Create embedded SQL migrations (endpoints + voice_aliases tables)
  - [x] Implement `SQLiteStore` with `modernc.org/sqlite`
  - [x] Implement all CRUD methods for endpoints
  - [x] Implement all CRUD methods for voice aliases
  - [x] Implement `Migrate()` (idempotent, runs on startup)
  - [x] Write tests with in-memory SQLite (`:memory:`)
  - [x] Test constraint violations (duplicate names, FK references)
- [ ] Implement PostgreSQL Store backend
  - [ ] Implement `PostgresStore` with `github.com/jackc/pgx/v5`
  - [ ] Reuse same SQL migrations (compatible syntax)
  - [ ] Write tests using testcontainers-go for PostgreSQL
- [ ] Implement TTS HTTP client
  - [ ] `POST {base_url}/audio/speech` with streaming response
  - [ ] Set `Authorization: Bearer {api_key}` header
  - [ ] Request body: model, voice, input, response_format, speed, instructions (omit nil fields)
  - [ ] Read response body as stream (chunked iteration)
  - [ ] Handle HTTP error responses (4xx, 5xx) with descriptive errors
  - [ ] Write tests with `httptest.NewServer` mock
- [ ] Implement WAV header parser
  - [ ] Parse 44-byte RIFF/WAV header (extract rate, width, channels)
  - [ ] Handle WAV header split across chunk boundaries (buffering)
  - [ ] Handle non-standard WAV headers (extra chunks before `data`)
  - [ ] Strip header, pass through raw PCM
  - [ ] Write tests with real WAV file fragments
- [ ] Implement voice resolver
  - [ ] Resolve by alias name (lookup in Store, must be enabled)
  - [ ] Resolve by canonical name (`"voice (endpoint, model)"` format parsing)
  - [ ] Fall back to first enabled endpoint's first voice
  - [ ] Build canonical voice list from all endpoints (endpoint x model x voice)
  - [ ] Write tests for each resolution path and edge cases
- [ ] Implement custom input parser
  - [ ] JSON parser: detect `{` prefix, parse, extract input/instructions/speed/voice/model
  - [ ] Tag parser: detect `[` prefix, parse `[key: value, key2: value2]` syntax, extract overrides + remaining text
  - [ ] Plain text fallback: entire string is the input
  - [ ] Parameter merge: input overrides > alias defaults > endpoint defaults
  - [ ] Write tests for all formats including malformed input, nested brackets, edge cases
- [ ] Implement synthesis proxy orchestration
  - [ ] Receive `synthesize` event -> resolve voice -> parse input -> merge params -> call TTS client -> stream audio events
  - [ ] Send `audio-start` with format from WAV header
  - [ ] Send `audio-chunk` events (2048-byte chunks) with PCM data
  - [ ] Send `audio-stop` when done
  - [ ] Send `error` event on failure (do not crash)
  - [ ] Write integration test with mock HTTP server
- [ ] Implement Wyoming TCP server
  - [ ] TCP listener on configurable host:port
  - [ ] One goroutine per connection with event loop
  - [ ] Dispatch `describe` -> build and send `info` event
  - [ ] Dispatch `synthesize` -> run synthesis proxy
  - [ ] Dispatch `ping` -> send `pong`
  - [ ] Handle connection errors gracefully (log, close)
  - [ ] Graceful shutdown: stop accepting, drain active connections
  - [ ] Write protocol conformance tests with raw TCP client
- [ ] Implement Zeroconf/mDNS registration
  - [ ] Register `_wyoming._tcp.local.` service via `github.com/grandcat/zeroconf`
  - [ ] Configurable service name (default: hostname)
  - [ ] Disable via `--no-zeroconf` flag
  - [ ] Deregister on shutdown
- [ ] Implement Info response builder
  - [ ] Aggregate canonical voices from all enabled endpoints (endpoint x model x voice)
  - [ ] Include enabled voice aliases
  - [ ] Build complete `info` event with TtsProgram, voices, languages
  - [ ] Rebuild on demand (called after config changes)
- [ ] Wire everything into main.go
  - [ ] Init Store (SQLite or Postgres based on `--db-driver`)
  - [ ] Run migrations
  - [ ] Start Wyoming TCP server
  - [ ] Register Zeroconf
  - [ ] Graceful shutdown sequence
