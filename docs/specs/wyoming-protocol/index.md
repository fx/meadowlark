# Wyoming Protocol

Living specification for Meadowlark's Wyoming protocol implementation — the TCP server, wire format, event types, voice discovery info builder, and Zeroconf/mDNS service registration.

## Overview

Meadowlark implements a Wyoming protocol v1.8.0 TCP server that bridges Home Assistant voice requests to OpenAI-compatible TTS endpoints. The server accepts concurrent connections, dispatches events to a pluggable handler, and streams audio back as PCM chunks.

**Package:** `internal/wyoming/`

## Wire Format

### Three-Part Message Structure

Every Wyoming message consists of three parts transmitted sequentially over TCP:

1. **JSON header line** — terminated by `\n`
2. **Optional JSON data bytes** — length specified in the header's `data_length` field
3. **Optional raw binary payload** — length specified in the header's `payload_length` field

### Header Schema

```go
type header struct {
    Type          string         `json:"type"`
    Version       string         `json:"version"`       // Always "1.8.0"
    DataLength    int            `json:"data_length"`
    PayloadLength int            `json:"payload_length"`
    Data          map[string]any `json:"data,omitempty"` // Inline data (if DataLength == 0)
}
```

### Read/Write Contract

- `ReadEvent(reader)` reads: JSON header `\n` → data bytes (if `data_length > 0`) → payload bytes (if `payload_length > 0`).
- `WriteEvent(writer, event)` writes the reverse. Data is always written externally (never inlined in the header) for unambiguous framing.
- External data bytes override inline header `Data` when both are present.

### Requirements

- The protocol version MUST be `"1.8.0"`.
- Empty data/payload fields MUST be omitted from output (not sent as zero-length).
- Payloads up to 1MB+ MUST be supported (tested with 1MB audio chunks).
- Header parsing MUST handle data split across TCP read boundaries.

### Scenarios

**GIVEN** a Wyoming client sends a well-formed event with JSON data and a binary payload,
**WHEN** `ReadEvent` parses the stream,
**THEN** it MUST return an `Event` with the correct `Type`, `Data`, and `Payload` fields.

**GIVEN** an `Event` is written via `WriteEvent`,
**WHEN** it is read back via `ReadEvent`,
**THEN** the round-trip MUST produce an identical event.

## Event Types

### Constants

| Constant | Wire Type | Purpose |
|----------|-----------|---------|
| `TypeDescribe` | `"describe"` | Client requests service capabilities |
| `TypeInfo` | `"info"` | Server responds with capabilities |
| `TypeSynthesize` | `"synthesize"` | Client requests TTS synthesis |
| `TypeAudioStart` | `"audio-start"` | Server begins audio stream |
| `TypeAudioChunk` | `"audio-chunk"` | Server sends PCM audio data |
| `TypeAudioStop` | `"audio-stop"` | Server ends audio stream |
| `TypePing` | `"ping"` | Health check request |
| `TypePong` | `"pong"` | Health check response |
| `TypeError` | `"error"` | Error notification |

### Message Types

#### Describe / Info

`Describe` has no fields. The server responds with an `Info` event containing a `TtsProgram` with all available voices.

```go
type Info struct {
    Tts []TtsProgram
}

type TtsProgram struct {
    Name        string
    Description string
    Installed   bool
    Version     string
    Voices      []TtsVoice
    Attribution map[string]any  // REQUIRED by Home Assistant
}

type TtsVoice struct {
    Name        string
    Description string
    Installed   bool
    Languages   []string
    Speakers    []TtsVoiceSpeaker
    Attribution map[string]any  // REQUIRED by Home Assistant
}
```

**Home Assistant Compatibility Requirements:**

