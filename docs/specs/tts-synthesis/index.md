# TTS Synthesis

Living specification for Meadowlark's text-to-speech synthesis pipeline — the OpenAI-compatible HTTP client, WAV parsing, and proxy orchestration that bridges Wyoming events to TTS API calls.

## Overview

The TTS system receives Wyoming synthesis requests, resolves the voice configuration, calls an OpenAI-compatible `/audio/speech` endpoint, and streams PCM audio back as Wyoming events.

Two **upstream** modes are supported, selected per endpoint:

- **Buffered (WAV):** The default. The full WAV response is received, the header is parsed for audio format, and PCM data is chunked into Wyoming events. Works with all endpoints.
- **Streaming (PCM):** Opt-in per endpoint via `StreamingEnabled`. Sends `"stream": true` with `response_format: "pcm"`. Raw PCM bytes are forwarded to Wyoming events as they arrive, reducing time-to-first-audio. Audio format comes from endpoint configuration rather than a WAV header.

Two **inbound** modes are supported, selected by the Wyoming client:

- **Whole-message:** one `synthesize` event carrying the complete text produces one audio group.
- **Segmented streaming:** text arrives incrementally across a session, is aggregated into sentence-sized segments, and each segment produces its own audio group. See [Segmented Streaming Synthesis](#segmented-streaming-synthesis).

The two dimensions are orthogonal and compose freely.

**Package:** `internal/tts/`, with text segmentation in `internal/segment/`

## OpenAI-Compatible HTTP Client

### Client Structure

```go
type Client struct {
    baseURL    string
    apiKey     string
    httpClient *http.Client
}
```

`NewClient(baseURL, apiKey, httpClient)` creates a client. If `httpClient` is nil, `http.DefaultClient` is used.

### Synthesize (Buffered)

`Synthesize(ctx, req) → (io.ReadCloser, error)`

Sends a `POST {baseURL}/audio/speech` request with JSON body:

```go
type SynthesizeRequest struct {
    Model          string   `json:"model"`
    Voice          string   `json:"voice"`
    Input          string   `json:"input"`
    ResponseFormat string   `json:"response_format,omitempty"`
    Speed          *float64 `json:"speed,omitempty"`
    Instructions   *string  `json:"instructions,omitempty"`
}
```

#### Authorization

- If `apiKey` is non-empty, sets `Authorization: Bearer {apiKey}` header.
- If `apiKey` is empty, no authorization header is sent.
- This supports both authenticated (OpenAI) and unauthenticated (local) TTS endpoints.

#### Response Validation

The client validates the response is WAV audio by checking the first 4 bytes for the `"RIFF"` magic:

1. Read first 4 bytes of response body.
2. If `"RIFF"` → WAV response. Reconstruct full stream via `io.MultiReader(header, body)`.
3. If not `"RIFF"` → likely a JSON error. Read up to 4096 bytes, log, and return error.

#### Error Handling

| Scenario | Behavior |
|----------|----------|
| Network error | `fmt.Errorf("tts: send request: %w", err)` |
| Non-2xx status | Read body (truncated to 500 chars), return `"tts: API error %d: %s"` |
| Non-WAV response | Read up to 4096 bytes for diagnostics, return `"tts: endpoint returned non-WAV response: %s"` |

#### Requirements

- Optional fields (`Speed`, `Instructions`, `ResponseFormat`) MUST be omitted from JSON when nil/empty.
- The response body MUST be returned as an `io.ReadCloser` for streaming consumption.
- Error response bodies MUST be truncated to 500 characters for logging.

### SynthesizeStream (Streaming)

`SynthesizeStream(ctx, req) → (io.ReadCloser, error)`

Sends a `POST {baseURL}/audio/speech` request with `"stream": true` in the JSON body:

```go
type StreamSynthesizeRequest struct {
    Model          string   `json:"model"`
    Voice          string   `json:"voice"`
    Input          string   `json:"input"`
    ResponseFormat string   `json:"response_format"`  // MUST be "pcm"
    Speed          *float64 `json:"speed,omitempty"`
    Instructions   *string  `json:"instructions,omitempty"`
    Stream         bool     `json:"stream"`            // Always true
}
```

#### Response Format

The endpoint MUST return raw PCM audio bytes via HTTP chunked transfer encoding:
- **Content-Type:** `audio/pcm` (MAY vary by endpoint; not validated)
- **Format:** 16-bit signed little-endian PCM, mono, at the sample rate configured on the endpoint (typically 24000 Hz)
- **No WAV header** — the response is a flat stream of PCM samples

#### Response Validation

Unlike `Synthesize`, `SynthesizeStream` MUST NOT perform RIFF magic byte validation. The response body is returned directly as an `io.ReadCloser`. Non-2xx status codes MUST still be detected and reported as errors.

#### Requirements

- `SynthesizeStream` MUST set `"stream": true` and `"response_format": "pcm"` in the request body.
- The response body MUST be returned immediately without buffering — the caller reads PCM bytes incrementally.
- Authorization MUST follow the same rules as `Synthesize`.
- Non-2xx responses MUST be reported with the same error format as `Synthesize`.

#### Scenarios

**GIVEN** an endpoint that supports streaming,
**WHEN** `SynthesizeStream` is called,
**THEN** the request body MUST contain `"stream": true` and `"response_format": "pcm"`.

**GIVEN** a streaming endpoint returns a 500 error,
**WHEN** `SynthesizeStream` is called,
**THEN** it MUST return an error with the status code and truncated body.

**GIVEN** a streaming endpoint returns raw PCM bytes,
**WHEN** the caller reads from the returned `io.ReadCloser`,
**THEN** each read MUST return PCM bytes as they arrive from the HTTP response (no buffering).

### ListModels

`ListModels(ctx) → ([]Model, error)`

GET `{baseURL}/models`. Expects OpenAI-style response: `{"data": [{"id": "model-id"}]}`.

**Graceful degradation:** Returns `[]Model{}` (not error) on any failure — network error, non-2xx status, invalid JSON, or null/missing `data` field. This is intentional: model discovery is best-effort.

### ListVoices

`ListVoices(ctx) → ([]Voice, error)`

GET `{baseURL}/audio/voices`. Supports four response formats tried in order:

1. **OpenAI-style:** `{"data": [{"id": "...", "name": "..."}]}`
2. **Generic voices array:** `{"voices": [{"id": "...", "name": "..."}]}` (filters out empty IDs)
3. **Speaches-style:** `{"voices": [{"voice_id": "...", "name": "..."}]}` (maps `voice_id` → `id`)
4. **Plain string array:** `{"voices": ["voice1", "voice2"]}` (creates `Voice{ID: name, Name: name}`)

**Graceful degradation:** Returns `[]Voice{}` on any failure, same as `ListModels`.

#### Scenarios

**GIVEN** an endpoint returns voices in Speaches format,
**WHEN** `ListVoices` is called,
**THEN** it MUST map `voice_id` to `id` and return valid `Voice` entries.

**GIVEN** an endpoint returns a 404 for `/audio/voices`,
**WHEN** `ListVoices` is called,
**THEN** it MUST return an empty slice (not an error).

## WAV Parsing

### AudioFormat

```go
type AudioFormat struct {
    Rate     int  // Sample rate in Hz (e.g., 24000)
    Width    int  // Bytes per sample (e.g., 2 for 16-bit)
    Channels int  // Channel count (e.g., 1 for mono)
}
```

### WAVReader

```go
type WAVReader struct {
    r      io.Reader
    format *AudioFormat
    parsed bool
}
```

`ReadFormat()` parses the RIFF/WAVE header:

1. Read 12-byte RIFF header: `"RIFF"` + size (4 bytes LE) + `"WAVE"`.
2. Iterate chunks by 4-byte ID + 4-byte size:
   - `"fmt "` → extract PCM format (code MUST be 1), channels, sample rate, bits per sample. Calculate `Width = BitsPerSample / 8`.
   - `"data"` → PCM data begins. Wrap remaining reader in `LimitReader` if size is valid.
   - Other chunks (`LIST`, `JUNK`, etc.) → skip with byte-alignment padding (odd sizes padded to even).
3. After `ReadFormat()`, `Read()` returns raw PCM data with no header bytes.

### Requirements

- `ReadFormat()` MUST be called before `Read()`. Calling `Read()` first MUST return an error.
- `ReadFormat()` MUST be idempotent — calling it twice returns the same format.
- The parser MUST handle WAV headers split across TCP read boundaries.
- Non-standard WAV files with extra chunks before the `data` chunk MUST be supported.
- Non-RIFF files, non-WAVE files, and non-PCM format codes MUST be rejected with descriptive errors.
- Streaming WAV (data size 0 or `0x7FFFFFFF`) MUST be supported.

### Scenarios

**GIVEN** a WAV file with a `LIST` chunk between `fmt` and `data`,
**WHEN** `ReadFormat()` is called,
**THEN** it MUST skip the `LIST` chunk and correctly parse the format.

**GIVEN** a WAV header arrives in 1-byte increments (split across TCP reads),
**WHEN** `ReadFormat()` is called,
**THEN** it MUST buffer and correctly parse the complete header.

## Proxy Orchestration

### Proxy Structure

```go
type Proxy struct {
    resolver      *voice.Resolver
    endpoints     EndpointGetter
    clientFactory ClientFactory
    logger        *slog.Logger
}

type ClientFactory func(ep *model.Endpoint) *Client
```

### Synthesis Flow

`HandleSynthesize(ctx, ev *wyoming.Synthesize, w io.Writer)` orchestrates the full pipeline:

1. **Resolve voice** → `resolver.Resolve(ev.Voice)` returns `*model.ResolvedVoice`.
2. **Parse input** → `voice.ParseInput(ev.Text)` extracts overrides from JSON/tag/plain text.
3. **Build alias defaults** if `resolved.IsAlias == true`.
4. **Fetch endpoint** → `endpoints.GetEndpoint(ctx, resolved.EndpointID)`. Error if not found or disabled.
5. **Build endpoint defaults** (speed, instructions from endpoint config).
6. **Merge parameters** → `voice.MergeParams(input, aliasDefaults, endpointDefaults)`. Priority: input > alias > endpoint.
7. **Call TTS API**, branching on `endpoint.StreamingEnabled`:
   - **Streaming:** call `SynthesizeStream` with `response_format: "pcm"` and `stream: true`. `AudioFormat` is `{StreamSampleRate, 2, 1}`, defaulting the rate to `24000` when zero.
   - **Buffered:** call `Synthesize` with `response_format: "wav"`. Returns an error if the endpoint's `DefaultResponseFormat` is non-empty and not `"wav"`. `AudioFormat` comes from `WAVReader.ReadFormat()`.
8. **Stream audio chunks**:
   - Send `AudioStart` event with rate, width, channels.
   - Read PCM in 2048-byte chunks, send `AudioChunk` events.
   - Send `AudioStop` on EOF.

The pipeline is separable into three independently callable stages, so that a segmented streaming session can resolve once and synthesize many times:

| Stage | Steps | Writes to the client? |
|---|---|---|
| **Input parsing** | 2 — `voice.ParseInput` over the complete message text | No |
| **Resolution** | 1, 3–6 — takes the already-parsed overrides | No |
| **Open** | 7 — issue the upstream request and determine the audio format | No |
| **Emission** | 8 — `AudioStart`, `AudioChunk`+, `AudioStop` | Yes |

Two of those boundaries are load-bearing rather than cosmetic:

- **Input parsing is separate from resolution** because a segmented session must run it over the whole message or not at all, never over one segment.
- **Open is separate from emission** because a segment's audio format MUST be known and accepted before its `AudioStart` is written.

Both are explained under [Segmented Streaming Synthesis](#segmented-streaming-synthesis).

### Error Handling

All errors in `doSynthesize` are caught by `HandleSynthesize`, which:
1. Logs the error with `slog.Error`.
2. Sends a Wyoming `Error` event with `Code: "tts-error"` and the error message.
3. Does NOT crash or close the connection.

| Error Scenario | Error Message Pattern |
|----------------|----------------------|
| Voice resolution fails | `"resolve voice: ..."` |
| Endpoint not found | `"get endpoint: ..."` |
| Endpoint disabled | `"endpoint ... is disabled"` |
| Unsupported response format (buffered) | `"unsupported response format ...; only \"wav\" is supported by proxy"` |
| TTS API call fails (buffered) | `"tts api call: ..."` |
| TTS API call fails (streaming) | `"tts api call (streaming): ..."` |
| WAV parsing fails (buffered only) | `"parse wav header: ..."` |
| PCM read error | `"read pcm data: ..."` |
| Segment audio format differs from the session's first segment | `"segment audio format ... differs from session format ..."` |

### Constants

```go
const chunkSize = 2048  // Bytes per audio-chunk event
```

### Requirements

- In buffered mode the proxy MUST force `response_format = "wav"` regardless of client request.
- Audio MUST be chunked in exactly 2048-byte segments (final chunk MAY be smaller).
- Synthesis errors MUST result in a Wyoming `Error` event, never a crash.
- The connection MUST remain usable after a synthesis error.
- Streaming mode MUST be per-endpoint opt-in (default off), MUST use `response_format = "pcm"` and `stream = true`, and MUST derive audio format from endpoint config.

### Scenarios

**GIVEN** a synthesis request for voice `"alloy (OpenAI, tts-1)"`,
**WHEN** the TTS endpoint returns a valid WAV response with 4096 bytes of PCM,
**THEN** the proxy MUST send `AudioStart` (with format from WAV header) → 2 `AudioChunk` events (2048 bytes each) → `AudioStop`.

**GIVEN** a synthesis request with an invalid voice alias,
**WHEN** voice resolution fails,
**THEN** the proxy MUST send a Wyoming `Error` event with code `"tts-error"` and a descriptive message.

**GIVEN** a synthesis request to a disabled endpoint,
**WHEN** the endpoint is fetched,
**THEN** the proxy MUST return an error stating the endpoint is disabled.

**GIVEN** an endpoint that disconnects mid-response,
**WHEN** the proxy reads from the response body,
**THEN** it MUST send `AudioStop` for any audio already sent, then send a Wyoming `Error` event.

## Segmented Streaming Synthesis

When a Wyoming client streams text in rather than sending a whole message (see [wyoming-protocol — Streaming Synthesis Input](../wyoming-protocol/index.md#streaming-synthesis-input)), the proxy synthesizes one segment at a time.

### Text Segmentation

Incoming text is aggregated by a pure, I/O-free segmenter in `internal/segment/`. Segment boundaries are:

- an ASCII sentence-terminating rune — `.` `!` `?` `…` — optionally followed by a run of closing punctuation (`"` `'` `”` `’` `)` `]` `}` `»`), and then followed by whitespace;
- a full-width sentence-terminating rune — `。` `！` `？` — optionally followed by closing punctuation, with no trailing-whitespace requirement, because CJK text is not space-separated;
- a newline;
- a forced break at the maximum length.

Requiring trailing whitespace after an ASCII terminator is what makes the rule safe against partial input: a terminator at the very end of the buffer is not yet a boundary, because the next fragment may continue the token. Full-width terminators need no such guard.

A `.` does not create a boundary when it sits between digits, when the preceding token is a known abbreviation (`mr`, `mrs`, `ms`, `dr`, `prof`, `sr`, `jr`, `st`, `vs`, `etc`, `approx`, `no`, `fig`, `inc`, `ltd`, `co`, `e.g`, `i.e`), or when the preceding token is a single letter. **Token** here means the maximal run of non-whitespace characters immediately preceding the `.`, excluding the `.` itself — so in `e.g.` the token is `e.g`.

Length gating governs when a qualifying boundary actually flushes. The **candidate segment** it measures is everything buffered up to and including the boundary's terminator and any closing-punctuation run, with leading and trailing whitespace trimmed, counted in runes rather than bytes — so `"Hello."` is 6 runes.

| Threshold | Default | Meaning |
|---|---|---|
| `firstSegmentChars` | 24 | Minimum runes before the session's **first** segment may flush |
| `minSegmentChars` | 60 | Minimum runes before any later segment may flush |
| `maxSegmentChars` | 400 | Buffer size at which a break is forced with no boundary |

A forced break prefers, in order: the last soft break (`,` `;` `:` `—` `–` followed by whitespace) at or before the limit; the last whitespace at or before the limit; a rune-aligned hard cut at the limit.

The remainder is flushed unconditionally at the end of a session, unless it is empty or entirely whitespace.

**The tradeoff being tuned:** smaller segments lower time-to-first-audio but cost one upstream HTTP round-trip each and introduce an audible seam at every join, because each request is synthesized with independent prosody. Sentence boundaries put the seam where a speaker would pause anyway. `minSegmentChars = 60` is roughly 4 s of speech, comfortably longer than an upstream time-to-first-byte, so the next segment's request completes while the current one is still playing. `firstSegmentChars` is lower because the opening segment alone determines perceived latency and its own playback covers the next request.

Thresholds are process-level configuration, not per-endpoint: segmentation happens above endpoint resolution, since the first segment must exist before the endpoint is known.

| Flag | Env | Default |
|---|---|---|
| `--synthesize-first-segment-chars` | `MEADOWLARK_SYNTHESIZE_FIRST_SEGMENT_CHARS` | `24` |
| `--synthesize-min-segment-chars` | `MEADOWLARK_SYNTHESIZE_MIN_SEGMENT_CHARS` | `60` |
| `--synthesize-max-segment-chars` | `MEADOWLARK_SYNTHESIZE_MAX_SEGMENT_CHARS` | `400` |
| `--synthesize-session-timeout` | `MEADOWLARK_SYNTHESIZE_SESSION_TIMEOUT` | `30s` |

### Multi-Segment Proxy Behaviour

- Voice resolution and parameter merging run **once per session**; the resulting plan is reused verbatim for every segment.
- Input-override parsing needs the whole message, because `ParseInput`'s JSON form only unmarshals a complete object and its tag form needs its closing bracket. A session whose first non-whitespace character is `{` or `[` therefore buffers the entire message, flushes nothing until the session ends, then parses it and segments the resulting `Input`. Any other session is ordinary prose: override parsing is skipped entirely and text is segmented as it arrives.
- Each segment is **opened** before it is emitted: the upstream request is issued and the audio format determined without writing anything to the client. The session compares that format against the session format and only then emits.
- Segments are emitted in text order by a single emitter, so ordering is structural rather than a timing accident.
- Upstream requests are started ahead of emission, bounded at two in flight — the segment being emitted plus one prefetch — so an upstream's time-to-first-byte is spent while the previous segment is still playing. A prefetch is precisely an early open.

### Requirements

- Thresholds MUST satisfy `0 < firstSegmentChars ≤ minSegmentChars ≤ maxSegmentChars`; a configuration violating that ordering, or carrying a non-positive threshold, MUST log a warning at startup and fall back to all three defaults.
- A forced hard cut MUST be rune-aligned and MUST NOT split a multi-byte rune.
- `voice.ParseInput` MUST be given the complete message text or not run at all; it MUST NOT be run on an individual segment, because a fragment of a JSON-form input parses as plain text and would be spoken verbatim with its overrides dropped.
- The session's endpoint and model MUST be selected from the Wyoming voice name supplied when the session opened, resolved exactly as a whole-message request's voice is. An input-override `voice` MUST NOT participate in that selection — it overrides only the `voice` parameter sent upstream, at input priority.
- Audio for segment N MUST be fully emitted, including its `audio-stop`, before any audio for segment N+1.
- At most two upstream synthesis requests per session MUST be in flight at any time.
- A segment's audio format MUST be determined before any `AudioStart` for that segment is written, so a mismatch can be detected without emitting anything.
- All segments of a session MUST use an identical audio format. In streaming mode this holds by construction from `StreamSampleRate`; in buffered mode the format is parsed per response and MAY differ, in which case the mismatched segment MUST NOT be emitted — the session MUST log at warn, emit a Wyoming `Error` naming both formats, and terminate. Resampling is out of scope.
- Cancelling a session MUST abort in-flight upstream requests and close every held response body, including prefetched segments that were never emitted.

### Scenarios

**GIVEN** a session whose text segments into three parts,
**WHEN** the session completes successfully,
**THEN** exactly three `AudioStart`/`AudioChunk`+/`AudioStop` groups MUST be emitted in text order, followed by one `SynthesizeStopped`.

**GIVEN** a buffered-mode endpoint whose first segment returns 24000 Hz WAV and whose second returns 16000 Hz WAV,
**WHEN** the second segment's header is parsed,
**THEN** no `AudioStart` MUST be emitted for it and the session MUST terminate with a Wyoming `Error` naming both formats.

**GIVEN** a session whose text begins with a parameter override tag or a JSON-form object,
**WHEN** the session ends and its text is segmented,
**THEN** no segment MUST have been flushed before the session ended, the override MUST apply to every segment, and neither the tag nor any JSON syntax MUST appear in any synthesized segment.

**GIVEN** a session with one segment emitting and one prefetched,
**WHEN** the session is cancelled,
**THEN** both response bodies MUST be closed and no further audio MUST be emitted.

## Endpoint Streaming Configuration

The `Endpoint` model includes streaming configuration:

```go
type Endpoint struct {
    // ... existing fields ...
    StreamingEnabled  bool  `json:"streaming_enabled"`   // Opt-in streaming mode
    StreamSampleRate  int   `json:"stream_sample_rate"`  // PCM sample rate (default: 24000)
}
```

### Requirements

- `StreamingEnabled` MUST default to `false` for backwards compatibility.
- `StreamSampleRate` MUST default to `24000` when zero/unset.
- The audio format for streaming is fixed at 16-bit signed LE mono PCM — only the sample rate is configurable.
- These fields MUST be exposed in the HTTP API for endpoint CRUD and in the frontend endpoint form.
- Segmented streaming synthesis MUST work correctly with `StreamingEnabled = false`. The two settings are orthogonal: `StreamingEnabled` governs how a single segment's audio leaves the upstream, segmentation governs how many segments there are. With `StreamingEnabled = false` the latency win is smaller — each segment is one buffered WAV request — but segment 1 still begins as soon as the first segment's worth of text exists, and the cross-segment format-consistency rule becomes load-bearing.

### Audio Format Convention

The de facto standard across OpenAI, Qwen3-TTS, and Speaches for PCM streaming is:

| Parameter | Value |
|-----------|-------|
| Sample rate | 24000 Hz (configurable via `StreamSampleRate`) |
| Bit depth | 16-bit (Width = 2) |
| Encoding | Signed little-endian integer |
| Channels | 1 (mono) |

This format MUST be assumed for all streaming responses. WAV header parsing is not used in streaming mode.

## Files

| File | Purpose |
|------|---------|
| `internal/tts/tts.go` | Package declaration |
| `internal/tts/client.go` | OpenAI-compatible HTTP client (buffered + streaming) |
| `internal/tts/proxy.go` | Synthesis proxy orchestration (resolution and per-segment emission) |
| `internal/tts/stream_session.go` | Segmented streaming session: buffering, ordered pipeline, terminators |
| `internal/tts/wav.go` | WAV header parser and PCM reader (buffered mode only) |
| `internal/segment/segmenter.go` | Pure text segmenter (no I/O) |

## Changelog

| Date | Description | Document |
|------|-------------|----------|
| 2026-04-19 | Initial living spec created from implementation audit | --- |
| 2026-04-19 | Add streaming PCM synthesis mode (spec + changes) | [0001](../../changes/0001-streaming-tts-client.md), [0002](../../changes/0002-streaming-proxy-integration.md) |
| 2026-08-20 | Drop "planned" markers now that 0001/0002 have shipped; add segmented streaming synthesis, text segmentation, and multi-segment proxy behaviour | [0006](../../changes/0006-wyoming-synthesize-streaming.md) |
