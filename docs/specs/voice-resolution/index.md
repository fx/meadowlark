# Voice Resolution

Living specification for Meadowlark's voice resolution system — the priority-based resolver, custom input parser, parameter merging, and canonical voice naming.

## Overview

When a Wyoming `synthesize` event arrives, the voice name and input text must be resolved into a concrete TTS configuration: which endpoint to call, which model and voice to use, and what speed/instructions to apply. The voice resolution system handles this through a priority chain, input text parsing, and a three-level parameter merge.

**Packages:** `internal/voice/`, `internal/model/`

## Voice Resolver

### Structure

```go
type Resolver struct {
    endpoints EndpointLister    // Lists endpoints from DB
    aliases   AliasLister       // Lists voice aliases from DB
}
```

### Resolution Priority Chain

`Resolve(ctx, name) → (*model.ResolvedVoice, error)` resolves a voice name through four stages, returning the first successful match:

#### Stage 0: Default Voice

**Trigger:** `name == ""` or `name == "default"`

Scans all endpoints (ordered by DB query) for the first enabled endpoint that has a non-empty `DefaultVoice` **and** at least one enabled model. Endpoints with empty `Models` MUST be skipped (per [Default Model](#default-model)). Returns a `ResolvedVoice` with that endpoint's ID, the endpoint's **default model**, and the endpoint's `DefaultVoice`. Includes the endpoint's `DefaultSpeed` and `DefaultInstructions`.

**Error:** If no eligible endpoint exists, returns `"voice: no voice specified and no default voice configured"`.

#### Stage 1: Alias Lookup

**Trigger:** Always attempted if Stage 0 didn't match.

Searches all voice aliases by `Name` field. The alias MUST be `Enabled`. Returns a `ResolvedVoice` with `IsAlias: true` containing the alias's endpoint ID, model, voice, speed, instructions, and languages.

Disabled aliases are silently skipped (fall through to next stage).

#### Stage 2: Canonical Name

**Trigger:** Always attempted if Stage 1 didn't match.

Parses the voice name using the canonical format: `"<voice> (<endpoint_name>, <model_name>)"`.

```go
func ParseCanonicalName(name string) (voice, endpointName, modelName string, ok bool)
```

Parsing uses `strings.LastIndex(name, " (")` to find the separator, then splits the parenthetical on `", "`. If the format matches, looks up the endpoint by `Name` and verifies the model exists in the endpoint's `Models` list. The endpoint MUST be enabled.

#### Stage 3: Fallback

**Trigger:** Always attempted if Stage 2 didn't match.

Uses the first enabled endpoint that has at least one enabled model and applies that endpoint's **default model** (see [Default Model](#default-model)). Endpoints with empty `Models` MUST be skipped. The voice name is passed as-is to the TTS endpoint (no transformation). Returns `IsAlias: false`.

**Error:** If no enabled endpoint with at least one enabled model exists, returns an error.

### ResolvedVoice

```go
type ResolvedVoice struct {
    Name         string      // Display name (canonical or alias)
    EndpointID   string      // Target endpoint UUID
    Model        string      // Target model ID
    Voice        string      // Actual voice ID for the TTS API
    Speed        *float64    // Resolved speed (may be nil)
    Instructions *string     // Resolved instructions (may be nil)
    Languages    StringSlice // Language codes
    IsAlias      bool        // true if resolved from VoiceAlias
}
```

### Requirements

- Stage 0 MUST only match `""` and the literal string `"default"`.
- Disabled aliases and endpoints MUST be silently skipped, not cause errors.
- Canonical name parsing MUST use `LastIndex` to handle voice names containing `(` characters.
- Fallback MUST pass the raw voice name to the endpoint without modification.
- Database errors during resolution MUST be propagated as errors.

### Scenarios

**GIVEN** a Wyoming client sends `voice: "default"` and one enabled endpoint has `DefaultVoice: "alloy"`,
**WHEN** the resolver runs,
**THEN** it MUST return the endpoint's default voice with `IsAlias: false`.

**GIVEN** a voice alias `"angry-nova"` exists and is enabled,
**WHEN** the resolver receives `voice: "angry-nova"`,
**THEN** it MUST return a `ResolvedVoice` with `IsAlias: true` and the alias's configured endpoint/model/voice.

**GIVEN** a voice name `"alloy (OpenAI, tts-1)"` matches a canonical format,
**WHEN** the resolver runs,
**THEN** it MUST look up endpoint `"OpenAI"` by name, verify `"tts-1"` is in its models, and return a resolved voice.

**GIVEN** a voice name `"custom-voice-xyz"` doesn't match any alias or canonical format,
**WHEN** the resolver runs,
**THEN** it MUST fall back to the first enabled endpoint and pass `"custom-voice-xyz"` as the voice.

## Default Model

Each endpoint MAY designate one of its enabled models as the **default model**, persisted in `Endpoint.DefaultModel`. The default model is used by:

- **Stage 0 (Default Voice)** — combined with `DefaultVoice` to form the resolved voice.
- **Stage 3 (Fallback)** — combined with the raw voice name when no other stage matches.
- **Wyoming `synthesize`** — when input parsing produces no `model` override and no alias is matched.

### Requirements

- `Endpoint.DefaultModel`, when non-empty, MUST be a member of `Endpoint.Models` (the enabled set).
- If `Endpoint.DefaultModel` is empty, the resolver MUST treat the first entry of `Endpoint.Models` as the default. This preserves behavior for endpoints created before the field existed.
- If `Endpoint.Models` is empty, the endpoint MUST NOT be eligible for Stage 0 or Stage 3 selection.
- Persisting an endpoint with a `DefaultModel` that is not in `Models` MUST be rejected by the API (HTTP 400).

### Scenarios

**GIVEN** an endpoint with `Models: ["tts-1", "gpt-4o-mini-tts"]` and `DefaultModel: "gpt-4o-mini-tts"`,
**WHEN** Stage 0 resolves a `"default"` voice request against this endpoint,
**THEN** the result MUST use `Model: "gpt-4o-mini-tts"`.

**GIVEN** an endpoint with `Models: ["tts-1"]` and `DefaultModel: ""` (legacy data),
**WHEN** Stage 3 fallback runs,
**THEN** the result MUST use `Model: "tts-1"` (first model used as implicit default).

**GIVEN** a request to update an endpoint with `Models: ["tts-1"]` and `DefaultModel: "tts-2"`,
**WHEN** the API processes the request,
**THEN** the API MUST respond `400 Bad Request` with code `invalid_default_model`.

## Enabled Models and Voices

Endpoints maintain two distinct sets:

1. **Enabled models** (`Endpoint.Models`) — the subset of models the operator has explicitly opted in to. The full list of models reachable from the upstream TTS API is **not** persisted; it is rediscovered on demand via `GET /api/v1/endpoints/{id}/models` (live probe — see [http-api spec](../http-api/index.md#endpoints-management)).
2. **Enabled voices** (per-endpoint, persisted in `endpoint_voices`) — the subset of voices the operator has explicitly opted in to. Discovery via `GET /api/v1/endpoints/{id}/remote-voices` returns the live upstream list; the persisted enabled set filters which voices the rest of the system surfaces.

### Requirements

- Newly discovered models MUST default to **disabled** (i.e. NOT added to `Endpoint.Models`). The operator opts each one in explicitly through the management UI.
- Newly discovered voices MUST default to **disabled** (i.e. inserted into `endpoint_voices` with `enabled = false`). The operator opts each one in explicitly.
- `Resolve` MUST only consider enabled models. A canonical name (Stage 2) referencing a model that is not in `Endpoint.Models` MUST fail to match and fall through to Stage 3.
- The Wyoming `describe` voice list and the system `GET /api/v1/voices` view MUST only include voices that are enabled for an enabled endpoint.
- Voice aliases (Stage 1) bypass the enabled-voices filter — an alias MAY reference any voice the upstream provider accepts, even if it is disabled in the management UI. This is intentional: aliases are an explicit opt-in by name.

### Scenarios

**GIVEN** an endpoint card whose voice list is empty (no `endpoint_voices` rows persisted yet),
**WHEN** the operator clicks "Refresh voices" and the upstream returns `["clone:abc", "qwen-female-1"]`,
**THEN** both voices MUST be inserted into `endpoint_voices` with `enabled = false` and rendered in the toggle list with the Switch off.

**GIVEN** the operator enables `"clone:abc"` and disables `"qwen-female-1"`,
**WHEN** a Wyoming client calls `describe` against an enabled endpoint with `DefaultModel: "qwen3-tts"`,
**THEN** the response MUST include the canonical name `"clone:abc (<endpoint>, qwen3-tts)"` and MUST NOT include any name for `"qwen-female-1"`.

**GIVEN** an alias `"crisp-clone"` exists targeting voice `"qwen-female-1"` (which is currently disabled),
**WHEN** a Wyoming client requests `voice: "crisp-clone"`,
**THEN** Stage 1 alias resolution MUST succeed and the request MUST be sent to the upstream with `voice: "qwen-female-1"`.

## Canonical Voice Naming

### Format

`"<voice_id> (<endpoint_name>, <model_name>)"`

Examples:
- `"alloy (OpenAI, tts-1)"`
- `"nova (OpenAI, gpt-4o-mini-tts)"`
- `"af_sky (Local Speaches, kokoro-v1)"`

### Generation

`voice.CanonicalName(voiceID, endpointName, modelName) → string`

Used by the Wyoming `InfoBuilder` when building the voice list for `describe` responses. For each enabled endpoint, each combination of `enabled voice × enabled model` (per [Enabled Models and Voices](#enabled-models-and-voices)) gets a canonical name. Disabled models, disabled voices, and disabled endpoints MUST be excluded from the canonical voice list.

### Parsing

`voice.ParseCanonicalName(name) → (voice, endpointName, modelName string, ok bool)`

Returns `ok: false` if the name doesn't match the canonical format.

## Input Text Parsing

### Overview

The `synthesize` event's `text` field MAY contain parameter overrides in addition to the spoken text. Three formats are supported, tried in order:

### Format 1: JSON

**Trigger:** Text starts with `{`.

```json
{"input": "Hello world", "voice": "nova", "model": "tts-1", "speed": 1.5, "instructions": "speak angrily"}
```

All fields are OPTIONAL. If JSON parsing fails, the text falls through to plain text (not tag format).

### Format 2: Tag Format

**Trigger:** Text starts with `[` (and JSON didn't match).

```
[voice: nova, speed: 1.5, instructions: angry voice, shouting] Hello world
[speed: 1.5] [voice: nova] Multiple chained tags also work
```

**Parsing rules:**
- Tags are `[key: value, key2: value2]` blocks.
- Known keys: `speed`, `voice`, `model`, `instructions`.
- Keys are case-insensitive.
- `instructions` greedily consumes all remaining comma-separated values until the next known key.
- Multiple chained tags are supported — parameters merge left-to-right.
- Text after the last tag is the spoken input.

### Format 3: Plain Text

**Trigger:** Text doesn't start with `{` or `[`.

The entire text is the spoken input. No overrides are extracted.

### ParsedInput

```go
type ParsedInput struct {
    Input        string
    Voice        string
    Model        string
    Speed        *float64
    Instructions *string
}
```

### Requirements

- Invalid JSON MUST fall through to plain text, not cause an error.
- Invalid float values for `speed` in tags MUST be silently ignored (field remains nil).
- Unclosed brackets MUST treat the entire text as input.
- Unknown keys in tags MUST be silently ignored.
- Empty text MUST return `ParsedInput{Input: ""}`.

## Parameter Merging

### Three-Level Priority

`MergeParams(input, alias, endpoint) → ParsedInput`

For each parameter (voice, model, speed, instructions), the first non-nil/non-empty value wins:

1. **Input overrides** (from `ParseInput`) — highest priority
2. **Alias defaults** (from `ResolvedVoice` when `IsAlias == true`)
3. **Endpoint defaults** (from `Endpoint.DefaultSpeed`, `DefaultInstructions`)

### Scenarios

**GIVEN** an endpoint has `DefaultSpeed: 1.0`, an alias has `Speed: 1.5`, and input contains `[speed: 2.0]`,
**WHEN** parameters are merged,
**THEN** the result MUST have `Speed: 2.0` (input wins).

**GIVEN** an alias has `Instructions: "speak slowly"` and the input has no instructions,
**WHEN** parameters are merged,
**THEN** the result MUST have `Instructions: "speak slowly"` (alias fills the gap).

## Data Models

### Endpoint

```go
type Endpoint struct {
    ID                    string
    Name                  string      // Unique display name
    BaseURL               string      // OpenAI-compatible API base URL
    APIKey                string      // Optional Bearer token
    Models                StringSlice // Enabled model IDs (subset of upstream-available models)
    DefaultModel          string      // Selected default model; MUST be in Models when non-empty
    DefaultVoice          string      // Voice for empty/"default" requests
    DefaultSpeed          *float64    // Optional speed override
    DefaultInstructions   *string     // Optional instructions
    DefaultResponseFormat string      // Audio format (default: "wav")
    Enabled               bool        // Active/inactive toggle
    CreatedAt             time.Time
    UpdatedAt             time.Time
}

// EndpointVoice tracks per-endpoint voice enablement.
// One row per (EndpointID, VoiceID). Inserted with Enabled=false on discovery.
type EndpointVoice struct {
    EndpointID string
    VoiceID    string  // Voice ID as returned by the upstream provider
    Name       string  // Human-readable label as returned by the upstream provider
    Enabled    bool    // false by default; operator opts in via the UI
    CreatedAt  time.Time
    UpdatedAt  time.Time
}
```

### VoiceAlias

```go
type VoiceAlias struct {
    ID           string
    Name         string      // Unique friendly name
    EndpointID   string      // FK to Endpoint.ID
    Model        string      // Target model ID
    Voice        string      // Target voice ID
    Speed        *float64    // Optional speed override
    Instructions *string     // Optional instructions
    Languages    StringSlice // Language codes (default: ["en"])
    Enabled      bool        // Active/inactive toggle
    CreatedAt    time.Time
    UpdatedAt    time.Time
}
```

### StringSlice

Custom type implementing `sql.Scanner` and `driver.Valuer` for JSON array serialization in TEXT database columns. Handles both `string` and `[]byte` scan sources.

## Files

| File | Purpose |
|------|---------|
| `internal/voice/voice.go` | Package declaration |
| `internal/voice/resolver.go` | Voice resolution priority chain |
| `internal/voice/parser.go` | Input text parsing (JSON, tags, plain) and parameter merging |
| `internal/model/model.go` | Data models: Endpoint, VoiceAlias, ResolvedVoice, StringSlice |

## Changelog

| Date | Description | Document |
|------|-------------|----------|
| 2026-04-19 | Initial living spec created from implementation audit | --- |
| 2026-05-04 | Added `DefaultModel` field, `EndpointVoice` model, and explicit enabled-models / enabled-voices semantics; clarified Stage 0 and Stage 3 use the default model | [0004-endpoint-models-toggle](../../changes/0004-endpoint-models-toggle.md), [0005-endpoint-voice-toggle](../../changes/0005-endpoint-voice-toggle.md) |
