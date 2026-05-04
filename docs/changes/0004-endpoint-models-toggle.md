# 0004: Endpoint models — toggle list, default-disabled, default-model picker, form layout

## Summary

Replace the current chip-and-combobox model picker on the Endpoints page with a toggle list of upstream-discovered models. All discovered models start disabled; the operator opts each in. Add a `default_model` concept (column + radio in the toggle list). Restructure the endpoint form so the Enabled switch lives in the Connection section, not in a grid alongside Default Speed.

**Spec:** [voice-resolution](../specs/voice-resolution/), [frontend](../specs/frontend/), [data-persistence](../specs/data-persistence/), [http-api](../specs/http-api/)
**Status:** draft
**Depends On:** —

## Motivation

Three problems with the current Endpoints UI (`web/src/components/endpoint-form.tsx`):

1. **Bad model UX.** Models are managed via a Combobox + Badge dance (lines 202–238): type-ahead, click to add, click an `X` to remove. Every model the upstream exposes is implicitly opt-in if the user happens to surface it. There is no way to see "all upstream models, with the ones I've enabled checked" — only the enabled subset is visible.
2. **No default model.** Voice resolution silently uses `Endpoint.Models[0]` as the default for Stage 0 and Stage 3. The operator has no way to pick which model is the default; reordering the slice via the chip UI is awkward and not preserved.
3. **Layout inconsistency.** The Enabled switch sits in a `sm:grid-cols-2` grid next to Default Speed (lines 273–296), aligned with `pt-6`. It looks like a hanging form element. Default Speed and Enabled are unrelated concerns.

