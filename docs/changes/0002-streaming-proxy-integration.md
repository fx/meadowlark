# 0002: Streaming Proxy Integration

**Spec:** [tts-synthesis](../specs/tts-synthesis/), [data-persistence](../specs/data-persistence/), [http-api](../specs/http-api/), [frontend](../specs/frontend/)
**Status:** draft
**Depends on:** [0001-streaming-tts-client](0001-streaming-tts-client.md)

## Summary

Wire the streaming TTS client into the synthesis proxy, add per-endpoint streaming configuration (database + API + frontend), and update the proxy to select streaming vs buffered mode based on endpoint config.

## Background

Change 0001 adds `SynthesizeStream` to the TTS client. This change integrates it into the full pipeline:

1. **Endpoint model** — add `StreamingEnabled` (bool) and `StreamSampleRate` (int) fields.
2. **Database** — add columns via alter migration.
3. **HTTP API** — expose new fields in endpoint CRUD.
4. **Proxy** — branch on `ep.StreamingEnabled` to call `SynthesizeStream` or `Synthesize`.
5. **Frontend** — add streaming toggle and sample rate input to the endpoint form.

## Design

### Endpoint Model Changes

```go
type Endpoint struct {
    // ... existing fields ...
    StreamingEnabled  bool  `json:"streaming_enabled"`
    StreamSampleRate  int   `json:"stream_sample_rate"`
}
```

- `StreamingEnabled` defaults to `false` — streaming is opt-in.
- `StreamSampleRate` defaults to `24000` when zero/unset. Validated at the proxy level, not the model level.

### Database Migration

**SQLite:**
```sql
ALTER TABLE endpoints ADD COLUMN streaming_enabled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE endpoints ADD COLUMN stream_sample_rate INTEGER NOT NULL DEFAULT 24000;
```

**PostgreSQL:**
```sql
ALTER TABLE endpoints ADD COLUMN IF NOT EXISTS streaming_enabled BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE endpoints ADD COLUMN IF NOT EXISTS stream_sample_rate INTEGER NOT NULL DEFAULT 24000;
```

Both are idempotent alter migrations appended to the existing migration list.

### Proxy Changes

In `doSynthesize()`, after step 6 (merge parameters), replace the current response format check and client call with:

```go
var body io.ReadCloser
var format *AudioFormat

if ep.StreamingEnabled {
    // Streaming mode: PCM, no WAV header
    streamReq := &StreamSynthesizeRequest{
        Model:          merged.Model,
        Voice:          merged.Voice,
        Input:          merged.Input,
        ResponseFormat: "pcm",
        Speed:          merged.Speed,
        Instructions:   merged.Instructions,
        Stream:         true,
    }
    body, err = client.SynthesizeStream(ctx, streamReq)
    if err != nil {
        return fmt.Errorf("tts api call (streaming): %w", err)
    }

    sampleRate := ep.StreamSampleRate
    if sampleRate == 0 {
        sampleRate = 24000
    }
    format = &AudioFormat{Rate: sampleRate, Width: 2, Channels: 1}
} else {
    // Buffered mode: WAV response (existing behavior)
    // ... existing code unchanged ...
}
```

The PCM chunking loop remains identical — it reads from either `body` (streaming) or `wavReader` (buffered) into 2048-byte chunks.

### HTTP API Changes

The endpoint create/update request structs in `internal/api/endpoints.go` gain two new optional fields:

```go
StreamingEnabled  *bool `json:"streaming_enabled,omitempty"`
StreamSampleRate  *int  `json:"stream_sample_rate,omitempty"`
```

