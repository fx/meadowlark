package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fx/meadowlark/internal/model"
)

// PostgresStore implements Store using jackc/pgx/v5 with connection pooling.
type PostgresStore struct {
	pool *pgxpool.Pool
}

// NewPostgresStore opens a PostgreSQL connection pool at the given DSN.
func NewPostgresStore(ctx context.Context, dsn string) (*PostgresStore, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("store: ping postgres: %w", err)
	}
	return &PostgresStore{pool: pool}, nil
}

// pgMigrationSQL is the PostgreSQL-compatible migration.
const pgMigrationSQL = `
CREATE TABLE IF NOT EXISTS endpoints (
    id                      TEXT PRIMARY KEY,
    name                    TEXT NOT NULL UNIQUE,
    base_url                TEXT NOT NULL,
    api_key                 TEXT DEFAULT '',
    models                  TEXT NOT NULL DEFAULT '[]',
    default_model           TEXT NOT NULL DEFAULT '',
    default_voice           TEXT NOT NULL DEFAULT '',
    default_speed           DOUBLE PRECISION,
    default_instructions    TEXT,
    default_response_format TEXT NOT NULL DEFAULT 'wav',
    enabled                 BOOLEAN NOT NULL DEFAULT TRUE,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS voice_aliases (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL UNIQUE,
    endpoint_id     TEXT NOT NULL REFERENCES endpoints(id),
    model           TEXT NOT NULL,
    voice           TEXT NOT NULL,
    speed           DOUBLE PRECISION,
    instructions    TEXT,
    languages       TEXT NOT NULL DEFAULT '["en"]',
    enabled         BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS endpoint_voices (
    endpoint_id     TEXT NOT NULL,
    voice_id        TEXT NOT NULL,
    name            TEXT NOT NULL DEFAULT '',
    enabled         BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (endpoint_id, voice_id),
    FOREIGN KEY (endpoint_id) REFERENCES endpoints(id) ON DELETE CASCADE
);
`

func (s *PostgresStore) Migrate(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, pgMigrationSQL)
	if err != nil {
		return fmt.Errorf("store: migrate: %w", err)
	}
	for _, stmt := range pgAlterMigrations {
		if _, err := s.pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("store: alter migration: %w", err)
		}
	}
	return nil
}

// pgAlterMigrations use IF NOT EXISTS so they are idempotent and any
// real errors (permissions, connection) are properly propagated.
var pgAlterMigrations = []string{
	`ALTER TABLE endpoints ADD COLUMN IF NOT EXISTS default_voice TEXT NOT NULL DEFAULT ''`,
	`ALTER TABLE endpoints ADD COLUMN IF NOT EXISTS streaming_enabled BOOLEAN NOT NULL DEFAULT FALSE`,
	`ALTER TABLE endpoints ADD COLUMN IF NOT EXISTS stream_sample_rate INTEGER NOT NULL DEFAULT 24000`,
	`ALTER TABLE endpoints ADD COLUMN IF NOT EXISTS default_model TEXT NOT NULL DEFAULT ''`,
}

func (s *PostgresStore) Close() error {
	s.pool.Close()
	return nil
}

func (s *PostgresStore) ListEndpoints(ctx context.Context) ([]model.Endpoint, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, name, base_url, api_key, models, default_model, default_voice, default_speed, default_instructions, default_response_format, enabled, streaming_enabled, stream_sample_rate, created_at, updated_at FROM endpoints ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("store: list endpoints: %w", err)
	}
	defer rows.Close()
	var out []model.Endpoint
	for rows.Next() {
		ep, err := scanPgEndpoint(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan endpoint: %w", err)
		}
		out = append(out, ep)
	}
	return out, rows.Err()
}

func (s *PostgresStore) GetEndpoint(ctx context.Context, id string) (*model.Endpoint, error) {
	row := s.pool.QueryRow(ctx, `SELECT id, name, base_url, api_key, models, default_model, default_voice, default_speed, default_instructions, default_response_format, enabled, streaming_enabled, stream_sample_rate, created_at, updated_at FROM endpoints WHERE id = $1`, id)
	ep, err := scanPgEndpointRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: get endpoint: %w", err)
	}
	return &ep, nil
}

