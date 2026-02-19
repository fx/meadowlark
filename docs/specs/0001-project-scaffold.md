# 0001: Project Scaffold

## Overview

Set up the Meadowlark project structure, toolchain management, build system, CI/CD pipelines, and containerization. This spec produces a fully buildable (but functionally empty) Go binary with an embedded placeholder frontend, automated CI, Docker image publishing to GHCR, and release-please versioning.

## Background

Meadowlark is a Wyoming-to-OpenAI TTS bridge built as a single statically-linked Go binary (linux/amd64) with an embedded Preact frontend. All tooling is managed via `mise`. The project needs CI from day one to enforce quality gates on every PR.

See `docs/meadowlark.md` sections 9 (Go Backend), 12 (Toolchain), and 13 (CI/CD) for full requirements.

## Goals

- Buildable Go module with `cmd/meadowlark/main.go` entry point
- `mise.toml` managing Go, Bun, and Biome versions
- `Makefile` with standard targets (build, test, lint, dev, clean)
- Multi-stage `Dockerfile` producing a `scratch`-based image
- GitHub Actions CI (lint + test for Go and frontend)
- Docker workflow publishing to GHCR on main/tags
- Release-please for automated versioning and changelog
- Placeholder `web/` frontend scaffold (Preact + Vite + Bun)
- `//go:embed` wiring for the frontend `dist/` directory
- CLI skeleton with flag parsing and env var fallbacks
- Structured logging via `log/slog`
- Graceful shutdown on SIGTERM/SIGINT

## Non-Goals

- Actual Wyoming protocol implementation (spec 0002)
- Database layer (spec 0002)
- TTS proxy logic (spec 0002)
- HTTP API handlers (spec 0003)
- Frontend pages and components (spec 0004)

## Design

### Project Structure

```
meadowlark/
  cmd/meadowlark/main.go       # Entry point
  internal/                     # All internal packages (empty stubs)
  web/                          # Preact frontend
    src/
      main.tsx                  # Preact entry (placeholder "Meadowlark" text)
    index.html
    package.json
    vite.config.ts
    tsconfig.json
    biome.json
    vitest.config.ts
  docs/
    meadowlark.md
    specs/
  .github/workflows/
    ci.yml
    docker.yml
    release-please.yml
  mise.toml
  Makefile
  Dockerfile
  go.mod
  go.sum
  release-please-config.json
  .release-please-manifest.json
  .gitignore
  CHANGELOG.md
```

### mise.toml

```toml
[tools]
go = "latest"
bun = "latest"
biome = "latest"

[env]
CGO_ENABLED = "0"
```

### CLI Flags

The `main.go` should parse all flags from `docs/meadowlark.md` section 7.1 using a CLI library (cobra or urfave/cli). Each flag has a `MEADOWLARK_` prefixed env var fallback. The binary should:

1. Parse flags
2. Configure `log/slog` with the requested level and format
3. Log the configuration summary
4. Listen for SIGTERM/SIGINT for graceful shutdown
5. Exit cleanly (no servers started yet -- those come in later specs)

### Frontend Placeholder

The `web/` directory must be a valid Preact + Vite + Bun project that builds successfully. It should render a single page with "Meadowlark" as placeholder text. The full component library and pages are in spec 0004.

Configuration files (`biome.json`, `tsconfig.json`, `vite.config.ts`, `vitest.config.ts`, `package.json`) should be fully set up with the final production settings from `docs/meadowlark.md` section 10.

### Go Embed

```go
//go:embed web/dist/*
var webFS embed.FS
```

The Makefile `build` target must build the frontend first, then the Go binary.

### CI Workflows

Three GitHub Actions workflows, as specified in `docs/meadowlark.md` section 13:

1. **ci.yml** -- Two parallel jobs: `backend` (go vet, go test) and `frontend` (bun install, biome check, vitest, bun run build). Both use `jdx/mise-action@v2`.
2. **docker.yml** -- Build + push to GHCR with semver tags. PR builds validate only (no push).
3. **release-please.yml** -- Automated versioning with `release-type: go`.

