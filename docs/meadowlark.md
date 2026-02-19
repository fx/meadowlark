# Meadowlark -- Requirements Specification

Meadowlark is a Wyoming protocol to OpenAI-compatible TTS API bridge. It proxies text-to-speech requests from Wyoming clients (primarily Home Assistant) to one or more OpenAI-compatible TTS endpoints, with advanced voice aliasing, custom input parsing, and a web-based management UI.

---

## 1. Architecture Overview

```
                                    +-----------------+
                                    | OpenAI-compat   |
                              +---->| TTS Endpoint 1  |
                              |     | (e.g. OpenAI)   |
+------------------+    +-----+-----+     +-----------------+
| Wyoming Client   |    |           |
| (Home Assistant) +--->| Meadowlark+---->| OpenAI-compat   |
|                  |<---+           |     | TTS Endpoint 2  |
+------------------+    +-----+-----+     | (e.g. Speaches) |
                              |     +-----------------+
                              |
                        +-----+-----+
                        | Database  |
                        | (SQLite/  |
                        |  Postgres)|
                        +-----------+

                        +-----+-----+
                        | Preact    |
                        | Frontend  |
                        | (embedded)|
                        +-----------+
```

Meadowlark is a single statically-linked Go binary (linux/amd64) that:

1. Listens on a TCP port for Wyoming protocol connections
2. Serves an embedded Preact-based admin UI over HTTP
3. Proxies `synthesize` requests to configured OpenAI-compatible TTS endpoints
4. Persists configuration in SQLite (default) or PostgreSQL
5. Supports voice aliasing with per-alias default parameters
6. Parses custom input tags and raw JSON in synthesis text

---

## 2. Wyoming Protocol Implementation

### 2.1 Protocol Basics

