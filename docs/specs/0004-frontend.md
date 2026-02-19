# 0004: Frontend

## Overview

Implement the Preact-based admin UI for Meadowlark. This spec covers the full frontend application: routing, layout, theme system, all four pages (Endpoints, Voices, Aliases, Settings), API integration, and 100% test coverage.

## Background

Meadowlark's frontend is embedded in the Go binary and served as a single-page application. It provides a management interface for configuring TTS endpoints and voice aliases. The UI must work on both desktop and mobile.

See `docs/meadowlark.md` section 10 for full frontend requirements.

## Goals

- Preact + Vite + Bun application with production-ready configuration
- Tailwind CSS v4 with exact design tokens (zero border-radius, JetBrains Mono, OKLch cyan primary, dark mode)
- shadcn/ui components adapted for Preact (new-york style)
- SPA routing with 4 pages
- AppHeader with navigation, version display, theme toggle, mobile menu
- Endpoints page: full CRUD with inline forms, test connectivity, discover voices
- Voices page: read-only list with search/filter
- Aliases page: full CRUD with inline forms, endpoint/model/voice selectors, test TTS
- Settings page: server status display
- Lightweight data fetching with SWR-like caching
- Biome linting (single quotes, no semicolons, 2-space indent, 100 line width)
- 100% test coverage with Vitest + jsdom + @testing-library/preact

## Non-Goals

- Server-side rendering (the app is a static SPA)
- Authentication UI
- Real-time updates (polling or manual refresh is sufficient)
- Audio playback in the browser (test endpoints return success/failure, not audio)

## Design

### Technology Stack

| Tool | Version | Purpose |
|---|---|---|
| Preact | latest | UI framework (React-compatible, smaller bundle) |
| Vite | latest | Build tool with HMR |
| Bun | latest (via mise) | Package manager and script runner |
| Tailwind CSS v4 | latest | Utility-first CSS (native Vite plugin) |
| Biome | latest (via mise) | Linter and formatter |
| Vitest | latest | Test runner (jsdom environment) |
| wouter | latest | Lightweight SPA router (~1.5KB) |

### Directory Structure

```
web/
  src/
    components/
      ui/                     # shadcn/ui primitives
        button.tsx
        input.tsx
        label.tsx
        select.tsx
        switch.tsx
        textarea.tsx
        card.tsx
        dialog.tsx
        alert-dialog.tsx
        dropdown-menu.tsx
        menubar.tsx
        sheet.tsx
        badge.tsx
        tooltip.tsx
        table.tsx
        popover.tsx
      app-header.tsx          # Top navigation bar
      app-mobile-menu.tsx     # Mobile slide-out menu
      theme-provider.tsx      # Dark/light/system theme context
      theme-toggle.tsx        # Theme dropdown switcher
      endpoint-form.tsx       # Endpoint create/edit form
      alias-form.tsx          # Voice alias create/edit form
      expandable-row.tsx      # Shared expandable list item
    hooks/
      use-fetch.ts            # SWR-like data fetching hook
      use-mutation.ts         # Mutation hook with cache invalidation
    lib/
      api.ts                  # API client (typed fetch wrappers)
      utils.ts                # cn() utility, formatters
    pages/
      endpoints.tsx           # Endpoints CRUD page
      voices.tsx              # Voices list page
      aliases.tsx             # Aliases CRUD page
      settings.tsx            # Server status page
    styles/
      globals.css             # Tailwind v4 + theme tokens
    app.tsx                   # Root component (router, providers)
    main.tsx                  # Entry point (render to DOM)
  test/
    setup.ts                  # Vitest setup (mocks, polyfills)
  index.html
  package.json
  vite.config.ts
  vitest.config.ts
  tsconfig.json
  biome.json
```

### Styling

#### globals.css

```css
@import "tailwindcss";
@import "tw-animate-css";
@import "@fontsource/jetbrains-mono";

@custom-variant dark (&:where(.dark, .dark *));

@theme {
  --radius-sm: 0;
  --radius-md: 0;
  --radius-lg: 0;
  --radius-xl: 0;

  --font-mono: "JetBrains Mono", ui-monospace, SFMono-Regular, monospace;
  --font-sans: "JetBrains Mono", ui-sans-serif, system-ui, sans-serif;

  /* OKLch color palette */
  --color-background: oklch(1 0 0);
  --color-foreground: oklch(0.145 0 0);
  --color-primary: oklch(0.68 0.13 184);
  --color-primary-foreground: oklch(0.985 0 0);
  /* ... full shadcn/ui neutral + cyan palette ... */
}
```

#### shadcn/ui Adaptation

shadcn/ui components are originally React-based. For Preact:
- Use `preact/compat` for React API compatibility
- Import from `radix-ui` (Preact-compatible via compat)
- Use `class-variance-authority` for variant styling
- Use `clsx` + `tailwind-merge` for the `cn()` utility

### Layout

#### AppHeader

