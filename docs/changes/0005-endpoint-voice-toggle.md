# 0005: Endpoint voices — persist per-voice enable/disable, default-disabled discovery

## Summary

Persist a per-endpoint voice enable/disable set (`endpoint_voices` table). Discovery via `/remote-voices` upserts new voices with `enabled = false`. The Endpoints UI gains a Voices toggle list section. The system voice list (`GET /api/v1/voices`) and the Wyoming canonical voice list filter to enabled voices only. Voice aliases bypass the filter.

**Spec:** [voice-resolution](../specs/voice-resolution/), [frontend](../specs/frontend/), [data-persistence](../specs/data-persistence/), [http-api](../specs/http-api/)
**Status:** draft
**Depends On:** 0004

## Motivation

Today, all voices upstream-discovered for an enabled endpoint flow through to the Wyoming `describe` voice list and the system `/api/v1/voices` view with no filtering (`internal/api/system.go:121-195`, `internal/api/endpoints.go:338`). For providers with large voice catalogues — especially qwen3-tts with `clone:*` voices that get added at runtime — the operator has no way to curate which voices Home Assistant sees. The user has explicitly asked for parity with the Models behaviour from change 0004: discovered voices default to **disabled**, the operator opts each in.

The endpoint card has no voice management UI today. A "Discover voices" action exists conceptually but is not wired into a persistent state — it's a transient probe.

Aliases (Stage 1 in voice resolution) MUST bypass this filter so operators can name a `clone:*` voice as `"my-clone-voice"` even if the underlying voice is disabled in the management UI. This preserves the alias-as-explicit-opt-in semantics already in the voice-resolution spec.

## Requirements

### Testing Requirements

This change MUST satisfy the project's standing testing rules (see [frontend spec — Coverage Requirements](../specs/frontend/index.md#coverage-requirements) and [CLAUDE.md — Build Commands](../../CLAUDE.md)):

- Vitest 100% coverage MUST hold for all new and modified frontend code. New components MUST have dedicated test files.
- `go test -race ./...` MUST pass; new store, API, and resolver paths MUST have tests in `internal/store/`, `internal/api/`, and `internal/voice/` respectively.
- Migration tests MUST cover: fresh schema, existing schema without `endpoint_voices`, and idempotent re-migration.
- Filter behaviour MUST be tested at the `info_builder` and `system.ListVoices` layers — disabled voices MUST NOT appear, enabled voices MUST appear, and an alias targeting a disabled voice MUST still resolve.
- Biome MUST pass without new suppressions.

Skipping or weakening any of these rules to land the PR MUST be treated as a bug in the PR, not in the rule.

### R1: Persist `endpoint_voices`

