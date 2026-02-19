# 0003: HTTP API

## Overview

Implement the RESTful HTTP API that powers the admin frontend and serves the embedded Preact SPA. This spec adds the `internal/api/` package with CRUD handlers for endpoints and voice aliases, system status routes, and static file serving with SPA fallback.

## Background

Meadowlark's configuration (TTS endpoints, voice aliases) is managed through a web UI backed by a JSON REST API. The API runs on a separate HTTP port from the Wyoming TCP server. The Preact frontend (built in spec 0004) is embedded in the Go binary and served from the same HTTP server.

See `docs/meadowlark.md` sections 8 (HTTP API) and 10.9 (Build & Embedding) for full requirements.

## Goals

- HTTP server on `--http-host:--http-port` (default `0.0.0.0:8080`)
- RESTful JSON API under `/api/v1/`
- Endpoints CRUD: list, get, create, update, delete
- Endpoint connectivity test (`POST /api/v1/endpoints/:id/test`)
- Endpoint voice discovery (`GET /api/v1/endpoints/:id/voices`)
- Voice aliases CRUD: list, get, create, update, delete
- Alias TTS test (`POST /api/v1/aliases/:id/test`)
- System status (`GET /api/v1/status`)
- Resolved voices list (`GET /api/v1/voices`)
- Embedded static file serving at `/` with SPA fallback
- Request logging middleware
- Error response middleware (consistent JSON error format)
- CORS middleware (permissive for development)
- Integration with the Wyoming server's voice list (trigger info rebuild on config changes)

## Non-Goals

- Authentication/authorization (future consideration)
- WebSocket/SSE for live updates
- Frontend implementation (spec 0004)

## Design

### Router

Use `net/http` with a lightweight router (`chi` or similar). Route structure:

```
/api/v1/
  /endpoints          GET, POST
  /endpoints/:id      GET, PUT, DELETE
  /endpoints/:id/test POST
  /endpoints/:id/voices GET
  /aliases            GET, POST
  /aliases/:id        GET, PUT, DELETE
  /aliases/:id/test   POST
  /status             GET
  /voices             GET

/                     Static files (embedded SPA)
/assets/*             Static assets
/*                    SPA fallback -> index.html
```

### Middleware Stack

1. **Request logger** -- log method, path, status, duration via `log/slog`
2. **Recovery** -- catch panics, return 500
3. **CORS** -- allow all origins in development
4. **Content-Type** -- set `application/json` for `/api/` routes

### API Handlers (`internal/api/`)

#### Endpoints CRUD (`endpoints.go`)

| Handler | Method | Path | Request Body | Response |
|---|---|---|---|---|
| `ListEndpoints` | GET | `/endpoints` | - | `[]Endpoint` |
| `GetEndpoint` | GET | `/endpoints/:id` | - | `Endpoint` |
| `CreateEndpoint` | POST | `/endpoints` | `{name, base_url, api_key?, models, ...}` | `Endpoint` (201) |
| `UpdateEndpoint` | PUT | `/endpoints/:id` | `{name?, base_url?, ...}` | `Endpoint` |
| `DeleteEndpoint` | DELETE | `/endpoints/:id` | - | 204 |
| `TestEndpoint` | POST | `/endpoints/:id/test` | - | `{ok: bool, error?: string, latency_ms: int}` |
| `DiscoverVoices` | GET | `/endpoints/:id/voices` | - | `[]string` |

**Validation:**
- `name`: required, non-empty, unique
- `base_url`: required, valid URL
- `models`: required, non-empty array
- `default_speed`: if present, must be 0.25-4.0

**TestEndpoint** makes a lightweight API call to the endpoint (e.g., a small TTS request or a model list request) and reports success/failure with latency.

**DiscoverVoices** calls the endpoint's API to discover available voices. Returns the raw voice name list.

#### Voice Aliases CRUD (`aliases.go`)

| Handler | Method | Path | Request Body | Response |
|---|---|---|---|---|
| `ListAliases` | GET | `/aliases` | - | `[]VoiceAlias` |
| `GetAlias` | GET | `/aliases/:id` | - | `VoiceAlias` |
| `CreateAlias` | POST | `/aliases` | `{name, endpoint_id, model, voice, ...}` | `VoiceAlias` (201) |
| `UpdateAlias` | PUT | `/aliases/:id` | `{name?, model?, ...}` | `VoiceAlias` |
| `DeleteAlias` | DELETE | `/aliases/:id` | - | 204 |
| `TestAlias` | POST | `/aliases/:id/test` | `{text?: string}` | Audio file (WAV) or `{ok: bool, error?: string}` |

**Validation:**
- `name`: required, non-empty, unique
- `endpoint_id`: required, must reference existing endpoint
- `model`: required, non-empty
- `voice`: required, non-empty
- `speed`: if present, must be 0.25-4.0

**TestAlias** synthesizes a short sample text ("Hello, this is a test.") using the alias's configuration and returns success/failure.

#### System (`system.go`)

**GET `/api/v1/status`:**
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