**Validation:**
- `stream_sample_rate`: if present, MUST be in range 8000–48000.
- No validation on `streaming_enabled` (it's a bool).

### Frontend Changes

The `EndpointForm` component (`web/src/components/endpoint-form.tsx`) gains:
- A "Streaming" switch toggle (`streaming_enabled`)
- A "Sample Rate" number input (`stream_sample_rate`), shown only when streaming is enabled, defaulting to 24000, range 8000–48000

The API client types in `web/src/lib/api.ts` gain the two new fields on `Endpoint`, `CreateEndpoint`, and `UpdateEndpoint`.

## Requirements

- Streaming MUST be opt-in per endpoint — `StreamingEnabled` MUST default to `false`.
- When `StreamingEnabled` is `true`, the proxy MUST call `SynthesizeStream` with `response_format: "pcm"`.
- When `StreamingEnabled` is `false`, the proxy MUST call `Synthesize` with `response_format: "wav"` (existing behavior, unchanged).
- `StreamSampleRate` MUST default to `24000` when zero or unset.
- The `audio-start` event for streaming mode MUST use `rate = StreamSampleRate`, `width = 2`, `channels = 1`.
- The database migration MUST be idempotent.
- The frontend MUST only show the sample rate field when streaming is enabled.
- Existing endpoints with no streaming config MUST continue to work identically (buffered WAV mode).

## Scenarios

**GIVEN** an endpoint with `StreamingEnabled: false` (default),
**WHEN** a synthesis request is processed,
**THEN** the proxy MUST use `Synthesize` with WAV and parse the WAV header — behavior identical to before this change.

**GIVEN** an endpoint with `StreamingEnabled: true` and `StreamSampleRate: 24000`,
**WHEN** a synthesis request arrives,
**THEN** the proxy MUST call `SynthesizeStream`, send `AudioStart{Rate: 24000, Width: 2, Channels: 1}`, and forward PCM chunks as they arrive.

**GIVEN** an endpoint with `StreamingEnabled: true` and `StreamSampleRate: 0` (unset),
**WHEN** the proxy builds the audio format,
**THEN** it MUST default to `Rate: 24000`.

**GIVEN** an endpoint with `StreamingEnabled: true` and `StreamSampleRate: 16000`,
**WHEN** the proxy sends `AudioStart`,
**THEN** the rate MUST be `16000`.

**GIVEN** a database with existing endpoints (no streaming columns),
**WHEN** the migration runs,
**THEN** all existing endpoints MUST have `streaming_enabled = false` and `stream_sample_rate = 24000`.

**GIVEN** the frontend endpoint form with streaming enabled,
**WHEN** the user changes the sample rate to 44100,
**THEN** the API request MUST include `"stream_sample_rate": 44100`.

**GIVEN** a streaming endpoint that returns an HTTP error,
**WHEN** the proxy processes the error,
**THEN** it MUST send a Wyoming `Error` event with code `"tts-error"` — same as buffered mode.

**GIVEN** a streaming response that disconnects mid-stream,
**WHEN** the proxy encounters a read error (not EOF),
**THEN** it MUST return a `"read pcm data: ..."` error, which `HandleSynthesize` converts to a Wyoming `Error` event.

## Tasks

### Backend: Endpoint Model + Database
- [ ] Add `StreamingEnabled bool` and `StreamSampleRate int` to `model.Endpoint` in `internal/model/model.go`
  - JSON tags: `json:"streaming_enabled"` and `json:"stream_sample_rate"`
- [ ] Add alter migration for SQLite in `internal/store/sqlite.go`
  - `ALTER TABLE endpoints ADD COLUMN streaming_enabled INTEGER NOT NULL DEFAULT 0`
  - `ALTER TABLE endpoints ADD COLUMN stream_sample_rate INTEGER NOT NULL DEFAULT 24000`
  - Add to `alterMigrations` slice, check column existence before altering
- [ ] Add alter migration for PostgreSQL in `internal/store/postgres.go`
  - `ALTER TABLE endpoints ADD COLUMN IF NOT EXISTS streaming_enabled BOOLEAN NOT NULL DEFAULT FALSE`
  - `ALTER TABLE endpoints ADD COLUMN IF NOT EXISTS stream_sample_rate INTEGER NOT NULL DEFAULT 24000`
- [ ] Update SQLite `scanEndpoint` to read the two new columns
- [ ] Update PostgreSQL `scanEndpoint` to read the two new columns
- [ ] Update SQLite/PostgreSQL `CreateEndpoint` and `UpdateEndpoint` to write the two new columns
- [ ] Add store tests: round-trip create/read with streaming fields, migration idempotency

### Backend: Proxy Integration
- [ ] Modify `doSynthesize` in `internal/tts/proxy.go` to branch on `ep.StreamingEnabled`
  - Streaming path: build `StreamSynthesizeRequest`, call `client.SynthesizeStream`, build `AudioFormat` from config
  - Buffered path: existing code unchanged
  - Shared: PCM chunking loop reads from `body` (streaming) or `wavReader` (buffered)
- [ ] Default `StreamSampleRate` to 24000 when zero in the proxy (not the model)
- [ ] Add proxy tests for streaming mode in `internal/tts/proxy_test.go`
  - Test: streaming endpoint → mock server returns raw PCM → verify `AudioStart` has config-based format + correct `AudioChunk` events
  - Test: streaming endpoint with custom sample rate (16000) → verify `AudioStart.Rate == 16000`
  - Test: streaming endpoint with `StreamSampleRate: 0` → verify defaults to 24000
  - Test: streaming endpoint returns error → verify Wyoming `Error` event
  - Test: non-streaming endpoint → existing tests still pass (no regression)

### Backend: HTTP API
- [ ] Update `createEndpointRequest` and `updateEndpointRequest` in `internal/api/endpoints.go`
  - Add `StreamingEnabled *bool` and `StreamSampleRate *int` fields
  - Validate `stream_sample_rate` range 8000–48000 when present
- [ ] Update `CreateEndpoint` handler to map new fields to `model.Endpoint`
- [ ] Update `UpdateEndpoint` handler to map new fields
- [ ] Add API integration tests for streaming fields in `internal/api/integration_test.go`
  - Test: create endpoint with `streaming_enabled: true, stream_sample_rate: 24000`
  - Test: update existing endpoint to enable streaming
  - Test: invalid sample rate (e.g., 100) returns 400
  - Test: default values when fields omitted

### Frontend: Endpoint Form
- [ ] Add `streaming_enabled` and `stream_sample_rate` to `Endpoint`, `CreateEndpoint`, `UpdateEndpoint` types in `web/src/lib/api.ts`
- [ ] Add streaming toggle (Switch) to `EndpointForm` in `web/src/components/endpoint-form.tsx`
  - Label: "Streaming"
  - Description/tooltip: "Enable streaming PCM responses for lower time-to-first-audio"
- [ ] Add sample rate input (number) to `EndpointForm`, shown only when streaming is enabled
  - Label: "Sample Rate"
  - Default: 24000, min: 8000, max: 48000
- [ ] Add tests for new form fields in `web/src/components/endpoint-form.test.tsx`
  - Test: streaming toggle renders and submits correct value
  - Test: sample rate input shown/hidden based on streaming toggle
  - Test: sample rate validation (min/max)
  - Test: edit mode populates streaming fields from existing endpoint
- [ ] Update endpoint page tests if needed in `web/src/pages/endpoints.test.tsx`
