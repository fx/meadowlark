# 0001: Streaming TTS Client

**Spec:** [tts-synthesis](../specs/tts-synthesis/)
**Status:** complete
**Depends on:** —

## Summary

Add a `SynthesizeStream` method to the TTS HTTP client that sends `"stream": true` with `response_format: "pcm"` and returns the response body for incremental PCM consumption. This is the foundational building block — the proxy integration (0002) builds on this.

## Background

Currently, `client.Synthesize()` sends a non-streaming request with `response_format: "wav"`, validates the response starts with `"RIFF"` magic bytes, and returns the full response body. For streaming, we need a separate code path that:

1. Sends `"stream": true` in the request JSON
2. Uses `response_format: "pcm"` (streaming endpoints return raw PCM, not WAV)
3. Skips RIFF magic validation (PCM has no header)
4. Returns the response body immediately for incremental reading

The de facto standard across OpenAI, Qwen3-TTS FastAPI, and Speaches is: raw 16-bit signed LE mono PCM at 24kHz via HTTP chunked transfer encoding.

## Design

### New Request Type

```go
type StreamSynthesizeRequest struct {
    Model          string   `json:"model"`
    Voice          string   `json:"voice"`
    Input          string   `json:"input"`
    ResponseFormat string   `json:"response_format"`  // Always "pcm"
    Speed          *float64 `json:"speed,omitempty"`
    Instructions   *string  `json:"instructions,omitempty"`
    Stream         bool     `json:"stream"`            // Always true
}
```

A separate struct is used instead of adding `Stream *bool` to `SynthesizeRequest` to keep the two code paths cleanly separated and avoid accidentally sending `"stream": false` in non-streaming requests.

### New Method

```go
func (c *Client) SynthesizeStream(ctx context.Context, req *StreamSynthesizeRequest) (io.ReadCloser, error)
```

**Implementation:**
1. Marshal `req` to JSON (with `Stream: true` and `ResponseFormat: "pcm"` enforced).
2. Send `POST {baseURL}/audio/speech` with `Content-Type: application/json`.
3. Set `Authorization: Bearer {apiKey}` if `apiKey` is non-empty.
4. Check HTTP status: non-2xx → read body (truncated to 500 chars), return error.
5. Return `resp.Body` directly as `io.ReadCloser` — no RIFF validation, no header reconstruction.

### Why a Separate Method (Not a Flag)

- `Synthesize()` has RIFF magic validation that MUST NOT run for PCM streams.
- The request type differs (`Stream` field, forced `"pcm"` format).
- Callers (the proxy) need to know at call time which mode they're in to handle the response differently.
- A clean split avoids `if streaming { ... } else { ... }` sprawl inside one method.

## Requirements

- `SynthesizeStream` MUST always send `"stream": true` and `"response_format": "pcm"` regardless of caller input.
- `SynthesizeStream` MUST NOT perform RIFF magic byte validation on the response.
- `SynthesizeStream` MUST NOT buffer the response body — it MUST return `resp.Body` directly.
- Authorization MUST follow the same rules as `Synthesize` (Bearer token if set, omit if empty).
- Non-2xx responses MUST be handled identically to `Synthesize` (read body, truncate, return error).
- The existing `Synthesize` method MUST NOT be modified.

## Scenarios

**GIVEN** a `StreamSynthesizeRequest` with `Model: "tts-1"`, `Voice: "alloy"`, `Input: "Hello"`,
**WHEN** `SynthesizeStream` sends the HTTP request,
**THEN** the request body MUST be `{"model":"tts-1","voice":"alloy","input":"Hello","response_format":"pcm","stream":true}`.

**GIVEN** an endpoint with `apiKey: "sk-123"`,
**WHEN** `SynthesizeStream` is called,
**THEN** the request MUST include `Authorization: Bearer sk-123`.

**GIVEN** an endpoint with no API key,
**WHEN** `SynthesizeStream` is called,
**THEN** the request MUST NOT include an `Authorization` header.

**GIVEN** the endpoint returns HTTP 500 with body `{"error": "out of memory"}`,
**WHEN** `SynthesizeStream` processes the response,
**THEN** it MUST return `fmt.Errorf("tts: API error 500: {\"error\": \"out of memory\"}")`.

**GIVEN** the endpoint returns HTTP 200 with chunked PCM bytes,
**WHEN** `SynthesizeStream` returns,
**THEN** the caller MUST be able to read PCM bytes incrementally from the `io.ReadCloser`.

## Tasks

- [x] Add `StreamSynthesizeRequest` struct to `internal/tts/client.go` (PR #36)
  - `Model`, `Voice`, `Input`, `ResponseFormat`, `Speed`, `Instructions`, `Stream` fields
  - `ResponseFormat` is non-omitempty (always sent)
  - `Stream` is non-omitempty (always sent as `true`)
- [x] Add `SynthesizeStream` method to `Client` in `internal/tts/client.go` (PR #36)
  - Build HTTP request identical to `Synthesize` (same URL, same auth, same content-type)
  - Force `req.Stream = true` and `req.ResponseFormat = "pcm"` at the start of the method
  - On non-2xx: same error handling as `Synthesize` (read body, truncate 500 chars)
  - On success: return `resp.Body` directly (no RIFF check, no `io.MultiReader`)
- [x] Add tests for `SynthesizeStream` in `internal/tts/client_test.go` (PR #36)
  - Test: request body contains `"stream":true` and `"response_format":"pcm"`
  - Test: auth header present when API key set
  - Test: auth header absent when API key empty
  - Test: non-2xx returns error with status code and truncated body
  - Test: 200 response body is readable incrementally (mock server writes PCM bytes in chunks)
  - Test: optional fields (`Speed`, `Instructions`) omitted when nil, included when set
  - Test: context cancellation during streaming read
