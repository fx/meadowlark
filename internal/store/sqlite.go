package store

import (
	"context"
	"errors"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"

	"github.com/fx/meadowlark/internal/model"
)

// SQLiteStore implements Store using modernc.org/sqlite (pure Go, no CGO).
type SQLiteStore struct {
	db *sql.DB
	mu sync.Mutex
}

// NewSQLiteStore opens a SQLite database at the given DSN.
// Use ":memory:" for an in-memory database (useful for testing).
func NewSQLiteStore(dsn string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA foreign_keys=ON",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("store: exec %q: %w", pragma, err)
		}
	}
	return &SQLiteStore{db: db}, nil
}

// Migrate creates the database schema idempotently.
func (s *SQLiteStore) Migrate(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.ExecContext(ctx, migrationSQL)
	if err != nil {
		return fmt.Errorf("store: migrate: %w", err)
	}
	return nil
}

const migrationSQL = `
CREATE TABLE IF NOT EXISTS endpoints (
    id                      TEXT PRIMARY KEY,
    name                    TEXT NOT NULL UNIQUE,
    base_url                TEXT NOT NULL,
    api_key                 TEXT DEFAULT '',
    models                  TEXT NOT NULL DEFAULT '[]',
    default_speed           REAL,
    default_instructions    TEXT,
    default_response_format TEXT NOT NULL DEFAULT 'wav',
    enabled                 INTEGER NOT NULL DEFAULT 1,
    created_at              TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at              TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE TABLE IF NOT EXISTS voice_aliases (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL UNIQUE,
    endpoint_id     TEXT NOT NULL REFERENCES endpoints(id),
    model           TEXT NOT NULL,
    voice           TEXT NOT NULL,
    speed           REAL,
    instructions    TEXT,
    languages       TEXT NOT NULL DEFAULT '["en"]',
    enabled         INTEGER NOT NULL DEFAULT 1,
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    updated_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);
`

func (s *SQLiteStore) Close() error { return s.db.Close() }

func (s *SQLiteStore) ListEndpoints(ctx context.Context) ([]model.Endpoint, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, base_url, api_key, models, default_speed, default_instructions, default_response_format, enabled, created_at, updated_at FROM endpoints ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("store: list endpoints: %w", err)
	}
	defer rows.Close()
	var out []model.Endpoint
	for rows.Next() {
		ep, err := scanEndpoint(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan endpoint: %w", err)
		}
		out = append(out, ep)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) GetEndpoint(ctx context.Context, id string) (*model.Endpoint, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, name, base_url, api_key, models, default_speed, default_instructions, default_response_format, enabled, created_at, updated_at FROM endpoints WHERE id = ?`, id)
	ep, err := scanEndpointRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: get endpoint: %w", err)
	}
	return &ep, nil
}

func (s *SQLiteStore) CreateEndpoint(ctx context.Context, e *model.Endpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e.ID == "" {
		e.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	e.CreatedAt = now
	e.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `INSERT INTO endpoints (id, name, base_url, api_key, models, default_speed, default_instructions, default_response_format, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.Name, e.BaseURL, e.APIKey, e.Models, e.DefaultSpeed, e.DefaultInstructions, e.DefaultResponseFormat, e.Enabled, e.CreatedAt.Format(time.RFC3339), e.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("store: create endpoint: %w", err)
	}
	return nil
}

func (s *SQLiteStore) UpdateEndpoint(ctx context.Context, e *model.Endpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e.UpdatedAt = time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `UPDATE endpoints SET name = ?, base_url = ?, api_key = ?, models = ?, default_speed = ?, default_instructions = ?, default_response_format = ?, enabled = ?, updated_at = ? WHERE id = ?`,
		e.Name, e.BaseURL, e.APIKey, e.Models, e.DefaultSpeed, e.DefaultInstructions, e.DefaultResponseFormat, e.Enabled, e.UpdatedAt.Format(time.RFC3339), e.ID)
	if err != nil {
		return fmt.Errorf("store: update endpoint: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: update endpoint rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("store: endpoint %q not found", e.ID)
	}
	return nil
}

func (s *SQLiteStore) DeleteEndpoint(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.ExecContext(ctx, `DELETE FROM endpoints WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete endpoint: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete endpoint rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("store: endpoint %q not found", id)
	}
	return nil
}

func (s *SQLiteStore) ListVoiceAliases(ctx context.Context) ([]model.VoiceAlias, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, endpoint_id, model, voice, speed, instructions, languages, enabled, created_at, updated_at FROM voice_aliases ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("store: list voice aliases: %w", err)
	}
	defer rows.Close()
	var out []model.VoiceAlias
	for rows.Next() {
		a, err := scanAlias(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan voice alias: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) GetVoiceAlias(ctx context.Context, id string) (*model.VoiceAlias, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, name, endpoint_id, model, voice, speed, instructions, languages, enabled, created_at, updated_at FROM voice_aliases WHERE id = ?`, id)
	a, err := scanAliasRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: get voice alias: %w", err)
	}
	return &a, nil
}

