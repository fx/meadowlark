# 0006: Wyoming synthesize streaming — incremental text input from Home Assistant

## Summary

Implement the Wyoming streaming-synthesis input protocol (`synthesize-start`, `synthesize-chunk`, `synthesize-stop`, `synthesize-stopped`) so Home Assistant streams LLM text into Meadowlark as it is produced instead of buffering a whole message first. Meadowlark advertises `supports_synthesize_streaming: true` in its `info` response, aggregates incoming text into sentence-sized segments, synthesizes each segment against the resolved endpoint, and emits one `audio-start`/`audio-chunk*`/`audio-stop` group per segment followed by a single terminating `synthesize-stopped`.

This requires per-connection session state, which the Wyoming server does not have today, and serialized writes to the connection, which it also does not have today.

**Spec:** [wyoming-protocol](../specs/wyoming-protocol/), [tts-synthesis](../specs/tts-synthesis/)
**Status:** complete
**Depends On:** 0002

## Motivation

### Measured baseline

A Home Assistant Assist pipeline run through Meadowlark reported:

| Stage | Reported |
|---|---|
| STT | 1.17 s |
| LLM | 3.76 s |
| TTS | 0 s |
| **Pipeline total** | **4.93 s** |

First audio actually reached the ESPHome satellite at **10.20 s** — roughly **5.2 s unaccounted for** by the pipeline's own numbers.

That gap is structural, not a bug in any one component. Because Meadowlark advertises no streaming-input support, Home Assistant's Wyoming TTS entity takes the `async_get_tts_audio()` path, which:

1. waits for the **entire** LLM message before sending a single `synthesize`,
2. buffers **every** `audio-chunk` into a complete in-memory WAV before returning,
3. only then exposes the media URL, at which point the satellite issues its `GET` and Home Assistant transcodes the finished WAV to mp3.

Nothing downstream can start until the whole message has been generated *and* fully synthesized. The reported "TTS 0 s" is an artifact: the pipeline stops its TTS timer when it hands off, before any of that work happens.

### What streaming input changes

With `supports_synthesize_streaming: true`, Home Assistant takes `async_stream_tts_audio()` instead. It sends `synthesize-start` immediately, then `synthesize-chunk` as the LLM emits text, and streams our audio out as it arrives. The satellite's `GET` moves to **mid-generation**, and first byte arrives roughly **0.2 s** after it rather than after the full message.

Meadowlark is well placed to serve this: it already streams PCM out of upstream endpoints (change [0002](0002-streaming-proxy-integration.md)), so the only missing half is streaming text *in*.

**Residual dependency, stated plainly:** the win on the satellite leg assumes Home Assistant transcodes the streamed WAV to mp3 **incrementally**. If the installed HA version buffers the stream before transcoding, the satellite leg keeps its current cost and the win shrinks to the LLM-overlap portion (still substantial — synthesis of segment 1 starts while the LLM is still producing segment 3). This is HA-version dependent and MUST NOT be presented as guaranteed.

### Reversal of a prior decision

[`docs/meadowlark.md`](../meadowlark.md) declared synthesize streaming out of scope on the grounds that "Meadowlark is a proxy to HTTP APIs, so the latency characteristics differ from local TTS engines that benefit from streaming." The measurement above shows that reasoning was wrong in a specific way: the latency that matters is not the upstream engine's, it is Home Assistant's buffering, and that is triggered entirely by the missing capability flag. This change reverses that decision; the scope statement in §2.2 and the roadmap bullet in §16 are updated as part of it.

## Requirements

### Testing Requirements

This change MUST satisfy the project's standing testing rules (see [CLAUDE.md — Build Commands](../../CLAUDE.md), [meadowlark.md §15 — Testing Strategy](../meadowlark.md), and [frontend spec — Coverage Requirements](../specs/frontend/index.md#coverage-requirements)):

- `go test -race ./...` MUST pass. Every new goroutine, channel, and shared field introduced here is a data-race candidate; `-race` is the primary gate on this change and MUST NOT be skipped locally.
- `go vet ./...` MUST pass with no new suppressions.
- Every new package (`internal/segment`) and every new exported symbol MUST have direct unit tests; match the coverage bar of the existing `internal/wyoming`, `internal/tts`, and `internal/voice` packages rather than the ≥80% floor.
- The end-to-end Wyoming conformance test in R11 is REQUIRED, not optional — it is the only test that verifies the emitted event *order* over a real TCP connection.
- No frontend change is in scope, so Vitest MUST continue to pass at its existing 100% thresholds untouched. Biome MUST pass without new suppressions.

Skipping or weakening any of these rules to land the PR MUST be treated as a bug in the PR, not in the rule.

### R1: Advertise `supports_synthesize_streaming`

`TtsProgram` MUST gain a `SupportsSynthesizeStreaming bool` field, serialized as `supports_synthesize_streaming` inside each entry of `tts[]` in the `info` event, and parsed symmetrically by `InfoFromEvent`.

