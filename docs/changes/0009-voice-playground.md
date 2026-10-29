# 0009: Voice playground

## Summary

Bring back a web UI for trying voices, as a **Playground** page that is also where aliases are created and edited — both are the same flow of trying endpoints, models, voices, speed and instructions until the output sounds right. A new stateless `POST /api/v1/synthesize` returns the audio as WAV through the same segment-opening path the Wyoming proxy uses, so a streaming endpoint sounds in the playground exactly as it does in Home Assistant. The playground keeps an in-memory history of takes with latency and format details, renders each take as an interactive waveform, and downloads it as WAV or MP3. The separate Aliases page is merged into the Voices page, whose rows open the playground, and `POST /api/v1/aliases/{id}/test` is removed.

**Spec:** [frontend](../specs/frontend/), [http-api](../specs/http-api/), [tts-synthesis](../specs/tts-synthesis/)
**Status:** in-progress
**Depends On:** 0005, 0007

## Motivation

The only way to hear a voice today is to wire it into Home Assistant. The Aliases page has a **Test TTS** button, but `TestAlias` discards the audio and reports `{ok}`, and it only works for an alias that already exists. Choosing a voice, a speed or instructions is trial and error against an assistant, with no way to compare two attempts.

Creating an alias is that same trial and error, ending in a save. Splitting it across an Aliases form and a separate test action makes the user build the alias before they can hear it. The Voices page already lists aliases next to canonical voices, so a separate Aliases page is redundant.

## Requirements

### Testing Requirements

This change MUST satisfy the project's standing testing rules (see [frontend spec — Coverage Requirements](../specs/frontend/index.md#coverage-requirements) and [CLAUDE.md — Build Commands](../../CLAUDE.md)):

- Vitest 100% coverage MUST hold for all new and modified frontend code. New components and modules MUST have dedicated test files.
- `go test -race ./...` MUST pass; the new handler and TTS helpers MUST have tests in `internal/api/` and `internal/tts/`.
- `go vet ./...` MUST pass.
- Biome MUST pass without new suppressions.

Skipping or weakening any of these rules to land the PR MUST be treated as a bug in the PR, not in the rule.

### R1: Stateless synthesize endpoint

