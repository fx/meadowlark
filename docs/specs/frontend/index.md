# Frontend

Living specification for Meadowlark's Preact-based admin UI — routing, pages, data fetching hooks, theme system, and UI component library.

## Overview

Meadowlark's frontend is a Preact single-page application embedded in the Go binary via `//go:embed`. It provides a playground for trying voices and creating and editing voice aliases, a voice list that merges aliases with canonical voices, a management interface for TTS endpoints, and a server status dashboard. The UI uses Tailwind CSS v4 with an OKLch color palette, zero border-radius, JetBrains Mono font, and full dark mode support.

**Directory:** `web/`

## Technology Stack

| Layer | Technology | Version |
|-------|-----------|---------|
| Framework | Preact | 10.28.3 |
| Router | Wouter | 3.9.0 |
| Styling | Tailwind CSS v4 | 4.2.0 |
| Build Tool | Vite | 7.3.1 |
| Icons | Phosphor Icons | 2.1.10 |
| UI Primitives | Radix UI | 11 primitives |
| Variant Styling | CVA | 0.7.1 |
| Class Merging | clsx + tailwind-merge | 2.1.1 / 3.5.0 |
| Font | JetBrains Mono | 5.2.8 |
| MP3 encoding | @breezystack/lamejs (lazy-loaded) | 1.2.7 |
| Testing | Vitest + @testing-library/preact | 4.0.18 |
| Coverage | @vitest/coverage-v8 | v8 provider |
| E2E | Playwright | 1.58.2 |
| Linting | Biome | 2.3.11 |
| Language | TypeScript | 5.9.3 |

## Routing

Wouter provides client-side routing:

| Path | Page | Description |
|------|------|-------------|
| `/` | (redirect) | Redirects to `/playground` |
| `/playground` | PlaygroundPage | Try voices; save the settings as an alias |
| `/playground/:aliasId` | PlaygroundPage | Edit an alias in the playground |
| `/voices` | VoicesPage | Aliases and canonical voices |
| `/aliases` | (redirect) | Redirects to `/voices` |
| `/endpoints` | EndpointsPage | TTS endpoint management |
| `/settings` | SettingsPage | Server status dashboard |

Both playground paths are one `<Route path="/playground/:aliasId?">`, so moving between them does not remount the route.

The Go backend serves `index.html` for all non-API, non-file requests (SPA fallback).

## Layout

### AppHeader

Sticky top navigation bar with:
- **Left:** "Meadowlark" brand text + icon-only nav buttons (Phosphor Icons):
  - Waveform → Playground
  - SpeakerHigh → Voices
  - PlugsConnected → Endpoints
  - GearSix → Settings
  - A nav item is active on its own path and on any path below it (`/playground/abc` highlights Playground).
- **Right:** Version string + ThemeToggle dropdown
- **Mobile (<md):** Hamburger button opens a self-managed Sheet with full navigation + theme buttons

### Design Tokens

- Full-width layout (no `container mx-auto`), pages use `p-4` padding.
- Zero border-radius on all elements (`--radius-*: 0`).
- JetBrains Mono for all text (both `--font-mono` and `--font-sans`).
- OKLch color palette with cyan primary (`oklch(0.68 0.13 184)`).
- Dark mode primary dimmed to `oklch(0.56 0.12 184)`.
- Light borders: `oklch(0.85 0 0)`, dark borders: `oklch(0.35 0 0)`.

## Pages

### Endpoints Page (`/endpoints`)

CRUD list with expandable inline forms.

**Collapsed row:** Name, base URL (truncated), enabled-model count badge, enabled-voice count badge, enabled switch (immediate API call on toggle).

**Expanded form (EndpointForm):**

The form is organized into clearly delimited sections (Card-grouped `<fieldset>` blocks), not a single flat grid:

