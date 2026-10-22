# 0008: Endpoint test uses a real voice

## Summary

The endpoint **Test** action (`POST /api/v1/endpoints/{id}/test`) will synthesize with a voice the endpoint actually has, instead of the hard-coded voice `"test"`. It picks the endpoint's default voice, else its first saved voice that is enabled and has a non-empty id, else the first voice with a non-empty id that the endpoint reports, and only then falls back to `"test"`. It never sends an empty voice.

**Spec:** [http-api](../specs/http-api/)
**Status:** draft
**Depends On:** 0005

## Motivation

`TestEndpoint` sends `voice: "test"`. An OpenAI-compatible backend that has no voice by that name answers the request with an error, so the test reports a working endpoint as broken. vLLM-Omni is one: it treats `"test"` as a request for a built-in voice, and a server configured with only cloned voices fails it.

Every endpoint already carries better candidates — a configured default voice, the voices saved by a voice refresh ([0005](./0005-endpoint-voice-toggle.md)), or the voices the backend lists — so the test can ask for one that exists.

## Requirements

### Testing Requirements

This change MUST satisfy the project's standing testing rules (see [CLAUDE.md — Build Commands](../../CLAUDE.md) and [meadowlark.md §15 — Testing Strategy](../meadowlark.md#15-testing-strategy)). CI enforces these as merge gates:

- `go test -race ./...` MUST pass.
- `go vet ./...` MUST pass with no new suppressions.
- Each step of the selection order, and each fall-through, MUST have a direct handler test in `internal/api/endpoints_test.go` against a mock TTS server that records the voice it was asked for.
- No frontend change is in scope, so Vitest MUST continue to pass at its existing 100% thresholds untouched, and Biome MUST pass without new suppressions.

Skipping or weakening any of these rules to land the PR MUST be treated as a bug in the PR, not in the rule.

### Functional requirements

The [http-api spec — TestEndpoint](../specs/http-api/index.md#testendpoint) owns the voice selection order, what counts as "first", the fall-through on failure, the unchanged response shape, and their scenarios. The [data-persistence spec — Store Interface](../specs/data-persistence/index.md#requirements) owns the order `ListEndpointVoices` returns rows in, which "first enabled saved voice" depends on. Those scenarios are this change's acceptance criteria and are not restated here. What implementing them requires of this change:

- The handler MUST take the saved voices in the order the store returns them and MUST NOT re-sort them; the ordering is a store contract, not handler logic.
- Both list steps skip empty ids: the schema allows an empty saved `voice_id`, and `ListVoices` does not drop empty ids in every response format it parses, so an implementation that takes "the first row" literally could send an empty voice.
- Voice selection MUST run before the latency timer starts, so `latency_ms` keeps measuring the synthesis request alone, even when selection had to ask the endpoint for its voices.
- Nothing else about the test request changes: model, input text, response format, and the buffered (non-streaming) request path stay as they are.

## Design

### Approach

All code changes are in `internal/api/endpoints.go` and its tests. A small unexported helper — for example `testVoice(ctx, ep, client) string` — implements the selection order, and `TestEndpoint` passes its result as the request's voice in place of `"test"`. It reads `ep.DefaultVoice`, then `s.store.ListEndpointVoices`, then `client.ListVoices`.

### Decisions

- **No new response field.** The response stays `{ok, error?, latency_ms}`.
  - **Why**: The selection is deterministic from data the UI already shows (the endpoint's default voice and its voice list), the frontend has no place for it, and when synthesis fails the upstream's error usually names the voice anyway. A field would be an API surface with no consumer.
  - **Alternatives considered**: adding `voice` to the response — rejected for now; it can be added later without breaking anything.
- **A store error while listing saved voices falls through rather than failing the test.** It is logged at warn level and selection continues with the endpoint's reported voices.
  - **Why**: The endpoint was just read from the same store, so a failure here is transient, and the test exists to check the upstream, not the database. Failing it would report the endpoint as broken for a reason unrelated to the endpoint.
  - **Alternatives considered**: returning `500 internal_error` like a failed `GetEndpoint` — rejected for the reason above.
- **Disabled saved voices are skipped, but not excluded from the reported-voices step.** If no saved voice is both enabled and non-empty, the next step takes the backend's first reported voice with a non-empty id, which may be one of the disabled ones.
  - **Why**: The enabled flag governs which voices Meadowlark advertises to Wyoming clients ([0005](./0005-endpoint-voice-toggle.md)); it says nothing about whether the voice works. Preferring enabled voices keeps the test on voices the user actually uses, and the later steps exist precisely for when there are none.
- **`DefaultVoice` counts when non-empty, untrimmed** — the same test the resolver's default-voice lookup applies, so the two never disagree about whether an endpoint has a default voice.
- **The literal `"test"` remains the last resort.** An endpoint with no default voice, no saved voice that is enabled with a non-empty id, and no listed voice with a non-empty id still gets tested, exactly as today.

### Non-Goals

- `ProbeEndpoint` — it lists models and voices and never synthesizes, so it has no dummy voice to replace.
- `TestAlias` — it already uses the alias's configured voice.
- Exercising the streaming path from the Test action, a new endpoint configuration field, or reporting the chosen voice.
- Frontend changes.
- Configuration of any deployed instance.

## Tasks

- [ ] Pick a real voice for the endpoint test (one PR)
  - [ ] `internal/api/endpoints.go`: add the voice-selection helper and use it in `TestEndpoint`, selecting before the latency timer starts
  - [ ] `internal/api/endpoints_test.go`: extend the mock TTS server to record the requested voice and to serve `/audio/voices`; give `endpointMockStore.ListEndpointVoices` an injectable error
  - [ ] Test: `DefaultVoice` set → it is used, even when enabled saved voices and reported voices exist, and `/audio/voices` is never requested
  - [ ] Test: no `DefaultVoice`, saved voices `[a (disabled), b (enabled), c (enabled)]` → `b`
  - [ ] Test: no `DefaultVoice`, saved voices `["" (enabled), d (enabled)]` → `d` (an enabled row with an empty `voice_id` is skipped)
  - [ ] Test: no `DefaultVoice`, all saved voices disabled or none saved → the first voice `/audio/voices` reports
  - [ ] Test: no saved voice usable, `/audio/voices` lists an entry with an empty `id` first, then `e` → `e`; and a list containing only empty ids → `"test"`
  - [ ] Test: store error from `ListEndpointVoices` → falls through to the reported voices, and the test still returns `200`
  - [ ] Test: `/audio/voices` answers 404, an empty list, or malformed JSON → `"test"`
  - [ ] Update the [http-api spec — TestEndpoint](../specs/http-api/index.md#testendpoint) to drop the "planned" marker and add a changelog row
  - [ ] Set this document's **Status** to `complete` and update `docs/index.yml` and `docs/index.md` to match

## Open Questions

None.

## References

- Spec: [http-api](../specs/http-api/), [data-persistence](../specs/data-persistence/)
- Related changes: [0005-endpoint-voice-toggle](./0005-endpoint-voice-toggle.md), [0007-vllm-omni-streaming-compat](./0007-vllm-omni-streaming-compat.md)
