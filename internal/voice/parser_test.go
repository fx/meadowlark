package voice

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// --- ParseInput: Plain text ---

func TestParseInput_PlainText(t *testing.T) {
	result := ParseInput("Hello World")
	assert.Equal(t, "Hello World", result.Input)
	assert.Empty(t, result.Voice)
	assert.Empty(t, result.Model)
	assert.Nil(t, result.Speed)
	assert.Nil(t, result.Instructions)
}

func TestParseInput_EmptyString(t *testing.T) {
	result := ParseInput("")
	assert.Equal(t, "", result.Input)
}

func TestParseInput_WhitespaceOnly(t *testing.T) {
	result := ParseInput("   ")
	assert.Equal(t, "", result.Input)
}

func TestParseInput_PlainTextWithSpecialChars(t *testing.T) {
	result := ParseInput("Hello! How are you? I'm fine, thanks.")
	assert.Equal(t, "Hello! How are you? I'm fine, thanks.", result.Input)
}

// --- ParseInput: JSON format ---

func TestParseInput_JSONFull(t *testing.T) {
	input := `{"input": "Hello World", "voice": "nova", "model": "gpt-4o-mini-tts", "speed": 1.5, "instructions": "speak angrily"}`
	result := ParseInput(input)

	assert.Equal(t, "Hello World", result.Input)
	assert.Equal(t, "nova", result.Voice)
	assert.Equal(t, "gpt-4o-mini-tts", result.Model)
	assert.Equal(t, ptrFloat(1.5), result.Speed)
	assert.Equal(t, ptrStr("speak angrily"), result.Instructions)
}

func TestParseInput_JSONMinimal(t *testing.T) {
	input := `{"input": "Hello"}`
	result := ParseInput(input)

	assert.Equal(t, "Hello", result.Input)
	assert.Empty(t, result.Voice)
	assert.Nil(t, result.Speed)
}

func TestParseInput_JSONWithOnlyVoice(t *testing.T) {
	input := `{"input": "Hello", "voice": "alloy"}`
	result := ParseInput(input)

	assert.Equal(t, "Hello", result.Input)
	assert.Equal(t, "alloy", result.Voice)
}

func TestParseInput_JSONInvalid(t *testing.T) {
	// Starts with { but isn't valid JSON -- treated as plain text.
	input := `{not valid json`
	result := ParseInput(input)

	assert.Equal(t, "{not valid json", result.Input)
	assert.Empty(t, result.Voice)
}

func TestParseInput_JSONEmptyObject(t *testing.T) {
	result := ParseInput(`{}`)
	assert.Equal(t, "", result.Input)
}

func TestParseInput_JSONWithExtraFields(t *testing.T) {
	input := `{"input": "Hello", "unknown_field": "ignored", "speed": 2.0}`
	result := ParseInput(input)

	assert.Equal(t, "Hello", result.Input)
	assert.Equal(t, ptrFloat(2.0), result.Speed)
}

// --- ParseInput: JSON "message" key ---

func TestParseInput_JSONMessageKey(t *testing.T) {
	input := `{"message": "Hello World", "voice": "nova"}`
	result := ParseInput(input)

	assert.Equal(t, "Hello World", result.Input)
	assert.Equal(t, "nova", result.Voice)
}

func TestParseInput_JSONInputOverridesMessage(t *testing.T) {
	input := `{"input": "from input", "message": "from message"}`
	result := ParseInput(input)

	assert.Equal(t, "from input", result.Input)
}

func TestParseInput_JSONMessageWithAllFields(t *testing.T) {
	input := `{"voice": "sohee", "instructions": "bla bla bla", "message": "testing"}`
	result := ParseInput(input)

	assert.Equal(t, "testing", result.Input)
	assert.Equal(t, "sohee", result.Voice)
	assert.Equal(t, ptrStr("bla bla bla"), result.Instructions)
}

func TestParseInput_JSONEmptyInputFallsBackToMessage(t *testing.T) {
	input := `{"input": "", "message": "fallback"}`
	result := ParseInput(input)
	assert.Equal(t, "fallback", result.Input)
}

// --- ParseInput: Tag format ---