`POST /api/v1/synthesize` MUST synthesize `{endpoint_id, model, voice, text, speed?, instructions?}` against the named endpoint and return `200` with `Content-Type: audio/wav`, `Content-Length` and `Cache-Control: no-store`, and a body that is a canonical 44-byte PCM WAV header with correct sizes followed by the PCM. The [http-api spec — Synthesize](../specs/http-api/index.md#synthesize) owns the request shape, validation order and messages, status codes, size limits and timeouts.

- Omitted `speed` and `instructions` MUST fall back to the endpoint's defaults, exactly as for an alias in the Wyoming path.
- The text MUST be sent literally; Wyoming JSON/tag overrides are not parsed.
- The upstream request MUST be opened through the function the Wyoming proxy uses ([tts-synthesis spec — Open](../specs/tts-synthesis/index.md)), so streaming endpoints stream raw PCM at their configured sample rate and buffered endpoints return WAV.
- Every upstream or open failure MUST be `502 upstream_error`, and nothing may be written before an error.
- A request MUST be allowed to run for up to 5 minutes despite the server's 30-second `WriteTimeout`.

#### Scenario: streaming endpoint

- **GIVEN** an endpoint with `streaming_enabled = true` and `stream_sample_rate = 16000`
- **WHEN** the playground synthesizes text against it
- **THEN** the upstream request MUST carry `stream: true`, `response_format: "pcm"` and `stream_format: "audio"`
- **AND** the response MUST be a WAV at 16000 Hz, 16-bit, mono, whose data is the upstream PCM

#### Scenario: buffered endpoint with a streaming WAV header

- **GIVEN** a buffered endpoint whose WAV response declares a data size of `0` or `0xFFFFFFFF`
- **WHEN** the playground synthesizes against it
- **THEN** the response header MUST declare the real data size

### R2: Remove the alias test action

`POST /api/v1/aliases/{id}/test` MUST be removed and answer `404`; the playground replaces it. Its only consumer is the bundled UI.

### R3: Voice list entries carry the endpoint id

Each `GET /api/v1/voices` entry MUST include `endpoint_id`, so a canonical voice can open the playground preset to its endpoint. The change is additive.

### R4: Playground page

The frontend MUST serve a Playground at `/playground` (the default route; `/` redirects there) and `/playground/:aliasId` (alias mode). The [frontend spec — Playground Page](../specs/frontend/index.md#playground-page) owns its layout and behaviour. In summary:

- A settings panel (endpoint, model, free-text voice with the endpoint's enabled voices as suggestions, speed, instructions), a text area limited to 4096 characters, and Generate/Cancel.
- Each generation becomes a **take**: kept in memory (at most 20, oldest evicted) across in-app navigation, listed newest first with its parameters, latency and duration, and selectable into the main player for A/B comparison.
- **Restore settings** copies a take's parameters and text back into the panel.
- Each take downloads as the exact WAV bytes the server returned, or as MP3 encoded in the browser.
- Blank mode offers **Save as alias**. Alias mode loads the alias, and offers **Save** (enabled only when changed) and **Save as new**.
- `/playground?endpoint=&model=&voice=` presets a blank playground.

### R5: Waveform player

Every take MUST be shown as a waveform with peak and RMS layers, played and unplayed regions in theme colours, a smooth playhead, a hover cursor with its time, current and total time labels, and click, drag and keyboard seeking. The [frontend spec — Waveform](../specs/frontend/index.md#waveform) owns the details.

### R6: Merged Voices page

The Aliases page MUST be removed and `/aliases` MUST redirect to `/voices`. The Voices page MUST list every alias, disabled ones included, alongside the canonical voices, with search across both. An alias row opens the playground in alias mode and has its enabled switch and delete action; a canonical row opens the playground preset to it. Navigation becomes Playground, Voices, Endpoints, Settings.

## Design

### Approach

- **Backend.** `tts.OpenEndpointSegment` is extracted from `Proxy.openSegment`, which becomes a one-line delegate, and `tts.WAVHeader` encodes a PCM header. A new `Server.Synthesize` handler in `internal/api/synthesize.go` validates, merges parameters with `voice.MergeParams`, opens the segment, reads the PCM into memory, trims it to whole frames, and writes the rebuilt WAV. `statusWriter` gains `Unwrap` so `http.NewResponseController` can extend the write deadline through the request logger.
- **Frontend.** Pure modules `lib/wav.ts` (parse), `lib/peaks.ts` (peak/RMS buckets), `lib/mp3.ts` (lazy-loaded `@breezystack/lamejs`), `lib/download.ts` and `lib/takes.ts` (module-level take store); a `Waveform` SVG component; a `PlaygroundPage` with a `PlaygroundSettings` panel; and a rewritten `VoicesPage`.

### Decisions

- **The text is not parsed for overrides.**
  - **Why**: The playground has fields for every override; parsing would make a JSON-looking test sentence silently change the request.
- **Disabled endpoints may be synthesized.** They are marked "(disabled)" in the endpoint select.
  - **Why**: The playground is where a user checks an endpoint before enabling it, as the endpoint test already allows.
- **Every upstream and open failure is `502 upstream_error`.**
  - **Why**: The request reached a valid endpoint; the failure is the upstream's, and a single code keeps the client simple.
- **The server buffers the whole response and rebuilds the WAV header** (capped at 64 MiB).
  - **Why**: Streaming endpoints return headerless PCM, and buffered endpoints may declare a streaming size of `0` or `0xFFFFFFFF`; a browser needs one well-formed file with a known length, and a single header is what the download contains.
  - **Alternatives considered**: streaming the WAV with a placeholder size — rejected; browsers and downloads would get a header that lies.
- **A custom SVG waveform, not wavesurfer.js.**
  - **Why**: The audio is already parsed for MP3 and peaks, so a library would decode it a second time; theme colours come from Tailwind classes and follow dark mode with no JavaScript; and the component is fully testable in jsdom, which the 100% coverage gate requires.
- **MP3 is encoded in the browser with `@breezystack/lamejs`**, loaded on first use, and only for the standard MP3 sample rates.
  - **Why**: Re-synthesizing in another `response_format` would not be the take the user heard, and server-side transcoding is excluded for v1 ([meadowlark.md §16](../meadowlark.md)). `@breezystack/lamejs` is pure JavaScript, typed and ESM, unlike the original `lamejs`.
- **Takes live in memory only, at most 20.** Evicted, removed and cleared takes revoke their object URLs.
  - **Why**: Session history was what was asked for; persisting audio needs storage this change does not need.
- **Request parity.** The playground sends `speed` only when set and `instructions` only when non-empty, so a take uses exactly what an alias with the same fields would.
- **Saving an alias refreshes the panel from the server's response.**
  - **Why**: `UpdateAlias` cannot clear `speed` or `instructions` (pre-existing); showing the stored values keeps the UI honest until that is fixed.

### Non-Goals

- Persisting takes across reloads.
- Server-side transcoding or a `response_format` passthrough.
- Fixing pre-existing alias handler issues: `CreateAlias` accepting an unknown `endpoint_id`, `UpdateAlias`/`DeleteAlias` failing on a missing id, and `UpdateAlias` being unable to clear `speed`/`instructions`.
- A "Test connectivity" button on the Endpoints page, deduplicating the navigation items, or a catch-all SPA route.
- End-to-end tests against a real TTS backend.

## Tasks

- [x] Share endpoint segment opening and add a WAV header encoder
  - [x] `internal/tts/proxy.go`: extract `OpenEndpointSegment`; `Proxy.openSegment` delegates to it
  - [x] `internal/tts/wav.go`: add `WAVHeader`
  - [x] Test: `WAVHeader` round-trips through `NewWAVReader`; `OpenEndpointSegment` streaming (configured and default rate) and buffered paths
- [x] Add the stateless synthesize endpoint
  - [x] `internal/api/synthesize.go`: validation, parameter merge, open, buffer, trim, rebuilt WAV, timeouts
  - [x] `internal/api/middleware.go`: `statusWriter.Unwrap`
  - [x] Test: buffered and streaming success, header rebuild, every validation and error case, endpoint defaults
- [x] Include `endpoint_id` in voice list entries
- [x] Add frontend audio helpers: WAV parsing, peaks, MP3 encoding, downloads, take store, `api.synthesize`
- [x] Add the waveform player component
- [x] Add the playground page and merge aliases into the Voices page
  - [x] `/` → `/playground`, `/aliases` → `/voices`, `/playground/:aliasId`
  - [x] Navigation: Playground, Voices, Endpoints, Settings
  - [x] Remove the Aliases page and alias form
- [ ] Remove `POST /api/v1/aliases/{id}/test`
- [ ] Update the [frontend](../specs/frontend/), [http-api](../specs/http-api/) and [tts-synthesis](../specs/tts-synthesis/) specs and [meadowlark.md](../meadowlark.md), with changelog rows
- [ ] Set this document's **Status** to `complete` and update `docs/index.yml` and `docs/index.md` to match

## Open Questions

None.

## References

- Spec: [frontend](../specs/frontend/), [http-api](../specs/http-api/), [tts-synthesis](../specs/tts-synthesis/)
- Related changes: [0005-endpoint-voice-toggle](./0005-endpoint-voice-toggle.md), [0007-vllm-omni-streaming-compat](./0007-vllm-omni-streaming-compat.md), [0008-endpoint-test-real-voice](./0008-endpoint-test-real-voice.md)