func (s *SQLiteStore) CreateVoiceAlias(ctx context.Context, a *model.VoiceAlias) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if a.ID == "" {
		a.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	a.CreatedAt = now
	a.UpdatedAt = now
	if a.Languages == nil {
		a.Languages = model.StringSlice{"en"}
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO voice_aliases (id, name, endpoint_id, model, voice, speed, instructions, languages, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.Name, a.EndpointID, a.Model, a.Voice, a.Speed, a.Instructions, a.Languages, a.Enabled, a.CreatedAt.Format(time.RFC3339), a.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("store: create voice alias: %w", err)
	}
	return nil
}

func (s *SQLiteStore) UpdateVoiceAlias(ctx context.Context, a *model.VoiceAlias) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	a.UpdatedAt = time.Now().UTC()
	res, err := s.db.ExecContext(ctx, `UPDATE voice_aliases SET name = ?, endpoint_id = ?, model = ?, voice = ?, speed = ?, instructions = ?, languages = ?, enabled = ?, updated_at = ? WHERE id = ?`,
		a.Name, a.EndpointID, a.Model, a.Voice, a.Speed, a.Instructions, a.Languages, a.Enabled, a.UpdatedAt.Format(time.RFC3339), a.ID)
	if err != nil {
		return fmt.Errorf("store: update voice alias: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: update voice alias rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("store: voice alias %q not found", a.ID)
	}
	return nil
}

func (s *SQLiteStore) DeleteVoiceAlias(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.ExecContext(ctx, `DELETE FROM voice_aliases WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete voice alias: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete voice alias rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("store: voice alias %q not found", id)
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanEndpointFromScanner(sc scanner) (model.Endpoint, error) {
	var ep model.Endpoint
	var createdAt, updatedAt string
	err := sc.Scan(&ep.ID, &ep.Name, &ep.BaseURL, &ep.APIKey, &ep.Models,
		&ep.DefaultSpeed, &ep.DefaultInstructions, &ep.DefaultResponseFormat,
		&ep.Enabled, &createdAt, &updatedAt)
	if err != nil {
		return ep, fmt.Errorf("store: scan endpoint: %w", err)
	}
	var parseErr error
	ep.CreatedAt, parseErr = time.Parse(time.RFC3339, createdAt)
	if parseErr != nil {
		return ep, fmt.Errorf("store: parse endpoint created_at %q: %w", createdAt, parseErr)
	}
	ep.UpdatedAt, parseErr = time.Parse(time.RFC3339, updatedAt)
	if parseErr != nil {
		return ep, fmt.Errorf("store: parse endpoint updated_at %q: %w", updatedAt, parseErr)
	}
	return ep, nil
}

func scanEndpoint(rows *sql.Rows) (model.Endpoint, error)  { return scanEndpointFromScanner(rows) }
func scanEndpointRow(row *sql.Row) (model.Endpoint, error)  { return scanEndpointFromScanner(row) }

func scanAliasFromScanner(sc scanner) (model.VoiceAlias, error) {
	var a model.VoiceAlias
	var createdAt, updatedAt string
	err := sc.Scan(&a.ID, &a.Name, &a.EndpointID, &a.Model, &a.Voice,
		&a.Speed, &a.Instructions, &a.Languages,
		&a.Enabled, &createdAt, &updatedAt)
	if err != nil {
		return a, fmt.Errorf("store: scan voice alias: %w", err)
	}
	var parseErr error
	a.CreatedAt, parseErr = time.Parse(time.RFC3339, createdAt)
	if parseErr != nil {
		return a, fmt.Errorf("store: parse voice alias created_at %q: %w", createdAt, parseErr)
	}
	a.UpdatedAt, parseErr = time.Parse(time.RFC3339, updatedAt)
	if parseErr != nil {
		return a, fmt.Errorf("store: parse voice alias updated_at %q: %w", updatedAt, parseErr)
	}
	return a, nil
}

func scanAlias(rows *sql.Rows) (model.VoiceAlias, error)  { return scanAliasFromScanner(rows) }
func scanAliasRow(row *sql.Row) (model.VoiceAlias, error)  { return scanAliasFromScanner(row) }