The voice-resolution spec already documents the desired semantics (see [Default Model](../specs/voice-resolution/index.md#default-model) and [Enabled Models and Voices](../specs/voice-resolution/index.md#enabled-models-and-voices)). The frontend spec has been updated to describe the toggle list and form sections (see [Endpoints Page](../specs/frontend/index.md#endpoints-page-endpoints)).

## Requirements

### Testing Requirements

This change MUST satisfy the project's standing testing rules (see [frontend spec — Coverage Requirements](../specs/frontend/index.md#coverage-requirements) and [CLAUDE.md — Build Commands](../../CLAUDE.md)):

- Vitest 100% line/function/branch/statement coverage MUST hold; `web/src/components/endpoint-form.test.tsx` and `web/src/pages/endpoints.test.tsx` MUST cover every new branch.
- `go test -race ./...` MUST pass; tests for the new `default_model` validation and migration MUST live in `internal/api/endpoints_test.go` and `internal/store/sqlite_test.go` / `postgres_test.go`.
- The migration MUST be exercised against an existing schema (no `default_model` column) and against a fresh schema; both paths MUST round-trip persisted endpoints.
- Biome MUST pass; new `// biome-ignore` suppressions are not acceptable.

Skipping or weakening any of these rules to land the PR MUST be treated as a bug in the PR, not in the rule.

### R1: Persist `default_model`

Add `default_model TEXT NOT NULL DEFAULT ''` to the `endpoints` table and `DefaultModel string` to `model.Endpoint`. The migration MUST be idempotent (alter-if-not-exists pattern, matching how `streaming_enabled` was added in change 0002).

#### Scenario: existing data migrates without loss

- **GIVEN** a database created before this change with rows having `models = '["tts-1","tts-1-hd"]'`
- **WHEN** the binary starts and runs `Migrate(ctx)`
- **THEN** the `default_model` column MUST exist with value `''` for every existing row
- **AND** voice resolution MUST treat `models[0]` (`"tts-1"`) as the default for those rows (per voice-resolution spec)

### R2: Validate `default_model` on Create / Update

`POST /api/v1/endpoints` and `PUT /api/v1/endpoints/{id}` MUST reject requests where `default_model` is non-empty and not a member of `models`.

#### Scenario: invalid default_model rejected

- **GIVEN** a request with `models: ["tts-1"]` and `default_model: "tts-2"`
- **WHEN** the API processes it
- **THEN** the response MUST be `400 Bad Request`
- **AND** the JSON body MUST contain `{"error":{"code":"invalid_default_model", ...}}`

#### Scenario: empty default_model accepted

- **GIVEN** a request with `models: ["tts-1"]` and `default_model: ""`
- **WHEN** the API processes it
- **THEN** the request MUST succeed and the row MUST be persisted with `default_model = ""`

### R3: Resolver uses `DefaultModel`

`internal/voice/resolver.go` Stage 0 and Stage 3 MUST use `Endpoint.DefaultModel` when non-empty, and `Endpoint.Models[0]` otherwise.

#### Scenario: explicit default takes precedence

- **GIVEN** an endpoint with `Models: ["tts-1","gpt-4o-mini-tts"]` and `DefaultModel: "gpt-4o-mini-tts"`
- **WHEN** Stage 0 resolves a `"default"` request against this endpoint
- **THEN** the resolved `Model` MUST be `"gpt-4o-mini-tts"`

#### Scenario: empty default falls back to first

- **GIVEN** an endpoint with `Models: ["tts-1"]` and `DefaultModel: ""`
- **WHEN** Stage 3 fallback runs
- **THEN** the resolved `Model` MUST be `"tts-1"`

### R4: Endpoint form — Models section is a toggle list

The Models section in `EndpointForm` MUST render as a toggle list, not a Combobox + Badge group:

- One row per upstream-discovered model.
- Each row MUST contain a `Switch` (enabled/disabled), the model `id`, and a `RadioGroup` marker (single-select across all rows) for the **default model**.
- All discovered models MUST default to **disabled** (Switch off) when the endpoint is being created or when a probe surfaces a model not yet in the persisted `models` array.
- The default-model radio MUST be disabled (non-selectable) for rows whose Switch is off.
- If the operator enables a model when no default has been chosen, the form MUST set that model as the default automatically.
- If the operator disables the row that is currently the default, the form MUST move the default to the next still-enabled model in display order, or clear the default when none remain.

#### Scenario: empty endpoint, fresh probe

- **GIVEN** a new "+ Add Endpoint" form, base URL just probed, upstream returned `["tts-1","tts-1-hd","gpt-4o-mini-tts"]`
- **WHEN** the form renders
- **THEN** all three rows MUST have Switch off
- **AND** no row's default-model radio MUST be selected
- **AND** the form's submit button MUST be disabled (no enabled models)

#### Scenario: enabling first model auto-selects it as default

- **GIVEN** the state above
- **WHEN** the operator toggles `tts-1-hd`'s Switch on
- **THEN** `tts-1-hd`'s default-model radio MUST become selected
- **AND** the form value `default_model` MUST be `"tts-1-hd"`

#### Scenario: disabling the default model

- **GIVEN** Models toggled on for `tts-1` and `tts-1-hd`, with `default_model: "tts-1"`
- **WHEN** the operator toggles `tts-1`'s Switch off
- **THEN** the default-model radio MUST shift to `tts-1-hd`
- **AND** the form value `default_model` MUST be `"tts-1-hd"`

### R5: Endpoint form — layout restructure

The form MUST be organised into delimited sections per the [frontend spec — Endpoints Page](../specs/frontend/index.md#endpoints-page-endpoints):

1. Connection (Base URL, API key, **Enabled** switch)
2. Models (toggle list per R4)
3. Voices (handled in change 0005 — this change leaves a placeholder section heading)
4. Defaults (Default voice, Default speed, Default instructions — stacked, not gridded with Enabled)
5. Streaming (existing change 0002 fields)

The Enabled switch MUST NOT share a `grid-cols-2` row with Default Speed.

#### Scenario: Enabled switch lives in Connection

- **GIVEN** the endpoint form is open
- **WHEN** the user inspects the DOM
- **THEN** the Enabled switch MUST be a sibling of the Base URL and API key inputs (Connection section)
- **AND** the Default Speed input MUST NOT be a sibling of the Enabled switch

### R6: Collapsed row shows enabled-model count

The collapsed endpoint row MUST display a badge with the count of enabled models (`models.length`). This is unchanged from current behaviour but stays correct under the new toggle semantics.

## Design

### Approach

**Backend:**

1. Add `DefaultModel string \`json:"default_model"\`` to `model.Endpoint`.
2. Schema migration: add `default_model TEXT NOT NULL DEFAULT ''` to `endpoints` for both SQLite and Postgres (alter-if-not-exists).
3. Update SQLite and Postgres `CreateEndpoint`, `UpdateEndpoint`, scan paths to read/write the column.
4. Validate in `internal/api/endpoints.go` `CreateEndpoint` and `UpdateEndpoint` handlers: when `default_model` is non-empty, it MUST be a member of `models`. Return `400 invalid_default_model` otherwise.
5. Update `internal/voice/resolver.go` Stage 0 and Stage 3 to call a new helper `endpoint.EffectiveDefaultModel()` that returns `DefaultModel` when set, else `Models[0]`.
6. Update `internal/wyoming/info_builder.go` (or equivalent) to use `EffectiveDefaultModel()` rather than `Models[0]` for the canonical voice list ordering — same observable behaviour, less hidden coupling.

**Frontend:**

1. `web/src/lib/api.ts` — add `default_model: string` to `Endpoint`, `CreateEndpoint`, `UpdateEndpoint` types.
2. `web/src/components/endpoint-form.tsx`:
   - Replace the Combobox + Badge block (lines 202–238) with a new `<ModelToggleList>` component.
   - The list source is `useEndpointProbe` (already exists) — render `probe.models` as rows, with each row's enabled state derived from `selectedModels.includes(m.id)`.
   - Each row: `<Switch>`, `<span>{model.id}</span>`, `<RadioGroupItem value={model.id}>`.
   - On Switch change, update `selectedModels`. On Radio change, update `defaultModel`.
   - On Switch off for the current default, recompute the default to the next enabled row (in `probe.models` order, not insertion order).
   - Move the Enabled switch out of the grid into the Connection section (above or below API key, with its own row).
   - Default Speed and Default Instructions become a stacked Defaults section (no grid pairing with Enabled).
3. `web/src/components/endpoint-form.test.tsx` — replace combobox tests with toggle-list tests; cover R4 scenarios.

### Decisions

- **Decision**: Reuse `Endpoint.Models` as the enabled set, not introduce a separate `enabled_models` column.
  - **Why**: The current data shape already represents "configured = enabled". The semantic shift is purely UX: the upstream-available superset is rediscovered live, never persisted. Avoiding a schema split keeps migrations small.
  - **Alternatives considered**: Persist a `discovered_models` cache on the endpoint and derive `enabled` as a subset. Rejected — adds schema churn and a staleness problem (cache vs live probe) for negligible UX gain.

- **Decision**: Add `default_model` as a column rather than encoding it as `models[0]` (status quo).
  - **Why**: Operators want to choose the default explicitly without reordering the enabled list. Encoding via list order is fragile (any UI re-render that re-sorts breaks the contract).
  - **Alternatives considered**: Convert `models` from `[]string` to `[]{id, default}` JSON. Rejected — bigger migration, more fragile, harder to query.

- **Decision**: Toggle list rather than multi-select dropdown.
  - **Why**: Operator feedback ("the chip UI is garbage"). A toggle list shows discovered-but-not-enabled models alongside enabled ones — much clearer state visualisation.
  - **Alternatives considered**: Multi-select Select. Rejected — same opacity problem; users don't see what's disabled.

### Non-Goals

- Voice toggle list (change 0005).
- Alias form Voice fix (change 0003).
- Caching upstream-probed models on the endpoint row.
- Bulk-enable / bulk-disable controls — operators can toggle individually; if needed, defer to a follow-up.

## Tasks

- [ ] Backend: schema + model
  - [ ] Add `DefaultModel` field to `model.Endpoint` with `json:"default_model"` tag (`internal/model/model.go`)
  - [ ] Add `default_model` column to SQLite schema and add an idempotent `ALTER TABLE` migration in `internal/store/sqlite.go`
  - [ ] Add `default_model` column to Postgres schema with `ADD COLUMN IF NOT EXISTS` migration in `internal/store/postgres.go`
  - [ ] Update SQLite `CreateEndpoint`, `UpdateEndpoint`, `GetEndpoint`, `ListEndpoints` to read/write `default_model`
  - [ ] Update Postgres equivalents
  - [ ] Add store tests: round-trip create/read with `default_model`; migration idempotency on a pre-existing schema (`internal/store/sqlite_test.go`, `postgres_test.go`)
- [ ] Backend: API validation
  - [ ] Add `DefaultModel *string` (or required `string`) to `createEndpointRequest` and `updateEndpointRequest` in `internal/api/endpoints.go`
  - [ ] Validate: when present and non-empty, MUST be a member of `models`; otherwise return `400` with code `invalid_default_model`
  - [ ] Add API tests in `internal/api/endpoints_test.go`: valid default, empty default, invalid default (rejected)
- [ ] Backend: resolver
  - [ ] Add `(*Endpoint).EffectiveDefaultModel() string` helper in `internal/model/model.go` returning `DefaultModel` or `Models[0]`
  - [ ] Update `internal/voice/resolver.go` Stage 0 and Stage 3 to call the helper
  - [ ] Update `internal/wyoming/info_builder.go` (or equivalent voice-list builder) — verify it does NOT need to change because it iterates `Models`; if it special-cases `Models[0]`, switch to the helper
  - [ ] Add resolver tests covering both default scenarios from R3
- [ ] Frontend: types
  - [ ] Add `default_model: string` to `Endpoint`, `CreateEndpoint`, `UpdateEndpoint` types (`web/src/lib/api.ts`)
- [ ] Frontend: form refactor
  - [ ] Create `<ModelToggleList>` component (could be inline in `endpoint-form.tsx` or extracted to a sibling file)
  - [ ] Replace lines ~202–238 (Combobox + Badge block) with the toggle list
  - [ ] Wire up Switch + RadioGroup state management per R4 (auto-select default, move default on disable, clear when no enabled rows remain)
  - [ ] Move Enabled switch into the Connection section, above or beside API key (own row, no `pt-6` grid hack)
  - [ ] Restructure Defaults section: Default voice → Default speed → Default instructions, stacked (no `grid-cols-2` pairing with Enabled)
  - [ ] Submit button MUST stay disabled when `selectedModels.length === 0` or when `default_model` is empty
- [ ] Frontend: tests
  - [ ] Replace existing combobox tests in `web/src/components/endpoint-form.test.tsx` with toggle-list tests
  - [ ] Cover R4 scenarios: empty form (probe → all off), enable first → auto-default, disable current default → move default
  - [ ] Cover R5 layout: assert Enabled switch is sibling of API key, not Default Speed
  - [ ] Update `web/src/pages/endpoints.test.tsx` page-level tests if they still assume the old form shape
- [ ] Spec living-doc updates
  - [ ] Confirm the [voice-resolution](../specs/voice-resolution/) and [frontend](../specs/frontend/) spec amendments shipped with this change still match the implementation; add changelog entry once merged

## Open Questions

- [ ] Should there be a "Disable all" / "Enable all" affordance on the Models toggle list? Defer until operator feedback after this change ships.
- [ ] Should the form auto-enable the first discovered model on a fresh "+ Add Endpoint" probe to reduce zero-state friction? Current spec says no (all default-disabled). Confirm with operator usage.

## References

- Spec: [voice-resolution — Default Model](../specs/voice-resolution/index.md#default-model), [voice-resolution — Enabled Models and Voices](../specs/voice-resolution/index.md#enabled-models-and-voices), [frontend — Endpoints Page](../specs/frontend/index.md#endpoints-page-endpoints), [data-persistence — Schema](../specs/data-persistence/index.md#schema), [http-api — Endpoints Management](../specs/http-api/index.md#endpoints-management)
- Related changes: [0003-alias-form-voice-fix](./0003-alias-form-voice-fix.md), [0005-endpoint-voice-toggle](./0005-endpoint-voice-toggle.md)
- Code: `web/src/components/endpoint-form.tsx:202-296`, `internal/voice/resolver.go`, `internal/store/sqlite.go`, `internal/api/endpoints.go`