- `Attribution` MUST be present on both `TtsProgram` and `TtsVoice` (required by HA's `DataClassJsonMixin`).
- `Speakers` MUST be omitted entirely when empty (not serialized as `null` or `[]`).

#### Synthesize

```go
type Synthesize struct {
    Text     string
    Voice    string
    Speaker  string
    Language string
}
```

Wire format maps voice to nested object: `{"voice": {"name": "alloy"}}`.

#### Audio Events

```go
type AudioStart struct {
    Rate     int  // Sample rate in Hz (e.g., 24000)
    Width    int  // Bytes per sample (e.g., 2 for 16-bit)
    Channels int  // Channel count (1=mono)
}

type AudioChunk struct {
    Rate, Width, Channels int
    Audio                 []byte  // Raw PCM in Event.Payload
}

type AudioStop struct{}
```

#### Ping / Pong / Error

```go
type Ping struct{}
type Pong struct{}

type Error struct {
    Text string
    Code string
}
```

## TCP Server

### Architecture

```go
type Server struct {
    addr     string
    handler  Handler
    logger   *slog.Logger
    listener net.Listener
    conns    map[net.Conn]struct{}  // Active connection tracking
    wg       sync.WaitGroup         // Drain on shutdown
}

type Handler interface {
    HandleEvent(ctx context.Context, ev *Event, w io.Writer) error
}
```

### Connection Lifecycle

1. `ListenAndServe(ctx)` accepts TCP connections in a loop.
2. Each connection spawns a goroutine running `handleConn()`.
3. `handleConn` reads events via `bufio.Reader` → `ReadEvent()`.
4. Events are dispatched to `Handler.HandleEvent()`.
5. If the handler returns an error, an `Error` event is written to the client; the connection persists.
6. On EOF or connection reset, the connection is cleaned up silently.

### Requirements

- The server MUST support multiple concurrent clients (one goroutine per connection).
- Handler errors MUST NOT close the connection — an `Error` event MUST be sent and the connection MUST continue accepting events.
- Connection resets (`ECONNRESET`, `use of closed network connection`) MUST be handled gracefully without error logging.
- `Shutdown()` MUST close the listener, close all active connections, and wait for all goroutines to complete.

### Scenarios

**GIVEN** a Wyoming client sends a `ping` event,
**WHEN** the server processes it,
**THEN** the server MUST respond with a `pong` event.

**GIVEN** 5 concurrent clients connect and send events simultaneously,
**WHEN** the server processes them,
**THEN** all clients MUST receive correct responses without interference.

**GIVEN** the server is shutting down with active connections,
**WHEN** `Shutdown()` is called,
**THEN** all connections MUST be closed and all goroutines MUST complete before `Shutdown()` returns.

## Event Handler Routing

The `wyomingHandler` in `cmd/meadowlark/main.go` routes events:

| Event Type | Action |
|------------|--------|
| `describe` | Build `Info` via `InfoBuilder.Build(ctx)` and write response |
| `synthesize` | Delegate to `tts.Proxy.HandleSynthesize()` |
| `ping` | Respond with `pong` |
| Unknown | Log at debug level, ignore |

## Info Builder

### Purpose

Aggregates voices from all enabled TTS endpoints and voice aliases into a single `Info` response for Wyoming `describe` requests.

```go
type InfoBuilder struct {
    endpoints  EndpointLister
    aliases    AliasLister
    discoverer VoiceDiscoverer
    version    string
    cache      *Info  // Protected by sync.RWMutex
}
```

### Voice Discovery Process

1. List all enabled endpoints from the database.
2. For each endpoint, call `VoiceDiscoverer.DiscoverVoices(ctx, endpoint)` in parallel with a 5-second timeout per endpoint.
3. If voice discovery returns results, create canonical voice entries for each `voice x model` combination.
4. If voice discovery fails (returns nil), fall back to using model names as voice entries.
5. Append all enabled voice aliases to the voice list.

### Canonical Voice Naming

Canonical voice names follow the format: `"<voice_id> (<endpoint_name>, <model_name>)"`.

Example: `"alloy (OpenAI, tts-1)"`.

### Requirements

- Voice discovery MUST run in parallel across all endpoints with a 5-second timeout per endpoint.
- Discovery failure for one endpoint MUST NOT prevent other endpoints from being included.
- The `Cached()` method MUST return the last successfully built `Info`, or nil if never built.
- `Build()` MUST be called after endpoint/alias mutations to refresh the cache.

## Zeroconf / mDNS

### Service Registration

```go
type ZeroconfService struct {
    server *zeroconf.Server
    logger *slog.Logger
}
```

- Registers a `_wyoming._tcp.local.` service via `github.com/grandcat/zeroconf`.
- Service name defaults to the system hostname; configurable via `--zeroconf-name`.
- Disabled entirely via `--no-zeroconf`.
- `Shutdown()` deregisters the service and is idempotent.

### Scenarios

**GIVEN** Zeroconf is enabled,
**WHEN** the server starts,
**THEN** a `_wyoming._tcp.local.` service MUST be advertised on the Wyoming port.

**GIVEN** `--no-zeroconf` is set,
**WHEN** the server starts,
**THEN** no mDNS service MUST be registered.

## Files

| File | Purpose |
|------|---------|
| `internal/wyoming/wyoming.go` | Package declaration |
| `internal/wyoming/event.go` | Wire format `ReadEvent`/`WriteEvent` |
| `internal/wyoming/types.go` | Event type constants and message structs |
| `internal/wyoming/server.go` | TCP server and connection handling |
| `internal/wyoming/info.go` | Info builder and voice discovery aggregation |
| `internal/wyoming/zeroconf.go` | mDNS service registration |
| `cmd/meadowlark/main.go` | Event handler routing (`wyomingHandler`) |

## Changelog

| Date | Description | Document |
|------|-------------|----------|
| 2026-04-19 | Initial living spec created from implementation audit | --- |
