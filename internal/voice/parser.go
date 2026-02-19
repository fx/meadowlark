package voice

import (
	"encoding/json"
	"strconv"
	"strings"
)

// ParsedInput holds the result of parsing synthesis input text.
type ParsedInput struct {
	Input        string   `json:"input"`
	Voice        string   `json:"voice,omitempty"`
	Model        string   `json:"model,omitempty"`
	Speed        *float64 `json:"speed,omitempty"`
	Instructions *string  `json:"instructions,omitempty"`
}

// ParseInput parses the text field from a synthesize event.
//
// Parsing order:
//  1. If text starts with "{", try JSON parsing.
//  2. If text starts with "[", try tag format parsing.
//  3. Otherwise, treat entire text as plain input.
func ParseInput(text string) ParsedInput {
	text = strings.TrimSpace(text)
	if text == "" {
		return ParsedInput{Input: ""}
	}

	if strings.HasPrefix(text, "{") {
		if result, ok := parseJSON(text); ok {
			return result
		}
		// JSON parsing failed; fall through to plain text.
		return ParsedInput{Input: text}
	}

	if strings.HasPrefix(text, "[") {
		return parseTags(text)
	}

	return ParsedInput{Input: text}
}

// parseJSON attempts to parse the entire text as a JSON object.
func parseJSON(text string) (ParsedInput, bool) {
	var raw struct {
		Input        string   `json:"input"`
		Voice        string   `json:"voice"`
		Model        string   `json:"model"`
		Speed        *float64 `json:"speed"`
		Instructions *string  `json:"instructions"`
	}
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		return ParsedInput{}, false
	}
	return ParsedInput{
		Input:        raw.Input,
		Voice:        raw.Voice,
		Model:        raw.Model,
		Speed:        raw.Speed,
		Instructions: raw.Instructions,
	}, true
}

// parseTags parses bracket-based tag format: [key: value, key2: value2] Text
// Multiple tags can be chained: [key1: value1] [key2: value2] Text
func parseTags(text string) ParsedInput {
	result := ParsedInput{}
	remaining := text

	for strings.HasPrefix(remaining, "[") {
		// Find the matching closing bracket.
		closeIdx := findClosingBracket(remaining)
		if closeIdx < 0 {
			// No closing bracket; treat entire text as input.
			result.Input = text
			return result
		}

		tagContent := remaining[1:closeIdx]
		remaining = remaining[closeIdx+1:]

		// Parse key-value pairs from the tag content.
		parseTagContent(tagContent, &result)

		// Trim leading whitespace between tags or before the text.
		remaining = strings.TrimLeft(remaining, " ")
	}

	result.Input = remaining
	return result
}

// findClosingBracket finds the index of the first "]" that isn't nested.
// Returns -1 if no closing bracket is found.
func findClosingBracket(s string) int {
	depth := 0
	for i, ch := range s {
		switch ch {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// parseTagContent parses comma-separated key:value pairs from tag content.
// Handles the special case where "instructions" value may contain commas.
//
// Known keys: instructions, speed, voice, model.
// The instructions key greedily consumes everything until another known key is found.
func parseTagContent(content string, result *ParsedInput) {
	// Split on commas, then reassemble based on known keys.
	parts := strings.Split(content, ",")

	i := 0
	for i < len(parts) {
		part := strings.TrimSpace(parts[i])
		colonIdx := strings.Index(part, ":")
		if colonIdx < 0 {
			i++
			continue
		}

		key := strings.TrimSpace(part[:colonIdx])
		value := strings.TrimSpace(part[colonIdx+1:])

		switch strings.ToLower(key) {
		case "instructions":
			// Instructions may contain commas; consume subsequent parts
			// until we find another known key or run out of parts.
			j := i + 1
			for j < len(parts) {
				nextPart := strings.TrimSpace(parts[j])
				if isKnownKeyValue(nextPart) {
					break
				}
				value += ", " + nextPart
				j++
			}
			value = strings.TrimSpace(value)
			result.Instructions = &value
			i = j
		case "speed":
			if f, err := strconv.ParseFloat(value, 64); err == nil {
				result.Speed = &f
			}
			i++
		case "voice":
			result.Voice = value
			i++
		case "model":
			result.Model = value
			i++
		default:
			i++
		}
	}
}

// isKnownKeyValue checks if a string looks like "known_key: value".
func isKnownKeyValue(s string) bool {
	colonIdx := strings.Index(s, ":")
	if colonIdx < 0 {
		return false
	}
	key := strings.TrimSpace(strings.ToLower(s[:colonIdx]))
	return key == "speed" || key == "voice" || key == "model" || key == "instructions"
}

// MergeParams merges input overrides with alias/endpoint defaults.
// Priority: input overrides > alias defaults > endpoint defaults.
func MergeParams(input ParsedInput, alias *ParsedInput, endpoint *ParsedInput) ParsedInput {
	result := ParsedInput{
		Input: input.Input,
	}

	// Voice: input > alias > endpoint
	result.Voice = coalesceString(input.Voice, optStr(alias, func(p *ParsedInput) string { return p.Voice }), optStr(endpoint, func(p *ParsedInput) string { return p.Voice }))

	// Model: input > alias > endpoint
	result.Model = coalesceString(input.Model, optStr(alias, func(p *ParsedInput) string { return p.Model }), optStr(endpoint, func(p *ParsedInput) string { return p.Model }))

	// Speed: input > alias > endpoint
	result.Speed = coalesceFloat(input.Speed, optFloat(alias, func(p *ParsedInput) *float64 { return p.Speed }), optFloat(endpoint, func(p *ParsedInput) *float64 { return p.Speed }))

	// Instructions: input > alias > endpoint
	result.Instructions = coalesceStrPtr(input.Instructions, optStrPtr(alias, func(p *ParsedInput) *string { return p.Instructions }), optStrPtr(endpoint, func(p *ParsedInput) *string { return p.Instructions }))

	return result
}

func coalesceString(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func coalesceFloat(values ...*float64) *float64 {
	for _, v := range values {
		if v != nil {
			return v
		}
	}
	return nil
}

func coalesceStrPtr(values ...*string) *string {
	for _, v := range values {
		if v != nil {
			return v
		}
	}
	return nil
}

func optStr(p *ParsedInput, fn func(*ParsedInput) string) string {
	if p == nil {
		return ""
	}
	return fn(p)
}

func optFloat(p *ParsedInput, fn func(*ParsedInput) *float64) *float64 {
	if p == nil {
		return nil
	}
	return fn(p)
}

func optStrPtr(p *ParsedInput, fn func(*ParsedInput) *string) *string {
	if p == nil {
		return nil
	}
	return fn(p)
}
