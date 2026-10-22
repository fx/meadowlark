# 0007: vLLM-Omni streaming compatibility — request raw audio, reject SSE

## Summary

Make streaming synthesis work against vLLM-Omni. `SynthesizeStream` will always send `stream_format: "audio"` alongside `stream: true` and `response_format: "pcm"`, so a backend that would otherwise answer a streaming request with Server-Sent Events returns raw PCM instead. It will also reject a successful response whose media type is `text/event-stream` with an error, so an SSE body is never played as PCM.

**Spec:** [tts-synthesis](../specs/tts-synthesis/)
**Status:** draft
**Depends On:** 0001

## Motivation

A streaming-enabled endpoint backed by vLLM-Omni produces noise instead of speech. vLLM-Omni answers `stream: true` without a `stream_format` with an SSE event stream (`Content-Type: text/event-stream`), whose body is text events carrying encoded audio, not raw samples. `SynthesizeStream` does not look at the response's media type, so the proxy forwards that text verbatim as 16-bit PCM and the client plays it.

Sent `stream_format: "audio"`, vLLM-Omni returns raw PCM, which is what the streaming path already expects. `"audio"` is also one of the two values of OpenAI's own `stream_format` parameter for `/audio/speech`, and qwen3-tts ignores the field and keeps returning raw audio, so sending it unconditionally needs no per-endpoint setting.

The SSE guard covers the case the request field cannot: a backend that ignores `stream_format` and still sends events. Today that fails silently as noise; after this change it fails loudly as an error naming the media type.

## Requirements

### Testing Requirements

This change MUST satisfy the project's standing testing rules (see [CLAUDE.md — Build Commands](../../CLAUDE.md) and [meadowlark.md §15 — Testing Strategy](../meadowlark.md#15-testing-strategy)). CI enforces these as merge gates:

- `go test -race ./...` MUST pass.
- `go vet ./...` MUST pass with no new suppressions.
- New behaviour in `internal/tts` MUST have direct unit tests against a mock HTTP server, matching the existing `client_test.go` and `proxy_test.go` coverage rather than the >80% floor.
- No frontend change is in scope, so Vitest MUST continue to pass at its existing 100% thresholds untouched, and Biome MUST pass without new suppressions.

Skipping or weakening any of these rules to land the PR MUST be treated as a bug in the PR, not in the rule.

### Functional requirements

