# TTS Synthesis

Living specification for Meadowlark's text-to-speech synthesis pipeline — the OpenAI-compatible HTTP client, WAV parsing, and proxy orchestration that bridges Wyoming events to TTS API calls.

## Overview

The TTS system receives Wyoming `synthesize` events, resolves the voice configuration, calls an OpenAI-compatible `/audio/speech` endpoint, parses the WAV response, and streams PCM audio back as Wyoming events. The entire pipeline is non-streaming: the full WAV response is buffered before audio chunks are sent.

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

### Synthesize

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
7. **Call TTS API** → Forces `response_format = "wav"`. Returns error if endpoint's `DefaultResponseFormat` is non-empty and not `"wav"`.
8. **Parse WAV header** → `WAVReader.ReadFormat()` extracts `AudioFormat`.
9. **Stream audio chunks**:
   - Send `AudioStart` event with rate, width, channels.
   - Read PCM in 2048-byte chunks, send `AudioChunk` events.
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
| Unsupported response format | `"endpoint ... uses unsupported response format: ..."` |
| TTS API call fails | `"tts api call: ..."` |
| WAV parsing fails | `"parse wav header: ..."` |
| PCM read error | `"read pcm data: ..."` |

### Constants

```go
const chunkSize = 2048  // Bytes per audio-chunk event
```

### Requirements

- The proxy MUST force `response_format = "wav"` regardless of client request.
- Audio MUST be chunked in exactly 2048-byte segments (final chunk MAY be smaller).
- Synthesis errors MUST result in a Wyoming `Error` event, never a crash.
- The connection MUST remain usable after a synthesis error.

### Scenarios

**GIVEN** a synthesis request for voice `"alloy (OpenAI, tts-1)"`,
**WHEN** the TTS endpoint returns a valid WAV response with 4096 bytes of PCM,
**THEN** the proxy MUST send `AudioStart` (with correct format) → 2 `AudioChunk` events (2048 bytes each) → `AudioStop`.

**GIVEN** a synthesis request with an invalid voice alias,
**WHEN** voice resolution fails,
**THEN** the proxy MUST send a Wyoming `Error` event with code `"tts-error"` and a descriptive message.

**GIVEN** a synthesis request to a disabled endpoint,
**WHEN** the endpoint is fetched,
**THEN** the proxy MUST return an error stating the endpoint is disabled.

## Current Limitations

- **No HTTP streaming:** The full WAV response is buffered before chunking begins. The upstream Qwen3-TTS FastAPI supports `stream=true` for PCM/WAV, but Meadowlark does not use it.
- **WAV only:** The proxy rejects non-WAV response formats. MP3/AAC are not supported.
- **Fixed chunk size:** The 2048-byte chunk size is not configurable.
- **No response format negotiation:** `response_format` is always forced to `"wav"`.

## Files

| File | Purpose |
|------|---------|
| `internal/tts/tts.go` | Package declaration |
| `internal/tts/client.go` | OpenAI-compatible HTTP client |
| `internal/tts/proxy.go` | Synthesis proxy orchestration |
| `internal/tts/wav.go` | WAV header parser and PCM reader |

## Changelog

| Date | Description | Document |
|------|-------------|----------|
| 2026-04-19 | Initial living spec created from implementation audit | --- |
