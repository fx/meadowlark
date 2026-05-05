# 0003: Alias form — fix voice/model confusion

## Summary

Fix the voice alias form so the Voice dropdown populates from the endpoint's live remote voices (`/remote-voices`), not from the endpoint's configured model list (`/configured-models`). The current code shows model IDs in the Voice dropdown, which both duplicates the Model dropdown and hides upstream-discovered voices such as `clone:*` from qwen3-tts.

**Spec:** [voice-resolution](../specs/voice-resolution/), [frontend](../specs/frontend/), [http-api](../specs/http-api/)
**Status:** complete
**Depends On:** —

## Motivation

`web/src/components/alias-form.tsx:48` currently fetches:

```ts
fetch(`/api/v1/endpoints/${endpointId}/configured-models`, { signal: controller.signal })
  .then(async (res) => {
    if (res.ok) {
      const data = await res.json()
      setVoices(data as string[])
    }
  })
```

`/configured-models` returns `[]string` of **model IDs** (e.g. `["tts-1", "qwen3-tts"]`). The form treats this as the Voice list. Result:

- The Model and Voice dropdowns surface the same values.
- Voices that only exist in upstream voice discovery — including dynamic `clone:*` voices from qwen3-tts — never appear in the alias form, so operators cannot create aliases for them.
- The frontend spec already says: *"Voice selection (fetched from endpoint remote voices)"* (frontend spec line 105). The implementation contradicts the spec.

The backend already exposes the correct endpoint: `GET /api/v1/endpoints/{id}/remote-voices` (`internal/api/endpoints.go:338`), which calls `client.ListVoices` and returns `[]tts.Voice` (`{id, name}`).

## Requirements

### Testing Requirements

This change MUST satisfy the project's standing testing rules (see [frontend spec — Coverage Requirements](../specs/frontend/index.md#coverage-requirements) and [CLAUDE.md — Build Commands](../../CLAUDE.md)). CI enforces these as merge gates:

- Vitest MUST run with the configured 100% thresholds for lines, functions, branches, and statements.
- New behaviour in `alias-form.tsx` MUST be exercised by `web/src/components/alias-form.test.tsx` — every branch (success, empty list, fetch failure, abort during in-flight) MUST have an assertion.
- `go test -race ./...` MUST pass; race-mode CGO is required, so coverage tasks MUST NOT set `CGO_ENABLED=0`.
- Biome (`bunx biome check .`) MUST pass without `// biome-ignore` suppressions added in this change.

Skipping or weakening any of these rules to land the PR MUST be treated as a bug in the PR, not in the rule.

### R1: Voice dropdown source

The Voice select in `AliasForm` MUST be populated from `GET /api/v1/endpoints/{id}/remote-voices`. It MUST NOT call `/configured-models`.

#### Scenario: live voices populate the dropdown

- **GIVEN** an endpoint whose upstream returns `[{"id":"alloy","name":"Alloy"},{"id":"clone:abc","name":"Clone ABC"}]`
- **WHEN** the operator selects this endpoint in the alias form
- **THEN** the Voice select MUST list both `alloy` and `clone:abc` as options
- **AND** the displayed label MUST be the voice's `name` (with `id` shown as a secondary label when `id !== name`)
- **AND** the form value submitted MUST be the voice's `id`

#### Scenario: dynamic clone voices appear

- **GIVEN** a qwen3-tts endpoint with one or more `clone:*` voices in its `/v1/audio/voices` response
- **WHEN** the operator opens the alias form for that endpoint
- **THEN** the `clone:*` voices MUST be selectable in the Voice dropdown

### R2: Graceful fallback to free-text

When `/remote-voices` fails or returns an empty array, the form MUST render a free-text `Input` for the Voice field so the operator can configure aliases for voices the upstream did not enumerate. This behaviour exists today and MUST be preserved.

#### Scenario: probe failure falls back to text input

- **GIVEN** `/remote-voices` returns `502 Bad Gateway`
- **WHEN** the form renders
- **THEN** the Voice field MUST be a text `Input` (not a `Select`)
- **AND** the operator MUST still be able to submit the form with a manually typed voice ID

### R3: No conflation with the Model dropdown