The [tts-synthesis spec — SynthesizeStream (Streaming)](../specs/tts-synthesis/index.md#synthesizestream-streaming) owns the request-body contract (`stream_format`), the event-stream guard (what matches, what is accepted, its log line and its error), and their scenarios; [Proxy Orchestration](../specs/tts-synthesis/index.md#proxy-orchestration) owns how that error reaches a Wyoming client. Those scenarios are this change's acceptance criteria and are not restated here. What implementing them requires of this change:

- `stream_format` MUST be forced inside `SynthesizeStream`, beside `stream` and `response_format`, so every caller gets it and none can override it. The proxy's streaming branch needs no change to send it.
- No Wyoming-side or streaming-session code changes. The guard's error leaves `SynthesizeStream` before any audio format is known, so it takes the existing open-failure path: a whole-message request gets one `tts-error` and no `audio-start`; a segmented session keeps the audio of segments already emitted and terminates with that single `error`, exactly like any other upstream failure before audio ([wyoming-protocol — Streaming Synthesis Input](../specs/wyoming-protocol/index.md#streaming-synthesis-input)).
- The non-streaming `Synthesize` path MUST NOT change.

### Known limitation: speed under vLLM-Omni streaming

Recorded in the spec under [Known Limitation: Speed Under vLLM-Omni Streaming](../specs/tts-synthesis/index.md#known-limitation-speed-under-vllm-omni-streaming). This change makes no behaviour change for it: the fix is an unmade decision between dropping `speed` from streaming requests and falling back to buffered synthesis when speed is not `1.0` (see Open Questions).

## Design

### Approach

All code changes are in `internal/tts/client.go` and its tests:

1. `StreamSynthesizeRequest` gains `StreamFormat string` serialised as `stream_format`.
2. `SynthesizeStream` sets `StreamFormat = "audio"` on its copy of the request, beside the existing `Stream = true` and `ResponseFormat = "pcm"`.
3. After the non-2xx check, `SynthesizeStream` parses the response's `Content-Type` and, when its media type is `text/event-stream`, logs, closes the body, and returns the error.

### Decisions

- **Always send `stream_format: "audio"`; no endpoint setting.**
  - **Why**: Every backend this project targets either honours it with raw audio (vLLM-Omni, OpenAI) or ignores it (qwen3-tts). A setting would add a migration, an API field and a form control to choose between a value that works everywhere and one that produces noise.
  - **Alternatives considered**: a per-endpoint `stream_format` column — rejected as configuration with only one correct value.
- **Match on the parsed media type, not a substring.** Parse with `mime.ParseMediaType`, which lowercases the type and separates parameters, so `text/event-stream`, `Text/Event-Stream` and `text/event-stream; charset=utf-8` all match, while a value that merely contains the string inside a parameter does not.
  - **Why**: Header values are case-insensitive and commonly carry a `charset`. A substring match would be both too loose and too strict.
  - **Alternatives considered**: `strings.HasPrefix` on the lowercased header — rejected; it mis-handles leading whitespace and parameters inconsistently with how every other Go HTTP consumer reads the header.
- **A `Content-Type` with no parseable media type is accepted; a malformed parameter is not an escape hatch.** When the media type itself does not parse, the header names no media type, so it cannot name `text/event-stream`; rejecting it would turn today's working responses with sloppy headers into failures. `mime.ParseMediaType` still returns the media type alongside `mime.ErrInvalidMediaParameter` when only a parameter is malformed, and the guard uses it in that case, since parameters are ignored.
- **Log without reading the body.** The guard logs at error level with the same message prefix and the `status_code` and `content_type` keys as the existing non-2xx log, but no `response_body`.
  - **Why**: Unlike a non-2xx or non-WAV body, an SSE body is a live stream of encoded audio rather than a diagnostic message. Reading 4096 bytes of it would block on the upstream's time-to-first-byte only to log base64, and the media type alone says everything the operator needs.
  - **Alternatives considered**: reading and truncating like the non-2xx path — rejected for the reasons above.
- **Reject, do not decode.** Parsing the SSE events and extracting their audio would make the streaming path work with backends that ignore `stream_format`, but no such backend is known, and it would be a second response format to maintain. The guard exists to turn silent noise into a visible error.

### Non-Goals

- Speed handling under vLLM-Omni streaming — recorded above as a known limitation; no behaviour change.
- Validating any other media type on streaming responses (`application/json`, `text/plain`, …). Only `text/event-stream` is rejected.
- The non-streaming `Synthesize` path.
- Decoding SSE audio events.
- Configuration of any deployed instance — re-pointing aliases, choosing voices in Home Assistant, or re-enabling streaming on an endpoint.
- Frontend changes.

## Tasks

- [ ] Send `stream_format: "audio"` and reject SSE responses in `SynthesizeStream` (one PR)
  - [ ] `internal/tts/client.go`: add `StreamFormat` (`json:"stream_format"`) to `StreamSynthesizeRequest`; force it to `"audio"` in `SynthesizeStream`
  - [ ] `internal/tts/client.go`: after the non-2xx check, reject a `text/event-stream` media type — log, close the body, return the spec's error
  - [ ] `internal/tts/client_test.go`: extend `TestSynthesizeStream_RequestBody` to assert `stream_format == "audio"`, including when the caller passes a different `StreamFormat`
  - [ ] `internal/tts/client_test.go`: table test for the guard — `text/event-stream`, mixed case, with `; charset=utf-8`, with surrounding whitespace, with a malformed parameter (`text/event-stream; charset`) → error naming the media type, and the response body closed (assert via a body that records `Close`)
  - [ ] `internal/tts/client_test.go`: accepted cases — `audio/pcm`, `application/octet-stream`, no `Content-Type` at all (suppress the test server's sniffing by setting the header to `nil`), and a value with no parseable media type (`/`) → body returned and readable
  - [ ] `internal/tts/client_test.go`: a non-2xx response with `Content-Type: text/event-stream` still returns the existing `tts: API error <code>: ...` error
  - [ ] `internal/tts/proxy_test.go`: a streaming endpoint answering `text/event-stream` produces a single Wyoming `error` with code `tts-error` and no `audio-start`
  - [ ] Flip the "planned" markers for 0007 in [tts-synthesis](../specs/tts-synthesis/index.md) to current behaviour and add a changelog row
  - [ ] Set this document's **Status** to `complete` and update `docs/index.yml` and `docs/index.md` to match

No new streaming-session test is needed: the guard's error reaches the session through `openSegment`, the same path `TestStreamSession_UpstreamFailureBeforeAudio` already covers.

## Open Questions

- [ ] Speed under vLLM-Omni streaming — drop `speed` from streaming requests, or fall back to buffered synthesis when speed is not `1.0`? Out of scope here; see the known limitation above.

## References

- Spec: [tts-synthesis](../specs/tts-synthesis/)
- Related changes: [0001-streaming-tts-client](./0001-streaming-tts-client.md), [0006-wyoming-synthesize-streaming](./0006-wyoming-synthesize-streaming.md)
- External: OpenAI Audio API reference — `stream_format` for `POST /audio/speech` (`"sse"` or `"audio"`)