Sticky top bar:
- Left: "Meadowlark" brand
- Center: icon-only nav buttons using Phosphor Icons
  - Endpoints (PlugsConnected)
  - Voices (SpeakerHigh)
  - Aliases (TagSimple)
  - Settings (GearSix)
- Right: version string + ThemeToggle
- Mobile (<md): hamburger button opening a Sheet with full nav

#### Routing

```tsx
<Router>
  <Route path="/endpoints" component={EndpointsPage} />
  <Route path="/voices" component={VoicesPage} />
  <Route path="/aliases" component={AliasesPage} />
  <Route path="/settings" component={SettingsPage} />
  <Redirect from="/" to="/endpoints" />
</Router>
```

### Pages

#### Endpoints Page (`/endpoints`)

CRUD list following the shared expandable-row pattern:

- **Collapsed row**: name, base URL (truncated), model count badge, enabled switch
- **Expanded form fields**:
  - Name (input)
  - Base URL (input)
  - API Key (input, type=password, with show/hide toggle)
  - Models (comma-separated input or tag-style input)
  - Default Speed (input, type=number, step=0.05, range 0.25-4.0, optional)
  - Default Instructions (textarea, optional)
  - Enabled (switch)
- **Actions**: Test (lightning icon), Discover Voices (magnifying glass icon), Delete (trash icon with alert-dialog confirmation)
- **Create**: "+ Add Endpoint" button at top, expands a blank form at the top of the list

#### Voices Page (`/voices`)

Read-only list with search:

- **Search bar**: filters voices by name (client-side)
- **Table columns**: Voice Name, Endpoint, Model, Type (badge: "canonical" or "alias")
- **No CRUD**: voices are derived from endpoints and aliases

#### Aliases Page (`/aliases`)

CRUD list following the shared expandable-row pattern:

