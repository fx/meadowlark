# Data Persistence

Living specification for Meadowlark's database layer — the Store interface, SQLite and PostgreSQL implementations, schema, migrations, and data models.

## Overview

Meadowlark persists TTS endpoint configurations and voice aliases in a SQL database. The `Store` interface abstracts all persistence operations, with concrete implementations for SQLite (default, pure Go) and PostgreSQL (connection-pooled). Both share the same schema with minor dialect differences.

**Packages:** `internal/store/`, `internal/model/`

## Store Interface

```go
type Store interface {
    // Endpoints
    ListEndpoints(ctx context.Context) ([]model.Endpoint, error)
    GetEndpoint(ctx context.Context, id string) (*model.Endpoint, error)
    CreateEndpoint(ctx context.Context, e *model.Endpoint) error
    UpdateEndpoint(ctx context.Context, e *model.Endpoint) error
    DeleteEndpoint(ctx context.Context, id string) error

    // Voice Aliases
    ListVoiceAliases(ctx context.Context) ([]model.VoiceAlias, error)
    GetVoiceAlias(ctx context.Context, id string) (*model.VoiceAlias, error)
    CreateVoiceAlias(ctx context.Context, a *model.VoiceAlias) error
    UpdateVoiceAlias(ctx context.Context, a *model.VoiceAlias) error
    DeleteVoiceAlias(ctx context.Context, id string) error

    // Lifecycle
    Migrate(ctx context.Context) error
    Close() error
}
```

### Requirements

- `GetEndpoint` and `GetVoiceAlias` MUST return `nil, nil` (not an error) when the record doesn't exist.
- `UpdateEndpoint` and `UpdateVoiceAlias` MUST set `UpdatedAt` to the current UTC time.
- `UpdateEndpoint` and `DeleteEndpoint` MUST return an error if no rows are affected (record not found).
- All methods MUST accept and respect context cancellation.

## SQLite Implementation

### Configuration

- Library: `modernc.org/sqlite` (pure Go, no CGO required).
- MaxOpenConns: 1 (SQLite single-writer limitation).
- PRAGMAs: `journal_mode=WAL`, `foreign_keys=ON`.
- DSN from `--db-dsn` flag (default: `meadowlark.db`). Supports `:memory:` for testing.

### Concurrency

Uses `sync.Mutex` for all write operations (Create, Update, Delete) to serialize writes. Read operations do not acquire the mutex (WAL allows concurrent reads).

### Schema

```sql
CREATE TABLE IF NOT EXISTS endpoints (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    base_url TEXT NOT NULL,
    api_key TEXT DEFAULT '',
    models TEXT NOT NULL DEFAULT '[]',
    default_voice TEXT NOT NULL DEFAULT '',
    default_speed REAL,
    default_instructions TEXT,
    default_response_format TEXT NOT NULL DEFAULT 'wav',
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE TABLE IF NOT EXISTS voice_aliases (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    endpoint_id TEXT NOT NULL REFERENCES endpoints(id),
    model TEXT NOT NULL,
    voice TEXT NOT NULL,
    speed REAL,
    instructions TEXT,
    languages TEXT NOT NULL DEFAULT '["en"]',
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);
```

### Data Serialization

- **Timestamps:** Stored as RFC3339 TEXT strings, parsed on retrieval.
- **JSON arrays:** `models` and `languages` stored as JSON TEXT (e.g., `'["tts-1","tts-1-hd"]'`).
- **Booleans:** Stored as INTEGER (0/1).
- **Nullable floats:** `REAL` columns with NULL for unset.

## PostgreSQL Implementation

### Configuration

- Library: `jackc/pgx/v5` with connection pooling.
- DSN from `--db-dsn` flag (e.g., `postgres://user:pass@localhost/meadowlark`).
- Uses `pgxpool.New(ctx, dsn)` for connection management.
- Pings the connection on initialization.

### Schema Differences from SQLite

| SQLite | PostgreSQL |
|--------|------------|
| `REAL` | `DOUBLE PRECISION` |
| `INTEGER` (for booleans) | `BOOLEAN` |
| `TEXT` (for timestamps) | `TIMESTAMPTZ` |
| `strftime(...)` | `NOW()` |
| `?` parameters | `$1, $2, ...` parameters |

### Concurrency

No application-level mutex needed — `pgx` connection pool handles concurrency.

## Migrations

### Strategy

Migrations are embedded in Go source code and run on every startup via `Migrate(ctx)`.

### Migration Order