**GET `/api/v1/voices`:**
```json
[
  {
    "name": "alloy (OpenAI, tts-1)",
    "endpoint": "OpenAI",
    "model": "tts-1",
    "voice": "alloy",
    "is_alias": false
  },
  {
    "name": "my-narrator",
    "endpoint": "OpenAI",
    "model": "gpt-4o-mini-tts",
    "voice": "nova",
    "is_alias": true
  }
]
```

### Error Response Format

All API errors return:

```json
{
  "error": {
    "code": "not_found",
    "message": "Endpoint with id 'abc' not found"
  }
}
```

Standard codes: `bad_request`, `not_found`, `conflict`, `internal_error`.

### Static File Serving

The embedded `web/dist/` filesystem is served at `/`. Any request that:
- Does not match `/api/`
- Does not match a real file in `web/dist/`

is served `index.html` (SPA fallback). Proper MIME types are set for `.js`, `.css`, `.html`, `.svg`, `.woff2`, etc.

### Voice List Rebuild

When endpoints or aliases are created, updated, or deleted via the API, the Wyoming server's voice list must be rebuilt. This is done by calling a `RebuildVoices()` method on the Wyoming server (or a shared voice registry) after any mutation.

### Integration with main.go

Extend the startup sequence from spec 0002:

1. Parse flags, init logger
2. Init database, run migrations
3. Start Wyoming TCP server
4. **Start HTTP API server** on `--http-host:--http-port`
5. Register Zeroconf
6. Block until shutdown signal
7. Graceful shutdown: stop HTTP server (with timeout), stop Wyoming server, close DB

## Testing

### Unit Tests

- Each handler tested with `httptest.NewRecorder` and mock `Store`
- Validation edge cases (missing fields, invalid types, duplicate names)
- Error response format consistency
- SPA fallback logic (real file vs. fallback to index.html)

### Integration Tests

- Full CRUD cycle via real HTTP requests against an in-memory SQLite store
- Voice list rebuild triggered after mutations
- TestEndpoint with mock upstream API
- TestAlias with mock TTS response
- Concurrent request handling

### Coverage Target

> 80% for `internal/api/`.

## Tasks

- [x] Set up HTTP server and router
  - [x] Add `chi` (or chosen router) dependency
  - [x] Create `internal/api/server.go` with HTTP server setup
  - [x] Configure middleware stack: request logger, recovery, CORS, content-type
  - [x] Mount API routes under `/api/v1/`
  - [x] Mount static file server for embedded frontend at `/`
  - [x] Implement SPA fallback (non-API, non-asset routes serve index.html)
  - [x] Wire into main.go startup/shutdown sequence
- [x] Implement error response helpers
  - [x] Define consistent JSON error format (`{error: {code, message}}`)
  - [x] Create helper functions: `respondJSON`, `respondError`, `respondNoContent`
  - [x] Standard error codes: `bad_request`, `not_found`, `conflict`, `internal_error`
- [ ] Implement Endpoints CRUD handlers
  - [ ] `GET /api/v1/endpoints` -- list all endpoints
  - [ ] `POST /api/v1/endpoints` -- create endpoint (validate: name required+unique, base_url required+valid URL, models required+non-empty, speed range 0.25-4.0)
  - [ ] `GET /api/v1/endpoints/:id` -- get single endpoint
  - [ ] `PUT /api/v1/endpoints/:id` -- update endpoint (same validations)
  - [ ] `DELETE /api/v1/endpoints/:id` -- delete endpoint (cascade check: warn if aliases reference it)
  - [ ] Trigger voice list rebuild after each mutation
  - [ ] Write handler tests with mock Store and httptest
- [ ] Implement endpoint test and voice discovery
  - [ ] `POST /api/v1/endpoints/:id/test` -- make lightweight TTS request to verify connectivity, return `{ok, error?, latency_ms}`
  - [ ] `GET /api/v1/endpoints/:id/voices` -- call endpoint API to discover available voices, return `[]string`
  - [ ] Write tests with mock upstream HTTP server
- [ ] Implement Voice Aliases CRUD handlers
  - [ ] `GET /api/v1/aliases` -- list all aliases
  - [ ] `POST /api/v1/aliases` -- create alias (validate: name required+unique, endpoint_id must exist, model+voice required, speed range)
  - [ ] `GET /api/v1/aliases/:id` -- get single alias
  - [ ] `PUT /api/v1/aliases/:id` -- update alias (same validations)
  - [ ] `DELETE /api/v1/aliases/:id` -- delete alias
  - [ ] Trigger voice list rebuild after each mutation
  - [ ] Write handler tests with mock Store
- [ ] Implement alias TTS test
  - [ ] `POST /api/v1/aliases/:id/test` -- synthesize sample text using alias config, return `{ok, error?}`
  - [ ] Write tests with mock TTS client
- [ ] Implement system handlers
  - [ ] `GET /api/v1/status` -- return version, uptime, ports, db driver, counts
  - [ ] `GET /api/v1/voices` -- return resolved voice list (canonical + aliases) with endpoint/model metadata
  - [ ] Write tests
- [ ] Integration tests
  - [ ] Full CRUD cycle (create -> list -> update -> get -> delete) via real HTTP against in-memory SQLite
  - [ ] Voice list rebuild verification after mutations
  - [ ] Concurrent request handling test
  - [ ] SPA fallback test (request non-existent path -> get index.html)
