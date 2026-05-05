# Frontend

Living specification for Meadowlark's Preact-based admin UI — routing, pages, data fetching hooks, theme system, and UI component library.

## Overview

Meadowlark's frontend is a Preact single-page application embedded in the Go binary via `//go:embed`. It provides a management interface for TTS endpoints and voice aliases, a read-only voice discovery view, and a server status dashboard. The UI uses Tailwind CSS v4 with an OKLch color palette, zero border-radius, JetBrains Mono font, and full dark mode support.

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
| Testing | Vitest + @testing-library/preact | 4.0.18 |
| Coverage | @vitest/coverage-v8 | v8 provider |
| E2E | Playwright | 1.58.2 |
| Linting | Biome | 2.3.11 |
| Language | TypeScript | 5.9.3 |

## Routing

Wouter provides client-side routing with four routes:

| Path | Page | Description |
|------|------|-------------|
| `/` | (redirect) | Redirects to `/endpoints` |
| `/endpoints` | EndpointsPage | TTS endpoint management |
| `/voices` | VoicesPage | Voice discovery listing |
| `/aliases` | AliasesPage | Voice alias management |
| `/settings` | SettingsPage | Server status dashboard |

The Go backend serves `index.html` for all non-API, non-file requests (SPA fallback).

## Layout

### AppHeader

Sticky top navigation bar with:
- **Left:** "Meadowlark" brand text + icon-only nav buttons (Phosphor Icons):
  - PlugsConnected → Endpoints
  - SpeakerHigh → Voices
  - TagSimple → Aliases
  - GearSix → Settings
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

### Voices Page (`/voices`)

Read-only voice discovery table.

**Features:**
- Search/filter by voice name (client-side)
- Table columns: Voice Name, Voice, Endpoint, Model, Type (Badge: "canonical" or "alias")

### Aliases Page (`/aliases`)

CRUD list with expandable inline forms.

**Collapsed row:** Alias name, target voice, endpoint name badge, enabled switch.

**Expanded form (AliasForm):**
- Alias name
- Endpoint selection (dropdown, loads from API)
- Model selection — Select populated from the endpoint's enabled `models` list (`Endpoint.Models`).
- Voice selection — Select populated by fetching `GET /api/v1/endpoints/{id}/remote-voices`, which returns `[]tts.Voice` (`{id, name}`). The dropdown MUST display the voice's `name` (with the `id` as a secondary label when they differ) and submit the `id`. If the live probe fails or returns an empty list, the form MUST fall back to a free-text Input so operators can still configure aliases that target voices the upstream did not enumerate (e.g. cloned voices created out-of-band). The Voice select MUST NOT be populated from `/configured-models` — that endpoint returns model IDs, not voice IDs, and using it conflates the two fields.
- Speed (optional)
- Instructions (optional)
- Languages (comma-separated)
- Enabled switch

The Voice list is **not** filtered by the per-endpoint enabled-voices set. Aliases are an explicit opt-in by name and may target any voice the upstream provider exposes, including disabled or dynamic voices (e.g. `clone:*` from qwen3-tts).

**Actions:** Test TTS (play button), delete with AlertDialog confirmation.

### Settings Page (`/settings`)

Read-only server status displayed in Cards:
- Version, uptime (formatted as "Xd Xh Xm Xs")
- Wyoming port, HTTP port
- Database driver
- Voice count, endpoint count, alias count

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

api.aliases.list()                // GET /api/v1/aliases
api.aliases.create(data)          // POST /api/v1/aliases
api.aliases.update(id, data)      // PUT /api/v1/aliases/:id
api.aliases.delete(id)            // DELETE /api/v1/aliases/:id
api.aliases.test(id)              // POST /api/v1/aliases/:id/test

api.system.status()               // GET /api/v1/status
api.system.voices()               // GET /api/v1/voices
```

**Error handling:** `ApiRequestError` custom class with `status` and `code` fields.

## State Management

No global state library. Uses:

1. **Context API** — `ThemeProvider` for dark/light/system theme.
2. **Local useState** — form inputs and component state.
3. **Cache-based state** — `useFetch()` in-memory cache with TTL serves as server state management.

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
- `@radix-ui/react-presence`
- `react-remove-scroll`
- `ResizeObserver`, `matchMedia`, `scrollIntoView`, pointer capture APIs

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
| `web/src/hooks/use-fetch.ts` | SWR-like data fetching |
| `web/src/hooks/use-mutation.ts` | Mutation hook with cache invalidation |
| `web/src/hooks/use-endpoint-probe.ts` | Endpoint discovery hook |
| `web/src/components/app-header.tsx` | Navigation header |
| `web/src/components/app-mobile-menu.tsx` | Mobile navigation sheet |
| `web/src/components/theme-provider.tsx` | Dark/light/system theme context |
| `web/src/components/theme-toggle.tsx` | Theme dropdown |
| `web/src/components/endpoint-form.tsx` | Endpoint create/edit form |
| `web/src/components/alias-form.tsx` | Alias create/edit form |
| `web/src/components/expandable-row.tsx` | Collapsible row wrapper |
| `web/src/pages/endpoints.tsx` | Endpoints management page |
| `web/src/pages/voices.tsx` | Voice listing page |
| `web/src/pages/aliases.tsx` | Alias management page |
| `web/src/pages/settings.tsx` | Server status page |
| `web/src/styles/globals.css` | Tailwind + OKLch theme tokens |

## Changelog

| Date | Description | Document |
|------|-------------|----------|
| 2026-04-19 | Initial living spec created from implementation audit | --- |
| 2026-05-04 | Documented endpoint form section restructure (toggle lists for models/voices, default-model radio, Enabled out of the speed grid); fixed alias form voice selection to fetch `/remote-voices` instead of `/configured-models` | [0003-alias-form-voice-fix](../../changes/0003-alias-form-voice-fix.md), [0004-endpoint-models-toggle](../../changes/0004-endpoint-models-toggle.md), [0005-endpoint-voice-toggle](../../changes/0005-endpoint-voice-toggle.md) |
