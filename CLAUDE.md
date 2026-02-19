# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What is Meadowlark

Meadowlark is a Wyoming protocol to OpenAI-compatible TTS API bridge. It proxies text-to-speech requests from Wyoming clients (Home Assistant) to OpenAI-compatible TTS endpoints. Single statically-linked Go binary with an embedded Preact frontend.

## Build Commands

Toolchain is managed via `mise` (Go, Bun, Biome). Run `mise install` first.

```bash
make build              # Build frontend + Go binary
make build-frontend     # Build Preact frontend only (web/dist/)
make build-backend      # Build Go binary only (requires web/dist/ to exist)
make test               # Run all tests (Go + frontend)
make test-go            # Go tests only (go test -race ./...)
make test-frontend      # Frontend tests only (cd web && bun run test)
make lint               # Go vet + Biome check
make clean              # Remove binary + web/dist + web/node_modules
```

Frontend commands (from `web/` directory):
```bash
bun install             # Install dependencies
bun run dev             # Vite dev server (port 5173)
bun run build           # Production build to dist/
bun run test            # Vitest with coverage
bunx biome check .      # Lint + format check
bunx biome check . --fix  # Auto-fix lint/format issues
```

Go binary: `./meadowlark --help` for all flags. Every flag has a `MEADOWLARK_*` env var fallback (e.g., `--http-port` → `MEADOWLARK_HTTP_PORT`).

## Architecture

**Go backend** (`cmd/meadowlark/main.go`): Entry point using cobra/viper for CLI flags with env var fallbacks. Configures `log/slog`, handles graceful shutdown on SIGTERM/SIGINT. Version/commit injected via `-ldflags`.

**Frontend embed** (`web.go`): `//go:embed all:web/dist` bundles the built Preact frontend into the Go binary as `WebFS`.

**Internal packages** (`internal/`): All stubs, to be implemented per specs 0002-0004:
- `wyoming/` — Wyoming protocol TCP server
- `tts/` — OpenAI-compatible HTTP client and WAV parsing
- `voice/` — Voice resolution and custom input parsing
- `store/` — Database interface (SQLite + PostgreSQL)
- `api/` — HTTP API server and handlers
- `model/` — Data models (Endpoint, VoiceAlias)

**Frontend** (`web/`): Preact + Vite + Tailwind CSS v4 + Biome. Design uses JetBrains Mono font, OKLch color palette, zero border radius. Vitest enforces 100% coverage thresholds.

**Build pipeline**: Frontend builds first (Bun → Vite → `web/dist/`), then Go binary embeds `web/dist/` via `//go:embed`. The Makefile and Dockerfile both enforce this order.

**Docker**: Multi-stage build (bun → golang → scratch). Static binary with `CGO_ENABLED=0`.

## CI/CD

- `ci.yml` — Parallel Go (vet + test) and frontend (biome + vitest + build) jobs using `jdx/mise-action@v2`. Skips draft PRs.
- `docker.yml` — GHCR publish on main/tags, validate-only on PRs.
- `release-please.yml` — Automated versioning from conventional commits.

## Key Conventions

- **Conventional commits** required: `feat:`, `fix:`, `docs:`, `ci:`, etc.
- **Task completion**: Every PR must mark completed tasks as done (`- [x]`) in the relevant spec file under `docs/specs/`.
- **CGO_ENABLED=0** for production builds (set in Makefile/Dockerfile), but NOT in mise.toml env because `go test -race` requires CGO.
- **Specs** live in `docs/specs/` with numbered filenames. Full requirements in `docs/meadowlark.md`.
