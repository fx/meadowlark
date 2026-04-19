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

### Dependencies

The API server receives these dependencies at construction:

- `store.Store` — database CRUD operations
- `wyoming.InfoBuilder` — voice list building and caching
- `ClientFactory` — creates TTS clients for test/discovery routes
- `http.FileSystem` — embedded frontend assets (`web/dist/`)

## Middleware Stack

Applied to all routes in this order:

1. **requestLogger** — Logs method, path, status code, and duration via `slog`. Uses a `statusWriter` wrapper to capture response status.
2. **recovery** — Catches panics, logs the error, returns HTTP 500 JSON error.
3. **cors** — Permissive CORS (all origins). Allows GET, POST, PUT, DELETE, OPTIONS. Allows `Content-Type` and `Authorization` headers. Responds 204 to OPTIONS preflight.
4. **jsonContentType** (API routes only) — Sets `Content-Type: application/json`.

## Routes

### Endpoints Management

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| GET | `/api/v1/endpoints` | `ListEndpoints` | List all endpoints |
| POST | `/api/v1/endpoints` | `CreateEndpoint` | Create a new endpoint |
| GET | `/api/v1/endpoints/{id}` | `GetEndpoint` | Get endpoint by ID |
| PUT | `/api/v1/endpoints/{id}` | `UpdateEndpoint` | Update endpoint |
| DELETE | `/api/v1/endpoints/{id}` | `DeleteEndpoint` | Delete endpoint (cascading) |
| POST | `/api/v1/endpoints/{id}/test` | `TestEndpoint` | Test connectivity |
| POST | `/api/v1/endpoints/probe` | `ProbeEndpoint` | Probe URL without saving |
| GET | `/api/v1/endpoints/{id}/configured-models` | `ListEndpointConfiguredModels` | List configured models |
| GET | `/api/v1/endpoints/{id}/models` | `DiscoverModels` | Discover models from endpoint |
| GET | `/api/v1/endpoints/{id}/remote-voices` | `DiscoverRemoteVoices` | Discover voices from endpoint |

#### Create Endpoint Request

```json
{
  "name": "string",
  "base_url": "https://...",
  "api_key": "string",
  "models": ["tts-1", "tts-1-hd"],
  "default_voice": "string",
  "default_speed": 1.0,
  "default_instructions": "string",
  "default_response_format": "wav",
  "enabled": true
}
```

#### Validation Rules

- `name`: REQUIRED, non-empty, unique across all endpoints.
- `base_url`: REQUIRED, MUST be a valid URL.
- `models`: REQUIRED, MUST be a non-empty array.
- `default_speed`: if present, MUST be in range 0.25–4.0.
- `enabled`: defaults to `true`.
- `default_response_format`: defaults to `"wav"`.

#### TestEndpoint

Makes a lightweight TTS request to the endpoint and returns:

```json
{"ok": true, "latency_ms": 234}
```

Or on failure:

```json
{"ok": false, "error": "connection refused", "latency_ms": 0}
```

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
| POST | `/api/v1/aliases/{id}/test` | `TestAlias` | Synthesize test audio |

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

#### TestAlias

Accepts optional `{"text": "..."}`. Defaults to `"Hello, this is a test."`. Synthesizes audio using the alias's configuration and returns a test result.

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

Discovers voices from all enabled endpoints in parallel (5-second timeout per endpoint), falls back to model names if discovery fails, and appends enabled aliases. Returns:

```json
[
  {"name": "alloy (OpenAI, tts-1)", "endpoint": "OpenAI", "model": "tts-1", "voice": "alloy", "is_alias": false},
  {"name": "my-narrator", "endpoint": "OpenAI", "model": "gpt-4o-mini-tts", "voice": "nova", "is_alias": true}
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
| `not_found` | 404 | Resource not found |
| `conflict` | 409 | Duplicate name |
| `internal_error` | 500 | Database/server errors |
| `not_implemented` | 501 | Unimplemented endpoints |

### Response Helpers

- `respondJSON(w, status, v)` — writes JSON with status code.
- `respondError(w, status, code, message)` — writes error envelope.
- `respondNoContent(w)` — writes 204 (used for DELETE).

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
- `/api/` paths that don't match a route MUST return 404 JSON error, not `index.html`.

## Files

| File | Purpose |
|------|---------|
| `internal/api/server.go` | HTTP server, chi router, middleware, route mounting |
| `internal/api/endpoints.go` | Endpoint CRUD handlers, discovery, testing |
| `internal/api/aliases.go` | Alias CRUD handlers, testing |
| `internal/api/system.go` | Status, voice list, parallel discovery |
| `internal/api/response.go` | JSON response helpers, error formatting |
| `internal/api/middleware.go` | Logging, recovery, CORS, content-type |

## Changelog

| Date | Description | Document |
|------|-------------|----------|
| 2026-04-19 | Initial living spec created from implementation audit | --- |