A new table `endpoint_voices` MUST be created (see [data-persistence spec — Schema](../specs/data-persistence/index.md#schema)):

- Composite primary key `(endpoint_id, voice_id)`.
- `endpoint_id` references `endpoints(id)` with `ON DELETE CASCADE`.
- `enabled INTEGER NOT NULL DEFAULT 0` (i.e. **disabled by default**).
- Migration MUST be idempotent and safe on a database that already contains endpoints.

#### Scenario: cascade delete

- **GIVEN** an endpoint with three rows in `endpoint_voices`
- **WHEN** the endpoint is deleted (and no aliases reference it)
- **THEN** all three `endpoint_voices` rows MUST be removed automatically by the FK cascade

### R2: Refresh upserts discovered voices

`POST /api/v1/endpoints/{id}/voices/refresh` MUST live-probe the upstream provider and upsert each discovered voice into `endpoint_voices`:

- New rows: insert with `enabled = false`.
- Existing rows: update only `name` and `updated_at`. **MUST NOT** flip `enabled` back to `false` for voices the operator previously enabled.
- Voices that disappear from upstream MUST be left in the table (not deleted) so re-appearance preserves operator intent. A future change MAY add explicit pruning.

The response MUST be the full list of `endpoint_voices` rows for the endpoint after the upsert, sorted by `voice_id`.

#### Scenario: first refresh

- **GIVEN** an endpoint with no `endpoint_voices` rows; upstream returns `[{id:"alloy",name:"Alloy"},{id:"clone:a",name:"Clone A"}]`
- **WHEN** the operator clicks "Refresh voices"
- **THEN** the response MUST contain two rows, both with `enabled: false`

#### Scenario: refresh preserves operator state

- **GIVEN** rows `(alloy, enabled=true)` and `(clone:a, enabled=false)` exist; upstream returns the same list
- **WHEN** the operator clicks "Refresh voices"
- **THEN** the response MUST still report `(alloy, enabled=true)` and `(clone:a, enabled=false)`

#### Scenario: refresh keeps disappeared voices

- **GIVEN** row `(alloy, enabled=true)` exists; upstream now returns `[]`
- **WHEN** refresh runs
- **THEN** the row MUST remain in the table with `enabled=true`

### R3: Toggle a voice

`PUT /api/v1/endpoints/{id}/voices/{voiceId}` with body `{"enabled": bool}` MUST set the row's `enabled` flag and respond with the updated row.

#### Scenario: enable a voice

- **GIVEN** row `(clone:a, enabled=false)`
- **WHEN** the operator toggles it on
- **THEN** the row MUST be updated to `enabled=true`
- **AND** subsequent calls to `GET /api/v1/voices` (system) MUST include `clone:a` for this endpoint

### R4: List filters to enabled voices

`GET /api/v1/voices` (system view) and the Wyoming `describe` canonical voice list MUST only include voices where `endpoint_voices.enabled = true` for an `endpoints.enabled = true` endpoint.

When voice discovery for an endpoint has never been refreshed (i.e. `endpoint_voices` has no rows for that endpoint), the system view MUST treat that endpoint as having zero enabled voices and MUST NOT fall back to live-probing the upstream and assuming the result is enabled. This is a deliberate behaviour shift from the current `system.ListVoices` (which does live-probe and includes everything it finds).

#### Scenario: only enabled voices in describe

- **GIVEN** an endpoint with rows `(alloy, enabled=true)`, `(echo, enabled=false)`, default model `tts-1`
- **WHEN** Wyoming sends `describe`
- **THEN** the canonical voice list MUST include `"alloy (Endpoint, tts-1)"` and MUST NOT include `"echo (Endpoint, tts-1)"`

### R5: Aliases bypass the filter

Voice aliases (resolver Stage 1) MUST resolve regardless of the alias's target voice's `endpoint_voices.enabled` state. Synthesis MUST proceed against the upstream with the alias's `voice` field as-is.

#### Scenario: alias targets disabled voice

- **GIVEN** alias `"crisp-clone"` targets `(endpoint=qwen, model=qwen3-tts, voice=clone:a)`; row `(qwen, clone:a, enabled=false)` exists
- **WHEN** Wyoming requests `voice: "crisp-clone"`
- **THEN** the resolver MUST return the alias resolution
- **AND** synthesis MUST send `voice: "clone:a"` to the upstream qwen endpoint

### R6: Endpoints UI — Voices section

The endpoint form MUST gain a Voices toggle list section between Models and Defaults (per [frontend spec — Endpoints Page](../specs/frontend/index.md#endpoints-page-endpoints)):

- Source data: `GET /api/v1/endpoints/{id}/voices` on form open; in-memory state thereafter.
- One row per `endpoint_voice`: `<Switch enabled>`, voice `id`, voice `name`.
- A "Refresh voices from endpoint" button MUST trigger `POST /api/v1/endpoints/{id}/voices/refresh` and merge the result.
- Toggling a Switch MUST call `PUT .../voices/{voiceId}` immediately (optimistic update with rollback on failure), matching how the collapsed-row Enabled switch behaves today.
- An empty state ("No voices discovered yet — click Refresh") MUST render when the list is empty.
- Discovered voices MUST default to **disabled** in the UI (the backend already returns them that way; the UI MUST NOT auto-enable any).

#### Scenario: collapsed-row badge

The collapsed endpoint row MUST display a count badge for enabled voices alongside the existing enabled-models badge.

#### Scenario: Default voice select scoped to enabled voices

The Default Voice select in the Defaults section MUST be populated from the **enabled** voices for this endpoint (i.e. `endpoint_voices` rows with `enabled=true`). It MUST NOT include disabled voices.

### R7: Alias form is unaffected

Change 0003 keeps the alias form's Voice select sourced from live `/remote-voices`. This change MUST NOT add the enabled-set filter to the alias form. (See [voice-resolution — Enabled Models and Voices](../specs/voice-resolution/index.md#enabled-models-and-voices).)

## Design

### Approach

**Backend:**

1. New `EndpointVoice` model in `internal/model/model.go`.
2. New `endpoint_voices` schema + idempotent migrations for SQLite and Postgres.
3. New `Store` methods: `ListEndpointVoices`, `UpsertEndpointVoices`, `SetEndpointVoiceEnabled`. Implement for both SQLite and Postgres.
4. New API handlers in `internal/api/endpoints.go`:
   - `ListEndpointVoices` → `GET /endpoints/{id}/voices`
   - `RefreshEndpointVoices` → `POST /endpoints/{id}/voices/refresh` (calls `client.ListVoices`, then `UpsertEndpointVoices`)
   - `SetEndpointVoiceEnabled` → `PUT /endpoints/{id}/voices/{voiceId}`
5. Modify `internal/api/system.go` `ListVoices`: drop the live-probe path; for each enabled endpoint, query `endpoint_voices WHERE enabled=true` and emit canonical names using each voice × each enabled model.
6. Modify `internal/wyoming/info_builder.go` (or whichever component builds the canonical voice list for `describe`) to use the same query path. Consolidate the logic with `system.ListVoices` if both currently duplicate it.
7. Add `ON DELETE CASCADE` to the FK; verify it does not interfere with the existing RESTRICT on `voice_aliases.endpoint_id`.

**Frontend:**

1. `web/src/lib/api.ts` — add `EndpointVoice` type and `api.endpoints.voices.{list, refresh, setEnabled}` helpers.
2. `web/src/components/endpoint-form.tsx` — add a `<VoiceToggleList>` section. Mirrors the Models toggle list from change 0004 (Switch, id, name; per-row toggle calls API; "Refresh" button reloads).
3. `web/src/pages/endpoints.tsx` — collapsed-row enabled-voice count badge.
4. The Default Voice select in the Defaults section MUST be reworked to source from the enabled-voices list (was previously a free-text input or sourced from a separate probe).

### Decisions

- **Decision**: New `endpoint_voices` table rather than a JSON column on `endpoints`.
  - **Why**: Rows enable per-voice queries and indexing. JSON would force scanning the whole row for filtering.
  - **Alternatives considered**: `disabled_voices TEXT NOT NULL DEFAULT '[]'` JSON column. Rejected — bigger headache for filter queries and migrations.

- **Decision**: Default `enabled = false` for newly discovered voices.
  - **Why**: Direct user request; matches Models behaviour in change 0004.
  - **Alternatives considered**: Default `enabled = true` to preserve current behaviour. Rejected — defeats the purpose of the change.

- **Decision**: System `/api/v1/voices` and Wyoming `describe` MUST NOT live-probe.
  - **Why**: Live-probing on every Wyoming `describe` is the current source of unfiltered voices flooding Home Assistant. Filter happens at the persisted-state layer.
  - **Alternatives considered**: Probe + filter against persisted-disabled set. Rejected — race-prone (a voice that appears in a probe before the operator has had a chance to toggle it would leak through), more network calls.

- **Decision**: Aliases bypass the filter.
  - **Why**: Already in the voice-resolution spec; preserves explicit-opt-in alias semantics.

- **Decision**: Disappeared voices stay in the table.
  - **Why**: Preserves operator intent across upstream flakiness. Adds minor cruft. Pruning is a future feature.

### Non-Goals

- Pruning `endpoint_voices` rows for voices that no longer appear upstream.
- Surfacing voice metadata richer than `id` and `name` (e.g. language, gender) — defer.
- Alias form Voice filter changes (alias form keeps live-probe source).
- Changing how the Wyoming protocol handles concurrency or caching of `describe` responses.

## Tasks

- [ ] Backend: schema + model
  - [ ] Add `EndpointVoice` struct to `internal/model/model.go`
  - [ ] SQLite: add `CREATE TABLE IF NOT EXISTS endpoint_voices` + idempotent migration
  - [ ] Postgres: same with `BOOLEAN`, `TIMESTAMPTZ`, `ON DELETE CASCADE`
  - [ ] Add store tests covering migration on a pre-existing schema
- [ ] Backend: store methods
  - [ ] `ListEndpointVoices(ctx, endpointID) ([]EndpointVoice, error)` — both SQLite and Postgres
  - [ ] `UpsertEndpointVoices(ctx, endpointID, voices)` — INSERT ... ON CONFLICT DO UPDATE that updates only `name` and `updated_at`, never `enabled`
  - [ ] `SetEndpointVoiceEnabled(ctx, endpointID, voiceID, enabled)`
  - [ ] Tests for all three including the "preserve enabled on upsert" invariant
- [ ] Backend: API handlers
  - [ ] `ListEndpointVoices` handler in `internal/api/endpoints.go`
  - [ ] `RefreshEndpointVoices` handler — calls `client.ListVoices`, then `UpsertEndpointVoices`, returns the post-upsert list
  - [ ] `SetEndpointVoiceEnabled` handler with body validation `{"enabled": bool}`
  - [ ] Wire routes into `internal/api/server.go` (or wherever chi routes are registered)
  - [ ] API tests covering all three handlers + error paths (404 endpoint, 404 voice on toggle, upstream failure on refresh)
- [ ] Backend: filter the canonical voice list
  - [ ] Update `internal/api/system.go` `ListVoices` to source from `endpoint_voices` instead of live-probing
  - [ ] Update `internal/wyoming/info_builder.go` to use the same path
  - [ ] Tests: enabled endpoint with mixed voice states emits only enabled rows; disabled endpoint emits nothing
  - [ ] Tests: alias resolution for a disabled voice still succeeds (alias-bypass invariant)
- [ ] Frontend: types + API client
  - [ ] Add `EndpointVoice` type to `web/src/lib/api.ts`
  - [ ] Add `api.endpoints.voices.list(id)`, `api.endpoints.voices.refresh(id)`, `api.endpoints.voices.setEnabled(id, voiceId, enabled)`
- [ ] Frontend: Voices section in endpoint form
  - [ ] Add a `<VoiceToggleList>` component (separate file or inline) that mirrors the Models toggle list from 0004
  - [ ] Wire it between Models and Defaults
  - [ ] Add the "Refresh voices from endpoint" button
  - [ ] Optimistic toggle: flip Switch immediately, fire `setEnabled`, roll back on error
  - [ ] Empty state copy: "No voices discovered yet — click Refresh"
  - [ ] Default Voice Select in the Defaults section sources from enabled voices
- [ ] Frontend: collapsed row badge
  - [ ] Add an enabled-voice count badge to the row in `web/src/pages/endpoints.tsx`
- [ ] Frontend: tests
  - [ ] `web/src/components/endpoint-form.test.tsx` — cover R6 scenarios
  - [ ] `web/src/pages/endpoints.test.tsx` — cover collapsed-row badge
  - [ ] Optimistic update happy path + rollback on failure
- [ ] Spec living-doc updates
  - [ ] Confirm spec amendments shipped with this change still match implementation; update changelog entries if shape drifted during implementation

## Open Questions

- [ ] Should `RefreshEndpointVoices` also be triggered automatically the first time an endpoint is created via `CreateEndpoint` (so the operator sees the toggle list pre-populated rather than empty)? Lean toward yes; spec it during implementation.
- [ ] Wyoming `describe` is called by Home Assistant on connect. Do we cache the canonical voice list, or rebuild it from the DB each time? Today it's rebuilt; with the filter query added, perf MAY become a concern at scale. Defer; revisit if measured.
- [ ] Should we offer a "prune disappeared voices" button? Defer until operator asks for it.

## References

- Spec: [voice-resolution — Enabled Models and Voices](../specs/voice-resolution/index.md#enabled-models-and-voices), [data-persistence — Schema](../specs/data-persistence/index.md#schema), [http-api — Endpoints Management](../specs/http-api/index.md#endpoints-management), [frontend — Endpoints Page](../specs/frontend/index.md#endpoints-page-endpoints)
- Related changes: [0003-alias-form-voice-fix](./0003-alias-form-voice-fix.md), [0004-endpoint-models-toggle](./0004-endpoint-models-toggle.md)
- Code: `internal/api/system.go:121-195`, `internal/api/endpoints.go:338` (`DiscoverRemoteVoices`), `internal/wyoming/info_builder.go`