func (s *PostgresStore) CreateEndpoint(ctx context.Context, e *model.Endpoint) error {
	if e.ID == "" {
		e.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	e.CreatedAt = now
	e.UpdatedAt = now
	modelsJSON, err := e.Models.Value()
	if err != nil {
		return fmt.Errorf("store: marshal models: %w", err)
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO endpoints (id, name, base_url, api_key, models, default_model, default_voice, default_speed, default_instructions, default_response_format, enabled, streaming_enabled, stream_sample_rate, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`,
		e.ID, e.Name, e.BaseURL, e.APIKey, modelsJSON, e.DefaultModel, e.DefaultVoice, e.DefaultSpeed, e.DefaultInstructions, e.DefaultResponseFormat, e.Enabled, e.StreamingEnabled, e.StreamSampleRate, e.CreatedAt, e.UpdatedAt)
	if err != nil {
		return fmt.Errorf("store: create endpoint: %w", err)
	}
	return nil
}

func (s *PostgresStore) UpdateEndpoint(ctx context.Context, e *model.Endpoint) error {
	e.UpdatedAt = time.Now().UTC()
	modelsJSON, err := e.Models.Value()
	if err != nil {
		return fmt.Errorf("store: marshal models: %w", err)
	}
	ct, err := s.pool.Exec(ctx, `UPDATE endpoints SET name = $1, base_url = $2, api_key = $3, models = $4, default_model = $5, default_voice = $6, default_speed = $7, default_instructions = $8, default_response_format = $9, enabled = $10, streaming_enabled = $11, stream_sample_rate = $12, updated_at = $13 WHERE id = $14`,
		e.Name, e.BaseURL, e.APIKey, modelsJSON, e.DefaultModel, e.DefaultVoice, e.DefaultSpeed, e.DefaultInstructions, e.DefaultResponseFormat, e.Enabled, e.StreamingEnabled, e.StreamSampleRate, e.UpdatedAt, e.ID)
	if err != nil {
		return fmt.Errorf("store: update endpoint: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("store: endpoint %q not found", e.ID)
	}
	return nil
}

func (s *PostgresStore) DeleteEndpoint(ctx context.Context, id string) error {
	ct, err := s.pool.Exec(ctx, `DELETE FROM endpoints WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("store: delete endpoint: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("store: endpoint %q not found", id)
	}
	return nil
}

func (s *PostgresStore) ListVoiceAliases(ctx context.Context) ([]model.VoiceAlias, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, name, endpoint_id, model, voice, speed, instructions, languages, enabled, created_at, updated_at FROM voice_aliases ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("store: list voice aliases: %w", err)
	}
	defer rows.Close()
	var out []model.VoiceAlias
	for rows.Next() {
		a, err := scanPgAlias(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan voice alias: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *PostgresStore) GetVoiceAlias(ctx context.Context, id string) (*model.VoiceAlias, error) {
	row := s.pool.QueryRow(ctx, `SELECT id, name, endpoint_id, model, voice, speed, instructions, languages, enabled, created_at, updated_at FROM voice_aliases WHERE id = $1`, id)
	a, err := scanPgAliasRow(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: get voice alias: %w", err)
	}
	return &a, nil
}

func (s *PostgresStore) CreateVoiceAlias(ctx context.Context, a *model.VoiceAlias) error {
	if a.ID == "" {
		a.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	a.CreatedAt = now
	a.UpdatedAt = now
	if a.Languages == nil {
		a.Languages = model.StringSlice{"en"}
	}
	langsJSON, err := a.Languages.Value()
	if err != nil {
		return fmt.Errorf("store: marshal languages: %w", err)
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO voice_aliases (id, name, endpoint_id, model, voice, speed, instructions, languages, enabled, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		a.ID, a.Name, a.EndpointID, a.Model, a.Voice, a.Speed, a.Instructions, langsJSON, a.Enabled, a.CreatedAt, a.UpdatedAt)
	if err != nil {
		return fmt.Errorf("store: create voice alias: %w", err)
	}
	return nil
}

func (s *PostgresStore) UpdateVoiceAlias(ctx context.Context, a *model.VoiceAlias) error {
	a.UpdatedAt = time.Now().UTC()
	langsJSON, err := a.Languages.Value()
	if err != nil {
		return fmt.Errorf("store: marshal languages: %w", err)
	}
	ct, err := s.pool.Exec(ctx, `UPDATE voice_aliases SET name = $1, endpoint_id = $2, model = $3, voice = $4, speed = $5, instructions = $6, languages = $7, enabled = $8, updated_at = $9 WHERE id = $10`,
		a.Name, a.EndpointID, a.Model, a.Voice, a.Speed, a.Instructions, langsJSON, a.Enabled, a.UpdatedAt, a.ID)
	if err != nil {
		return fmt.Errorf("store: update voice alias: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("store: voice alias %q not found", a.ID)
	}
	return nil
}

func (s *PostgresStore) DeleteVoiceAlias(ctx context.Context, id string) error {
	ct, err := s.pool.Exec(ctx, `DELETE FROM voice_aliases WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("store: delete voice alias: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("store: voice alias %q not found", id)
	}
	return nil
}

// ListEndpointVoices returns all endpoint_voices rows for the given endpoint, ordered by voice_id.
func (s *PostgresStore) ListEndpointVoices(ctx context.Context, endpointID string) ([]model.EndpointVoice, error) {
	rows, err := s.pool.Query(ctx, `SELECT endpoint_id, voice_id, name, enabled, created_at, updated_at FROM endpoint_voices WHERE endpoint_id = $1 ORDER BY voice_id`, endpointID)
	if err != nil {
		return nil, fmt.Errorf("store: list endpoint voices: %w", err)
	}
	defer rows.Close()
	var out []model.EndpointVoice
	for rows.Next() {
		v, err := scanPgEndpointVoice(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan endpoint voice: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// UpsertEndpointVoices inserts new rows or updates name and updated_at on conflict.
// Preserves the enabled flag.
func (s *PostgresStore) UpsertEndpointVoices(ctx context.Context, endpointID string, voices []model.EndpointVoice) error {
	if len(voices) == 0 {
		return nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("store: upsert endpoint voices: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	now := time.Now().UTC()
	for _, v := range voices {
		_, err := tx.Exec(ctx, `INSERT INTO endpoint_voices (endpoint_id, voice_id, name, enabled, created_at, updated_at) VALUES ($1, $2, $3, FALSE, $4, $4) ON CONFLICT (endpoint_id, voice_id) DO UPDATE SET name = EXCLUDED.name, updated_at = EXCLUDED.updated_at`,
			endpointID, v.VoiceID, v.Name, now)
		if err != nil {
			return fmt.Errorf("store: upsert endpoint voice %q: %w", v.VoiceID, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("store: upsert endpoint voices: commit: %w", err)
	}
	return nil
}

// SetEndpointVoiceEnabled flips the enabled flag for an existing endpoint_voices row.
func (s *PostgresStore) SetEndpointVoiceEnabled(ctx context.Context, endpointID, voiceID string, enabled bool) (*model.EndpointVoice, error) {
	now := time.Now().UTC()
	ct, err := s.pool.Exec(ctx, `UPDATE endpoint_voices SET enabled = $1, updated_at = $2 WHERE endpoint_id = $3 AND voice_id = $4`,
		enabled, now, endpointID, voiceID)
	if err != nil {
		return nil, fmt.Errorf("store: set endpoint voice enabled: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return nil, ErrEndpointVoiceNotFound
	}
	row := s.pool.QueryRow(ctx, `SELECT endpoint_id, voice_id, name, enabled, created_at, updated_at FROM endpoint_voices WHERE endpoint_id = $1 AND voice_id = $2`, endpointID, voiceID)
	v, err := scanPgEndpointVoiceRow(row)
	if err != nil {
		return nil, fmt.Errorf("store: re-fetch endpoint voice: %w", err)
	}
	return &v, nil
}

// pgScanner abstracts pgx row scanning.
type pgScanner interface {
	Scan(dest ...any) error
}

func scanPgEndpointFromScanner(sc pgScanner) (model.Endpoint, error) {
	var ep model.Endpoint
	var modelsJSON string
	err := sc.Scan(&ep.ID, &ep.Name, &ep.BaseURL, &ep.APIKey, &modelsJSON,
		&ep.DefaultModel, &ep.DefaultVoice, &ep.DefaultSpeed, &ep.DefaultInstructions, &ep.DefaultResponseFormat,
		&ep.Enabled, &ep.StreamingEnabled, &ep.StreamSampleRate, &ep.CreatedAt, &ep.UpdatedAt)
	if err != nil {
		return ep, err
	}
	if err := ep.Models.Scan(modelsJSON); err != nil {
		return ep, fmt.Errorf("store: scan models: %w", err)
	}
	return ep, nil
}

func scanPgEndpoint(rows pgx.Rows) (model.Endpoint, error)  { return scanPgEndpointFromScanner(rows) }
func scanPgEndpointRow(row pgx.Row) (model.Endpoint, error)  { return scanPgEndpointFromScanner(row) }

func scanPgAliasFromScanner(sc pgScanner) (model.VoiceAlias, error) {
	var a model.VoiceAlias
	var langsJSON string
	err := sc.Scan(&a.ID, &a.Name, &a.EndpointID, &a.Model, &a.Voice,
		&a.Speed, &a.Instructions, &langsJSON,
		&a.Enabled, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return a, err
	}
	if err := a.Languages.Scan(langsJSON); err != nil {
		return a, fmt.Errorf("store: scan languages: %w", err)
	}
	return a, nil
}

func scanPgAlias(rows pgx.Rows) (model.VoiceAlias, error)  { return scanPgAliasFromScanner(rows) }
func scanPgAliasRow(row pgx.Row) (model.VoiceAlias, error)  { return scanPgAliasFromScanner(row) }

func scanPgEndpointVoiceFromScanner(sc pgScanner) (model.EndpointVoice, error) {
	var v model.EndpointVoice
	err := sc.Scan(&v.EndpointID, &v.VoiceID, &v.Name, &v.Enabled, &v.CreatedAt, &v.UpdatedAt)
	return v, err
}

func scanPgEndpointVoice(rows pgx.Rows) (model.EndpointVoice, error)  { return scanPgEndpointVoiceFromScanner(rows) }
func scanPgEndpointVoiceRow(row pgx.Row) (model.EndpointVoice, error)  { return scanPgEndpointVoiceFromScanner(row) }
