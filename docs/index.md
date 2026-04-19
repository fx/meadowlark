# Documentation

## Specs

| Spec | Description | Status |
|------|-------------|--------|
| [wyoming-protocol](specs/wyoming-protocol/) | TCP server, wire format, event types, info builder, and Zeroconf/mDNS | active |
| [tts-synthesis](specs/tts-synthesis/) | OpenAI-compatible HTTP client, WAV parsing, and proxy orchestration | active |
| [voice-resolution](specs/voice-resolution/) | Priority-based resolver, input parsing, parameter merging, canonical naming | active |
| [http-api](specs/http-api/) | REST endpoints, middleware, SPA serving, error format | active |
| [data-persistence](specs/data-persistence/) | Store interface, SQLite/PostgreSQL implementations, schema, migrations | active |
| [frontend](specs/frontend/) | Preact SPA, pages, hooks, theme system, UI components | active |

## Changes

| # | Change | Spec | Status | Depends On |
|---|--------|------|--------|------------|
| 0001 | [streaming-tts-client](changes/0001-streaming-tts-client.md) | [tts-synthesis](specs/tts-synthesis/) | draft | — |
| 0002 | [streaming-proxy-integration](changes/0002-streaming-proxy-integration.md) | [tts-synthesis](specs/tts-synthesis/) | draft | 0001 |
