# Wyoming Protocol

Living specification for Meadowlark's Wyoming protocol implementation — the TCP server, wire format, event types, voice discovery info builder, and Zeroconf/mDNS service registration.

## Overview

Meadowlark implements a Wyoming protocol v1.8.0 TCP server that bridges Home Assistant voice requests to OpenAI-compatible TTS endpoints. The server accepts concurrent connections, dispatches events to a per-connection handler, and streams audio back as PCM chunks.

Two synthesis input modes are supported:

- **Whole-message (`synthesize`)** — the client sends one event carrying the complete text and receives one `audio-start`/`audio-chunk*`/`audio-stop` group.
- **Streaming (`synthesize-start`/`synthesize-chunk`/`synthesize-stop`)** — the client streams text in as it is produced. Meadowlark segments it, synthesizes each segment, emits one audio group per segment, and terminates the session with `synthesize-stopped`.

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
- `WriteEvent` MUST assemble the complete message into a single buffer and issue exactly one `Write` call on the underlying writer. This is what makes an event atomic when several goroutines share one connection; see [Write Serialization](#write-serialization).

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
| `TypeSynthesize` | `"synthesize"` | Client requests TTS synthesis for a whole message |
| `TypeSynthesizeStart` | `"synthesize-start"` | Client opens a streaming synthesis session |
| `TypeSynthesizeChunk` | `"synthesize-chunk"` | Client sends a fragment of text into the open session |
| `TypeSynthesizeStop` | `"synthesize-stop"` | Client signals no further text |
| `TypeSynthesizeStopped` | `"synthesize-stopped"` | Server terminates a completed streaming session |
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
    Name                       string
    Description                string
    Installed                  bool
    Version                    string
    Voices                     []TtsVoice
    SupportsSynthesizeStreaming bool
}

type TtsVoice struct {
    Name        string
    Description string
    Installed   bool
    Languages   []string
    Speakers    []TtsVoiceSpeaker
}
```

`Attribution` is not a struct field; `Info.ToEvent()` emits a constant `attribution` object on every program and voice.

**Home Assistant Compatibility Requirements:**

- An `attribution` object MUST be present on both `tts[]` entries and voice entries (required by HA's `DataClassJsonMixin`).
- `Speakers` MUST be omitted entirely when empty (not serialized as `null` or `[]`).
- `SupportsSynthesizeStreaming` MUST be serialized as `supports_synthesize_streaming` on each `tts[]` entry, and MUST be parsed symmetrically by `InfoFromEvent`, defaulting to `false` when the key is absent.
- Meadowlark MUST advertise `supports_synthesize_streaming: true` unconditionally. The flag describes what the Wyoming service accepts on its input side, which Meadowlark can always satisfy by segmenting text, and `info` carries a single service-level program aggregating every endpoint — so there is no per-endpoint answer to give.

Home Assistant reads this flag to choose between its buffering `async_get_tts_audio()` path and its streaming `async_stream_tts_audio()` path.

#### Synthesize

```go
type Synthesize struct {
    Text     string
    Voice    string
    Speaker  string
    Language string
}
```

Wire format nests **only** the voice name: `{"text": ..., "voice": {"name": "alloy"}, "speaker": ..., "language": ...}`. `speaker` and `language` sit at the top level of the data object, not inside `voice`, and each optional field is omitted when empty.

A `synthesize` event received while a streaming session is open on the same connection is suppressed; see [Streaming Synthesis Input](#streaming-synthesis-input).

#### Streaming Synthesis Events

```go
type SynthesizeStart struct {
    Voice      string
    Language   string
    Speaker    string
    TextFormat string  // "text" (default) or "ssml"
    Context    any     // opaque, round-tripped unchanged
}

type SynthesizeChunk struct {
    Text string
}

type SynthesizeStop struct{}
type SynthesizeStopped struct{}
```

- `SynthesizeStart` nests all three voice fields under `voice` — `{"voice": {"name": ..., "language": ..., "speaker": ...}}` — omitting each field when empty, and omitting the object itself only when all three are empty. A start event carrying only `language` or only `speaker` is valid and MUST survive a round trip.
- **`Synthesize` and `SynthesizeStart` do not share a voice encoding.** `Synthesize` nests only `name` under `voice` and emits `speaker` and `language` at the top level of its data object. That is the shape existing Wyoming clients speak and it MUST NOT be changed to match `SynthesizeStart`; the two are encoded separately.
- `text_format: "ssml"` is accepted without error and treated as plain text. SSML rendering is not supported.
- `Context` is round-tripped but otherwise unused.
- `SynthesizeStopped` is the only one of the four sent server → client.

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

// HandlerFactory produces a fresh Handler for each accepted connection.
type HandlerFactory interface {
    NewConnHandler() Handler
}

// ConnHandler is implemented by per-connection handlers that need to release
// resources when their connection is torn down.
type ConnHandler interface {
    CloseConn()
}
```