Model and Voice MUST come from different sources. The Model dropdown continues to be sourced from `selectedEndpoint.models` (the endpoint's persisted enabled-models list). The Voice dropdown comes from live discovery.

#### Scenario: distinct lists

- **GIVEN** an endpoint with `models: ["qwen3-tts"]` and remote voices `[{"id":"clone:abc"}, {"id":"qwen-female-1"}]`
- **WHEN** the operator opens the alias form
- **THEN** the Model dropdown MUST contain only `"qwen3-tts"`
- **AND** the Voice dropdown MUST contain `"clone:abc"` and `"qwen-female-1"` and MUST NOT contain `"qwen3-tts"`

## Design

### Approach

A single-file change in `web/src/components/alias-form.tsx`:

1. Change the `useEffect` fetch URL from `/api/v1/endpoints/${endpointId}/configured-models` to `/api/v1/endpoints/${endpointId}/remote-voices`.
2. Update the response parsing: `const data = await res.json()` is now `tts.Voice[]` (`{id: string; name: string}[]`), not `string[]`. Map to the local state shape.
3. Update the local `voices` state type from `string[]` to `{id: string; name: string}[]` and the Select rendering to use `id` as the option `value` and `name` as the visible label (fall back to `id` when `name` is empty).

No backend, schema, or API changes. The endpoint and the response shape already exist.

### Decisions

- **Decision**: Use `/remote-voices` (live probe) rather than introducing a cached/persisted voice list for this change.
  - **Why**: Aliases must be able to target voices the operator has *not* explicitly enabled (e.g. dynamic `clone:*` voices). Per-endpoint persistence of voices and the enabled-set semantics belong to change [0005-endpoint-voice-toggle](./0005-endpoint-voice-toggle.md). This change is a bug fix and stays narrowly scoped.
  - **Alternatives considered**: (a) wait for 0005 and source from the persisted `endpoint_voices` table — rejected because the bug is shippable today and 0005 is bigger; (b) source from the system `/api/v1/voices` endpoint — rejected because that endpoint applies enabled-set filtering, which would defeat the alias use case.

- **Decision**: Keep the free-text fallback for empty / failing probes.
  - **Why**: Some self-hosted providers (especially during clone enrolment) don't return the voice ID in `/v1/audio/voices` until later. Operators must still be able to configure the alias.

### Non-Goals

- Per-endpoint voice toggle UI (handled in [0005](./0005-endpoint-voice-toggle.md)).
- Endpoint form Models redesign (handled in [0004](./0004-endpoint-models-toggle.md)).
- Caching `/remote-voices` responses across alias-form opens.

## Tasks

- [ ] Fix Voice dropdown source in `web/src/components/alias-form.tsx`
  - [ ] Change `useEffect` fetch URL from `/configured-models` to `/remote-voices`
  - [ ] Update local `voices` state type from `string[]` to `{id: string; name: string}[]`
  - [ ] Update Select rendering: option `value={v.id}`, label `{v.name || v.id}`; show `id` as secondary text when `name !== id`
  - [ ] Preserve free-text fallback for empty / failing probes
- [ ] Update tests in `web/src/components/alias-form.test.tsx`
  - [ ] Update existing fetch mocks to return `[]tts.Voice` JSON shape
  - [ ] Add test: dropdown lists names from `/remote-voices`, submits IDs
  - [ ] Add test: fetch returns 502 → form falls back to text input and submits typed value
  - [ ] Add test: fetch returns `[]` → form falls back to text input
  - [ ] Add test: in-flight request is aborted when endpoint selection changes
- [ ] Update `web/src/lib/api.ts` if a typed helper for `/remote-voices` is added (OPTIONAL — only if the existing fetch is moved into the API client)
- [ ] Verify `web/src/components/alias-form.test.tsx` and `web/src/pages/aliases.test.tsx` retain 100% coverage with `cd web && bun run test`

## Open Questions

- [ ] Should the alias form display remote-voice errors inline (e.g. a small "couldn't load voices, enter a voice ID below" hint) instead of silently falling back? Current behaviour is silent; this change preserves that. Defer to operator feedback.

## References

- Spec: [voice-resolution](../specs/voice-resolution/), [frontend](../specs/frontend/), [http-api](../specs/http-api/)
- Related changes: [0004-endpoint-models-toggle](./0004-endpoint-models-toggle.md), [0005-endpoint-voice-toggle](./0005-endpoint-voice-toggle.md)
- Code: `web/src/components/alias-form.tsx:48`, `internal/api/endpoints.go:338` (`DiscoverRemoteVoices`), `internal/tts/client.go:19` (`Voice` struct)
