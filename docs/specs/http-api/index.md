# HTTP API

Living specification for Meadowlark's RESTful HTTP API — endpoint/alias CRUD, system status, voice discovery, embedded SPA serving, and middleware stack.

## Overview

Meadowlark exposes a JSON REST API on a configurable HTTP port (default 8080) for managing TTS endpoints and voice aliases. The same server hosts the embedded Preact frontend as a single-page application. The API uses chi for routing with a middleware stack providing logging, panic recovery, CORS, and content-type handling.

**Package:** `internal/api/`

## Server Configuration

### HTTP Timeouts

| Setting | Value |
|---------|-------|
| ReadHeaderTimeout | 10 seconds |
| WriteTimeout | 30 seconds |
| IdleTimeout | 120 seconds |
| Graceful shutdown | 10 seconds |

`POST /api/v1/synthesize` extends its own write deadline past the 30-second `WriteTimeout` (see [Synthesize — Timeouts](#timeouts)).

### Dependencies

The API server receives these dependencies at construction:

- `store.Store` — database CRUD operations
- `wyoming.InfoBuilder` — voice list building and caching
- `ClientFactory` — creates TTS clients for test/discovery routes
- `http.FileSystem` — embedded frontend assets (`web/dist/`)

## Middleware Stack

Applied to all routes in this order:

1. **requestLogger** — Logs method, path, status code, and duration via `slog`. Uses a `statusWriter` wrapper to capture response status; `statusWriter` implements `Unwrap() http.ResponseWriter`, so `http.NewResponseController` reaches the connection through it.
2. **recovery** — Catches panics, logs the error, returns HTTP 500 JSON error.
3. **cors** — Permissive CORS (all origins). Allows GET, POST, PUT, DELETE, OPTIONS. Allows `Content-Type` and `Authorization` headers. Responds 204 to OPTIONS preflight.
4. **jsonContentType** (API routes only) — Sets `Content-Type: application/json`. A handler that answers with something else (`Synthesize`) overrides it before writing the status.

## Routes

### Endpoints Management

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| GET | `/api/v1/endpoints` | `ListEndpoints` | List all endpoints |
| POST | `/api/v1/endpoints` | `CreateEndpoint` | Create a new endpoint |
| GET | `/api/v1/endpoints/{id}` | `GetEndpoint` | Get endpoint by ID |
| PUT | `/api/v1/endpoints/{id}` | `UpdateEndpoint` | Update endpoint |
| DELETE | `/api/v1/endpoints/{id}` | `DeleteEndpoint` | Delete endpoint (fails if aliases exist) |
| POST | `/api/v1/endpoints/{id}/test` | `TestEndpoint` | Test connectivity |
| POST | `/api/v1/endpoints/probe` | `ProbeEndpoint` | Probe URL without saving |
| GET | `/api/v1/endpoints/{id}/configured-models` | `ListEndpointConfiguredModels` | List the endpoint's enabled models. Response: `[]string` (model IDs). |
| GET | `/api/v1/endpoints/{id}/models` | `DiscoverModels` | Live-probe upstream for available models. Response: `[]tts.Model`. |
| GET | `/api/v1/endpoints/{id}/remote-voices` | `DiscoverRemoteVoices` | Live-probe upstream for available voices. Response: `[]tts.Voice` (`{id, name}`). |
| GET | `/api/v1/endpoints/{id}/voices` | `ListEndpointVoices` | List persisted per-endpoint voices and their enabled state. Response: `[]EndpointVoice`. |
| PUT | `/api/v1/endpoints/{id}/voices/{voiceId}` | `SetEndpointVoiceEnabled` | Toggle a single voice's `enabled` flag. Body: `{"enabled": bool}`. |
| POST | `/api/v1/endpoints/{id}/voices/refresh` | `RefreshEndpointVoices` | Re-probe upstream and upsert into `endpoint_voices`. New voices default to `enabled = false`. Response: `[]EndpointVoice`. |

#### Create Endpoint Request

```json
{
  "name": "string",
  "base_url": "https://...",
  "api_key": "string",
  "models": ["tts-1", "tts-1-hd"],
  "default_model": "tts-1",
  "default_voice": "string",
  "default_speed": 1.0,
  "default_instructions": "string",
  "default_response_format": "wav",
  "enabled": true,
  "streaming_enabled": false,
  "stream_sample_rate": 24000
}
```

#### Validation Rules

- `name`: REQUIRED, non-empty, unique across all endpoints.
- `base_url`: REQUIRED, MUST be a valid URL.
- `models`: REQUIRED, MUST be a non-empty array. Represents the **enabled** model subset for this endpoint.
- `default_model`: OPTIONAL. When non-empty, MUST be a member of `models`; otherwise the API MUST respond `400 Bad Request` with code `invalid_default_model`. When empty, the resolver treats `models[0]` as the implicit default.
- `default_speed`: if present, MUST be in range 0.25–4.0.
- `enabled`: defaults to `true`.
- `default_response_format`: defaults to `"wav"`.
- `streaming_enabled`: defaults to `false`.
- `stream_sample_rate`: defaults to `24000`; if present, MUST be in range 8000–48000.

#### TestEndpoint

Makes a lightweight buffered TTS request to the endpoint — model `EffectiveDefaultModel()`, input `"test"`, `response_format: "wav"` — and returns `404 not_found` if the endpoint does not exist (`500 internal_error` if it cannot be read). Otherwise it responds `200` with:

```json
{"ok": true, "latency_ms": 234}
```

Or on failure:

```json
{"ok": false, "error": "connection refused", "latency_ms": 0}
```

`latency_ms` measures the synthesis request alone.

##### Test voice

The request's voice is the first of these that yields one:

1. the endpoint's `default_voice`, when non-empty;
2. the `voice_id` of the first voice saved for the endpoint that is **enabled** and has a non-empty `voice_id`, in the order the store lists them (ascending `voice_id` — see [data-persistence — Store Interface](../data-persistence/index.md#requirements)); a saved row that is disabled or has an empty `voice_id` is skipped;
3. the first voice with a non-empty `id` that the endpoint reports from `GET {base_url}/audio/voices`;
4. the literal `"test"`.

A step that cannot produce a voice falls through to the next: a failure reading the saved voices is logged at warn level and does not fail the test, and a voice listing that fails, is empty, or is not understood yields nothing, as [`ListVoices`](../tts-synthesis/index.md#listvoices) already guarantees. Later steps are not attempted once a voice is found, so an endpoint with a `default_voice` is never asked for its voice list. The response does not report which voice was used.

- `TestEndpoint` MUST select its voice by the order above and MUST fall back to `"test"` only when no earlier step yields a voice.
- `TestEndpoint` MUST NOT send an empty voice: steps 2 and 3 MUST skip any candidate whose id is empty.
- A failure to read saved voices or to list the endpoint's voices MUST NOT change the HTTP status of the test response.

**GIVEN** an endpoint with `default_voice` `"narrator"` and enabled saved voices,
**WHEN** the endpoint is tested,
**THEN** the synthesis request MUST use voice `"narrator"` and `/audio/voices` MUST NOT be requested.

**GIVEN** an endpoint with no `default_voice` whose saved voices are `alpha` (disabled), `bravo` (enabled), `charlie` (enabled),
**WHEN** the endpoint is tested,
**THEN** the synthesis request MUST use voice `"bravo"`.

**GIVEN** an endpoint with no `default_voice` whose saved voices are `""` (enabled) then `delta` (enabled),
**WHEN** the endpoint is tested,
**THEN** the synthesis request MUST use voice `"delta"`.

**GIVEN** an endpoint with no `default_voice` and no saved voice that is both enabled and has a non-empty `voice_id`, whose `/audio/voices` lists `clone-a` then `clone-b`,
**WHEN** the endpoint is tested,
**THEN** the synthesis request MUST use voice `"clone-a"`.

**GIVEN** an endpoint with no `default_voice`, no saved voice that is both enabled and has a non-empty `voice_id`, and a `/audio/voices` that answers 404,
**WHEN** the endpoint is tested,
**THEN** the synthesis request MUST use voice `"test"`.

**GIVEN** an endpoint with no `default_voice` whose saved voices cannot be read from the store,
**WHEN** the endpoint is tested,
**THEN** the response status MUST be `200`, and the synthesis request MUST use the endpoint's first reported voice.

#### ProbeEndpoint

Accepts `{"url": "...", "api_key": "..."}` and discovers models/voices without saving to the database. Returns `{"models": [...], "voices": [...]}`.

### Voice Aliases Management

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| GET | `/api/v1/aliases` | `ListAliases` | List all aliases |
| POST | `/api/v1/aliases` | `CreateAlias` | Create a new alias |
| GET | `/api/v1/aliases/{id}` | `GetAlias` | Get alias by ID |
| PUT | `/api/v1/aliases/{id}` | `UpdateAlias` | Update alias |
| DELETE | `/api/v1/aliases/{id}` | `DeleteAlias` | Delete alias |

#### Create Alias Request

```json
{
  "name": "string",
  "endpoint_id": "uuid",
  "model": "string",
  "voice": "string",
  "speed": 1.0,
  "instructions": "string",
  "languages": ["en"],
  "enabled": true
}
```

#### Validation Rules

- `name`: REQUIRED, non-empty, unique across all aliases.
- `endpoint_id`: REQUIRED, MUST reference an existing endpoint.
- `model`: REQUIRED, non-empty.
- `voice`: REQUIRED, non-empty.
- `speed`: if present, MUST be in range 0.25–4.0.
- `languages`: defaults to `["en"]`.
- `enabled`: defaults to `true`.

There is no alias test route: the playground synthesizes through [`POST /api/v1/synthesize`](#synthesize) instead.

### Synthesis

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| POST | `/api/v1/synthesize` | `Synthesize` | Synthesize text with explicit parameters; returns WAV |

#### Synthesize

Stateless synthesis for the playground: nothing is resolved by voice name and nothing is stored.

**Request** (body capped at 64 KiB):

```json
{
  "endpoint_id": "uuid",
  "model": "string",
  "voice": "string",
  "text": "string",
  "speed": 1.0,
  "instructions": "string"
}
```

`speed` and `instructions` are optional.

**Validation**, in order, each failing with `400 bad_request`:

| Rule | Message |
|------|---------|
| Body decodes as JSON within 64 KiB | `invalid JSON body` |
| `endpoint_id` non-empty after trimming | `endpoint_id is required` |
| `model` non-empty after trimming | `model is required` |
| `voice` non-empty after trimming | `voice is required` |
| `text` non-empty after trimming | `text is required` |
| `text` at most 4096 code points | `text must be at most 4096 characters` |
| `speed`, if present, in 0.25–4.0 | `speed must be between 0.25 and 4.0` |

**Endpoint:** a store error is `500 internal_error` (`failed to get endpoint`); a missing endpoint is `400 bad_request` (`endpoint not found`). A disabled endpoint is still synthesized.

**Parameters:** `voice.MergeParams` merges the request over the endpoint's `default_speed` and `default_instructions`, so an omitted `speed` or `instructions` falls back to the endpoint default exactly as it does for an alias in the Wyoming path. The text is sent literally — `voice.ParseInput` is not applied.

**Upstream:** the request is opened with `tts.OpenEndpointSegment`, the function the Wyoming proxy uses ([tts-synthesis spec](../tts-synthesis/)): a streaming endpoint streams raw PCM at its `stream_sample_rate` (default 24000 Hz, 16-bit mono), a buffered endpoint returns WAV. The handler reads all PCM into memory (at most 64 MiB) and trims it to whole frames.

**Responses:**

| Status | Code | When |
|--------|------|------|
| 200 | — | Success. `Content-Type: audio/wav`, `Content-Length`, `Cache-Control: no-store`. The body is a canonical 44-byte PCM WAV header with correct RIFF and data sizes, then the PCM; the upstream's own header and extra chunks are not passed through. |
| 502 | `upstream_error` | Any open or read failure — upstream non-2xx (its body included), non-WAV body, event stream, unsupported `default_response_format`, unparseable WAV header — as `synthesis failed: <error>` |
| 502 | `upstream_error` | No whole frame of PCM: `endpoint returned no audio` |
| 502 | `upstream_error` | More than 64 MiB of PCM: `audio exceeds the 64 MiB limit` |

Nothing is written before an error.

**Logging:** the handler never logs the request's `text` field. Its success log carries `endpoint_id`, `model`, `voice`, `text_len`, `bytes`, and `duration`; its synthesis-failure log carries `endpoint_id`, `model`, `voice`, and the error. Upstream error bodies are not the request text but MAY echo the input: the TTS client logs a non-2xx body truncated to 500 characters, as on the Wyoming path ([tts-synthesis spec](../tts-synthesis/)), and the same truncated body is part of the error that the failure log and the `502` message carry.

##### Timeouts

The upstream call runs under a 5-minute context. The handler extends the connection's write deadline to 5 minutes 10 seconds with `http.NewResponseController(w).SetWriteDeadline`, so a long text is not cut off by the server's 30-second `WriteTimeout`. A writer that does not support deadlines (e.g. `httptest.ResponseRecorder`) is logged at debug level and does not fail the request.

### System Routes

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| GET | `/api/v1/status` | `GetStatus` | Server status |
| GET | `/api/v1/voices` | `ListVoices` | All resolved voices |

#### Status Response

```json
{
  "version": "0.1.0",
  "uptime_seconds": 3600,
  "wyoming_port": 10300,
  "http_port": 8080,
  "db_driver": "sqlite",
  "voice_count": 12,
  "endpoint_count": 2,
  "alias_count": 5
}
```

#### ListVoices

Discovers voices from all enabled endpoints in parallel (5-second timeout per endpoint), falls back to model names if discovery fails, and appends enabled aliases. Every entry carries the `endpoint_id` it targets, so a client can open the playground preset to a canonical voice. Returns:

```json
[
  {"name": "alloy (OpenAI, tts-1)", "endpoint_id": "uuid", "endpoint": "OpenAI", "model": "tts-1", "voice": "alloy", "is_alias": false},
  {"name": "my-narrator", "endpoint_id": "uuid", "endpoint": "OpenAI", "model": "gpt-4o-mini-tts", "voice": "nova", "is_alias": true}
]
```

## Error Response Format

All API errors MUST use this envelope:

```json
{
  "error": {
    "code": "not_found",
    "message": "endpoint not found"
  }
}
```

### Error Codes

| Code | HTTP Status | Usage |
|------|-------------|-------|
| `bad_request` | 400 | Validation errors |
| `invalid_default_model` | 400 | `default_model` is non-empty but is not a member of `models` (Create/Update Endpoint) |
| `not_found` | 404 | Resource not found |
| `conflict` | 409 | Duplicate name |
| `internal_error` | 500 | Database/server errors |
| `upstream_error` | 502 | The TTS endpoint failed or returned unusable audio (`Synthesize`) |
| `not_implemented` | 501 | Unimplemented endpoints |

### Response Helpers

- `respondJSON(w, status, v)` — writes JSON with status code.
- `respondError(w, status, code, message)` — writes error envelope.
- `respondNoContent(w)` — writes 204 (used for DELETE).

A list response MUST encode an empty result as `[]`, never `null`: every list handler replaces a nil slice before encoding, and `model.StringSlice` fields such as `models` and `languages` marshal a nil slice as `[]`.

## Voice List Rebuild

After any endpoint or alias mutation (create, update, delete), the server calls `infoBuilder.Build(ctx)` to rebuild the Wyoming voice list cache. Errors are logged but not returned to the client.

### Requirements

- Voice list rebuild MUST be triggered after every mutation.
- Rebuild errors MUST NOT affect the HTTP response to the client.

## Static File Serving & SPA Fallback

The embedded `web/dist/` filesystem is mounted at `/`. The SPA fallback logic:

1. If the request path starts with `/api/`, return 404 (not SPA fallback).
2. If the request matches a real file in `web/dist/`, serve it with correct MIME type.
3. Otherwise, serve `index.html` (enables client-side routing).

### Requirements

- Static files MUST be served with correct MIME types.
- Non-API, non-file requests MUST serve `index.html` for SPA routing.
- `/api/` paths that don't match a route MUST return a 404 response, not `index.html`. Currently this uses the default `http.NotFound` handler (plain text), not the JSON error envelope.

## Files

| File | Purpose |
|------|---------|
| `internal/api/server.go` | HTTP server, chi router, middleware, route mounting |
| `internal/api/endpoints.go` | Endpoint CRUD handlers, discovery, testing |
| `internal/api/aliases.go` | Alias CRUD handlers |
| `internal/api/synthesize.go` | Stateless WAV synthesis for the playground |
| `internal/api/system.go` | Status, voice list, parallel discovery |
| `internal/api/response.go` | JSON response helpers, error formatting |
| `internal/api/middleware.go` | Logging, recovery, CORS, content-type |

## Changelog

| Date | Description | Document |
|------|-------------|----------|
| 2026-04-19 | Initial living spec created from implementation audit | --- |
| 2026-05-04 | Documented `default_model` field on Create/Update Endpoint, response shapes for `/configured-models` vs `/remote-voices`, and the new `/endpoints/{id}/voices` toggle routes | [0003-alias-form-voice-fix](../../changes/0003-alias-form-voice-fix.md), [0004-endpoint-models-toggle](../../changes/0004-endpoint-models-toggle.md), [0005-endpoint-voice-toggle](../../changes/0005-endpoint-voice-toggle.md) |
| 2026-10-22 | Documented TestEndpoint's request and 404; specified its test-voice selection order (planned) | [0008](../../changes/0008-endpoint-test-real-voice.md) |
| 2026-10-22 | TestEndpoint selects a real test voice (default voice, first enabled saved voice, first reported voice, then `"test"`); planned marker removed | [0008](../../changes/0008-endpoint-test-real-voice.md) |
| 2026-10-29 | Added `POST /api/v1/synthesize` (stateless WAV synthesis through the proxy's segment opening, with a per-request write deadline) and `endpoint_id` on voice list entries; removed `POST /api/v1/aliases/{id}/test`; documented `statusWriter.Unwrap` and the `upstream_error` code | [0009](../../changes/0009-voice-playground.md) |
| 2026-10-29 | List responses and `StringSlice` fields encode an empty list as `[]`, never `null` (`ListAliases` and `ListEndpointConfiguredModels` returned `null`); clarified that `Synthesize` never logs the request text, while upstream error bodies, which may echo it, are logged truncated by the TTS client | [0009](../../changes/0009-voice-playground.md) |