The Wyoming protocol is a JSONL + binary framing protocol over TCP, created by the [Rhasspy/OHF-Voice](https://github.com/OHF-Voice/wyoming) project (current version: 1.8.0).

**Wire format**: Each message consists of:
1. A JSON header line terminated by `\n`, containing `type`, `version`, optional `data`, `data_length`, `payload_length`
2. Optional UTF-8 JSON data bytes (length = `data_length`)
3. Optional raw binary payload bytes (length = `payload_length`)

### 2.2 Supported Event Types

Meadowlark must implement the following events as a **TTS server**:

| Event Type | Direction | Purpose |
|---|---|---|
| `describe` | client -> server | Request service capabilities |
| `info` | server -> client | Respond with available voices/capabilities |
| `synthesize` | client -> server | Non-streaming TTS request (text + voice) |
| `audio-start` | server -> client | Begin audio output (rate, width, channels) |
| `audio-chunk` | server -> client | Raw PCM audio chunk (binary payload) |
| `audio-stop` | server -> client | End audio output |
| `ping` | client -> server | Health check |
| `pong` | server -> client | Health check response |
| `error` | server -> client | Error notification (text + code) |

**Streaming synthesis** (`synthesize-start`, `synthesize-chunk`, `synthesize-stop`, `synthesize-stopped`) is out of scope for the initial implementation. Meadowlark is a proxy to HTTP APIs, so the latency characteristics differ from local TTS engines that benefit from streaming.

### 2.3 TTS Request Flow

```
Client                              Meadowlark                         OpenAI Endpoint
  |                                    |                                    |
  |--- describe ---------------------->|                                    |
  |<-- info (voices list) -------------|                                    |
  |                                    |                                    |
  |--- synthesize(text, voice) ------->|                                    |
  |                                    |-- POST /v1/audio/speech ---------->|
  |                                    |   {model, voice, input, speed,     |
  |                                    |    response_format, instructions}  |
  |                                    |                                    |
  |                                    |<-- streaming WAV/PCM response -----|
  |<-- audio-start --------------------|                                    |
  |<-- audio-chunk (PCM) --------------|   (strip WAV header, forward      |
  |<-- audio-chunk (PCM) --------------|    raw PCM chunks)                |
  |<-- audio-stop ---------------------|                                    |
```

### 2.4 Audio Format

- Wyoming uses **raw PCM audio** (signed little-endian integers)
- Meadowlark requests `response_format=wav` from the OpenAI API
- The WAV header is parsed from the first chunk to extract `rate`, `width`, `channels`
- The WAV header is stripped; only raw PCM data is forwarded to Wyoming clients
- Default chunk size: 2048 bytes per `audio-chunk` event
- Typical TTS output: 24000 Hz, 16-bit, mono (but varies by endpoint/model)

### 2.5 Service Discovery (Zeroconf/mDNS)

Meadowlark must register itself via Zeroconf so Home Assistant can auto-discover it:

- Service type: `_wyoming._tcp.local.`
- Configurable instance name (defaults to hostname)
- Advertises the TCP port

Manual configuration (host + port) must also be supported.

### 2.6 Info Response Structure

The `info` event advertises available voices. Each voice exposed by Meadowlark is a combination of an endpoint + model + voice name:

```json
{
  "tts": [{
    "name": "meadowlark",
    "description": "Meadowlark TTS Bridge",
    "installed": true,
    "version": "0.1.0",
    "voices": [
      {
        "name": "alloy (OpenAI, tts-1)",
        "description": "alloy (OpenAI, tts-1)",
        "installed": true,
        "languages": ["en"],
        "speakers": null
      },
      {
        "name": "my-custom-alias",
        "description": "My Custom Alias",
        "installed": true,
        "languages": ["en"],
        "speakers": null
      }
    ]
  }]
}
```

---

## 3. OpenAI-Compatible TTS API Integration

### 3.1 Endpoint Configuration

Meadowlark supports **multiple** OpenAI-compatible TTS endpoints, each with independent configuration. Each endpoint has:

| Field | Type | Required | Description |
|---|---|---|---|
| `id` | UUID | auto | Unique identifier |
| `name` | string | yes | Human-readable name (e.g. "OpenAI", "Local Speaches") |
| `base_url` | string | yes | API base URL (e.g. `https://api.openai.com/v1`) |
| `api_key` | string | no | Bearer token for authentication |
| `models` | string[] | yes | List of model names to use (e.g. `["tts-1", "gpt-4o-mini-tts"]`) |
| `default_speed` | float | no | Default speed (0.25-4.0, omit = API default) |
| `default_instructions` | string | no | Default instructions (only for supported models like `gpt-4o-mini-tts`) |
| `default_response_format` | string | no | Response format to request (default: `wav`) |
| `enabled` | bool | yes | Whether this endpoint is active |
| `created_at` | timestamp | auto | Creation time |
| `updated_at` | timestamp | auto | Last update time |

### 3.2 API Call

For each `synthesize` request, Meadowlark calls:

```
POST {base_url}/audio/speech
Content-Type: application/json
Authorization: Bearer {api_key}

{
  "model": "<resolved model>",
  "voice": "<resolved voice name>",
  "input": "<processed text>",
  "response_format": "wav",
  "speed": <resolved speed>,
  "instructions": "<resolved instructions>"
}
```

The response is streamed -- Meadowlark reads the response body in chunks, parses the WAV header from the first chunk, then forwards raw PCM data as Wyoming `audio-chunk` events.

### 3.3 Voice Discovery

Each endpoint may provide its own voice list. Meadowlark should support:

1. **Hardcoded voice lists** per endpoint (configured by the user)
2. **Auto-discovery** where supported (e.g. Speaches' `GET /audio/speech/voices` or `GET /models/{model}`)

### 3.4 Voice Naming Convention

Voices are exposed to Wyoming clients with the naming format:

```
<voice_name> (<endpoint_name>, <model_name>)
```

Examples:
- `alloy (OpenAI, tts-1)`
- `nova (OpenAI, gpt-4o-mini-tts)`
- `af_sky (Local Speaches, kokoro-v1)`

This ensures unique voice identifiers when the same voice name exists across multiple endpoints/models.

---

## 4. Voice Aliasing System

### 4.1 Purpose

Wyoming only supports selecting voices by name, and integrations like Home Assistant seldom expose custom parameters. Voice aliases solve this by:

1. Providing friendly, short names for specific voice + endpoint + model combinations
2. Embedding default parameters (speed, instructions, etc.) into the alias
3. Appearing as regular voices in the Wyoming `info` response

### 4.2 Alias Configuration

| Field | Type | Required | Description |
|---|---|---|---|
| `id` | UUID | auto | Unique identifier |
| `name` | string | yes | Alias name (what Wyoming clients see) |
| `endpoint_id` | UUID | yes | Target endpoint |
| `model` | string | yes | Target model name |
| `voice` | string | yes | Target voice name on the endpoint |
| `speed` | float | no | Override speed for this alias |
| `instructions` | string | no | Override instructions for this alias |
| `languages` | string[] | no | Language codes (default: `["en"]`) |
| `enabled` | bool | yes | Whether this alias is active |
| `created_at` | timestamp | auto | Creation time |
| `updated_at` | timestamp | auto | Last update time |

### 4.3 Resolution Priority

When a `synthesize` request arrives with a voice name:

1. Check if the name matches a **voice alias** -- if so, use the alias's endpoint, model, voice, and default parameters
2. Check if the name matches a **canonical voice** (`<voice> (<endpoint>, <model>)`) -- if so, use the endpoint's defaults
3. Fall back to the **first available** endpoint and its first voice

Parameters are resolved with the following priority (highest wins):
1. Values extracted from custom input tags (see Section 5)
2. Voice alias defaults
3. Endpoint defaults
4. API defaults (omitted from request)

---

## 5. Custom Input Parsing

### 5.1 Overview

Meadowlark supports two custom input parsing modes that allow additional parameters to be embedded in the synthesis text. This is useful because Wyoming's `synthesize` event only carries `text` and `voice` -- there is no mechanism for passing custom parameters like `instructions` or `speed`.

### 5.2 Tag Format

A custom bracket-based tagging format that prepends parameter overrides to the text:

```
[key1: value1, key2: value2] Actual text to be spoken
```

**Parsing rules:**
- Tags must appear at the **start** of the text
- Multiple tags can be chained: `[key1: value1] [key2: value2] Text`
- Or comma-separated within a single tag: `[key1: value1, key2: value2] Text`
- The text after all tags is the actual input to be spoken
- If no tags are present, the entire text is the input

**Supported tag keys:**

| Key | Maps to | Example |
|---|---|---|
| `instructions` | OpenAI `instructions` field | `[instructions: speak angrily] Hello!` |
| `speed` | OpenAI `speed` field | `[speed: 1.5] Hello!` |
| `voice` | Override voice name | `[voice: nova] Hello!` |
| `model` | Override model name | `[model: gpt-4o-mini-tts] Hello!` |

**Example:**
```
[instructions: angry voice, shouting, speed: 1.5] Hello World
```
Parses to:
- `input`: `"Hello World"`
- `instructions`: `"angry voice, shouting"`
- `speed`: `1.5`

### 5.3 Raw JSON Format

If the text begins with `{`, Meadowlark attempts to parse the entire text as JSON. This JSON is expected to conform to the OpenAI TTS request body:

```json
{
  "input": "Hello World",
  "instructions": "angry voice, shouting",
  "speed": 1.5,
  "voice": "nova",
  "model": "gpt-4o-mini-tts"
}
```

**Rules:**
- The `input` field is required and becomes the text to be spoken
- All other fields override the resolved defaults
- If JSON parsing fails, the text is treated as plain text

### 5.4 Parsing Order

1. Try raw JSON parsing (if text starts with `{`)
2. Try tag format parsing (if text starts with `[`)
3. Fall back to plain text (entire text is the input)

---

## 6. Database Layer

### 6.1 Requirements

- **SQLite** as the default storage backend (zero-config, file-based)
- **PostgreSQL** support for production deployments
- Schema must be identical across both backends
- Migrations must be managed automatically on startup
- The database driver is selected via CLI flag

### 6.2 Schema

#### `endpoints` Table

| Column | Type | Constraints | Description |
|---|---|---|---|
| `id` | TEXT (UUID) | PRIMARY KEY | Unique identifier |
| `name` | TEXT | NOT NULL, UNIQUE | Human-readable name |
| `base_url` | TEXT | NOT NULL | API base URL |
| `api_key` | TEXT | | Encrypted API key |
| `models` | TEXT (JSON array) | NOT NULL | Available models |
| `default_speed` | REAL | | Default speed |
| `default_instructions` | TEXT | | Default instructions |
| `default_response_format` | TEXT | DEFAULT 'wav' | Response format |
| `enabled` | BOOLEAN | NOT NULL DEFAULT TRUE | Active flag |
| `created_at` | TIMESTAMP | NOT NULL DEFAULT NOW | Creation time |
| `updated_at` | TIMESTAMP | NOT NULL DEFAULT NOW | Last update |

#### `voice_aliases` Table

| Column | Type | Constraints | Description |
|---|---|---|---|
| `id` | TEXT (UUID) | PRIMARY KEY | Unique identifier |
| `name` | TEXT | NOT NULL, UNIQUE | Alias name |
| `endpoint_id` | TEXT (UUID) | NOT NULL, FK -> endpoints.id | Target endpoint |
| `model` | TEXT | NOT NULL | Target model |
| `voice` | TEXT | NOT NULL | Target voice name |
| `speed` | REAL | | Override speed |
| `instructions` | TEXT | | Override instructions |
| `languages` | TEXT (JSON array) | DEFAULT '["en"]' | Language codes |
| `enabled` | BOOLEAN | NOT NULL DEFAULT TRUE | Active flag |
| `created_at` | TIMESTAMP | NOT NULL DEFAULT NOW | Creation time |
| `updated_at` | TIMESTAMP | NOT NULL DEFAULT NOW | Last update |

### 6.3 Database Abstraction

A Go interface must be defined that both SQLite and PostgreSQL implementations satisfy:

```go
type Store interface {
    // Endpoints
    ListEndpoints(ctx context.Context) ([]Endpoint, error)
    GetEndpoint(ctx context.Context, id string) (*Endpoint, error)
    CreateEndpoint(ctx context.Context, e *Endpoint) error
    UpdateEndpoint(ctx context.Context, e *Endpoint) error
    DeleteEndpoint(ctx context.Context, id string) error

    // Voice Aliases
    ListVoiceAliases(ctx context.Context) ([]VoiceAlias, error)
    GetVoiceAlias(ctx context.Context, id string) (*VoiceAlias, error)
    CreateVoiceAlias(ctx context.Context, a *VoiceAlias) error
    UpdateVoiceAlias(ctx context.Context, a *VoiceAlias) error
    DeleteVoiceAlias(ctx context.Context, id string) error

    // Lifecycle
    Migrate(ctx context.Context) error
    Close() error
}
```

---

## 7. CLI Interface

### 7.1 Command Structure

Meadowlark is a single binary with the following CLI:

```
meadowlark [flags]

Flags:
  --wyoming-host string    Wyoming TCP listen address (default "0.0.0.0")
  --wyoming-port int       Wyoming TCP listen port (default 10300)
  --http-host string       HTTP server listen address (default "0.0.0.0")
  --http-port int          HTTP server listen port (default 8080)

  --db-driver string       Database driver: "sqlite" or "postgres" (default "sqlite")
  --db-dsn string          Database connection string (default "meadowlark.db" for sqlite)

  --zeroconf-name string   Zeroconf/mDNS service name (default: hostname)
  --no-zeroconf            Disable Zeroconf registration

  --log-level string       Log level: debug, info, warn, error (default "info")
  --log-format string      Log format: text, json (default "text")

  --version                Print version and exit
  --help                   Print help and exit
```

### 7.2 Environment Variable Fallbacks

Every CLI flag has an environment variable equivalent with the prefix `MEADOWLARK_`:

| Flag | Environment Variable |
|---|---|
| `--wyoming-port` | `MEADOWLARK_WYOMING_PORT` |
| `--http-port` | `MEADOWLARK_HTTP_PORT` |
| `--db-driver` | `MEADOWLARK_DB_DRIVER` |
| `--db-dsn` | `MEADOWLARK_DB_DSN` |
| ... | ... |

CLI flags take precedence over environment variables.

---

## 8. HTTP API

Meadowlark exposes a RESTful JSON API for the admin frontend. All endpoints are under `/api/v1/`.

### 8.1 Endpoints API

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/v1/endpoints` | List all endpoints |
| `POST` | `/api/v1/endpoints` | Create an endpoint |
| `GET` | `/api/v1/endpoints/:id` | Get an endpoint |
| `PUT` | `/api/v1/endpoints/:id` | Update an endpoint |
| `DELETE` | `/api/v1/endpoints/:id` | Delete an endpoint |
| `POST` | `/api/v1/endpoints/:id/test` | Test endpoint connectivity |
| `GET` | `/api/v1/endpoints/:id/voices` | Discover available voices |

### 8.2 Voice Aliases API

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/v1/aliases` | List all voice aliases |
| `POST` | `/api/v1/aliases` | Create a voice alias |
| `GET` | `/api/v1/aliases/:id` | Get a voice alias |
| `PUT` | `/api/v1/aliases/:id` | Update a voice alias |
| `DELETE` | `/api/v1/aliases/:id` | Delete a voice alias |
| `POST` | `/api/v1/aliases/:id/test` | Test TTS with this alias |

### 8.3 System API

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/v1/status` | Server status (version, uptime, voice count) |
| `GET` | `/api/v1/voices` | List all resolved voices (canonical + aliases) |

### 8.4 Static Files

| Method | Path | Description |
|---|---|---|
| `GET` | `/` | Serve embedded Preact frontend (SPA) |
| `GET` | `/assets/*` | Serve embedded static assets |

---

## 9. Go Backend Requirements

### 9.1 Build Constraints

- **Go version**: Latest stable (managed via `mise`)
- **Target**: `GOOS=linux GOARCH=amd64`
- **Linking**: Fully static binary (`CGO_ENABLED=0` for pure-Go SQLite, or `CGO_ENABLED=1` with static musl linking if using `mattn/go-sqlite3`)
- **Frontend embedding**: Use `//go:embed` to bundle the built Preact frontend into the binary
- **Build tags**: `sqlite` and `postgres` build tags for conditional compilation

### 9.2 Key Dependencies

| Purpose | Library |
|---|---|
| HTTP router | `net/http` (stdlib) or lightweight router (e.g. `chi`) |
| SQLite driver | `modernc.org/sqlite` (pure Go, no CGO) or `github.com/mattn/go-sqlite3` |
| PostgreSQL driver | `github.com/jackc/pgx/v5` |
| Zeroconf/mDNS | `github.com/grandcat/zeroconf` |
| CLI flags | `github.com/spf13/cobra` + `github.com/spf13/viper` or `github.com/urfave/cli/v2` |
| Logging | `log/slog` (stdlib) |
| UUID generation | `github.com/google/uuid` |
| OpenAI HTTP client | `net/http` (stdlib) -- no SDK needed, just raw HTTP calls |
| WAV parsing | Manual (44-byte WAV header) or `github.com/go-audio/wav` |
| Testing | `testing` (stdlib) + `github.com/stretchr/testify` |

### 9.3 Project Structure

```
meadowlark/
  cmd/
    meadowlark/
      main.go               # Entry point, CLI parsing, server startup
  internal/
    wyoming/
      event.go              # Event type, ReadEvent, WriteEvent
      server.go             # TCP server, connection handler
      types.go              # Synthesize, AudioStart, AudioChunk, etc.
      info.go               # Info response builder
      zeroconf.go           # mDNS registration
    tts/
      client.go             # OpenAI-compatible HTTP client
      proxy.go              # Synthesis orchestration (resolve voice -> call API -> stream audio)
      wav.go                # WAV header parsing, PCM extraction
    voice/
      resolver.go           # Voice resolution (alias -> canonical -> fallback)
      parser.go             # Custom input parsing (tags, JSON)
    store/
      store.go              # Store interface
      sqlite.go             # SQLite implementation
      postgres.go           # PostgreSQL implementation
      migrate.go            # Schema migrations
    api/
      server.go             # HTTP server setup, middleware
      endpoints.go          # Endpoint CRUD handlers
      aliases.go            # Voice alias CRUD handlers
      system.go             # Status, voices list
    model/
      endpoint.go           # Endpoint model
      alias.go              # VoiceAlias model
      voice.go              # Resolved voice model
  web/                      # Preact frontend (separate build)
    ...
  docs/
    meadowlark.md           # This file
  go.mod
  go.sum
  mise.toml                 # Toolchain management (go, bun, biome)
  Makefile
  Dockerfile
```

### 9.4 Concurrency Model

- Wyoming TCP server: one goroutine per connection
- Each synthesis request: goroutine reads streaming HTTP response and writes Wyoming events
- Database access: connection pooling (pgx for Postgres, single connection for SQLite with mutex)
- HTTP API server: standard `net/http` concurrency

### 9.5 Error Handling

- Wyoming protocol errors: send `error` event to client with descriptive text and code
- API errors: respond with appropriate HTTP status codes and JSON error bodies
- Endpoint failures: log error, return Wyoming `error` event, do not crash
- Database errors: log and return appropriate API/protocol errors

---

## 10. Frontend Requirements

### 10.1 Technology Stack

| Aspect | Technology |
|---|---|
| Framework | **Preact** (lightweight React alternative, chosen for minimal bundle size since we embed in Go binary) |
| Build tool | **Vite** (latest) |
| Package manager | **Bun** (latest, managed via `mise`) |
| Routing | **Preact Router** or **wouter** (lightweight SPA routing) |
| Styling | **Tailwind CSS v4** (native Vite plugin, CSS-only config) |
| Components | **shadcn/ui** (new-york style, adapted for Preact) |
| Icons | **Phosphor Icons** (navigation) + **Lucide** (actions/shadcn defaults) |
| Linting | **Biome** (latest) |
| Testing | **Vitest** (jsdom environment, 100% coverage) |
| Font | **JetBrains Mono** (for both sans and mono) |

### 10.2 Styling Details

The following design tokens must be used:

```css
@import "tailwindcss";
@import "tw-animate-css";
@import "@fontsource/jetbrains-mono";

@custom-variant dark (&:where(.dark, .dark *));

@theme {
  /* Zero border radius (sharp edges) */
  --radius-sm: 0;
  --radius-md: 0;
  --radius-lg: 0;
  --radius-xl: 0;

  /* JetBrains Mono everywhere */
  --font-mono: "JetBrains Mono", ui-monospace, SFMono-Regular, monospace;
  --font-sans: "JetBrains Mono", ui-sans-serif, system-ui, sans-serif;

  /* OKLch color palette -- cyan primary, neutral base */
  --color-primary: oklch(0.68 0.13 184);     /* light */
  --color-primary-dark: oklch(0.56 0.12 184); /* dark */
  /* ... full neutral scale ... */
}
```

### 10.3 Biome Configuration

```json
{
  "$schema": "https://biomejs.dev/schemas/2.3.11/schema.json",
  "vcs": { "enabled": true, "clientKind": "git", "useIgnoreFile": true },
  "formatter": {
    "enabled": true,
    "indentStyle": "space",
    "indentWidth": 2,
    "lineWidth": 100
  },
  "linter": { "enabled": true, "rules": { "recommended": true } },
  "javascript": {
    "formatter": { "quoteStyle": "single", "semicolons": "asNeeded" }
  },
  "assist": {
    "actions": { "source": { "organizeImports": "on" } }
  },
  "css": { "parser": { "tailwindDirectives": true } }
}
```

### 10.4 Layout Structure

**Top header navigation, no sidebar**:

```
+----------------------------------------------------------+
| [Meadowlark]  [Endpoints] [Voices] [Aliases]  [Settings] |
+----------------------------------------------------------+
|                                                          |
|                    Page Content                           |
|                                                          |
+----------------------------------------------------------+
```

- **AppHeader**: Sticky top bar using shadcn/ui `Menubar`
  - Brand "Meadowlark" as menu trigger
  - Icon-only nav buttons (Phosphor icons)
  - Right side: version string + theme toggle (dark/light/system)
  - Mobile: hamburger `Sheet` menu
- **Section layouts**: Horizontal tab navigation with `border-b-2` underline style
- **Theme**: Class-based dark mode with `ThemeProvider` context

### 10.5 Page Structure

#### Endpoints Page (`/endpoints`)
- List all configured OpenAI-compatible endpoints
- CRUD operations with inline edit forms (expandable rows)
- Test connectivity button per endpoint
- Discover voices button per endpoint
- Fields: name, base URL, API key (masked), models, default speed, default instructions, enabled toggle

#### Voices Page (`/voices`)
- Read-only list of all resolved voices (canonical + aliases)
- Shows: voice name, endpoint, model, type (canonical/alias)
- Search/filter functionality

#### Aliases Page (`/aliases`)
- List all voice aliases
- CRUD operations with inline edit forms
- Endpoint + model + voice selector (populated from configured endpoints)
- Override fields: speed slider, instructions textarea
- Test button (synthesize sample text)
- Fields: alias name, target endpoint, model, voice, speed, instructions, languages, enabled toggle

#### Settings Page (`/settings`)
- Display server status (version, uptime, Wyoming port, HTTP port)
- Database info (driver, connection status)
- Zeroconf status

### 10.6 CRUD Pattern

All list pages follow this shared pattern:
1. Header with title, description, and "+ Add" button
2. Items in `div.border.divide-y` containers
3. Each item is an expandable row: click to expand inline edit form
4. Actions: Edit (Pencil icon), Delete (Trash icon), Expand/Collapse (ChevronDown with rotate animation)
5. State: `expandedId` (only one expanded at a time, `'new'` for create mode)
6. Data fetching via lightweight fetch wrapper with SWR-like caching
7. Optimistic updates with error rollback

### 10.7 shadcn/ui Components Needed

Based on the UI requirements, the following components are needed:

- `button` (with custom `xs`, `icon-xs`, `icon-sm`, `icon-lg` size variants)
- `input`
- `label`
- `select`
- `switch`
- `textarea`
- `card`
- `dialog` (for confirmations)
- `alert-dialog` (for delete confirmations)
- `dropdown-menu`
- `menubar` (for header navigation)
- `sheet` (for mobile menu)
- `badge`
- `tooltip`
- `table`
- `popover`

### 10.8 Testing Requirements

- **100% test coverage** for all frontend code
- Test runner: **Vitest** with `jsdom` environment
- Test utilities: `@testing-library/preact`, `@testing-library/jest-dom`
- All components must have corresponding `.test.tsx` files
- API calls mocked at the fetch level
- Vitest config:
  - `globals: true`
  - `environment: 'jsdom'`
  - `coverage.thresholds: { lines: 100, functions: 100, branches: 100, statements: 100 }`

### 10.9 Build & Embedding

- Frontend is built with `bun run build` (Vite production build)
- Output goes to `web/dist/`
- Go binary embeds `web/dist/` via `//go:embed`
- The HTTP server serves embedded files at `/` with proper MIME types
- SPA fallback: all non-API, non-asset routes serve `index.html`

---

## 11. Reference Implementation Analysis

### 11.1 wyoming_openai (Python)

[roryeckel/wyoming_openai](https://github.com/roryeckel/wyoming_openai) is the primary reference implementation. Key takeaways:

**What it does well:**
- Streaming WAV response parsing with header extraction
- Concurrent TTS requests for sentence-boundary chunked streaming (up to 3 concurrent)
- Backend auto-detection (OpenAI, Speaches, Kokoro-FastAPI, LocalAI)
- Comprehensive voice + model Cartesian product generation

**Limitations that Meadowlark addresses:**
1. **No per-voice parameters**: Speed and instructions are global. Meadowlark's alias system solves this.
2. **No custom input parsing**: Cannot override parameters at synthesis time. Meadowlark's tag/JSON parsing solves this.
3. **Single endpoint only**: Only one OpenAI-compatible endpoint per instance. Meadowlark supports multiple.
4. **No persistent configuration**: All settings via CLI/env vars. Meadowlark uses a database.
5. **No management UI**: No web interface for configuration. Meadowlark has a Preact frontend.
6. **Confusing voice naming**: Cartesian product of models x voices creates many entries. Meadowlark's aliasing provides cleaner naming.
7. **No health check endpoint**: Meadowlark exposes `/api/v1/status`.
8. **Python-only**: Cannot produce a single static binary. Meadowlark is a single Go binary.

---

## 12. Toolchain Management

### 12.1 mise Configuration

All project dependencies (runtimes, tools) are managed via [mise](https://mise.jdx.dev/). A root `mise.toml` defines all required tools:

```toml
[tools]
go = "latest"
bun = "latest"
biome = "latest"
air = "latest"

[env]
CGO_ENABLED = "0"
GOARCH = "amd64"
GOOS = "linux"
```

All contributors and CI must use `mise install` to set up the development environment. No global tool installations should be assumed.

### 12.2 Makefile Targets

```makefile
build:          # Build frontend + Go binary
build-frontend: # Build Preact frontend only
build-backend:  # Build Go binary only (with embedded frontend)
test:           # Run all tests (Go + frontend)
test-go:        # Run Go tests only
test-frontend:  # Run frontend tests only
lint:           # Run all linters (Go + Biome)
dev:            # Run in development mode (hot reload)
clean:          # Clean build artifacts
```

### 12.3 Docker

Multi-stage build producing a minimal `scratch`-based image with just the static binary:

```dockerfile
# Stage 1: Build frontend
FROM oven/bun:1-alpine AS frontend
WORKDIR /app/web
COPY web/package.json web/bun.lock ./
RUN bun install --frozen-lockfile
COPY web/ .
ARG GIT_SHA=dev
ENV GIT_SHA=$GIT_SHA
RUN bun run build

# Stage 2: Build Go binary with embedded frontend
FROM golang:1-alpine AS backend
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/web/dist ./web/dist
ARG VERSION=dev
ARG GIT_SHA=dev
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -ldflags="-s -w -X main.version=${VERSION} -X main.commit=${GIT_SHA}" \
  -o /meadowlark ./cmd/meadowlark

# Stage 3: Minimal runtime
FROM scratch
COPY --from=backend /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=backend /meadowlark /meadowlark

# OCI labels for GHCR linking
LABEL org.opencontainers.image.source="https://github.com/OWNER/meadowlark"
LABEL org.opencontainers.image.description="Meadowlark -- Wyoming to OpenAI TTS Bridge"

EXPOSE 10300 8080
ENTRYPOINT ["/meadowlark"]
```

### 12.4 Build Command

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -ldflags="-s -w -X main.version=$(VERSION) -X main.commit=$(GIT_SHA)" \
  -o meadowlark \
  ./cmd/meadowlark
```

---

## 13. CI/CD

### 13.1 Overview

CI/CD is implemented via GitHub Actions. Three workflows handle linting/testing, Docker image publishing, and release management.

### 13.2 CI Workflow (`.github/workflows/ci.yml`)

Runs on every push to `main` and on pull requests. Validates both Go backend and frontend.

```yaml
name: CI

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]
    types: [opened, synchronize, reopened, ready_for_review]

jobs:
  backend:
    name: Go Tests & Lint
    runs-on: ubuntu-latest
    if: github.event.pull_request.draft == false || github.event_name == 'push'
    steps:
      - uses: actions/checkout@v4

      - uses: jdx/mise-action@v2

      - name: Go mod download
        run: go mod download

      - name: Lint
        run: go vet ./...

      - name: Test
        run: go test -race -coverprofile=coverage.out ./...

  frontend:
    name: Frontend Tests & Lint
    runs-on: ubuntu-latest
    if: github.event.pull_request.draft == false || github.event_name == 'push'
    steps:
      - uses: actions/checkout@v4

      - uses: jdx/mise-action@v2

      - name: Install dependencies
        working-directory: web
        run: bun install --frozen-lockfile

      - name: Lint and format check
        working-directory: web
        run: bunx biome check .

      - name: Test
        working-directory: web
        run: bun run test

      - name: Build
        working-directory: web
        run: bun run build
```

### 13.3 Docker Workflow (`.github/workflows/docker.yml`)

Builds and publishes Docker images to GitHub Container Registry (GHCR). Triggers on pushes to `main` and version tags. PRs only build (no push) to validate the Dockerfile.

```yaml
name: Docker

on:
  push:
    branches: [main]
    tags: ['v*']
  pull_request:

permissions:
  contents: read
  packages: write

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3

      - name: Log in to GitHub Container Registry
        if: github.event_name != 'pull_request'
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Get short SHA
        id: sha
        run: echo "short=${GITHUB_SHA::7}" >> $GITHUB_OUTPUT

      - name: Extract metadata for Docker
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: ghcr.io/${{ github.repository }}
          tags: |
            type=ref,event=branch
            type=sha,prefix=sha-
            type=semver,pattern={{version}}
            type=semver,pattern={{major}}.{{minor}}
            type=semver,pattern={{major}}

      - name: Build and push Docker image
        uses: docker/build-push-action@v6
        with:
          context: .
          push: ${{ github.event_name != 'pull_request' }}
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
          build-args: |
            VERSION=${{ steps.meta.outputs.version }}
            GIT_SHA=${{ steps.sha.outputs.short }}
          cache-from: type=gha
          cache-to: type=gha,mode=max
```

**Tag strategy:**

| Trigger | Tags produced |
|---|---|
| Push to `main` | `main`, `sha-abc1234` |
| Tag `v1.2.3` | `1.2.3`, `1.2`, `1`, `sha-abc1234` |
| Pull request | Build only, no push |

### 13.4 Release Please Workflow (`.github/workflows/release-please.yml`)

Automates versioning and changelog generation using [release-please](https://github.com/googleapis/release-please). When conventional commits land on `main`, release-please opens/updates a release PR. Merging the release PR creates a GitHub release and tags the commit, which triggers the Docker workflow to publish a versioned image.

```yaml
name: Release Please

on:
  push:
    branches: [main]

permissions:
  contents: write
  pull-requests: write

jobs:
  release-please:
    runs-on: ubuntu-latest
    steps:
      - uses: googleapis/release-please-action@v4
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
```

### 13.5 Release Please Configuration

**`release-please-config.json`:**

```json
{
  "packages": {
    ".": {
      "release-type": "go",
      "changelog-path": "CHANGELOG.md",
      "bump-minor-pre-major": true,
      "bump-patch-for-minor-pre-major": true
    }
  }
}
```

**`.release-please-manifest.json`:**

```json
{
  ".": "0.1.0"
}
```

**Commit convention**: All commits must follow [Conventional Commits](https://www.conventionalcommits.org/). Release-please uses these to determine version bumps:

| Prefix | Version bump | Example |
|---|---|---|
| `fix:` | Patch (0.0.x) | `fix: handle WAV header edge case` |
| `feat:` | Minor (0.x.0) | `feat: add PostgreSQL support` |
| `feat!:` / `BREAKING CHANGE:` | Major (x.0.0) | `feat!: restructure voice alias API` |
| `chore:`, `docs:`, `ci:` | No release | `docs: update README` |

### 13.6 CI/CD Flow Summary

```
Developer pushes to feature branch
  -> PR opened against main
  -> CI workflow: lint + test (Go & frontend)
  -> Docker workflow: build only (validates Dockerfile)

PR merged to main
  -> CI workflow: lint + test
  -> Docker workflow: build + push (tagged "main", "sha-xxx")
  -> Release Please: opens/updates release PR with changelog

Release PR merged
  -> Release Please: creates GitHub release + version tag (v1.2.3)
  -> Docker workflow: build + push (tagged "1.2.3", "1.2", "1", "sha-xxx")
```

---

## 14. Non-Functional Requirements

### 14.1 Performance

- Wyoming event parsing: zero-allocation hot path where possible
- Audio streaming: forward chunks as they arrive, do not buffer entire response
- Startup time: < 1 second (including database migration)
- Memory: < 50MB baseline, scales linearly with concurrent connections
- Binary size: < 30MB (including embedded frontend)

### 14.2 Reliability

- Graceful shutdown on SIGTERM/SIGINT
- Connection timeout handling for both Wyoming and HTTP
- Automatic reconnection for database connections
- Request timeouts for upstream TTS API calls (configurable, default 30s)

### 14.3 Observability

- Structured logging via `log/slog`
- Request logging for both Wyoming and HTTP
- Metrics: synthesis count, latency histogram, error rate (logged, not exported initially)

### 14.4 Security

- API keys stored encrypted at rest in the database
- No authentication on the admin UI initially (assumed to be on a trusted network)
- Input sanitization for all user-provided text
- No shell injection vectors in any code path

---

## 15. Testing Strategy

### 15.1 Go Tests

- **Unit tests**: All packages, especially `wyoming/`, `tts/`, `voice/`, `store/`
- **Integration tests**: Full request flow with mock HTTP server for OpenAI API
- **Database tests**: Both SQLite and PostgreSQL (using testcontainers for Postgres)
- **Coverage target**: > 80%

### 15.2 Frontend Tests

- **Component tests**: Every component with `@testing-library/preact`
- **Integration tests**: Full page rendering with mocked API
- **Coverage target**: 100%

### 15.3 End-to-End Tests

- Wyoming protocol conformance tests using raw TCP connections
- Full synthesis flow with mock TTS API

---

## 16. Future Considerations (Out of Scope for v1)

- Streaming synthesis support (`synthesize-start/chunk/stop`)
- STT (speech-to-text) proxying
- Audio format conversion / resampling
- Response caching for repeated phrases
- Rate limiting per endpoint
- Multiple concurrent synthesis requests per connection
- WebSocket transport for Wyoming
- ARM64 build target
- Authentication for admin UI
- OpenAI Realtime API support