Meadowlark MUST advertise `true` **unconditionally**. See [Decisions](#decisions) for why this is not gated on endpoint configuration.

#### Scenario: info advertises the flag

- **GIVEN** any configuration of endpoints, including none
- **WHEN** a Wyoming client sends `describe`
- **THEN** the `info` event's single `tts[0]` entry MUST contain `"supports_synthesize_streaming": true`

#### Scenario: parsing an info event without the flag

- **GIVEN** an `info` event whose `tts[0]` object has no `supports_synthesize_streaming` key
- **WHEN** `InfoFromEvent` parses it
- **THEN** `TtsProgram.SupportsSynthesizeStreaming` MUST be `false`

### R2: Streaming event types

`internal/wyoming/types.go` MUST gain four event type constants and four message structs following the existing `ToEvent`/`FromEvent` house pattern:

| Constant | Wire type | Direction | Data |
|---|---|---|---|
| `TypeSynthesizeStart` | `synthesize-start` | client → server | `voice` (optional object `{name, language, speaker}`), `text_format` (optional, `"text"` \| `"ssml"`), `context` (optional, opaque) |
| `TypeSynthesizeChunk` | `synthesize-chunk` | client → server | `text` (required string) |
| `TypeSynthesizeStop` | `synthesize-stop` | client → server | none |
| `TypeSynthesizeStopped` | `synthesize-stopped` | server → client | none |

`SynthesizeStart` MUST carry `Voice`, `Language`, `Speaker`, `TextFormat`, and `Context any`. Its `voice` object nests all three voice fields:

```json
{"voice": {"name": "alloy", "language": "en", "speaker": "s1"}, "text_format": "text"}
```

The `voice` object MUST be omitted only when **all three** of `Voice`, `Language` and `Speaker` are empty. Omitting it whenever the *name* alone is empty would silently discard a `synthesize-start` that carries only `language` or only `speaker` — which upstream permits, since `voice` is optional as an object rather than conditional on its name — and would break the round-trip symmetry this requirement demands. Within the object, each individual field is omitted when empty.

`Context` MUST round-trip unchanged so a future change can echo it back.

> **This is NOT the same shape as the existing `Synthesize` event, and the two MUST NOT be harmonised in this change.** `Synthesize.ToEvent` in `internal/wyoming/types.go` nests only `name` under `voice` and emits `speaker` and `language` at the **top level** of the data object. That divergence is pre-existing and it is wrong: upstream `rhasspy/wyoming` `wyoming/tts.py` builds `SynthesizeVoice.to_dict()` as `{"name": ...}` with `speaker` nested inside it — or `{"language": ...}` when there is no name — and `Synthesize.event()` assigns that whole object to `data["voice"]`, so upstream nests `speaker` and `language` under `voice` for `synthesize` as well. Home Assistant sends `Synthesize(text=..., voice=SynthesizeVoice(name=..., language=...))`, so its `language` arrives nested, and `SynthesizeFromEvent` — which reads `ev.Data["language"]` at the top level — silently drops it.
>
> That mismatch is nonetheless behaviourally inert today: `Synthesize.Language` and `Synthesize.Speaker` are read nowhere outside `internal/wyoming/types.go` and its tests — `internal/tts/proxy.go` uses only `Voice` and `Text` — so nothing observes the dropped fields. This change does not touch `Synthesize`'s encoding, and a coder implementing 0006 MUST NOT "fix" it here: correcting it alters an existing wire contract and belongs with a requirement that actually needs those fields. `SynthesizeStart` follows the upstream `wyoming/tts.py` shape; `Synthesize` keeps its existing shape. Implement them as two separate encoders. The correction is recorded in [Open Questions](#open-questions).

`SynthesizeChunk` MUST carry `Text`. `SynthesizeStop` and `SynthesizeStopped` MUST have no fields.

#### Scenario: round-trip symmetry

- **GIVEN** a `SynthesizeStart{Voice: "alloy (OpenAI, tts-1)", TextFormat: "text"}`
- **WHEN** it is converted with `ToEvent` and read back with `SynthesizeStartFromEvent`
- **THEN** the result MUST equal the original

#### Scenario: voice metadata without a name survives

- **GIVEN** a `SynthesizeStart{Language: "de", Speaker: "s2"}` with an empty `Voice`
- **WHEN** it is round-tripped
- **THEN** the `voice` object MUST be present in the event, and the result MUST still carry `Language: "de"` and `Speaker: "s2"`

#### Scenario: fully empty voice is omitted

- **GIVEN** a `SynthesizeStart` whose `Voice`, `Language` and `Speaker` are all empty
- **WHEN** `ToEvent` is called
- **THEN** the event data MUST contain no `voice` key at all

### R3: Per-connection handler construction

`internal/wyoming/server.go` MUST allow a `Handler` to be constructed per connection, because streaming sessions are per-connection state and `HandleEvent(ctx, ev, w)` carries no connection identity today.

Two optional interfaces MUST be introduced:

```go
// HandlerFactory produces a fresh Handler for each accepted connection.
type HandlerFactory interface {
    NewConnHandler() Handler
}

// ConnHandler is implemented by per-connection handlers that need to release
// resources when their connection is torn down.
type ConnHandler interface {
    CloseConn()
}
```

`Server.handleConn` MUST, for each accepted connection:

1. If the server's `Handler` also implements `HandlerFactory`, call `NewConnHandler()` exactly once and dispatch that connection's events to the returned handler; otherwise dispatch to the shared `Handler` exactly as today.
2. If the resulting per-connection handler also implements `ConnHandler`, call `CloseConn()` exactly once from the same `defer` that closes the connection and removes it from `s.conns`.

`CloseConn()` MUST block until the connection's background work has finished, so that `Shutdown()`'s existing `wg.Wait()` genuinely drains in-flight synthesis.

Handlers that do not implement `HandlerFactory` — including `HandlerFunc` and every existing test — MUST behave exactly as before.

#### Scenario: one handler per connection

- **GIVEN** a server whose handler implements `HandlerFactory`
- **WHEN** three clients connect and each sends one event
- **THEN** `NewConnHandler()` MUST have been called exactly three times, and no two connections MUST share a handler instance

#### Scenario: teardown notification

- **GIVEN** a connected client whose per-connection handler implements `ConnHandler`
- **WHEN** the client disconnects, or `Shutdown()` is called
- **THEN** `CloseConn()` MUST be called exactly once for that connection before the connection goroutine exits

### R4: Event-atomic writes on a shared connection

A streaming session writes audio events from a background goroutine while the connection's read loop may concurrently write `pong` or `error` events to the same `net.Conn`. Wyoming events are multi-part, so an interleaved write corrupts framing for every subsequent event.

Two changes MUST be made together:

- `WriteEvent` MUST assemble the complete message — header line, `\n`, data bytes, payload bytes — into a single buffer and issue **exactly one** `Write` call on the underlying writer.
- `Server.handleConn` MUST wrap the connection in an internal mutex-guarded `io.Writer` and pass **that** writer to `HandleEvent`, and MUST use the same wrapper for the error events it writes itself.

Taken together these make every emitted event atomic with respect to any other goroutine writing to the same connection. Neither change alone is sufficient: a mutex around a multi-`Write` `WriteEvent` still interleaves.

#### Scenario: single write per event

- **GIVEN** an event with both data and a binary payload
- **WHEN** `WriteEvent` writes it to a writer that counts `Write` calls
- **THEN** the counter MUST read exactly 1

#### Scenario: concurrent writers do not interleave

- **GIVEN** the mutex-guarded connection writer
- **WHEN** two goroutines each write 100 events concurrently under `-race`
- **THEN** a reader MUST parse exactly 200 well-formed events with no framing error

### R5: Session lifecycle

A connection MUST hold at most one streaming session. The session MUST be created on `synthesize-start` and terminated on `synthesize-stop`, on error, on idle timeout, or on connection teardown.

A session is in exactly one of **three** states. The third exists because a session that fails mid-stream MUST keep suppressing until Home Assistant has finished sending the events for that message.

| Event | `idle` (no session) | `open` | `terminated` (errored, awaiting `synthesize-stop`) |
|---|---|---|---|
| `synthesize-start` | Open a session. Record voice, language, speaker, text format. | **Quiesce** the current session (below), emit `synthesize-stopped`, then open a new one. | Discard the tombstone and open a new session. |
| `synthesize-chunk` | Ignore; log at debug. | Append `text` to the buffer and flush any completed segments (R6). | Absorb silently. |
| `synthesize` | Existing behaviour, unchanged: delegate to `Proxy.HandleSynthesize`. | Suppress (R7). | Absorb silently — **never** delegate. |
| `synthesize-stop` | Ignore; log at debug. Emit nothing. | Flush the remainder, wait for all segments to finish emitting, emit `synthesize-stopped`, return to `idle`. | Emit nothing; return to `idle`. |

**The emitter is the only goroutine that writes audio events.** That is a load-bearing invariant, not an implementation detail: it is what makes an ordered shutdown possible at all. Because the emitter alone writes `audio-start`, `audio-chunk` and `audio-stop`, it is also the only thing that can correctly close a group it has opened.

**Quiescing a session.** Every path that ends a session early — a restart, a failure, the idle timeout, connection teardown — MUST perform the same ordered shutdown before anything else is written to the connection:

1. Cancel the session context, so in-flight and prefetched upstream requests abort and every held response body is closed.
2. **Wait for the emitter goroutine to exit.** On observing cancellation the emitter stops mid-segment and, if it had written an `audio-start` with no matching `audio-stop`, writes that `audio-stop` itself as its final act before returning. After the join, no audio event for this session can ever be written again.
3. Discard buffered text and any queued segments.

Only then may a terminator (`synthesize-stopped` or `error`) be written, and only then may a replacement session open.

**The join MUST come before any closing `audio-stop`, and the emitter MUST write it.** Having the terminating path write the `audio-stop` itself and *then* join is a race: cancelling a context does not synchronously stop a goroutine that is already inside a write or about to enter one, so the emitter can land an `audio-chunk` after that `audio-stop` and produce an invalid group. Giving the emitter sole ownership of its own closing write removes the race by construction rather than by timing.

**On connection teardown the emitter skips its closing write and no terminator is sent** — the connection is gone, so nothing may be written to it. Steps 1–3 still run, and `CloseConn` returns only once step 2 has completed.

**Why `terminated` is a distinct state and not simply a closed session.** Home Assistant sends its compatibility `synthesize` *after* the chunks and *before* `synthesize-stop`. If a segment fails early — an upstream error, a format mismatch, or an idle timeout — and the session were closed immediately, that compatibility event would arrive to find no session, fall through the `idle` row, and be synthesized in full: Meadowlark would speak the entire message a second time, long after Home Assistant had already raised on the `error`. The tombstone absorbs every remaining event of the failed message and is cleared only by `synthesize-stop`, a new `synthesize-start`, or connection teardown.

**Voice.** `synthesize-start`'s `voice` is the Wyoming voice name for the whole session and is the only thing that selects the **endpoint**. It MUST be resolved exactly as a `synthesize` event's voice is resolved today, including the empty case, which falls to the resolver's default-voice stage. Resolution also yields the session's baseline `model` and `voice`.

Input overrides then apply on top, unchanged from today: `voice.MergeParams` gives input priority over alias defaults over endpoint defaults, so a JSON or tag `model` or `voice` override MUST still win for the parameters sent upstream. What an override MUST NOT do is change which endpoint the session talks to — that is fixed by the start voice for the whole session.

The session MUST derive its context from the `ctx` passed to `HandleEvent` at `synthesize-start`, via `context.WithCancel`. Cancelling it MUST abort in-flight upstream HTTP requests and close their response bodies. Both server shutdown (parent cancellation) and connection teardown (`CloseConn`) MUST therefore stop synthesis promptly.

**Idle timeout.** The session MUST run an idle timer, armed when the session opens and **reset on every subsequent event belonging to that session** — each `synthesize-chunk`, and the compatibility `synthesize`. Only client events reset it; Meadowlark's own progress, such as a segment finishing, does not, because a session whose client has gone silent is dead regardless of how much audio is still draining.

When the timer expires the session MUST be abandoned: quiesce as above, emit a Wyoming `error` with code `synthesize-timeout`, and enter the `terminated` state without emitting `synthesize-stopped`. The timeout MUST be configurable and MUST be disable-able (R9).

The `synthesize-stop` case is not a timer expiry — it terminates the session normally and disarms the timer.

#### Scenario: connection drops mid-synthesis

- **GIVEN** a session with one segment being streamed from an upstream endpoint
- **WHEN** the client's TCP connection drops
- **THEN** `CloseConn()` MUST cancel the session context, the in-flight upstream request MUST be aborted, and `CloseConn()` MUST NOT return until the session's goroutines have exited

#### Scenario: restart without stop

- **GIVEN** an open session with buffered text
- **WHEN** a second `synthesize-start` arrives
- **THEN** the buffered text MUST be discarded without being synthesized, a single `synthesize-stopped` MUST be emitted for the abandoned session, and a fresh session MUST be opened

#### Scenario: restart while audio is mid-flight

- **GIVEN** an open session whose second segment has emitted `audio-start` and some `audio-chunk` events
- **WHEN** a second `synthesize-start` arrives
- **THEN** the emitted order MUST be exactly: that segment's `audio-stop` written by the emitter as it exits, then `synthesize-stopped`, then nothing from the old session at all — no `audio-chunk` or `audio-stop` from the old session MUST appear after that `audio-stop`, after the `synthesize-stopped`, or interleaved with the new session's audio

### R6: Text segmentation

Home Assistant emits `synthesize-chunk` at roughly token or phrase granularity, which is far too fine for one upstream HTTP request each. Incoming text MUST be aggregated into segments by a pure, I/O-free segmenter.

**Boundary rules.** A segment boundary is:

- an ASCII sentence-terminating rune — `.` `!` `?` `…` — optionally followed by any run of closing punctuation (`"` `'` `”` `’` `)` `]` `}` `»`), and then followed by whitespace; **or**
- a full-width sentence-terminating rune — `。` `！` `？` — optionally followed by closing punctuation, with **no** trailing-whitespace requirement, because CJK text is not space-separated; **or**
- a newline; **or**
- a forced break (below).

The trailing-whitespace requirement on the ASCII terminators is what makes the rule safe against partial input: a `.` at the very end of the buffer is not yet a boundary, because the next chunk may continue the token. Full-width terminators need no such guard — they are unambiguous sentence enders and never appear inside a token.

**Boundary suppression.** A `.` MUST NOT create a boundary when:

- it sits between two digits, as in `3.14`; **or**
- the token immediately preceding it matches a known abbreviation, case-insensitively: `mr`, `mrs`, `ms`, `dr`, `prof`, `sr`, `jr`, `st`, `vs`, `etc`, `approx`, `no`, `fig`, `inc`, `ltd`, `co`, `e.g`, `i.e`; **or**
- the token immediately preceding it is a single letter (an initial, e.g. `A. Smith`).

Here **token** means the maximal run of non-whitespace characters immediately preceding the `.`, not including the `.` itself. That definition is what makes the multi-part entries matchable: in `e.g.` the token before the final `.` is `e.g`, so it matches the list directly rather than needing a nested rule.

**Length gating.** A boundary MUST NOT flush unless the candidate segment is at least `minSegmentChars` runes long — except for the first segment of a session, which uses the lower `firstSegmentChars` threshold. Below the threshold, accumulation continues and the boundary is passed over.

The **candidate segment** is everything buffered up to and including the boundary's terminator and any closing-punctuation run, with leading and trailing whitespace trimmed, measured in runes rather than bytes. Every threshold in this document is measured that way, so `"Hello."` is 6 runes, not 5.

**Forced break.** When the buffer reaches `maxSegmentChars` runes with no qualifying boundary, the segmenter MUST flush, preferring in order: the last soft break (`,` `;` `:` `—` `–` followed by whitespace) at or before the limit; else the last whitespace at or before the limit; else a hard cut exactly at the limit. A hard cut MUST be rune-aligned and MUST NOT split a multi-byte rune.

**Final flush.** On `synthesize-stop` the remainder MUST be flushed regardless of length. A remainder that is empty or entirely whitespace MUST flush nothing.

**Defaults.** `firstSegmentChars = 24`, `minSegmentChars = 60`, `maxSegmentChars = 400`. Rationale and the tradeoff being tuned are in [Decisions](#decisions).

**Override-form input suspends segmentation.** If the session's first non-whitespace character is `{` or `[`, the session MUST buffer the whole message and flush nothing until `synthesize-stop`, then parse it with `voice.ParseInput` and segment the resulting `Input`. Otherwise `voice.ParseInput` MUST NOT be run at all and the raw text is segmented as it arrives. The reasoning is in [Override parsing](#per-segment-synthesis-inside-the-proxy).

#### Scenario: JSON-form input is not spoken

- **GIVEN** a session opened with `synthesize-start` carrying voice `"nova (OpenAI, tts-1)"`, whose chunks together spell `{"voice": "alloy", "input": "Turning on the lights now."}`, arriving in fragments
- **WHEN** `synthesize-stop` arrives
- **THEN** no segment MUST have been flushed before `synthesize-stop`, the endpoint MUST be the one resolved from `"nova (OpenAI, tts-1)"`, the `model` MUST be the resolved baseline `tts-1` because this input carries no `model` override, the `voice` sent upstream MUST be `alloy`, the synthesized text MUST be `Turning on the lights now.`, and no brace or key name MUST appear in any synthesized segment

#### Scenario: model override survives a streaming session

- **GIVEN** a session opened with voice `"alloy (OpenAI, tts-1)"` whose chunks together spell `[model: gpt-4o-mini-tts] Turning the lights on.`
- **WHEN** the session is synthesized
- **THEN** the endpoint MUST be the one resolved from the start voice, and every segment's upstream request MUST carry `model: gpt-4o-mini-tts` rather than `tts-1`

#### Scenario: tag-form input strips the tag

- **GIVEN** a session whose chunks together spell `[speed: 1.2] The kitchen light is now on.`, with the `]` arriving in a later chunk than the `[`
- **WHEN** `synthesize-stop` arrives
- **THEN** the merged speed MUST be `1.2` and the synthesized text MUST NOT contain `[speed: 1.2]`

#### Scenario: prose is never override-parsed

- **GIVEN** a session whose text is ordinary prose
- **WHEN** segments are flushed
- **THEN** `voice.ParseInput` MUST NOT be called on any segment, and segments MUST flush incrementally rather than waiting for `synthesize-stop`

#### Scenario: one chunk containing several sentences

- **GIVEN** a session and defaults
- **WHEN** a single `synthesize-chunk` carries `"The weather is sunny today and quite warm. Tomorrow will bring rain across the whole region. Bring an umbrella."`, followed by `synthesize-stop`
- **THEN** exactly two segments MUST be produced: `"The weather is sunny today and quite warm."` and `"Tomorrow will bring rain across the whole region. Bring an umbrella."`

Working through why, because this case exercises three rules at once and an implementer who expects three segments has misread one of them:

| Boundary | Candidate length | Outcome |
|---|---|---|
| `.` after `warm` | 42 runes | Flushes — it is the session's first segment, so `firstSegmentChars = 24` applies |
| `.` after `region` | 49 runes | Passed over — 49 < `minSegmentChars = 60`, so accumulation continues |
| `.` after `umbrella` | — | Not a boundary at all: it ends the buffer, so the trailing-whitespace guard has not been satisfied |
| `synthesize-stop` | 68 runes | Final flush, unconditional |

#### Scenario: text that never reaches punctuation

- **GIVEN** a session and defaults
- **WHEN** 500 runes of text arrive containing no sentence-terminating rune, no newline, no soft break, and no whitespace
- **THEN** exactly one forced segment of 400 runes MUST flush, and the remaining 100 runes MUST stay buffered until `synthesize-stop`

#### Scenario: forced break prefers a soft break

- **GIVEN** defaults, and 500 runes of text with no sentence terminator but a comma followed by a space at rune 380
- **WHEN** the forced break fires
- **THEN** the emitted segment MUST end at that comma, not at rune 400

#### Scenario: abbreviation does not split

- **GIVEN** a session and defaults
- **WHEN** the accumulated text is `"I asked Dr. Nakamura about the results of the second trial and she was optimistic."`, followed by `synthesize-stop`
- **THEN** exactly one segment MUST be produced, containing the whole sentence — the `.` after `Dr` is suppressed as an abbreviation, and the final `.` ends the buffer so it never satisfies the trailing-whitespace guard, leaving the final flush to produce the segment

#### Scenario: short sentence coalesces

- **GIVEN** a session and defaults, and no prior segment in this session
- **WHEN** the text `"Sure."` arrives, then `" The living room lights are now at forty percent brightness."`, then `synthesize-stop`
- **THEN** the `.` after `"Sure"` MUST NOT flush on its own, because the 5-rune candidate is below `firstSegmentChars`, and a single segment covering both sentences MUST be produced by the final flush

#### Scenario: multi-byte runes survive a hard cut

- **GIVEN** `maxSegmentChars = 400` and a run of 500 emoji with no break characters
- **WHEN** the forced break fires
- **THEN** the emitted segment MUST be valid UTF-8 and MUST contain exactly 400 runes

### R7: Compatibility `synthesize` suppression

Home Assistant's `_write_tts_message` sends, in order: `synthesize-start`, N × `synthesize-chunk`, a **full `synthesize` carrying the entire message** for backwards compatibility, then `synthesize-stop`. Synthesizing that compatibility event while a session is open would speak the whole message a second time.

While a session is open, a `synthesize` event MUST NOT be synthesized. Its text MUST instead be recorded as the session's fallback text.

On `synthesize-stop`, if the session received **zero** `synthesize-chunk` events and fallback text is present, that fallback text MUST become the session's content. Otherwise the fallback text MUST be discarded.

Fallback text MUST take exactly the same path as chunked text: it is a complete message, so R6's override-form rule applies to it unchanged. If its first non-whitespace character is `{` or `[`, it MUST be passed through `voice.ParseInput` and only its resulting `Input` segmented; otherwise it MUST be segmented directly with no override parsing. It MUST NOT be handed to the segmenter raw — a fallback of `[speed: 1.2] Hello.` would otherwise speak the tag aloud and drop the override, which is precisely the failure R6 exists to prevent.

This is straightforward to satisfy: the fallback is already a whole message when it arrives, so the session applies the same detection it would have applied to a fully buffered override-form message.

In the `idle` state — and only there — a `synthesize` event MUST be handled exactly as it is today, with no behavioural difference whatsoever. This is the non-streaming path used by Wyoming clients that do not implement streaming input. A `terminated` session is **not** idle: it absorbs the event (R10).

#### Scenario: compatibility event is suppressed

- **GIVEN** an open session that has received three `synthesize-chunk` events
- **WHEN** a `synthesize` event carrying the full concatenated message arrives, followed by `synthesize-stop`
- **THEN** the audio emitted MUST correspond to the chunked text exactly once, and the `synthesize` event MUST produce no additional audio

#### Scenario: zero-chunk fallback

- **GIVEN** an open session that has received no `synthesize-chunk` events
- **WHEN** a `synthesize` event carrying `"Hello world."` arrives, followed by `synthesize-stop`
- **THEN** the session MUST synthesize `"Hello world."` exactly once

#### Scenario: zero-chunk fallback carrying an override

- **GIVEN** an open session that has received no `synthesize-chunk` events
- **WHEN** a `synthesize` event carrying `"[speed: 1.2] Hello."` arrives, followed by `synthesize-stop`
- **THEN** the merged speed MUST be `1.2` and the synthesized text MUST be `Hello.` — the tag MUST NOT be spoken

#### Scenario: bare synthesize is untouched

- **GIVEN** a connection on which no `synthesize-start` has ever been sent
- **WHEN** a `synthesize` event arrives
- **THEN** the proxy MUST handle it exactly as before this change: `audio-start`, `audio-chunk*`, `audio-stop`, and no `synthesize-stopped`

### R8: Audio framing and format consistency

Each synthesized segment MUST be framed as `audio-start`, one or more `audio-chunk`, `audio-stop`. A single `synthesize-stopped` MUST follow the final segment of a successful session and MUST NOT be emitted per segment.

Home Assistant's `_read_tts_audio` writes its WAV header from the **first** `audio-start` it sees and ignores every later one, so the rate, width, and channels of the first segment govern the entire session. Every segment within a session MUST therefore use an identical audio format.

A segment's audio format MUST be determined before any `audio-start` for that segment is written. That is what the `openSegment`/`emitSegment` split in [Per-segment synthesis inside the proxy](#per-segment-synthesis-inside-the-proxy) exists for: opening a segment issues the upstream request and parses the format while writing nothing, so a mismatch is caught with no bytes on the wire.

The session MUST record the format of its first segment. If a later segment's format differs:

- In streaming/PCM mode (`ep.StreamingEnabled == true`) this cannot occur — the format is `{ep.StreamSampleRate, 2, 1}` by construction and is constant for the session's endpoint.
- In buffered/WAV mode the format comes from each response's WAV header and CAN differ. Meadowlark MUST NOT emit the mismatched segment. It MUST log at warn, emit a Wyoming `error` naming both formats, and terminate the session per R10. Resampling is out of scope (see [meadowlark.md §16](../meadowlark.md)), and emitting mismatched PCM under the first header produces audibly wrong pitch and speed — an error is the honest outcome.

#### Scenario: framing for a three-segment message

- **GIVEN** a session whose text segments into three parts
- **WHEN** the session completes successfully
- **THEN** the emitted event sequence MUST be exactly: `audio-start`, `audio-chunk`+, `audio-stop`, `audio-start`, `audio-chunk`+, `audio-stop`, `audio-start`, `audio-chunk`+, `audio-stop`, `synthesize-stopped`

#### Scenario: format changes mid-session in buffered mode

- **GIVEN** a buffered-mode endpoint whose first segment returns 24000 Hz WAV and whose second returns 16000 Hz WAV
- **WHEN** the second segment's header is parsed
- **THEN** no `audio-start` MUST be emitted for it, a Wyoming `error` naming both formats MUST be emitted, and the session MUST terminate without `synthesize-stopped`

### R9: Segmentation and session configuration

Segmentation thresholds and the session idle timeout MUST be process-level configuration, exposed as cobra flags with the project's standard `MEADOWLARK_*` env fallbacks:

| Flag | Env | Default |
|---|---|---|
| `--synthesize-first-segment-chars` | `MEADOWLARK_SYNTHESIZE_FIRST_SEGMENT_CHARS` | `24` |
| `--synthesize-min-segment-chars` | `MEADOWLARK_SYNTHESIZE_MIN_SEGMENT_CHARS` | `60` |
| `--synthesize-max-segment-chars` | `MEADOWLARK_SYNTHESIZE_MAX_SEGMENT_CHARS` | `400` |
| `--synthesize-session-timeout` | `MEADOWLARK_SYNTHESIZE_SESSION_TIMEOUT` | `30s` |

Character thresholds MUST satisfy `0 < firstSegmentChars ≤ minSegmentChars ≤ maxSegmentChars`. A configuration violating that ordering, or carrying a non-positive threshold, MUST log a warning at startup and fall back to all three defaults rather than starting with an incoherent mix.

The session timeout MUST be validated separately, because a `time.Duration` flag accepts values the character thresholds cannot take:

| Value | Behaviour |
|---|---|
| `> 0` | Idle timeout enabled at that duration. |
| `= 0` | Idle timeout disabled entirely; sessions never time out. |
| `< 0` | **Rejected.** Log a warning naming the supplied value and fall back to the `30s` default. |

The negative case is called out explicitly because Cobra and Viper both accept `-1s` without complaint, and handing a negative duration to a timer fires it immediately — every session would fail with `synthesize-timeout` the instant it opened. Silently treating a negative as "disabled" would be equally wrong, since it hides an operator's mistake. Rejecting it with a warning and using the default is the only behaviour that neither breaks synthesis nor conceals the misconfiguration.

#### Scenario: incoherent configuration falls back

- **GIVEN** `--synthesize-min-segment-chars=600` and `--synthesize-max-segment-chars=400`
- **WHEN** the process starts
- **THEN** a warning MUST be logged and the segmenter MUST use `24`/`60`/`400`

#### Scenario: negative session timeout is rejected

- **GIVEN** `--synthesize-session-timeout=-1s`
- **WHEN** the process starts
- **THEN** a warning naming the supplied value MUST be logged, the timeout MUST be `30s`, and sessions MUST NOT time out immediately

#### Scenario: zero session timeout disables the timer

- **GIVEN** `--synthesize-session-timeout=0`
- **WHEN** a session sits idle far longer than the default timeout
- **THEN** no `synthesize-timeout` error MUST be emitted and the session MUST stay open

### R10: Error handling and terminator discipline

A session MUST emit a Wyoming `error` (code `tts-error`, or `synthesize-timeout` for the idle case) at most once, and MUST NOT emit `synthesize-stopped` for a session that has errored.

This is deliberate. Home Assistant's `_read_tts_audio` raises on `error` and breaks its read loop on `synthesize-stopped`. Emitting both leaves an unconsumed `synthesize-stopped` in the connection buffer, which the *next* stream would read first and treat as an immediate end-of-stream — silently producing a stream with no audio. Exactly one terminator per session, and never both.

Consequently:

- Session completed successfully → exactly one `synthesize-stopped`, no `error`.
- Session failed → exactly one `error`, no `synthesize-stopped`. The session MUST then enter the `terminated` state of R5 rather than closing, so that **every** remaining event of that message — the compatibility `synthesize` included — is absorbed silently. Absorbing only the trailing `synthesize-stop` is not sufficient: a failure that happens before Home Assistant sends its compatibility `synthesize` would otherwise let the full message be synthesized a second time.
- No session was ever opened and a `synthesize-stop` arrives → emit nothing.

The session — not the Wyoming handler — owns reporting synthesis failures. A session method MUST emit its own `error` and return `nil`, never surface a synthesis failure as a returned error, because `Server.handleConn` converts a returned error into a second `error` event with code `handler-error`. Emitting both would break exactly-one-terminator and report the wrong code. See [the `tts.StreamSession` API](#ttsstreamsession-api) for the full split between domain failures and connection-write failures.

This applies identically to every way a session can fail — an upstream error, a mid-session format mismatch (R8), and the idle timeout (R5).

On failure the session MUST **quiesce** as defined in R5 — cancel, join the emitter, discard the queue — before the `error` is written. That ordering is what guarantees the `error` is the last thing the client sees for that message; without joining the emitter first, a stale `audio-chunk` can arrive after it. The connection MUST remain open and usable, matching the existing contract that handler errors never close the connection.

A segment failing **after** its `audio-start` was emitted is covered by the emitter's own exit path: it writes that group's `audio-stop` as it returns, and only then — once the join completes — is the `error` written. The client is therefore never left inside an unterminated audio group, and no audio can follow the `error`.

#### Scenario: upstream failure on the second segment

- **GIVEN** a three-segment session whose second segment's upstream returns HTTP 500 before any audio
- **WHEN** the failure is observed
- **THEN** segment 1's group MUST have been emitted in full, no `audio-start` MUST be emitted for segments 2 or 3, exactly one `error` with code `tts-error` MUST be emitted, no `synthesize-stopped` MUST be emitted, and the connection MUST stay open

#### Scenario: compatibility synthesize after an early failure

- **GIVEN** a session whose first segment's upstream fails before Home Assistant has sent its compatibility `synthesize`
- **WHEN** that `synthesize` arrives carrying the entire message, followed by `synthesize-stop`
- **THEN** it MUST be absorbed silently, `Proxy.HandleSynthesize` MUST NOT be called, no audio MUST be emitted for it, and no second `error` or `synthesize-stopped` MUST be emitted

#### Scenario: idle timeout then compatibility synthesize

- **GIVEN** a session abandoned by the idle timeout, which emitted `error` with code `synthesize-timeout`
- **WHEN** a late compatibility `synthesize` arrives on the same connection before any new `synthesize-start`
- **THEN** it MUST be absorbed silently rather than synthesized

#### Scenario: an upstream failure produces exactly one error event

- **GIVEN** a streaming session whose upstream returns HTTP 500
- **WHEN** the failure is handled
- **THEN** exactly one `error` event MUST reach the client, with code `tts-error`, and no `handler-error` event MUST be emitted — the session method MUST have returned `nil`

#### Scenario: upstream disconnects mid-segment

- **GIVEN** a segment whose `audio-start` and some `audio-chunk` events have been emitted
- **WHEN** the upstream response body fails with a non-EOF read error
- **THEN** `audio-stop` MUST be emitted for that segment, then exactly one `error`, and no `synthesize-stopped`

### R11: End-to-end conformance test

A table-driven end-to-end test MUST drive a fake Wyoming client over a real TCP connection to a running `Server`, replaying the exact Home Assistant event sequence, and MUST assert the **ordered** list of emitted event types.

The baseline case MUST be the HA sequence verified against `homeassistant/components/wyoming/tts.py`:

```
→ synthesize-start {voice}
→ synthesize-chunk {text} × N
→ synthesize        {text: <entire message>, voice}   (compatibility)
→ synthesize-stop
← audio-start / audio-chunk+ / audio-stop   (per segment)
← synthesize-stopped
```

The table MUST additionally cover: a single chunk containing several sentences; zero chunks with only the compatibility `synthesize`; a bare `synthesize` with no session; `synthesize-stop` with no session; an upstream error mid-session; and a `describe` asserting `supports_synthesize_streaming: true`.

## Design

### Approach

#### Package boundaries and the import constraint

`internal/tts` imports `internal/wyoming`. **`internal/wyoming` therefore MUST NOT import `internal/tts`** — session orchestration cannot live in the `wyoming` package. The work is split as:

| Package | Gains |
|---|---|
| `internal/wyoming` | The four event types + constants (R2); `SupportsSynthesizeStreaming` on `TtsProgram` (R1); `HandlerFactory` / `ConnHandler` and per-connection dispatch (R3); single-`Write` `WriteEvent` and the mutex-guarded connection writer (R4). No session logic, no `tts` dependency. |
| `internal/segment` *(new)* | Pure text segmenter (R6). No I/O, no logging, no dependencies beyond the standard library. Trivially unit-testable. |
| `internal/tts` | `StreamSession` — buffering via the segmenter, the ordered pipeline, per-segment synthesis, format consistency, error handling, and the terminal `synthesize-stopped`. |
| `cmd/meadowlark` | `wyomingHandler` implements `wyoming.HandlerFactory`; a thin `connHandler` dispatches events to its `*tts.StreamSession` or to the existing singleton paths. |

Keeping the logic in `internal/*` and the dispatch in `cmd/` matters because `cmd/meadowlark` has **no test file today**; anything put there needs a new `package main` test, so as little as possible goes there.

#### `tts.StreamSession` API

```go
// NewStreamSession creates an idle session bound to a proxy. One per connection.
func NewStreamSession(p *Proxy, cfg segment.Config, idleTimeout time.Duration, logger *slog.Logger) *StreamSession

func (s *StreamSession) Active() bool
func (s *StreamSession) Start(ctx context.Context, w io.Writer, ev *wyoming.SynthesizeStart) error
func (s *StreamSession) Chunk(ev *wyoming.SynthesizeChunk) error
func (s *StreamSession) Compat(ev *wyoming.Synthesize) bool  // true = suppressed
func (s *StreamSession) Stop() error
func (s *StreamSession) Close()                              // connection teardown; blocks
```

**Error ownership.** The `error` these methods return is **not** a synthesis failure. `Server.handleConn` turns any non-nil handler error into a Wyoming `error` event with code `handler-error`, so if a session returned an upstream failure the client would receive both that event and the session's own `tts-error` — two errors for one message, breaking R10's exactly-one-terminator rule and reporting the wrong code.

The split is therefore:

| Failure | Who reports it | Method returns |
|---|---|---|
| Synthesis failure — upstream error, format mismatch, idle timeout | The session, as a Wyoming `error` per R10 | `nil` |
| Writing to the connection itself failed | Nobody; the connection is already broken | the write error |

So a session method MUST return `nil` for every domain failure, having already emitted its own `error` and entered `terminated`. It MUST return non-nil only when a write to the connection failed. `connHandler` MUST pass that through unchanged, so the server's existing logging and teardown path runs; the follow-up `handler-error` write will fail too, which is harmless on a connection that is already gone.

`Compat` returns `true` when the session absorbed the event and `false` only when the session is `idle`, in which case the caller MUST fall through to the existing `Proxy.HandleSynthesize`. It therefore returns `true` in **both** the `open` state (R7 suppression) and the `terminated` state (R10 tombstone) — a failed session must keep absorbing, or the compatibility event escapes and the message is spoken twice. `Active()` reports the same two states for the same reason. That single boolean is the whole suppression contract, and it lives in a tested internal package rather than in `main`.

#### Per-segment synthesis inside the proxy

`doSynthesize` currently resolves the voice, parses input overrides, fetches the endpoint, merges parameters, calls the client, and emits the audio group — all for one text. Segment synthesis needs the same pipeline minus the emission, run repeatedly with the same resolution.

Factor it into **three** pieces, so the session resolves once per session, and — critically — learns each segment's audio format **before** any byte of that segment is written:

- `Proxy.resolveSynthesis(ctx, voiceName, parsed voice.ParsedInput) (*synthesisPlan, error)` — steps 1 and 3–6 of the [existing flow](../specs/tts-synthesis/index.md#synthesis-flow) (resolve voice, alias defaults, fetch endpoint, endpoint defaults, merge), returning the endpoint, the client, and the merged parameters. Step 2, input parsing, is **not** part of it — the caller supplies the already-parsed overrides, for the reason in [Override parsing](#per-segment-synthesis-inside-the-proxy) below.
- `Proxy.openSegment(ctx, plan, text) (*OpenSegment, error)` — step 7 only. Issues the upstream request and determines the audio format: in streaming mode from `{ep.StreamSampleRate, 2, 1}`, in buffered mode by calling `WAVReader.ReadFormat()` on the response. Writes **nothing**. Returns an `*OpenSegment` exposing `Format() *AudioFormat`, a PCM `io.Reader` positioned at the first sample, and `Close() error`.
- `Proxy.emitSegment(w io.Writer, seg *OpenSegment) error` — step 8 only. Writes `audio-start`, the `audio-chunk` events, and `audio-stop` for an already-opened segment.

The split is what makes R8 implementable: the session calls `openSegment`, compares `seg.Format()` against the session format, and only then calls `emitSegment` — so a mismatched segment is closed without a single `audio-start` reaching the client. A contract that returned the format *after* emitting could not satisfy R8 at all.

The same split is what the prefetch in [Ordered pipelining](#ordered-pipelining) needs: prefetching segment N+1 is exactly an early `openSegment` call, and the emitter's job is exactly `emitSegment`. `OpenSegment.Close()` is what a cancelled or discarded prefetch releases.

`doSynthesize` MUST be re-expressed as `resolveSynthesis` → `openSegment` → `emitSegment` so the non-streaming path and the streaming path share one implementation. This is the load-bearing refactor of the change; keeping two copies of the synthesis pipeline would guarantee divergence.

**Override parsing must see the whole message, so segmentation is deferred when overrides are present.**

`voice.ParseInput` dispatches on the first non-whitespace character of the text and has two very different behaviours (`internal/voice/parser.go`):

- **`{` — JSON form.** `parseJSON` unmarshals the **entire** string as one JSON object. It does not extract JSON from a prefix. Handed a fragment, `json.Unmarshal` fails and `ParseInput` falls through to treating the fragment as plain input — so the braces would be **spoken aloud** and every override silently dropped.
- **`[` — tag form.** `parseTags` consumes leading `[...]` groups and returns the rest as `Input`. It works on a prefix, but only once the closing `]` has actually arrived.

Running `ParseInput` on the first flushed segment therefore corrupts both forms. The rule instead keys on the same character `ParseInput` itself dispatches on:

- **The session's first non-whitespace character is `{` or `[`** → the session MUST NOT flush any segment. It buffers the entire message and, on `synthesize-stop`, runs `ParseInput` on the complete text exactly as the whole-message path does, then segments the resulting `Input` and emits those segments. The latency win is forfeited for this message; correctness is not. Log at debug that override-form input disabled incremental segmentation.
- **Anything else** → this is ordinary prose, which is what Home Assistant streams. `ParseInput` MUST NOT be run at all, and the raw text is segmented as it arrives. Not running it is deliberate: a prose message is not an override form, and parsing per-segment could only misfire.

In both cases the overrides are known before the first segment is synthesized, so `resolveSynthesis` runs once and its plan is reused verbatim for every segment.

**What selects the endpoint, and what only overrides a request parameter.** These are two different things today and this change MUST NOT merge them:

- **`synthesize-start`'s `voice`** is the Wyoming voice name. It is what `resolver.Resolve` consumes, and it alone selects the **endpoint**, together with the baseline `model` and `voice` that resolution yields. When it is absent — R2 makes it optional — resolution falls to Stage 0 exactly as an empty `synthesize` voice does today, picking the first enabled endpoint with a default voice, and erroring with `"voice: no voice specified and no default voice configured"` when there is none.
- **A JSON or tag override** never reaches the resolver. It flows through `voice.MergeParams` at input priority and sets the `voice`, `model`, `speed` and `instructions` fields of the upstream `/audio/speech` request. That is the existing whole-message behaviour and it MUST be preserved unchanged — including the `model` override, which wins over the resolved baseline exactly as it does for a whole-message request.

Concretely, the layering per segment is: the endpoint is fixed by the start voice; every request parameter is `MergeParams(input, alias, endpoint)`.

So a session may legitimately have no start voice and still carry a JSON `voice` and `model`: the endpoint comes from Stage 0, while the upstream voice and model parameters come from the JSON.

Because the caller now owns input parsing, `resolveSynthesis` takes the already-parsed overrides rather than raw text:

```go
func (p *Proxy) resolveSynthesis(ctx context.Context, voiceName string, parsed voice.ParsedInput) (*synthesisPlan, error)
```

`doSynthesize` supplies `voice.ParseInput(ev.Text)`; a prose streaming session supplies a zero `voice.ParsedInput`; an override-form streaming session supplies `voice.ParseInput(<whole buffered message>)`.

#### Ordered pipelining

Segments MUST be emitted in text order. Emission is performed by a single **emitter** goroutine consuming an ordered FIFO of segment jobs.

Upstream requests are started ahead of emission, bounded at `maxInFlight = 2` — the segment currently being emitted plus exactly one prefetch. The prefetched segment's HTTP request is issued and its `io.ReadCloser` held undrained; the upstream begins generating on receipt, so its time-to-first-byte is spent while the previous segment is still playing. The emitter never touches job N+1 before writing job N's `audio-stop`, so ordering is structural rather than a timing accident.

`maxInFlight` is a package constant, not a flag: upstream TTFB (~0.2–0.6 s) is far shorter than a segment's playback duration (≥ ~2 s at the configured minimum), so a depth of 2 already hides it completely. Deeper prefetch multiplies upstream cost and wasted work on cancellation for no perceptible gain.

On failure or cancellation the session MUST cancel the contexts of all queued and in-flight jobs and close their bodies, so a prefetched-but-never-emitted segment cannot leak a connection.

#### Handler dispatch after this change

| Event | Action |
|---|---|
| `describe` | Unchanged — `InfoBuilder.Build(ctx)`, now carrying `supports_synthesize_streaming: true`. |
| `synthesize-start` | `session.Start(ctx, w, ev)` |
| `synthesize-chunk` | `session.Chunk(ev)` |
| `synthesize` | `if !session.Compat(ev) { proxy.HandleSynthesize(ctx, synth, w) }` |
| `synthesize-stop` | `session.Stop()` |
| `ping` | Unchanged. |
| Unknown | Unchanged — log at debug, ignore. |

### Decisions

- **Decision:** Advertise `supports_synthesize_streaming: true` unconditionally rather than gating it on endpoint configuration.
  - **Why:** The flag describes what the *Wyoming service* accepts on its input side, which is a property of Meadowlark, not of any upstream. Meadowlark can always segment text and issue one upstream request per segment, whatever the endpoint supports. It is also structurally impossible to gate: `info` advertises a single service-level `TtsProgram` aggregating every endpoint, so there is no per-endpoint place to put a per-endpoint answer, and Home Assistant reads the flag once for the whole service.
  - **Alternatives considered:** Gate on `ep.StreamingEnabled` for some endpoint — rejected: wrong layer, ambiguous when endpoints disagree, and it would deny the (large) buffering win to every operator who has not enabled upstream PCM streaming. A config flag to force it off — rejected as unnecessary surface; the non-streaming path remains fully supported for clients that ignore the flag.

- **Decision:** Introduce a handler factory (`HandlerFactory` / `ConnHandler`) rather than a session map keyed by connection inside the existing singleton handler.
  - **Why:** The factory gives each connection a genuinely private handler, so session state is an ordinary struct field with no shared map and no lock around it. It also solves cleanup properly: a map-based design has no way to learn that a connection dropped — `HandleEvent` is never called again, so entries leak, in-flight upstream requests are never cancelled, and `Shutdown()` cannot drain them. `ConnHandler.CloseConn()` is called from the same `defer` that already closes the connection, which is the only place that reliably knows.
  - **Alternatives considered:** A `map[io.Writer]*session` keyed on the `w` argument — rejected: it makes the writer do double duty as an identity token, depends on `w` being a stable comparable value, needs a mutex on every event, and leaks on disconnect. Changing `Handler.HandleEvent`'s signature to carry a connection identity — rejected: it breaks every existing implementation and test for no gain over the optional-interface approach, which is fully backwards compatible.

- **Decision:** Make `WriteEvent` issue exactly one `Write` per event, and wrap the connection in a mutex-guarded writer.
  - **Why:** `WriteEvent` currently issues up to four `Write` calls. A mutex around each `Write` would not make an event atomic, and no amount of locking in the *server* can fix a multi-`Write` writer used by a *handler*. Buffering the whole event makes one-`Write`-per-event true, at which point a single mutex is sufficient and the invariant is testable with a counting writer. It also cuts syscalls per audio chunk.
  - **Alternatives considered:** An `EventWriter` type with a locked `WriteEvent` method — rejected: `Handler.HandleEvent` and `Proxy.HandleSynthesize` take `io.Writer`, so this would change both signatures and every call site. Giving each session its own connection — rejected outright; Wyoming is single-connection.

- **Decision:** Segment on sentence boundaries with a minimum length, a lower threshold for the first segment, and a bounded forced flush.
  - **Why:** This is the tradeoff being tuned. Smaller segments lower time-to-first-audio but cost one upstream HTTP round-trip each and introduce an audible seam at every join, because each request is synthesized with independent prosody. Larger segments sound better and cost less but delay first audio. Sentence boundaries are the natural seam — a join at a sentence end is where a speaker would pause anyway, so the seam is nearly inaudible; a join mid-clause is not.
  - **Defaults, and why those numbers:** `minSegmentChars = 60` is roughly 10 words, ≈4 s of speech at ~150 wpm — comfortably longer than an upstream TTFB, so the next segment's request completes while the current one is still playing and the pipeline never starves. `firstSegmentChars = 24` applies only to the session's opening segment, which alone determines perceived latency; a short opener ("Sure, turning that on.") gets audio moving ~1 s earlier and its own playback covers the next request. `maxSegmentChars = 400` is ≈28 s of speech — reached only by genuinely pathological run-on text, and it bounds both latency and buffer growth.
  - **Alternatives considered:** Fixed-size chunking — rejected: seams land mid-word and prosody breaks badly. Flush every `synthesize-chunk` — rejected: one HTTP request per token is absurd, and the seams would be continuous. A full sentence tokenizer / NLP dependency — rejected: the punctuation-plus-abbreviation heuristic covers the realistic input (LLM-generated assistant replies) and adds no dependency to a single-static-binary project.

- **Decision:** Pipeline with `maxInFlight = 2` and strictly ordered emission, rather than synthesizing segments strictly sequentially.
  - **Why:** Sequential synthesis pays the upstream TTFB *between every pair of segments*, which is audible as a gap at each seam — the very seams the segmentation is trying to hide. One segment of prefetch removes that gap entirely at the cost of at most one speculative request. Ordering is preserved structurally by a single emitter goroutine, so there is no reordering hazard to reason about.
  - **Alternatives considered:** Fully sequential — rejected: leaves a real, audible gap at every seam for no benefit beyond simplicity. Unbounded concurrency — rejected: multiplies upstream cost, holds many connections open, wastes work on cancellation, and buys nothing once TTFB is already hidden.

- **Decision:** Abort the session on a mid-session audio-format change in buffered mode, rather than resampling or emitting anyway.
  - **Why:** Home Assistant honours only the first `audio-start`, so mismatched PCM is played at the wrong rate — audibly wrong, with no error anywhere. Resampling is explicitly out of scope for v1. An explicit error is the only honest option, and the case is pathological in practice because every segment of a session goes to the same endpoint, model, and voice.

- **Decision:** Emit either `error` or `synthesize-stopped`, never both.
  - **Why:** Home Assistant raises on `error` and breaks on `synthesize-stopped`. Emitting both leaves an unconsumed terminator in the connection buffer that the next stream reads first, silently ending it with no audio. This is a subtle, hard-to-diagnose failure mode, so the discipline is stated as a hard rule (R10).

- **Decision:** Segmentation thresholds are process-level flags, not per-endpoint columns.
  - **Why:** Segmentation happens in the Wyoming session, above endpoint resolution — the first segment must exist before the endpoint is known. Per-endpoint tuning would require resolving the endpoint at `synthesize-start`, which is a larger change for a knob nobody has yet asked to vary per endpoint. Flags with `MEADOWLARK_*` fallbacks match every other tunable in the project and require no migration, no API change, and no frontend work.

### Interaction with `StreamingEnabled` and `StreamSampleRate`

These settings are **orthogonal** to synthesize streaming and both dimensions compose:

| | `StreamingEnabled = false` (buffered WAV upstream) | `StreamingEnabled = true` (PCM upstream) |
|---|---|---|
| **Client without streaming input** | Today's behaviour. One request, full WAV buffered by HA. | Today's behaviour with 0002. One request, PCM forwarded, HA still buffers the whole thing. |
| **Client with streaming input (this change)** | Works, with a smaller win. Each segment is one buffered WAV request; segment 1's synthesis starts at the first sentence boundary past `firstSegmentChars` instead of after the whole LLM message. Cross-segment format consistency must be enforced (R8). | Full win. Segment 1's *first bytes* flow out as the upstream produces them, and the format is fixed at `{StreamSampleRate, 2, 1}` so the R8 hazard cannot arise. |

`StreamSampleRate` keeps its existing meaning and its `24000` default and is unchanged by this change. Operators seeking the lowest latency SHOULD enable `StreamingEnabled` on endpoints that support PCM streaming, but MUST NOT be required to: synthesize streaming MUST work correctly with `StreamingEnabled = false`.

### Non-Goals

- Per-endpoint segmentation tuning, and any frontend surface for it.
- Audio resampling or format conversion to reconcile a mid-session format change.
- SSML rendering. `text_format: "ssml"` MUST be accepted without error and treated as plain text; a session MAY log at debug when it sees it.
- Echoing `synthesize-start`'s `context` field back to the client. It is round-tripped through the type (R2) but otherwise unused.
- Wyoming ASR, wake-word, or intent-handling programs.
- Any change to voice resolution, aliases, or endpoint CRUD semantics.
- Caching or deduplicating repeated segments across sessions.

## Tasks

- [x] Wyoming event types and info flag
  - [x] Add `TypeSynthesizeStart`, `TypeSynthesizeChunk`, `TypeSynthesizeStop`, `TypeSynthesizeStopped` to `internal/wyoming/types.go` (PR #44)
  - [x] Add `SynthesizeStart`, `SynthesizeChunk`, `SynthesizeStop`, `SynthesizeStopped` structs with `ToEvent` and `…FromEvent`, matching the existing house pattern (PR #44)
  - [x] Encode `SynthesizeStart`'s voice with `name`, `language` and `speaker` all nested under `voice` — and leave `Synthesize`'s existing encoding (only `name` nested; `speaker` and `language` top-level) untouched. Add a test asserting `Synthesize.ToEvent`'s wire shape is unchanged (PR #44)
  - [x] Add `SupportsSynthesizeStreaming bool` to `TtsProgram`; emit it in `Info.ToEvent()` and parse it in `InfoFromEvent` (PR #44)
  - [x] Set it to `true` in `internal/wyoming/info.go` where the `TtsProgram` is constructed — deliberately deferred to the PR that lands `tts.StreamSession`, because advertising the capability makes Home Assistant take its streaming path, and it must not do so before there is a session behind it (PR #47)
  - [x] Tests in `internal/wyoming/types_test.go`: round-trip symmetry for all four types, voice-object nesting, `context` passthrough, flag present in `info`, flag defaults to `false` when absent (PR #44)
- [x] Event-atomic writes (PR #44)
  - [x] Rewrite `WriteEvent` in `internal/wyoming/event.go` to assemble one buffer and issue exactly one `Write`
  - [x] Add an unexported mutex-guarded connection writer in `internal/wyoming/server.go`; use it for both handler dispatch and the read loop's own error writes
  - [x] Tests: counting writer asserts exactly one `Write` per event; concurrent-writer test under `-race` asserts no interleaving
- [x] Per-connection handlers (PR #44)
  - [x] Add `HandlerFactory` and `ConnHandler` interfaces to `internal/wyoming/server.go`
  - [x] Use them in `handleConn`: build once per connection when available, `CloseConn()` from the existing teardown `defer`
  - [x] Tests: one handler per connection; `CloseConn` called exactly once on disconnect and on `Shutdown()`; non-factory handlers and `HandlerFunc` unchanged
- [x] `internal/segment` package (PR #44)
  - [x] `segment.Config{FirstSegmentChars, MinSegmentChars, MaxSegmentChars int}` with defaults and the `0 < First ≤ Min ≤ Max` validation from R9
  - [x] `segment.Segmenter` with `Add(text string) []string` and `Flush() []string`
  - [x] Boundary detection, closing-punctuation run, trailing-whitespace guard, newline boundary
  - [x] Abbreviation, decimal, and single-initial suppression
  - [x] Length gating with the first-segment threshold
  - [x] Forced break with soft-break → whitespace → rune-aligned hard-cut preference
  - [x] Table-driven tests for every R6 scenario plus: CJK punctuation, ellipsis, closing quote after period, whitespace-only remainder, text arriving one rune at a time
- [x] Proxy refactor (PR #45)
  - [x] Extract `resolveSynthesis(ctx, voiceName, parsed voice.ParsedInput)` from `doSynthesize` in `internal/tts/proxy.go` — resolve, alias/endpoint defaults, merge, fetch endpoint, build client. It MUST NOT call `voice.ParseInput` itself; the caller owns that, because a streaming session must parse the whole message or not at all
  - [x] Extract `openSegment` — issues the upstream request and determines the `*AudioFormat` (WAV header in buffered mode, endpoint config in streaming mode) while writing nothing; returns an `*OpenSegment` with `Format()`, a PCM reader, and `Close()`
  - [x] Extract `emitSegment` — writes `audio-start`/chunks/`audio-stop` for an already-opened segment
  - [x] Re-express `doSynthesize` as `resolveSynthesis` → `openSegment` → `emitSegment` so streaming and non-streaming share one pipeline
  - [x] Test that a buffered-mode format mismatch is detectable between `openSegment` and `emitSegment` — i.e. `openSegment` writes nothing to `w`
  - [x] Confirm existing `internal/tts/proxy_test.go` passes unchanged — the non-streaming path MUST be behaviourally identical
- [x] `tts.StreamSession`
  - [x] `NewStreamSession`, `Active`, `Start`, `Chunk`, `Compat`, `Stop`, `Close` per the API above (PR #46)
  - [x] Session context via `context.WithCancel` from the `Start` call's `ctx`; cancel on `Close`, error, and idle timeout (PR #46)
  - [x] Ordered job FIFO with `maxInFlight = 2` prefetch and a single emitter goroutine (PR #46)
  - [x] Override-form detection on the session's first non-whitespace character: `{` or `[` suspends segmentation until `synthesize-stop`, then `ParseInput` the whole message and segment its `Input`; anything else skips `ParseInput` entirely and segments incrementally (PR #46)
  - [x] Resolve once, before the first segment is synthesized; reuse the plan for all later segments (PR #46)
  - [x] Format recording and R8 mismatch abort (PR #46)
  - [x] Three-state machine from R5: `idle` / `open` / `terminated`, with `Active()` and `Compat()` both treating `terminated` as absorbing (PR #46)
  - [x] Error ownership: session methods emit their own Wyoming `error` and return `nil` for every domain failure; a non-nil return means only that a connection write failed (PR #46)
  - [x] Test that an upstream failure yields exactly one `error` event with code `tts-error` and no `handler-error` (PR #46)
  - [x] Test that an input `model` override reaches every segment's upstream request while the endpoint stays the one resolved from the start voice (PR #46)
  - [x] Terminator discipline from R10, including `audio-stop` before `error` when a segment fails after `audio-start`, and the tombstone that absorbs the compatibility `synthesize` after an early failure (PR #46)
  - [x] Test that a failure occurring *before* the compatibility `synthesize` still suppresses it — the message MUST NOT be synthesized a second time (PR #46)
  - [x] Idle timer: armed on `Start`, reset by `Chunk` and `Compat`, disarmed by `Stop`, `synthesize-timeout` error on expiry, disabled entirely when the timeout is `0` (PR #46)
  - [x] Test that a session receiving one chunk and then stalling is abandoned after the timeout, and that a session receiving a chunk every half-timeout is not (PR #46)
  - [x] A single `quiesce` helper implementing R5's three ordered steps — cancel, **join the emitter**, discard the queue — used by the restart, failure, timeout and teardown paths alike, so none of them can skip the join (PR #46)
  - [x] The emitter writes the closing `audio-stop` for a group it opened, as its own final act on cancellation; no other goroutine writes audio events, and no terminating path writes that `audio-stop` itself (PR #46)
  - [x] `synthesize-start` while active: quiesce, emit `synthesize-stopped`, restart (PR #46)
  - [x] Test under `-race` that a restart mid-audio emits the old group's `audio-stop`, then `synthesize-stopped`, and that no old-session audio event appears afterwards or interleaves with the new session's (PR #46)
  - [x] Zero-chunk fallback text handling from R7 (PR #46)
  - [x] Tests in `internal/tts/stream_session_test.go`: ordered emission with prefetch; suppression and fallback; buffered and streaming endpoint modes; format mismatch; upstream error before and after `audio-start`; cancellation closes bodies; idle timeout; JSON-form and tag-form input arriving in fragments; prose never override-parsed; `-race` clean (PR #46)
- [x] Configuration
  - [x] Add the four flags from R9 to `cmd/meadowlark/main.go` with `MEADOWLARK_*` fallbacks (PR #47)
  - [x] Validate and fall back to defaults with a warning on incoherent character thresholds (PR #47)
  - [x] Validate the session timeout separately: positive enables, `0` disables, negative is rejected with a warning and falls back to `30s` (PR #47)
  - [x] Pass the resulting `segment.Config` and idle timeout into the handler factory (PR #47)
- [x] Handler wiring
  - [x] Make `wyomingHandler` implement `wyoming.HandlerFactory` (PR #47)
  - [x] Add `connHandler` with a `*tts.StreamSession`, dispatching per the table above, and `CloseConn()` delegating to `session.Close()` (PR #47)
  - [x] Add `cmd/meadowlark/main_test.go` covering the dispatch table, including the bare-`synthesize` and stray-event paths (PR #47)
- [x] End-to-end conformance test (R11)
  - [x] Table-driven test over a real TCP connection asserting the ordered emitted event types for every case listed in R11 (PR #47)
- [x] Documentation
  - [x] **Remove every "planned" marker for this change from the living specs.** The spec-only PR added them because the specs must not assert unshipped behaviour; the implementing PR is what makes them false, so removing them is part of this change, not a follow-up. All three files carry them (PR #47):
    - `docs/specs/wyoming-protocol/index.md` — Overview streaming bullet; the `WriteEvent` single-`Write` blockquote under Read/Write Contract; the four `*planned*` cells in the event-type constants table; the commented-out `SupportsSynthesizeStreaming` field and the planned blockquote under Home Assistant Compatibility Requirements; the suppression blockquote under Synthesize; the `Streaming Synthesis Events — *Planned*` heading suffix and its blockquote; the commented-out `HandlerFactory`/`ConnHandler` block and the blockquote under Architecture; the planned blockquote under Connection Lifecycle; the blockquote under `Write Serialization`; the planned blockquote under TCP Server Requirements; the italicised planned scenario; the planned blockquote under Event Handler Routing; the blockquote under `Streaming Synthesis Input`; the planned blockquote under Info Builder Requirements; and the two "gains … with 0006" notes in the Files table
    - `docs/specs/tts-synthesis/index.md` — the Overview inbound-modes bullet and the `internal/segment/` package note; the four-stage pipeline blockquote under Synthesis Flow; the `*planned*` cell in the error table; the blockquote under `Segmented Streaming Synthesis`; the planned blockquote under Endpoint Streaming Configuration Requirements; and the separate "planned file" table under Files, whose two rows fold back into the main table
    - `docs/meadowlark.md` — the four `*planned*` cells in the §2.2 event table; the "Specified but not yet implemented" sentence in §2.2; the planned note on the §2.3 streaming input flow diagram; the planned sub-list in §9.4 Concurrency Model, whose bullets fold back into the main list; and the §16 parenthetical
  - [x] Restore present tense throughout those sections as the markers come off — several were rewritten to "will" when the markers went on (PR #47)
  - [x] Note that three headings deliberately kept their original text — `Write Serialization`, `Streaming Synthesis Input`, `Segmented Streaming Synthesis` — because other documents link to their anchors; do not rename them while removing the markers (PR #47)
  - [x] Add a changelog row to each amended living spec recording that the behaviour now ships, and reword the existing 0006 rows from "Specify" to reflect implementation (PR #47)
  - [x] Correct the tts-synthesis spec's synthesis-flow step ordering: `doSynthesize` now parses input before resolving the voice, because `resolveSynthesis` takes already-parsed input, so the numbered flow listing resolution as step 1 and parsing as step 2 is stale. Nothing observable changed — `voice.ParseInput` is pure and total — but the documented order no longer matches the code (PR #47)
  - [x] Confirm the living specs still match the implementation; correct them if the shape drifted during implementation (PR #47)
  - [x] Flip this document's status to `complete` and update `docs/index.yml` **and `docs/index.md`** (both carry a status for 0006) (PR #47)

## Open Questions

- [ ] Should the abbreviation list be locale-aware, or extended from a wordlist? The current list is English-only and deliberately short. Revisit if non-English assistant replies segment badly.
- [ ] Is `firstSegmentChars = 24` too aggressive for slow upstreams? A very short opener followed by a slow segment 2 could still stutter despite prefetch. Measure once deployed; the flag exists precisely so this can be tuned without a release.
- [ ] Should a future change echo `synthesize-start`'s `context` field back on `synthesize-stopped`? Nothing in Home Assistant reads it today. Deferred.
- [ ] Does the installed Home Assistant version transcode the streamed WAV to mp3 incrementally? This determines how much of the ~5.2 s gap this change actually recovers on the satellite leg. Verify by measuring first-audio time at the satellite after deployment.
- [ ] Should `no` stay in R6's abbreviation list? It is far more common as a sentence-final word ("The answer is no. Next question.") than as the abbreviation "No." for a number, so listing it suppresses boundaries that are legitimate sentence ends. The list is specified verbatim in R6, so removing it is a contract amendment rather than a code fix, and it is deliberately left alone here. The impact is bounded: a suppressed boundary neither loses nor corrupts text, it only defers the flush to the next boundary or to the forced cut, and it can only matter once the candidate segment has already reached the minimum length — below that threshold the boundary would not have flushed anyway.
- [ ] Should `Synthesize` be corrected to nest `language` and `speaker` inside its `voice` object, matching upstream? Upstream `rhasspy/wyoming` `wyoming/tts.py` builds `SynthesizeVoice.to_dict()` as `{"name": ...}` with `speaker` nested inside it — or `{"language": ...}` when there is no name — and `Synthesize.event()` assigns that object to `data["voice"]`; Home Assistant sends `Synthesize(text=..., voice=SynthesizeVoice(name=..., language=...))`, so `language` arrives nested and `SynthesizeFromEvent`, which reads `ev.Data["language"]` at the top level, drops it. Inert today because nothing reads `Synthesize.Language` or `Synthesize.Speaker` outside `internal/wyoming/types.go` and its tests. Deferred out of 0006 because it changes an existing wire contract; see the note in [R2](#r2-streaming-event-types).

## References

- Spec: [wyoming-protocol — Streaming Synthesis Input](../specs/wyoming-protocol/index.md#streaming-synthesis-input), [wyoming-protocol — Per-Connection Session State](../specs/wyoming-protocol/index.md#per-connection-session-state), [tts-synthesis — Segmented Streaming Synthesis](../specs/tts-synthesis/index.md#segmented-streaming-synthesis)
- Related changes: [0001-streaming-tts-client](0001-streaming-tts-client.md), [0002-streaming-proxy-integration](0002-streaming-proxy-integration.md)
- Upstream protocol: `rhasspy/wyoming` — `wyoming/info.py` (`TtsProgram.supports_synthesize_streaming`), `wyoming/tts.py` (`SynthesizeStart`, `SynthesizeChunk`, `SynthesizeStop`, `SynthesizeStopped`)
- Home Assistant: `homeassistant/components/wyoming/tts.py` — `async_supports_streaming_input`, `_write_tts_message`, `_read_tts_audio`
- Code: `internal/wyoming/types.go`, `internal/wyoming/event.go`, `internal/wyoming/server.go`, `internal/wyoming/info.go`, `internal/tts/proxy.go`, `internal/tts/client.go`, `cmd/meadowlark/main.go`