`HandlerFactory` and `ConnHandler` are optional interfaces. A `Handler` that implements neither — including `HandlerFunc` — is used as a process-wide singleton exactly as before they existed.

### Connection Lifecycle

1. `ListenAndServe(ctx)` accepts TCP connections in a loop.
2. Each connection spawns a goroutine running `handleConn()`.
3. If the server's handler implements `HandlerFactory`, `handleConn` calls `NewConnHandler()` exactly once and dispatches that connection's events to the result; otherwise it dispatches to the shared handler.
4. `handleConn` wraps the connection in a mutex-guarded writer and reads events via `bufio.Reader` → `ReadEvent()`.
5. Events are dispatched to `Handler.HandleEvent()`, receiving the guarded writer.
6. If the handler returns an error, an `Error` event is written to the client through the same guarded writer; the connection persists.
7. On EOF or connection reset, the connection is cleaned up: `CloseConn()` is called if the per-connection handler implements `ConnHandler`, the connection is closed, and it is removed from the active set.

### Write Serialization

A streaming session writes audio events from a background goroutine while the connection's read loop may concurrently write `pong` or `error` events to the same `net.Conn`. Wyoming events are multi-part, so an interleaved write corrupts framing for every event that follows.

Two properties together make every emitted event atomic:

- `WriteEvent` issues exactly one `Write` call per event (see [Read/Write Contract](#readwrite-contract)).
- The server passes handlers a mutex-guarded writer wrapping the connection, and uses that same wrapper for the error events it writes itself.

Neither is sufficient alone: a mutex around a multi-`Write` `WriteEvent` still interleaves, and one-`Write`-per-event without a mutex still depends on the underlying writer's own atomicity.

### Requirements

- The server MUST support multiple concurrent clients (one goroutine per connection).
- Handler errors MUST NOT close the connection — an `Error` event MUST be sent and the connection MUST continue accepting events.
- Connection resets (`ECONNRESET`, `use of closed network connection`) MUST be handled gracefully without error logging.
- `Shutdown()` MUST close the listener, close all active connections, and wait for all goroutines to complete.
- When the handler implements `HandlerFactory`, `NewConnHandler()` MUST be called exactly once per accepted connection, and no two connections MUST share a handler instance.
- When a per-connection handler implements `ConnHandler`, `CloseConn()` MUST be called exactly once per connection, before that connection's goroutine exits.
- `CloseConn()` MUST block until the connection's background work has finished, so `Shutdown()` drains in-flight synthesis rather than abandoning it.
- Every event written to a connection MUST be atomic with respect to any other goroutine writing to the same connection.

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

**GIVEN** a handler implementing `HandlerFactory` and `ConnHandler`,
**WHEN** three clients connect and then disconnect,
**THEN** `NewConnHandler()` MUST have been called three times and `CloseConn()` MUST have been called three times, once per connection.

## Event Handler Routing

The `wyomingHandler` in `cmd/meadowlark/main.go` implements `HandlerFactory`. It produces a `connHandler` per connection, holding that connection's streaming session and delegating everything else to the process-wide `InfoBuilder` and `tts.Proxy`:

| Event Type | Action |
|------------|--------|
| `describe` | Build `Info` via `InfoBuilder.Build(ctx)` and write response |
| `synthesize-start` | Open the connection's streaming session |
| `synthesize-chunk` | Feed text into the open session |
| `synthesize` | Suppressed if a session is open; otherwise delegate to `tts.Proxy.HandleSynthesize()` |
| `synthesize-stop` | Flush and terminate the open session |
| `ping` | Respond with `pong` |
| Unknown | Log at debug level, ignore |

## Streaming Synthesis Input

Home Assistant's Wyoming TTS entity chooses its path from `supports_synthesize_streaming`. On the streaming path (`async_stream_tts_audio()`) it sends, in order:

```
synthesize-start {voice}
synthesize-chunk {text}      × N, as the LLM produces text
synthesize       {text: <entire message>, voice}    ← backwards compatibility
synthesize-stop
```

and reads back, per synthesized segment, `audio-start` / `audio-chunk`+ / `audio-stop`, terminated by a single `synthesize-stopped`.

Two Home Assistant behaviours constrain what Meadowlark may emit:

- **Only the first `audio-start` is honoured.** HA writes its WAV header from the first one it sees, sets a flag, and ignores every later one. Every segment of a session therefore MUST use an identical rate, width, and channel count.
- **`synthesize-stopped` breaks HA's read loop; `error` raises.** HA does not break on `audio-stop`.

### Session Semantics

A connection holds at most one streaming session, in one of three states.

| Event | `idle` (no session) | `open` | `terminated` (errored, awaiting `synthesize-stop`) |
|---|---|---|---|
| `synthesize-start` | Open a session; record voice, language, speaker, text format. | Quiesce the current session (below), emit `synthesize-stopped`, then open a new one. | Discard the tombstone and open a new session. |
| `synthesize-chunk` | Ignore; log at debug. | Append text and flush any completed segments. | Absorb silently. |
| `synthesize` | Handled as a whole-message request, unchanged. | Suppressed: recorded as fallback text, never synthesized directly. | Absorb silently; never handled as a whole-message request. |
| `synthesize-stop` | Ignore; emit nothing. | Flush the remainder, wait for all segments to be emitted, emit `synthesize-stopped`, return to `idle`. | Emit nothing; return to `idle`. |

### Quiescing a Session

Every path that ends a session early — a restart, a failure, the idle timeout, connection teardown — performs the same ordered shutdown before anything further is written to the connection:

1. Cancel the session context, aborting in-flight and prefetched upstream requests and closing every held response body.
2. Write the `audio-stop` for any segment whose `audio-start` was written but whose group is not yet closed.
3. Wait for the emitter to exit; no further audio event for that session may be written afterwards.
4. Discard buffered text and queued segments.

Only then may a terminator be written or a replacement session open. On connection teardown step 2 and the terminator are skipped, since nothing may be written to a closed connection; steps 1, 3 and 4 still run.

- Quiescing MUST complete before a `synthesize-stopped` or `error` is written, and before a replacement session opens.
- No audio event belonging to an ended session MUST be written after its terminator, or interleaved with a later session's audio.

The `terminated` state exists because Home Assistant sends its compatibility `synthesize` after the chunks and before `synthesize-stop`. A session that failed early and simply closed would let that event fall through to the whole-message path and speak the entire message a second time, after the client had already raised on the `error`.

### Requirements

- A `synthesize` event received while a session is open MUST NOT produce audio; its text MUST be recorded as the session's fallback text.
- If a session ends having received zero `synthesize-chunk` events and holding fallback text, that fallback text MUST be synthesized as the session's content — exactly once, and via the same whole-message override detection that chunked text receives, so a fallback carrying a tag or JSON form has its overrides applied rather than spoken.
- A `synthesize` event received in the `idle` state MUST be handled exactly as a whole-message request, with no behavioural difference from a connection that never uses streaming. A `terminated` session is not idle and absorbs the event instead.
- Each synthesized segment MUST be framed as `audio-start`, one or more `audio-chunk`, `audio-stop`.
- Exactly one `synthesize-stopped` MUST terminate a successful session, after the final segment's `audio-stop`. It MUST NOT be emitted per segment.
- All segments within a session MUST use an identical audio format.
- A session MUST emit either a Wyoming `error` or a `synthesize-stopped`, never both, and never more than one of either. Emitting both would leave an unconsumed terminator that the next stream would read as an immediate end-of-stream.
- A session terminated by an error MUST silently absorb **every** remaining event of that message — further `synthesize-chunk` events, the compatibility `synthesize`, and the trailing `synthesize-stop` — and MUST NOT hand any of them to the whole-message path.
- When a segment fails after its `audio-start` was emitted, `audio-stop` MUST be emitted for that segment before the `error`.
- A synthesis error MUST NOT close the connection.

### Per-Connection Session State

Session state is a field on the per-connection handler produced by `HandlerFactory`, not an entry in a shared map. That is what makes cleanup possible: `HandleEvent` is never called again after a client disconnects, so a map-keyed design has no point at which to cancel in-flight upstream requests, and `ConnHandler.CloseConn()` is called from the same teardown path that closes the connection.

- The session's context MUST derive from the `ctx` passed to `HandleEvent` at `synthesize-start`, via `context.WithCancel`.
- Cancelling that context MUST abort in-flight upstream HTTP requests and close their response bodies.
- Connection teardown MUST cancel the session; server shutdown MUST cancel it through the parent context.
- A session MUST run an idle timer, armed when the session opens, reset by every subsequent client event belonging to that session — each `synthesize-chunk` and the compatibility `synthesize` — and disarmed by `synthesize-stop`. Meadowlark's own progress MUST NOT reset it.
- When the idle timer expires the session MUST be abandoned: quiesce, emit an `error` with code `synthesize-timeout`, and enter the `terminated` state without emitting `synthesize-stopped`. It MUST NOT return directly to `idle` — a timed-out session is an errored session, so its tombstone MUST keep absorbing until `synthesize-stop`, or a late compatibility `synthesize` would reach the whole-message path and speak the message after the timeout error. A timeout of `0` disables the timer entirely; a negative timeout is rejected at startup with a warning and the default is used, since a negative duration would otherwise fire the timer immediately and fail every session.

### Scenarios

**GIVEN** a Wyoming client sends `synthesize-start`, three `synthesize-chunk` events, a full `synthesize`, and `synthesize-stop`,
**WHEN** the text segments into two segments,
**THEN** the emitted sequence MUST be exactly `audio-start`, `audio-chunk`+, `audio-stop`, `audio-start`, `audio-chunk`+, `audio-stop`, `synthesize-stopped` — with the `synthesize` event contributing no audio.

**GIVEN** a client sends `synthesize-start`, no `synthesize-chunk`, a `synthesize` carrying `"Hello world."`, and `synthesize-stop`,
**WHEN** the session terminates,
**THEN** `"Hello world."` MUST be synthesized exactly once.

**GIVEN** a connection on which no `synthesize-start` has ever been sent,
**WHEN** a `synthesize` event arrives,
**THEN** the response MUST be `audio-start`, `audio-chunk`+, `audio-stop`, with no `synthesize-stopped`.

**GIVEN** an open session whose second segment's upstream returns an error before any audio,
**WHEN** the failure is observed,
**THEN** the first segment's group MUST have been emitted in full, exactly one `error` MUST be emitted, no `synthesize-stopped` MUST be emitted, and the connection MUST remain open.

**GIVEN** an open session with a segment being streamed,
**WHEN** the client's connection drops,
**THEN** the in-flight upstream request MUST be aborted and `CloseConn()` MUST NOT return until the session's goroutines have exited.

## Info Builder

### Purpose

Aggregates persisted enabled voices from all enabled TTS endpoints, plus voice aliases, into a single `Info` response for Wyoming `describe` requests.

```go
type InfoBuilder struct {
    endpoints      EndpointLister
    aliases        AliasLister
    endpointVoices EndpointVoiceLister
    version        string
    cache          *Info  // Protected by sync.RWMutex
}
```

### Voice List Assembly

1. List all endpoints from the database and skip the disabled ones.
2. For each enabled endpoint, list its persisted `endpoint_voices` rows and skip the disabled ones (see [voice-resolution — Enabled Models and Voices](../voice-resolution/index.md#enabled-models-and-voices)).
3. Create a canonical voice entry for each enabled `voice × enabled model` combination.
4. Append all enabled voice aliases to the voice list.

Assembly reads persisted state only; it does not live-probe upstream endpoints.

### Canonical Voice Naming

Canonical voice names follow the format: `"<voice_id> (<endpoint_name>, <model_name>)"`.

Example: `"alloy (OpenAI, tts-1)"`.

### Requirements

- Only voices whose `endpoint_voices` row is enabled, on an enabled endpoint, MUST appear in the canonical voice list.
- `Build()` MUST NOT live-probe upstream endpoints.
- The `Cached()` method MUST return the last successfully built `Info`, or nil if never built.
- `Build()` MUST be called after endpoint/alias mutations to refresh the cache.
- The built `TtsProgram` MUST set `SupportsSynthesizeStreaming: true`, regardless of endpoint configuration or how many endpoints exist.

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
| `internal/wyoming/server.go` | TCP server, per-connection handler construction, write serialization |
| `internal/wyoming/info.go` | Info builder and voice discovery aggregation |
| `internal/wyoming/zeroconf.go` | mDNS service registration |
| `cmd/meadowlark/main.go` | Event handler routing (`wyomingHandler`, `connHandler`) |

## Changelog

| Date | Description | Document |
|------|-------------|----------|
| 2026-04-19 | Initial living spec created from implementation audit | --- |
| 2026-08-20 | Add streaming synthesis input: four event types, `supports_synthesize_streaming`, per-connection handlers and session state, write serialization | [0006](../../changes/0006-wyoming-synthesize-streaming.md) |
