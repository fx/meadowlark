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

Scans all endpoints (ordered by DB query) for the first enabled endpoint with a non-empty `DefaultVoice`. Returns a `ResolvedVoice` with that endpoint's ID, first model, and default voice. Includes the endpoint's `DefaultSpeed` and `DefaultInstructions`.

**Error:** If no enabled endpoint has a `DefaultVoice` configured, returns `"voice: no voice specified and no default voice configured"`.

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

Uses the first enabled endpoint's first model. The voice name is passed as-is to the TTS endpoint (no transformation). Returns `IsAlias: false`.

**Error:** If no enabled endpoints exist, returns an error.

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

## Canonical Voice Naming

### Format

`"<voice_id> (<endpoint_name>, <model_name>)"`

Examples:
- `"alloy (OpenAI, tts-1)"`
- `"nova (OpenAI, gpt-4o-mini-tts)"`
- `"af_sky (Local Speaches, kokoro-v1)"`

### Generation

`voice.CanonicalName(voiceID, endpointName, modelName) → string`

Used by the Wyoming `InfoBuilder` when building the voice list for `describe` responses. For each enabled endpoint, each `voice x model` combination gets a canonical name.

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
    Models                StringSlice // Available model IDs
    DefaultVoice          string      // Voice for empty/"default" requests
    DefaultSpeed          *float64    // Optional speed override
    DefaultInstructions   *string     // Optional instructions
    DefaultResponseFormat string      // Audio format (default: "wav")
    Enabled               bool        // Active/inactive toggle
    CreatedAt             time.Time
    UpdatedAt             time.Time
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