### Makefile

```makefile
.PHONY: build build-frontend build-backend test test-go test-frontend lint dev clean

build: build-frontend build-backend

build-frontend:
	cd web && bun install && bun run build

build-backend:
	go build -ldflags="-s -w" -o meadowlark ./cmd/meadowlark

test: test-go test-frontend

test-go:
	go test -race ./...

test-frontend:
	cd web && bun run test

lint:
	go vet ./...
	cd web && bunx biome check .

dev:
	# Run both frontend dev server and Go binary in parallel
	# (implementation detail: use a process manager or two terminals)

clean:
	rm -f meadowlark
	rm -rf web/dist web/node_modules
```

## Testing

- `go test ./...` passes (even if there are no test files yet, it should not error)
- `bun run build` in `web/` produces `web/dist/index.html`
- `bun run test` in `web/` passes (placeholder test)
- `bunx biome check .` in `web/` passes
- `go build ./cmd/meadowlark` produces a static binary
- The binary starts, logs its config, and exits cleanly on SIGINT
- `docker build .` succeeds and produces a minimal image

## Tasks

- [x] Initialize Go module and project skeleton
  - [x] `go mod init` with module path
  - [x] Create `cmd/meadowlark/main.go` with CLI flag parsing (all flags from meadowlark.md section 7.1)
  - [x] Wire env var fallbacks (`MEADOWLARK_*` prefix) for all flags
  - [x] Configure `log/slog` (level + format from flags)
  - [x] Implement graceful shutdown (SIGTERM/SIGINT handler)
  - [x] Create empty `internal/` package stubs (wyoming, tts, voice, store, api, model)
- [x] Set up mise and Makefile
  - [x] Create `mise.toml` (go, bun, biome)
  - [x] Create `Makefile` with targets: build, build-frontend, build-backend, test, test-go, test-frontend, lint, dev, clean
  - [x] Create `.gitignore` (Go binaries, web/node_modules, web/dist, .env, *.db)
- [x] Scaffold Preact frontend
  - [x] Initialize `web/` with Bun (`bun init`)
  - [x] Install Preact, Vite, Tailwind CSS v4, Vitest, testing-library, shadcn/ui deps
  - [x] Create `vite.config.ts` with Tailwind plugin and Preact preset
  - [x] Create `tsconfig.json` with strict mode and `@` path alias
  - [x] Create `biome.json` with exact config from meadowlark.md section 10.3
  - [x] Create `vitest.config.ts` (jsdom, globals, 100% coverage thresholds)
  - [x] Create `web/src/styles/globals.css` with full design tokens from meadowlark.md section 10.2
  - [x] Create `web/src/main.tsx` placeholder (renders "Meadowlark")
  - [x] Create `web/index.html`
  - [x] Create placeholder test that passes
  - [x] Verify `bun run build` produces `web/dist/`
- [x] Wire Go embed and static binary build
  - [x] Add `//go:embed web/dist/*` in appropriate package
  - [x] Verify `CGO_ENABLED=0 go build` produces static amd64 binary
  - [x] Verify binary starts, logs config, exits cleanly on SIGINT
- [x] Create Dockerfile
  - [x] Multi-stage build: bun frontend -> golang backend -> scratch runtime
  - [x] Include CA certificates, OCI labels, EXPOSE directives
  - [x] Verify `docker build .` succeeds
- [x] Set up CI workflows
  - [x] Create `.github/workflows/ci.yml` (backend + frontend parallel jobs, mise-action)
  - [x] Create `.github/workflows/docker.yml` (GHCR publish with semver tags)
  - [x] Create `.github/workflows/release-please.yml`
  - [x] Create `release-please-config.json` (release-type: go)
  - [x] Create `.release-please-manifest.json` (starting at 0.1.0)
  - [x] Create initial `CHANGELOG.md`
- [x] Add task-completion instructions
  - [x] Create `CLAUDE.md` at project root with task-completion language
  - [x] Create `.github/copilot-instructions.md` with task-completion rule