1. **Connection** — Base URL with validation + auto-probe button; API key with show/hide toggle; Enabled switch (lives at the top of this section, not in a grid alongside numeric inputs).
2. **Models** — Toggle list of upstream-discovered models. One row per discovered model with a `Switch`, the model ID, and a `RadioGroup` marker that designates the **default model**. All discovered models MUST default to disabled. The default-model radio MUST only be selectable for enabled rows. If the user enables a model and no default has been chosen, the form MUST auto-select that model as the default. If the user disables the current default, the form MUST move the default to the next enabled model (or clear it when none remain).
3. **Voices** — Toggle list of upstream-discovered voices. One row per discovered voice with a `Switch`, the voice ID, and the human-readable name. All discovered voices MUST default to disabled. A "Refresh from endpoint" button MUST re-probe and merge new voices into the list (preserving enabled state of voices already present).
4. **Defaults** — Default voice selection (Select populated from the voices the user has enabled in section 3); default speed (Input with `step=0.05`, `min=0.25`, `max=4.0`); default instructions (Textarea). These three fields are stacked vertically; they MUST NOT be co-located in a grid with the Enabled switch.
5. **Streaming** (existing) — Streaming toggle and stream sample rate (unchanged from change 0002).

**Actions:** Test connectivity (measures latency), refresh voices, delete with AlertDialog confirmation.

**Create:** "+ Add Endpoint" button expands a blank form at the top of the list. The Models and Voices sections render an empty state ("Probe the endpoint to discover models and voices") until the first probe succeeds.

### Playground Page (`/playground`)

Where voices are tried and aliases are made — both are the same loop of trying endpoints, models, voices, speed, and instructions until the output sounds right. Two columns at `md` and up (a `20rem` settings panel, then the text, player, and takes), stacked below.

**Settings panel (PlaygroundSettings):**
- Endpoint Select; disabled endpoints are listed with a "(disabled)" suffix and may still be synthesized. Changing the endpoint resets the model to its `default_model` (else its first model) and the voice to its `default_voice` (else empty).
- Model Select over the endpoint's `models`.
- Voice Combobox suggesting the endpoint's enabled saved voices (`GET /api/v1/endpoints/{id}/voices`, label `name || voice_id`, value `voice_id`). Free text is accepted, for `clone:*` and other voices the endpoint does not list. The lookup is aborted when the endpoint changes, and a late answer is ignored.
- Speed (number, step 0.05, 0.25–4.0) and Instructions; empty means "endpoint default".
- Alias mode only: an "Alias" fieldset with Name (required) and Languages (comma-separated).

**Text:** a Textarea with an `n / 4096` counter in code points. Over the limit the counter turns destructive and Generate is disabled. Ctrl+Enter or ⌘+Enter generates. The text lives in the take store, so it survives navigation and switching aliases.

**Generate:** disabled until endpoint, model, voice, and text are set. While a request is in flight it becomes Cancel (aborts it; no error is shown). It calls `api.synthesize` with `speed` only when set and `instructions` only when non-empty — the same fields an alias with those settings would send — and measures latency from send to the full body with `performance.now()`. The WAV is parsed and becomes a **take**, which is selected and auto-played (a rejected `play()` is ignored). API errors show inline in `text-destructive`; audio that does not parse shows "Could not decode audio".

**Main player:** the selected take's [Waveform](#waveform), a format line (`24 kHz · 16-bit · mono · 3.42 s · 160 KB · 1.24 s latency`), and its downloads. With no takes, a dashed placeholder.

