# TTS Synthesis

Living specification for Meadowlark's text-to-speech synthesis pipeline — the OpenAI-compatible HTTP client, WAV parsing, and proxy orchestration that bridges Wyoming events to TTS API calls.

## Overview

The TTS system receives Wyoming `synthesize` events, resolves the voice configuration, calls an OpenAI-compatible `/audio/speech` endpoint, and streams PCM audio back as Wyoming events.

Two synthesis modes are supported:

- **Buffered (WAV):** The default. The full WAV response is received, the header is parsed for audio format, and PCM data is chunked into Wyoming events. Works with all endpoints.
- **Streaming (PCM):** Opt-in per endpoint. Sends `"stream": true` with `response_format: "pcm"` to endpoints that support it. Raw PCM bytes are forwarded to Wyoming events as they arrive, reducing time-to-first-audio. Audio format is determined from endpoint configuration rather than a WAV header.

**Package:** `internal/tts/`

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
7. **Select synthesis mode** based on `endpoint.StreamingEnabled`:
   - **Streaming:** Call `client.SynthesizeStream(ctx, req)` with `response_format: "pcm"`. Build `AudioFormat` from endpoint's `StreamSampleRate` (default 24000), width 2, channels 1.
   - **Buffered:** Call `client.Synthesize(ctx, req)` with `response_format: "wav"`. Parse WAV header via `WAVReader.ReadFormat()` to determine `AudioFormat`.
8. **Stream audio chunks**:
   - Send `AudioStart` event with rate, width, channels.
   - Read PCM from the appropriate source (streaming: response body directly, buffered: WAVReader) in 2048-byte chunks, send `AudioChunk` events.
   - Send `AudioStop` on EOF.

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
| Unsupported response format (buffered) | `"endpoint ... uses unsupported response format: ..."` |
| TTS API call fails | `"tts api call: ..."` |
| WAV parsing fails (buffered only) | `"parse wav header: ..."` |
| PCM read error | `"read pcm data: ..."` |

### Constants

```go
const chunkSize = 2048  // Bytes per audio-chunk event
```

### Requirements

- When streaming is disabled, the proxy MUST force `response_format = "wav"`.
- When streaming is enabled, the proxy MUST use `response_format = "pcm"` and `stream = true`.
- Audio MUST be chunked in exactly 2048-byte segments (final chunk MAY be smaller).
- Synthesis errors MUST result in a Wyoming `Error` event, never a crash.
- The connection MUST remain usable after a synthesis error.
- Streaming mode MUST be a per-endpoint opt-in — it MUST NOT be the default.

### Scenarios

**GIVEN** a synthesis request to an endpoint with `StreamingEnabled: false`,
**WHEN** the TTS endpoint returns a valid WAV response with 4096 bytes of PCM,
**THEN** the proxy MUST send `AudioStart` (with format from WAV header) → 2 `AudioChunk` events (2048 bytes each) → `AudioStop`.

**GIVEN** a synthesis request to an endpoint with `StreamingEnabled: true` and `StreamSampleRate: 24000`,
**WHEN** the TTS endpoint returns streaming PCM bytes,
**THEN** the proxy MUST immediately send `AudioStart` (rate=24000, width=2, channels=1) and forward PCM chunks as they arrive from the HTTP response.

**GIVEN** a synthesis request with an invalid voice alias,
**WHEN** voice resolution fails,
**THEN** the proxy MUST send a Wyoming `Error` event with code `"tts-error"` and a descriptive message.

**GIVEN** a synthesis request to a disabled endpoint,
**WHEN** the endpoint is fetched,
**THEN** the proxy MUST return an error stating the endpoint is disabled.

**GIVEN** a streaming endpoint that disconnects mid-response,
**WHEN** the proxy reads from the response body,
**THEN** it MUST send `AudioStop` for any audio already sent, then send a Wyoming `Error` event.

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
| `internal/tts/proxy.go` | Synthesis proxy orchestration |
| `internal/tts/wav.go` | WAV header parser and PCM reader (buffered mode only) |

## Changelog

| Date | Description | Document |
|------|-------------|----------|
| 2026-04-19 | Initial living spec created from implementation audit | --- |
| 2026-04-19 | Add streaming PCM synthesis mode (spec + changes) | [0001](../../changes/0001-streaming-tts-client.md), [0002](../../changes/0002-streaming-proxy-integration.md) |
