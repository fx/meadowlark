package model

import (
	"database/sql/driver"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStringSlice_Scan(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		want    StringSlice
		wantErr bool
	}{
		{name: "nil input", input: nil, want: nil},
		{name: "string input", input: `["a","b","c"]`, want: StringSlice{"a", "b", "c"}},
		{name: "byte input", input: []byte(`["x","y"]`), want: StringSlice{"x", "y"}},
		{name: "empty array string", input: `[]`, want: StringSlice{}},
		{name: "invalid JSON", input: `not json`, wantErr: true},
		{name: "wrong type int", input: 123, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s StringSlice
			err := s.Scan(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, s)
		})
	}
}

func TestStringSlice_Value(t *testing.T) {
	tests := []struct {
		name string
		s    StringSlice
		want driver.Value
	}{
		{name: "nil slice", s: nil, want: "[]"},
		{name: "empty slice", s: StringSlice{}, want: "[]"},
		{name: "populated slice", s: StringSlice{"tts-1", "gpt-4o-mini-tts"}, want: `["tts-1","gpt-4o-mini-tts"]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.s.Value()
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEndpoint_JSONRoundTrip(t *testing.T) {
	speed := 1.5
	instructions := "speak clearly"
	now := time.Now().Truncate(time.Second)
	ep := Endpoint{
		ID: "ep-001", Name: "OpenAI", BaseURL: "https://api.openai.com/v1",
		APIKey: "sk-test", Models: StringSlice{"tts-1", "gpt-4o-mini-tts"},
		DefaultSpeed: &speed, DefaultInstructions: &instructions,
		DefaultResponseFormat: "wav", Enabled: true, CreatedAt: now, UpdatedAt: now,
	}
	data, err := json.Marshal(ep)
	require.NoError(t, err)
	var got Endpoint
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, ep.ID, got.ID)
	assert.Equal(t, ep.Name, got.Name)
	assert.Equal(t, ep.Models, got.Models)
	assert.Equal(t, *ep.DefaultSpeed, *got.DefaultSpeed)
	assert.Equal(t, *ep.DefaultInstructions, *got.DefaultInstructions)
}

func TestEndpoint_JSONOmitsOptionalFields(t *testing.T) {
	ep := Endpoint{ID: "ep-002", Name: "Local", BaseURL: "http://localhost:8080",
		Models: StringSlice{"kokoro-v1"}, DefaultResponseFormat: "wav", Enabled: true}
	data, err := json.Marshal(ep)
	require.NoError(t, err)
	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))
	assert.NotContains(t, raw, "default_speed")
	assert.NotContains(t, raw, "default_instructions")
	assert.NotContains(t, raw, "api_key")
}

func TestVoiceAlias_JSONRoundTrip(t *testing.T) {
	speed := 0.8
	instructions := "whisper"
	alias := VoiceAlias{
		ID: "va-001", Name: "my-whisper", EndpointID: "ep-001",
		Model: "tts-1", Voice: "nova", Speed: &speed, Instructions: &instructions,
		Languages: StringSlice{"en", "es"}, Enabled: true,
	}
	data, err := json.Marshal(alias)
	require.NoError(t, err)
	var got VoiceAlias
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, alias.ID, got.ID)
	assert.Equal(t, alias.Languages, got.Languages)
	assert.Equal(t, *alias.Speed, *got.Speed)
}

func TestVoiceAlias_JSONOmitsOptionalFields(t *testing.T) {
	alias := VoiceAlias{ID: "va-002", Name: "simple", EndpointID: "ep-001",
		Model: "tts-1", Voice: "alloy", Languages: StringSlice{"en"}, Enabled: true}
	data, err := json.Marshal(alias)
	require.NoError(t, err)
	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))
	assert.NotContains(t, raw, "speed")
	assert.NotContains(t, raw, "instructions")
}

func TestResolvedVoice_JSON(t *testing.T) {
	speed := 1.2
	rv := ResolvedVoice{Name: "alloy (OpenAI, tts-1)", EndpointID: "ep-001",
		Model: "tts-1", Voice: "alloy", Speed: &speed, Languages: StringSlice{"en"}, IsAlias: false}
	data, err := json.Marshal(rv)
	require.NoError(t, err)
	var got ResolvedVoice
	require.NoError(t, json.Unmarshal(data, &got))
	assert.Equal(t, rv.Name, got.Name)
	assert.Equal(t, rv.IsAlias, got.IsAlias)
}

func TestStringSlice_ScanAndValue_RoundTrip(t *testing.T) {
	original := StringSlice{"tts-1", "gpt-4o-mini-tts", "kokoro-v1"}
	val, err := original.Value()
	require.NoError(t, err)
	var restored StringSlice
	require.NoError(t, restored.Scan(val))
	assert.Equal(t, original, restored)
}