func TestParseInput_SingleTag(t *testing.T) {
	result := ParseInput("[speed: 1.5] Hello World")

	assert.Equal(t, "Hello World", result.Input)
	assert.Equal(t, ptrFloat(1.5), result.Speed)
}

func TestParseInput_MultipleTagsSameBlock(t *testing.T) {
	result := ParseInput("[speed: 1.5, voice: nova] Hello World")

	assert.Equal(t, "Hello World", result.Input)
	assert.Equal(t, ptrFloat(1.5), result.Speed)
	assert.Equal(t, "nova", result.Voice)
}

func TestParseInput_InstructionsWithCommas(t *testing.T) {
	result := ParseInput("[instructions: angry voice, shouting, speed: 1.5] Hello World")

	assert.Equal(t, "Hello World", result.Input)
	assert.Equal(t, ptrStr("angry voice, shouting"), result.Instructions)
	assert.Equal(t, ptrFloat(1.5), result.Speed)
}

func TestParseInput_ChainedTags(t *testing.T) {
	result := ParseInput("[speed: 1.5] [voice: nova] Hello World")

	assert.Equal(t, "Hello World", result.Input)
	assert.Equal(t, ptrFloat(1.5), result.Speed)
	assert.Equal(t, "nova", result.Voice)
}

func TestParseInput_TagAllKeys(t *testing.T) {
	result := ParseInput("[voice: nova, model: gpt-4o-mini-tts, speed: 1.2, instructions: whisper softly] Speak this")

	assert.Equal(t, "Speak this", result.Input)
	assert.Equal(t, "nova", result.Voice)
	assert.Equal(t, "gpt-4o-mini-tts", result.Model)
	assert.Equal(t, ptrFloat(1.2), result.Speed)
	assert.Equal(t, ptrStr("whisper softly"), result.Instructions)
}

func TestParseInput_TagNoClosingBracket(t *testing.T) {
	// Malformed: no closing bracket. Entire text treated as input.
	result := ParseInput("[speed: 1.5 Hello World")

	assert.Equal(t, "[speed: 1.5 Hello World", result.Input)
	assert.Nil(t, result.Speed)
}

func TestParseInput_TagEmptyBrackets(t *testing.T) {
	result := ParseInput("[] Hello World")
	assert.Equal(t, "Hello World", result.Input)
}

func TestParseInput_TagNoText(t *testing.T) {
	result := ParseInput("[speed: 1.5]")
	assert.Equal(t, "", result.Input)
	assert.Equal(t, ptrFloat(1.5), result.Speed)
}

func TestParseInput_TagInvalidSpeed(t *testing.T) {
	result := ParseInput("[speed: fast] Hello")
	assert.Equal(t, "Hello", result.Input)
	assert.Nil(t, result.Speed) // "fast" can't parse as float
}

func TestParseInput_TagUnknownKey(t *testing.T) {
	result := ParseInput("[unknown: value] Hello")
	assert.Equal(t, "Hello", result.Input)
	assert.Empty(t, result.Voice)
}

func TestParseInput_TagNestedBrackets(t *testing.T) {
	// The outer brackets are the tag; inner text with brackets in the body.
	result := ParseInput("[voice: nova] Hello [world]")
	assert.Equal(t, "Hello [world]", result.Input)
	assert.Equal(t, "nova", result.Voice)
}

func TestParseInput_TagInstructionsAtEnd(t *testing.T) {
	// Instructions is the last key, so it consumes everything after it.
	result := ParseInput("[instructions: be calm, gentle, and slow] Speak this")
	assert.Equal(t, "Speak this", result.Input)
	assert.Equal(t, ptrStr("be calm, gentle, and slow"), result.Instructions)
}

func TestParseInput_TagCaseInsensitiveKeys(t *testing.T) {
	result := ParseInput("[Speed: 1.5, Voice: nova] Hello")
	assert.Equal(t, ptrFloat(1.5), result.Speed)
	assert.Equal(t, "nova", result.Voice)
}

func TestParseInput_TagWithLeadingWhitespace(t *testing.T) {
	result := ParseInput("  [speed: 1.5] Hello")
	// After TrimSpace, starts with [.
	assert.Equal(t, "Hello", result.Input)
	assert.Equal(t, ptrFloat(1.5), result.Speed)
}

// --- ParseInput: Edge cases ---