**Takes:** newest first, at most 20. Each row shows `#n`, voice · model · endpoint, speed and instructions when set, the text (truncated), latency → duration, and a compact waveform drawn in the played colour when selected. Clicking the waveform selects and plays the take in the main player, for A/B comparison. Row actions: **Restore settings** (copies the take's endpoint, model, voice, speed, instructions, and text into the panel), WAV and MP3 downloads, and Remove. "Clear" empties the list.

**Downloads:** WAV is an `<a download>` over the take's object URL, so the file is the exact bytes the server returned. MP3 is encoded in the browser from those bytes at 128 kbps; it is disabled, with the title "MP3 needs a standard sample rate (got N Hz)", unless the rate is 8000, 11025, 12000, 16000, 22050, 24000, 32000, 44100, or 48000 Hz. Files are named `meadowlark-<voice>-<YYYYMMDD-HHMMSS>.<ext>`, the voice made filename-safe (`clone:foo` → `clone-foo`).

**Blank mode (`/playground`):** starts on the first enabled endpoint (else the first), its default model and voice, empty speed and instructions. `?endpoint=<id>&model=<m>&voice=<v>` presets it; an unknown endpoint is ignored, an unknown model for that endpoint falls back to its default, and the voice is taken as free text. With no endpoints the page links to `/endpoints`. **Save as alias** opens a dialog (Name required, Languages default `en`) that `POST`s the panel to `/api/v1/aliases`, shows errors such as `409` inside the dialog, and on success opens `/playground/<new id>`.

**Alias mode (`/playground/:aliasId`):** loads `GET /api/v1/aliases/:id` into the panel, reloading when the id changes, under an "Editing alias <name>" header (with a "disabled" badge when it is). A missing alias shows "Alias not found" and a link to `/playground`. **Save** `PUT`s the panel and is enabled only when it differs from the alias; afterwards the panel is replaced from the response, so it shows what the server stored. **Save as new** opens the same dialog prefilled with `<name> copy` and the current languages, and opens the new alias.

Every save invalidates the `/api/v1/aliases` and `/api/v1/voices` caches.

### Voices Page (`/voices`)

Every voice Meadowlark can speak with, in one table.

- **Rows:** every alias from `GET /api/v1/aliases`, disabled ones included, sorted by name; then the canonical voices from `GET /api/v1/voices` (entries with `is_alias = false`). Alias endpoint names come from `GET /api/v1/endpoints`, falling back to "Unknown".
- **Columns:** Voice Name, Voice, Endpoint, Model, Type (Badge "canonical" or "alias"; a disabled alias also gets a "disabled" badge and muted text), Actions.
- **Alias row:** the name links to `/playground/:id`; an "Open in playground" button does the same; an enabled Switch (`PUT {enabled}`); delete with an AlertDialog confirmation. Each mutation refreshes both lists.
- **Canonical row:** "Try in playground" opens `/playground?endpoint=<endpoint_id>&model=<m>&voice=<v>` (URL-encoded).
- **"New alias"** opens a blank playground.
- Search filters both kinds by name, case-insensitively. The page shows "Loading voices..." until aliases and voices have both loaded, and an error if either fails.

### Settings Page (`/settings`)

Read-only server status displayed in Cards:
- Version, uptime (formatted as "Xd Xh Xm Xs")
- Wyoming port, HTTP port
- Database driver
- Voice count, endpoint count, alias count

## Playground Components

### Waveform

`web/src/components/waveform.tsx` — a custom SVG player, themed entirely by Tailwind fill classes so it follows light and dark mode with no JavaScript.

- **Rendering:** an `aria-hidden` SVG with `viewBox="0 0 N 100"` and `preserveAspectRatio="none"`, one bucket per unit (200 buckets in the main player, 64 in the list). Each bucket is a mirrored bar (`<rect>`, no rounding) in two layers: the peak envelope, lighter, behind the RMS body. Bars are normalised to the loudest bucket and never shorter than a thin baseline. The bars are drawn twice — unplayed in `muted-foreground` tones, played in `primary` tones clipped by a `clipPath` rect of width `progress × N`. They rise from the centre line on mount, except under `prefers-reduced-motion`.
- **Playhead:** a 1px `primary` line with a soft glow, moved every animation frame while playing (`requestAnimationFrame`, stopped on pause, end, and unmount) for a smooth sweep.
- **Hover:** a ghost line and a monospace tooltip with the time under the pointer, hidden when the pointer leaves.
- **Labels:** current and total time as `m:ss.s`.
- **Seeking:** a transparent native `<input type="range" aria-label="Seek">` overlays the SVG (zero-width thumb), with `aria-valuetext` such as `0:01.2 of 0:03.4`. Click and drag seek natively; Home and End jump to the ends; the arrow keys step by 5% of the duration (at least 100 ms); Space plays and pauses. A focus ring shows on the waveform while the input has keyboard focus.
- **Playback:** a play/pause button and an unrendered `Audio` element on the take's object URL. `autoPlay` plays on mount. On unmount or a source change the element is paused, its `src` attribute removed and `load()` called, so the browser releases the old take's media resource.
- **Compact:** bars only — no audio, seeking, or labels; `active` draws them in the played colour.

### Take store

`web/src/lib/takes.ts` keeps takes at module level, so they survive navigation but not a reload. Each take holds its parameters (with the endpoint name), text, the WAV `Blob` and its object URL, format, duration, main and mini peaks, latency, and creation time; raw samples are not kept (MP3 encoding re-parses the Blob). Adding a 21st take evicts the oldest. Evicted, removed, and cleared takes revoke their object URLs. `useTakes()` subscribes in a layout effect, so no update is missed between render and subscription.

### Audio helpers

- `lib/wav.ts` — `parseWav` decodes PCM WAV at 8, 16, 24, or 32 bits, skipping unknown chunks (with odd-size padding), and rejects non-RIFF, non-WAVE, non-PCM, a missing `fmt` or `data` chunk, `data` before `fmt`, and truncation.
- `lib/peaks.ts` — `computePeaks(channelData, buckets)` returns per-bucket peak and RMS across channels.
- `lib/mp3.ts` — `encodeMp3` loads `@breezystack/lamejs` with `import()` (its own chunk) and encodes mono or stereo; more channels fold to stereo.
- `lib/download.ts` — download filenames and a temporary-anchor `downloadBlob` that revokes its URL afterwards.

## Data Fetching

### useFetch Hook

```typescript
function useFetch<T>(url: string, ttl?: number): {
    data: T | undefined
    error: Error | undefined
    isLoading: boolean
    mutate: () => void
}
```

- 5-second TTL cache (configurable).
- Request deduplication (multiple components = 1 in-flight request).
- AbortController cleanup on unmount.

`useFetchList<T>(url, ttl?)` is `useFetch<T[]>` for a list endpoint, and the list methods of `api` (`endpoints.list`, `endpoints.voices.list` and `.refresh`, `aliases.list`, `system.voices`) do the same: a `null` body reads as an empty list, so a page never mistakes an empty list for one still loading. Every list a page fetches goes through one of them.
- `mutate()` invalidates cache and refetches.
- `invalidateCache(prefix)` for bulk invalidation.
- `clearCache()` exposed for testing.

### useMutation Hook

```typescript
function useMutation<TInput, TOutput>(url: string, method: string): {
    trigger: (body?: TInput) => Promise<TOutput>
    isMutating: boolean
    error: Error | undefined
}
```

- After success, invalidates the mutation URL's cache.
- Also invalidates parent resource list cache (e.g., `PUT /endpoints/123` invalidates `/endpoints`).

### useEndpointProbe Hook

Specialized hook for endpoint URL discovery:
- Auto-probes URL when it changes (500ms debounce).
- Manual `refresh()` function for button trigger.
- Returns `{ models, voices, status, error, refresh }`.
- Status: `'idle'` | `'loading'` | `'success'` | `'error'`.

### API Client

Typed fetch wrappers in `lib/api.ts`:

```typescript
api.endpoints.list()              // GET /api/v1/endpoints
api.endpoints.get(id)             // GET /api/v1/endpoints/:id
api.endpoints.create(data)        // POST /api/v1/endpoints
api.endpoints.update(id, data)    // PUT /api/v1/endpoints/:id
api.endpoints.delete(id)          // DELETE /api/v1/endpoints/:id
api.endpoints.probe(url, apiKey)  // POST /api/v1/endpoints/probe

api.endpoints.voices.list(id, signal?)  // GET /api/v1/endpoints/:id/voices

api.aliases.list()                // GET /api/v1/aliases
api.aliases.get(id)               // GET /api/v1/aliases/:id
api.aliases.create(data)          // POST /api/v1/aliases
api.aliases.update(id, data)      // PUT /api/v1/aliases/:id
api.aliases.delete(id)            // DELETE /api/v1/aliases/:id

api.synthesize(req, signal?)      // POST /api/v1/synthesize → ArrayBuffer (WAV)

api.system.status()               // GET /api/v1/status
api.system.voices()               // GET /api/v1/voices
```

**Error handling:** `ApiRequestError` custom class with `status` and `code` fields. `api.synthesize` cannot use the JSON request helper: on success it returns the body as an `ArrayBuffer`; on failure it throws the JSON error envelope as an `ApiRequestError`, or `ApiRequestError(status, 'http_error', 'HTTP <status>')` when the body is not one.

## State Management

No global state library. Uses:

1. **Context API** — `ThemeProvider` for dark/light/system theme.
2. **Local useState** — form inputs and component state.
3. **Cache-based state** — `useFetch()` in-memory cache with TTL serves as server state management.
4. **Take store** — the playground's takes and text, at module level (see [Take store](#take-store)).

## Theme System

### ThemeProvider

- Three modes: `'light'`, `'dark'`, `'system'`.
- Persists to `localStorage` key `"meadowlark-theme"`.
- Applies `.light` or `.dark` class to `<html>` root.
- Listens to `prefers-color-scheme` media query when mode is `'system'`.

### ThemeToggle

- Dropdown menu with Light/Dark/System options.
- Text-only dropdown items, no icons.
- Located in header (desktop) and mobile menu (mobile).

## UI Components

17 Radix UI-wrapped components in `components/ui/`:

| Component | Radix Primitive | Key Features |
|-----------|----------------|--------------|
| Button | — | Variants: default, destructive, outline, secondary, ghost, link. Sizes: xs–lg, icon variants |
| Input | — | Standard text input |
| Textarea | — | Multi-line text |
| Label | @radix-ui/react-label | Form labels |
| Select | @radix-ui/react-select | Styled select dropdown |
| Combobox | Custom (Popover-based) | Searchable dropdown with filtering |
| Switch | @radix-ui/react-switch | Toggle switch |
| Badge | — | Status badges |
| Card | — | Content cards |
| Table | — | Data tables |
| Dialog | @radix-ui/react-dialog | Modal dialogs |
| AlertDialog | @radix-ui/react-alert-dialog | Confirmation dialogs |
| DropdownMenu | @radix-ui/react-dropdown-menu | Context menus |
| Menubar | @radix-ui/react-menubar | Navigation bar |
| Sheet | @radix-ui/react-dialog | Slide-out panels |
| Popover | @radix-ui/react-popover | Floating content |
| Tooltip | @radix-ui/react-tooltip | Hover tooltips |

All components use CVA for variant definitions and `cn()` (clsx + tailwind-merge) for class merging.

The Dialog, AlertDialog and Sheet overlays are `forwardRef` components. Radix's dialog portal wraps each child in `Presence`, which reads the child's computed style through a ref; Preact hands a ref on a plain function component the component instance, not its DOM node, so an overlay that does not forward its ref throws and the dialog never opens.

## Testing

### Coverage Requirements

Vitest enforces 100% thresholds:

```
lines: 100
functions: 100
branches: 100
statements: 100
```

Excludes: `src/test-*.tsx`, `src/main.tsx`.

### Test Mocks

Required mocks for Radix UI compatibility in jsdom:
- `@floating-ui/react-dom`
- `@radix-ui/react-focus-scope`
- `@phosphor-icons/react`
- `react-remove-scroll`
- `ResizeObserver`, `matchMedia`, `scrollIntoView`, pointer capture APIs
- `URL.createObjectURL` (returns `blob:mock/<n>`) and `URL.revokeObjectURL`, `HTMLMediaElement.prototype.play` (resolves), `pause` and `load`, `requestAnimationFrame` and `cancelAnimationFrame` — fresh mocks per test
- `@/components/ui/select` via `src/test-select-mock.tsx`, where a test drives a select through its `onValueChange`, since Radix Select does not open in jsdom
- `@radix-ui/react-presence` runs for real, so a dialog portal child that drops its ref fails the tests as it fails in a browser
- wouter runs for real (inlined, with its `use-sync-external-store` shim aliased to `preact/compat`); page tests route with `wouter/memory-location`

### Build Configuration

**Vite:** Preact preset, Tailwind plugin, `@` path alias to `src/`, `host: '0.0.0.0'`.

**TypeScript:** `target: ES2020`, `jsx: react-jsx`, `jsxImportSource: preact`, strict mode.

**Biome:** 2-space indent, 100 line width, single quotes, no semicolons.

**Playwright:** `baseURL: http://localhost:8080`, chromium, 15s timeout.

## Embedding

The frontend is embedded in the Go binary:

```go
// web.go
//go:embed all:web/dist
var WebFS embed.FS
```

Build order: `bun run build` produces `web/dist/` → `go build` embeds it.

## Files

| File | Purpose |
|------|---------|
| `web/src/main.tsx` | Entry point (Preact render) |
| `web/src/app.tsx` | Root component with routing |
| `web/src/lib/api.ts` | Typed API client + interfaces |
| `web/src/lib/utils.ts` | `cn()` utility |
| `web/src/lib/wav.ts` | PCM WAV decoder |
| `web/src/lib/peaks.ts` | Waveform peak/RMS buckets |
| `web/src/lib/mp3.ts` | Lazy-loaded MP3 encoding |
| `web/src/lib/download.ts` | Download filenames and blob downloads |
| `web/src/lib/takes.ts` | Playground take store |
| `web/src/hooks/use-fetch.ts` | SWR-like data fetching |
| `web/src/hooks/use-mutation.ts` | Mutation hook with cache invalidation |
| `web/src/hooks/use-endpoint-probe.ts` | Endpoint discovery hook |
| `web/src/components/app-header.tsx` | Navigation header |
| `web/src/components/app-mobile-menu.tsx` | Mobile navigation sheet |
| `web/src/components/theme-provider.tsx` | Dark/light/system theme context |
| `web/src/components/theme-toggle.tsx` | Theme dropdown |
| `web/src/components/endpoint-form.tsx` | Endpoint create/edit form |
| `web/src/components/playground-settings.tsx` | Playground settings panel |
| `web/src/components/take-player.tsx` | Main take player, take list, downloads |
| `web/src/components/waveform.tsx` | SVG waveform player |
| `web/src/components/expandable-row.tsx` | Collapsible row wrapper |
| `web/src/pages/endpoints.tsx` | Endpoints management page |
| `web/src/pages/playground.tsx` | Playground page (blank and alias modes) |
| `web/src/pages/voices.tsx` | Aliases and canonical voices |
| `web/src/pages/settings.tsx` | Server status page |
| `web/src/styles/globals.css` | Tailwind + OKLch theme tokens |

## Changelog

| Date | Description | Document |
|------|-------------|----------|
| 2026-04-19 | Initial living spec created from implementation audit | --- |
| 2026-05-04 | Documented endpoint form section restructure (toggle lists for models/voices, default-model radio, Enabled out of the speed grid); fixed alias form voice selection to fetch `/remote-voices` instead of `/configured-models` | [0003-alias-form-voice-fix](../../changes/0003-alias-form-voice-fix.md), [0004-endpoint-models-toggle](../../changes/0004-endpoint-models-toggle.md), [0005-endpoint-voice-toggle](../../changes/0005-endpoint-voice-toggle.md) |
| 2026-10-29 | Added the Playground page (default route; blank and alias modes, takes, waveform player, WAV/MP3 downloads) and merged aliases into the Voices page; removed the Aliases page and its form, whose `/remote-voices` description had drifted from the implementation | [0009](../../changes/0009-voice-playground.md) |
| 2026-10-29 | Dialog, AlertDialog and Sheet overlays forward their refs, so dialogs open in a browser; `@radix-ui/react-presence` is no longer mocked; list fetches read a `null` body as an empty list (`useFetchList` and the `api` list methods); the waveform releases its media source on cleanup | [0009](../../changes/0009-voice-playground.md) |