1. **Table creation** — `CREATE TABLE IF NOT EXISTS` for both tables. Idempotent.
2. **Alter migrations** — Column additions applied idempotently:
   - SQLite: `ALTER TABLE endpoints ADD COLUMN default_voice TEXT NOT NULL DEFAULT ''` (checks column existence first).
   - PostgreSQL: `ALTER TABLE endpoints ADD COLUMN IF NOT EXISTS default_voice TEXT NOT NULL DEFAULT ''`.

### Requirements

- `Migrate()` MUST be idempotent — safe to run on every startup.
- New columns MUST have default values to avoid breaking existing data.
- Foreign key constraints MUST be enforced (SQLite: `PRAGMA foreign_keys=ON`).
- `voice_aliases.endpoint_id` MUST reference `endpoints.id`. The FK uses default RESTRICT behavior — deleting an endpoint with existing aliases MUST fail with a foreign key error. The API layer handles this by returning an error to the user.

## Data Models

### Endpoint

```go
type Endpoint struct {
    ID                    string      `json:"id"`
    Name                  string      `json:"name"`
    BaseURL               string      `json:"base_url"`
    APIKey                string      `json:"api_key,omitempty"`
    Models                StringSlice `json:"models"`
    DefaultVoice          string      `json:"default_voice"`
    DefaultSpeed          *float64    `json:"default_speed"`
    DefaultInstructions   *string     `json:"default_instructions"`
    DefaultResponseFormat string      `json:"default_response_format"`
    Enabled               bool        `json:"enabled"`
    CreatedAt             time.Time   `json:"created_at"`
    UpdatedAt             time.Time   `json:"updated_at"`
}
```

### VoiceAlias

```go
type VoiceAlias struct {
    ID           string      `json:"id"`
    Name         string      `json:"name"`
    EndpointID   string      `json:"endpoint_id"`
    Model        string      `json:"model"`
    Voice        string      `json:"voice"`
    Speed        *float64    `json:"speed"`
    Instructions *string     `json:"instructions"`
    Languages    StringSlice `json:"languages"`
    Enabled      bool        `json:"enabled"`
    CreatedAt    time.Time   `json:"created_at"`
    UpdatedAt    time.Time   `json:"updated_at"`
}
```

### StringSlice

Custom type for JSON array columns:

```go
type StringSlice []string
```

Implements:
- `sql.Scanner` — deserializes JSON string from database TEXT column. Handles both `string` and `[]byte` scan sources.
- `driver.Valuer` — serializes to JSON string for database storage.
- Null/empty database values result in an empty `StringSlice`.

### ResolvedVoice

Result of voice resolution (not persisted):

```go
type ResolvedVoice struct {
    Name         string
    EndpointID   string
    Model        string
    Voice        string
    Speed        *float64
    Instructions *string
    Languages    StringSlice
    IsAlias      bool
}
```

## Driver Selection

The `--db-driver` flag (or `MEADOWLARK_DB_DRIVER` env var) selects the implementation:

| Value | Implementation | Default DSN |
|-------|---------------|-------------|
| `sqlite` (default) | `NewSQLiteStore(dsn)` | `meadowlark.db` |
| `postgres` | `NewPostgresStore(ctx, dsn)` | (connection string required) |

## Scenarios

**GIVEN** a fresh SQLite database,
**WHEN** `Migrate(ctx)` runs,
**THEN** both tables MUST be created with all columns and constraints.

**GIVEN** a database that already has the tables,
**WHEN** `Migrate(ctx)` runs again,
**THEN** no errors MUST occur and no data MUST be lost.

**GIVEN** an endpoint with ID "ep-1" exists,
**WHEN** a voice alias is created with `endpoint_id: "ep-1"`,
**THEN** the alias MUST be persisted with the foreign key.

**GIVEN** an endpoint is deleted,
**WHEN** voice aliases reference that endpoint,
**THEN** the delete MUST fail with a foreign key constraint error (RESTRICT behavior). Aliases MUST be deleted first before the endpoint can be removed.

## Files

| File | Purpose |
|------|---------|
| `internal/store/store.go` | Store interface definition |
| `internal/store/sqlite.go` | SQLite implementation (pure Go) |
| `internal/store/postgres.go` | PostgreSQL implementation (pgx pool) |
| `internal/model/model.go` | Data models: Endpoint, VoiceAlias, ResolvedVoice, StringSlice |

## Changelog

| Date | Description | Document |
|------|-------------|----------|
| 2026-04-19 | Initial living spec created from implementation audit | --- |