func TestParseInput_BracketInMiddle(t *testing.T) {
	// Bracket not at start -- treated as plain text.
	result := ParseInput("Hello [speed: 1.5] World")
	assert.Equal(t, "Hello [speed: 1.5] World", result.Input)
}

func TestParseInput_BraceInMiddle(t *testing.T) {
	result := ParseInput("Hello {json} World")
	assert.Equal(t, "Hello {json} World", result.Input)
}

// --- MergeParams tests ---

func TestMergeParams_InputOverridesAll(t *testing.T) {
	input := ParsedInput{
		Input:        "hello",
		Voice:        "nova",
		Model:        "model-a",
		Speed:        ptrFloat(2.0),
		Instructions: ptrStr("shout"),
	}
	alias := &ParsedInput{
		Voice:        "alloy",
		Model:        "model-b",
		Speed:        ptrFloat(1.0),
		Instructions: ptrStr("whisper"),
	}
	endpoint := &ParsedInput{
		Voice:        "echo",
		Model:        "model-c",
		Speed:        ptrFloat(0.5),
		Instructions: ptrStr("default"),
	}

	result := MergeParams(input, alias, endpoint)
	assert.Equal(t, "hello", result.Input)
	assert.Equal(t, "nova", result.Voice)
	assert.Equal(t, "model-a", result.Model)
	assert.Equal(t, ptrFloat(2.0), result.Speed)
	assert.Equal(t, ptrStr("shout"), result.Instructions)
}

func TestMergeParams_AliasFallback(t *testing.T) {
	input := ParsedInput{Input: "hello"}
	alias := &ParsedInput{
		Voice:        "alloy",
		Speed:        ptrFloat(1.0),
		Instructions: ptrStr("whisper"),
	}

	result := MergeParams(input, alias, nil)
	assert.Equal(t, "alloy", result.Voice)
	assert.Equal(t, ptrFloat(1.0), result.Speed)
	assert.Equal(t, ptrStr("whisper"), result.Instructions)
}

func TestMergeParams_EndpointFallback(t *testing.T) {
	input := ParsedInput{Input: "hello"}
	endpoint := &ParsedInput{
		Voice: "echo",
		Speed: ptrFloat(0.5),
	}

	result := MergeParams(input, nil, endpoint)
	assert.Equal(t, "echo", result.Voice)
	assert.Equal(t, ptrFloat(0.5), result.Speed)
}

func TestMergeParams_NilAliasAndEndpoint(t *testing.T) {
	input := ParsedInput{Input: "hello", Voice: "nova"}
	result := MergeParams(input, nil, nil)
	assert.Equal(t, "hello", result.Input)
	assert.Equal(t, "nova", result.Voice)
	assert.Nil(t, result.Speed)
}

func TestMergeParams_PartialOverrides(t *testing.T) {
	input := ParsedInput{Input: "hello", Speed: ptrFloat(2.0)}
	alias := &ParsedInput{Voice: "nova", Instructions: ptrStr("calm")}
	endpoint := &ParsedInput{Model: "tts-1"}

	result := MergeParams(input, alias, endpoint)
	assert.Equal(t, ptrFloat(2.0), result.Speed)        // from input
	assert.Equal(t, "nova", result.Voice)                // from alias
	assert.Equal(t, ptrStr("calm"), result.Instructions) // from alias
	assert.Equal(t, "tts-1", result.Model)               // from endpoint
}

// --- findClosingBracket tests ---

func TestFindClosingBracket(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"[hello]", 6},
		{"[a[b]c]", 6},
		{"[a[b[c]d]e]", 10},
		{"[no close", -1},
		{"[]", 1},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, findClosingBracket(tt.input))
		})
	}
}

// --- isKnownKeyValue tests ---

func TestIsKnownKeyValue(t *testing.T) {
	assert.True(t, isKnownKeyValue("speed: 1.5"))
	assert.True(t, isKnownKeyValue("voice: nova"))
	assert.True(t, isKnownKeyValue("model: tts-1"))
	assert.True(t, isKnownKeyValue("instructions: hello"))
	assert.False(t, isKnownKeyValue("unknown: value"))
	assert.False(t, isKnownKeyValue("no colon"))
	assert.True(t, isKnownKeyValue(" Speed : 1.5")) // case-insensitive with spaces
}