- **Collapsed row**: alias name, target voice, endpoint name badge, enabled switch
- **Expanded form fields**:
  - Alias Name (input)
  - Endpoint (select, populated from endpoints API)
  - Model (select, populated based on selected endpoint's models)
  - Voice (select, populated from endpoint's voice discovery or manual input)
  - Speed (input, type=number, optional)
  - Instructions (textarea, optional)
  - Languages (comma-separated input, default "en")
  - Enabled (switch)
- **Actions**: Test (play icon), Delete (trash icon with confirmation)
- **Create**: "+ Add Alias" button

#### Settings Page (`/settings`)

Read-only status cards:

- **Server Info**: version, uptime, build SHA
- **Wyoming**: host, port, connected clients count
- **HTTP**: host, port
- **Database**: driver (sqlite/postgres), connection status
- **Voices**: total count, endpoint count, alias count

### Data Fetching

#### `useFetch<T>(url)` Hook

```ts
function useFetch<T>(url: string): {
  data: T | undefined
  error: Error | undefined
  isLoading: boolean
  mutate: () => void  // refetch
}
```

Simple fetch wrapper with:
- Deduplication of in-flight requests
- Cache with configurable TTL (default 5s)
- `mutate()` to force refetch (used after mutations)
- Error state tracking

#### `useMutation(url, method)` Hook

```ts
function useMutation<TInput, TOutput>(
  url: string,
  method: 'POST' | 'PUT' | 'DELETE'
): {
  trigger: (body?: TInput) => Promise<TOutput>
  isMutating: boolean
  error: Error | undefined
}
```

After successful mutation, invalidates related cache entries.

#### API Client (`lib/api.ts`)

Typed wrapper around `fetch`:

```ts
const api = {
  endpoints: {
    list: () => get<Endpoint[]>('/api/v1/endpoints'),
    get: (id: string) => get<Endpoint>(`/api/v1/endpoints/${id}`),
    create: (data: CreateEndpoint) => post<Endpoint>('/api/v1/endpoints', data),
    update: (id: string, data: UpdateEndpoint) => put<Endpoint>(`/api/v1/endpoints/${id}`, data),
    delete: (id: string) => del(`/api/v1/endpoints/${id}`),
    test: (id: string) => post<TestResult>(`/api/v1/endpoints/${id}/test`),
    voices: (id: string) => get<string[]>(`/api/v1/endpoints/${id}/voices`),
  },
  aliases: {
    list: () => get<VoiceAlias[]>('/api/v1/aliases'),
    // ... same pattern
  },
  system: {
    status: () => get<ServerStatus>('/api/v1/status'),
    voices: () => get<ResolvedVoice[]>('/api/v1/voices'),
  },
}
```

### Theme System

`ThemeProvider` stores the theme preference (`light`, `dark`, `system`) in `localStorage`. Applies the `dark` class to the document root element based on the preference and system `prefers-color-scheme` media query.

`ThemeToggle` renders a dropdown menu with three options (Sun/Moon/Monitor icons).

## Testing

### Strategy

Every component, hook, page, and utility must have a corresponding `.test.tsx` or `.test.ts` file.

### Test Setup (`test/setup.ts`)

- Import `@testing-library/jest-dom/vitest` for DOM matchers
- Mock `fetch` globally
- Provide `IntersectionObserver`, `ResizeObserver`, `matchMedia` mocks

### Component Tests

- Each shadcn/ui component: renders, accepts className, forwards ref
- AppHeader: renders nav links, highlights active route, mobile menu opens/closes
- ThemeToggle: switches themes, persists to localStorage
- ExpandableRow: expand/collapse behavior, only one expanded at a time
- EndpointForm: validation, submission, error display
- AliasForm: cascading selects (endpoint -> model -> voice), validation

### Page Tests

- EndpointsPage: renders list, create flow, edit flow, delete with confirmation, test connectivity
- VoicesPage: renders list, search/filter works
- AliasesPage: renders list, create flow, edit flow, delete with confirmation
- SettingsPage: renders status data

### Hook Tests

- `useFetch`: loading state, success, error, cache hit, mutate refetch
- `useMutation`: trigger, loading state, success, error, cache invalidation

### Coverage

Vitest coverage thresholds enforced:

```ts
coverage: {
  provider: 'v8',
  thresholds: {
    lines: 100,
    functions: 100,
    branches: 100,
    statements: 100,
  }
}
```

## Tasks

- [x] Set up shadcn/ui component library for Preact
  - [x] Configure `preact/compat` aliases in Vite for React compatibility
  - [x] Install Radix UI, class-variance-authority, clsx, tailwind-merge
  - [x] Create `lib/utils.ts` with `cn()` utility
  - [x] Port shadcn/ui primitives: button (with xs/icon variants), input, label, select, switch, textarea
  - [x] Port shadcn/ui primitives: card, dialog, alert-dialog, dropdown-menu
  - [x] Port shadcn/ui primitives: menubar, sheet, badge, tooltip, table, popover
  - [x] Write render + className tests for every component
- [x] Implement theme system
  - [x] Create `ThemeProvider` context (light/dark/system, localStorage persistence)
  - [x] Create `ThemeToggle` dropdown (Sun/Moon/Monitor icons)
  - [x] Apply `dark` class to document root based on preference + system media query
  - [x] Write tests: toggle switches theme, persistence works, system preference respected
- [x] Implement app layout and navigation
  - [x] Create `AppHeader` with Menubar (brand, icon nav buttons with Phosphor icons, version, theme toggle)
  - [x] Create `AppMobileMenu` with Sheet for responsive nav
  - [x] Set up wouter routing (4 routes + redirect `/` -> `/endpoints`)
  - [x] Create `App` root component wrapping router + providers
  - [x] Write tests: nav renders, links navigate, mobile menu opens/closes, active route highlighted
- [x] Implement data fetching hooks
  - [x] Create `useFetch<T>(url)` hook with caching, deduplication, loading/error states, mutate()
  - [x] Create `useMutation(url, method)` hook with cache invalidation
  - [x] Create typed API client (`lib/api.ts`) for all endpoints
  - [x] Write tests: loading states, cache hits, error handling, refetch on mutate
- [x] Implement shared CRUD components
  - [x] Create `ExpandableRow` component (collapsed summary, expand on click, one-at-a-time logic)
  - [x] Write tests: expand/collapse, only-one-expanded invariant, create mode ('new' id)
- [x] Implement Endpoints page
  - [x] List endpoints with collapsed rows (name, base URL, model count badge, enabled switch)
  - [x] Inline create/edit form (name, base URL, API key with show/hide, models, speed, instructions, enabled)
  - [x] Delete with AlertDialog confirmation
  - [x] Test connectivity button (calls POST /test, shows result)
  - [x] Discover voices button (calls GET /voices, shows result)
  - [x] Write page tests: render list, create flow, edit flow, delete flow, test connectivity, discover voices
- [x] Implement Voices page
  - [x] Fetch and display resolved voices in a table (name, endpoint, model, type badge)
  - [x] Client-side search/filter by voice name
  - [x] Write tests: render table, search filters correctly, empty state
- [x] Implement Aliases page
  - [x] List aliases with collapsed rows (name, target voice, endpoint badge, enabled switch)
  - [x] Inline create/edit form with cascading selects (endpoint -> model -> voice)
  - [x] Speed input (number, optional), instructions textarea (optional), languages input
  - [x] Delete with AlertDialog confirmation
  - [x] Test TTS button (calls POST /test, shows result)
  - [x] Write page tests: render list, cascading selects, create flow, edit flow, delete flow, test TTS
- [x] Implement Settings page
  - [x] Fetch and display server status in cards (version, uptime, ports, DB driver, voice/endpoint/alias counts)
  - [x] Write tests: renders all status fields, handles loading/error
- [x] Achieve 100% test coverage
  - [x] Audit coverage report, identify uncovered branches/lines
  - [x] Add missing tests until all thresholds hit 100%
  - [x] Verify `bun run test` passes with coverage enforcement
