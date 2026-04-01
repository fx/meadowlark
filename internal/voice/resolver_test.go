package voice

import (
	"context"
	"errors"
	"testing"

	"github.com/fx/meadowlark/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- test doubles ---

type mockEndpointLister struct {
	endpoints []model.Endpoint
	err       error
}

func (m *mockEndpointLister) ListEndpoints(_ context.Context) ([]model.Endpoint, error) {
	return m.endpoints, m.err
}

type mockAliasLister struct {
	aliases []model.VoiceAlias
	err     error
}

func (m *mockAliasLister) ListVoiceAliases(_ context.Context) ([]model.VoiceAlias, error) {
	return m.aliases, m.err
}

// --- helpers ---

func ptrFloat(f float64) *float64 { return &f }
func ptrStr(s string) *string     { return &s }

func defaultEndpoints() []model.Endpoint {
	return []model.Endpoint{
		{
			ID:      "ep-1",
			Name:    "OpenAI",
			BaseURL: "https://api.openai.com/v1",
			Models:  model.StringSlice{"tts-1", "gpt-4o-mini-tts"},
			Enabled: true,
		},
		{
			ID:      "ep-2",
			Name:    "Local Speaches",
			BaseURL: "http://localhost:8000",
			Models:  model.StringSlice{"kokoro-v1"},
			Enabled: true,
			DefaultSpeed: ptrFloat(1.2),
		},
	}
}

func defaultAliases() []model.VoiceAlias {
	return []model.VoiceAlias{
		{
			ID:           "alias-1",
			Name:         "angry-nova",
			EndpointID:   "ep-1",
			Model:        "gpt-4o-mini-tts",
			Voice:        "nova",
			Speed:        ptrFloat(1.5),
			Instructions: ptrStr("speak angrily"),
			Languages:    model.StringSlice{"en"},
			Enabled:      true,
		},
		{
			ID:         "alias-2",
			Name:       "disabled-voice",
			EndpointID: "ep-1",
			Model:      "tts-1",
			Voice:      "alloy",
			Enabled:    false,
		},
	}
}

// --- Resolve tests ---

func TestResolve_AliasPriority(t *testing.T) {
	r := NewResolver(
		&mockEndpointLister{endpoints: defaultEndpoints()},
		&mockAliasLister{aliases: defaultAliases()},
	)

	resolved, err := r.Resolve(context.Background(), "angry-nova")
	require.NoError(t, err)
	require.NotNil(t, resolved)

	assert.Equal(t, "angry-nova", resolved.Name)
	assert.Equal(t, "ep-1", resolved.EndpointID)
	assert.Equal(t, "gpt-4o-mini-tts", resolved.Model)
	assert.Equal(t, "nova", resolved.Voice)
	assert.Equal(t, ptrFloat(1.5), resolved.Speed)
	assert.Equal(t, ptrStr("speak angrily"), resolved.Instructions)
	assert.True(t, resolved.IsAlias)
	assert.Equal(t, model.StringSlice{"en"}, resolved.Languages)
}

func TestResolve_DisabledAliasSkipped(t *testing.T) {
	r := NewResolver(
		&mockEndpointLister{endpoints: defaultEndpoints()},
		&mockAliasLister{aliases: defaultAliases()},
	)

	// "disabled-voice" exists but is disabled; should fall through to fallback.
	resolved, err := r.Resolve(context.Background(), "disabled-voice")
	require.NoError(t, err)
	require.NotNil(t, resolved)

	assert.False(t, resolved.IsAlias)
	assert.Equal(t, "ep-1", resolved.EndpointID) // fallback to first enabled
}

func TestResolve_CanonicalName(t *testing.T) {
	r := NewResolver(
		&mockEndpointLister{endpoints: defaultEndpoints()},
		&mockAliasLister{aliases: nil},
	)

	resolved, err := r.Resolve(context.Background(), "alloy (OpenAI, tts-1)")
	require.NoError(t, err)
	require.NotNil(t, resolved)

	assert.Equal(t, "alloy (OpenAI, tts-1)", resolved.Name)
	assert.Equal(t, "ep-1", resolved.EndpointID)
	assert.Equal(t, "tts-1", resolved.Model)
	assert.Equal(t, "alloy", resolved.Voice)
	assert.False(t, resolved.IsAlias)
}

func TestResolve_CanonicalWithEndpointDefaults(t *testing.T) {
	r := NewResolver(
		&mockEndpointLister{endpoints: defaultEndpoints()},
		&mockAliasLister{aliases: nil},
	)

	resolved, err := r.Resolve(context.Background(), "af_sky (Local Speaches, kokoro-v1)")
	require.NoError(t, err)
	require.NotNil(t, resolved)

	assert.Equal(t, "ep-2", resolved.EndpointID)
	assert.Equal(t, ptrFloat(1.2), resolved.Speed) // endpoint default
	assert.Equal(t, "af_sky", resolved.Voice)
}

func TestResolve_CanonicalDisabledEndpointSkipped(t *testing.T) {
	eps := []model.Endpoint{
		{ID: "ep-1", Name: "Disabled", Models: model.StringSlice{"tts-1"}, Enabled: false},
		{ID: "ep-2", Name: "Enabled", Models: model.StringSlice{"tts-1"}, Enabled: true},
	}
	r := NewResolver(
		&mockEndpointLister{endpoints: eps},
		&mockAliasLister{aliases: nil},
	)

	// Try canonical with disabled endpoint name -- should not resolve.
	resolved, err := r.Resolve(context.Background(), "alloy (Disabled, tts-1)")
	require.NoError(t, err)
	// Falls back to first enabled endpoint.
	assert.Equal(t, "ep-2", resolved.EndpointID)
}

func TestResolve_CanonicalModelNotFound(t *testing.T) {
	r := NewResolver(
		&mockEndpointLister{endpoints: defaultEndpoints()},
		&mockAliasLister{aliases: nil},
	)

	// Valid format but model doesn't exist on the endpoint.
	resolved, err := r.Resolve(context.Background(), "alloy (OpenAI, nonexistent-model)")
	require.NoError(t, err)
	// Falls back to first enabled endpoint.
	assert.Equal(t, "ep-1", resolved.EndpointID)
	assert.Equal(t, "tts-1", resolved.Model)
}

func TestResolve_Fallback(t *testing.T) {
	r := NewResolver(
		&mockEndpointLister{endpoints: defaultEndpoints()},
		&mockAliasLister{aliases: nil},
	)

	resolved, err := r.Resolve(context.Background(), "some-unknown-voice")
	require.NoError(t, err)
	require.NotNil(t, resolved)

	assert.Equal(t, "some-unknown-voice", resolved.Name)
	assert.Equal(t, "some-unknown-voice", resolved.Voice)
	assert.Equal(t, "ep-1", resolved.EndpointID)
	assert.Equal(t, "tts-1", resolved.Model) // first model on first endpoint
	assert.False(t, resolved.IsAlias)
}

func TestResolve_FallbackSkipsDisabledEndpoints(t *testing.T) {
	eps := []model.Endpoint{
		{ID: "ep-1", Name: "Disabled", Models: model.StringSlice{"tts-1"}, Enabled: false},
		{ID: "ep-2", Name: "Enabled", Models: model.StringSlice{"kokoro"}, Enabled: true},
	}
	r := NewResolver(
		&mockEndpointLister{endpoints: eps},
		&mockAliasLister{aliases: nil},
	)

	resolved, err := r.Resolve(context.Background(), "voice")
	require.NoError(t, err)
	assert.Equal(t, "ep-2", resolved.EndpointID)
	assert.Equal(t, "kokoro", resolved.Model)
}

func TestResolve_FallbackSkipsEndpointsWithNoModels(t *testing.T) {
	eps := []model.Endpoint{
		{ID: "ep-1", Name: "NoModels", Models: nil, Enabled: true},
		{ID: "ep-2", Name: "HasModels", Models: model.StringSlice{"tts-1"}, Enabled: true},
	}
	r := NewResolver(
		&mockEndpointLister{endpoints: eps},
		&mockAliasLister{aliases: nil},
	)

	resolved, err := r.Resolve(context.Background(), "voice")
	require.NoError(t, err)
	assert.Equal(t, "ep-2", resolved.EndpointID)
}

func TestResolve_NoEndpoints(t *testing.T) {
	r := NewResolver(
		&mockEndpointLister{endpoints: nil},
		&mockAliasLister{aliases: nil},
	)

	_, err := r.Resolve(context.Background(), "voice")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no enabled endpoints")
}

func TestResolve_AliasListError(t *testing.T) {
	r := NewResolver(
		&mockEndpointLister{endpoints: defaultEndpoints()},
		&mockAliasLister{err: errors.New("db error")},
	)

	_, err := r.Resolve(context.Background(), "anything")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db error")
}

func TestResolve_EndpointListError(t *testing.T) {
	r := NewResolver(
		&mockEndpointLister{err: errors.New("connection lost")},
		&mockAliasLister{aliases: nil},
	)

	_, err := r.Resolve(context.Background(), "alloy (OpenAI, tts-1)")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection lost")
}

func TestResolve_EndpointListErrorOnFallback(t *testing.T) {
	r := NewResolver(
		&mockEndpointLister{err: errors.New("connection lost")},
		&mockAliasLister{aliases: nil},
	)

	_, err := r.Resolve(context.Background(), "plain-name")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection lost")
}

// --- ParseCanonicalName tests ---

func TestParseCanonicalName_Valid(t *testing.T) {
	tests := []struct {
		input    string
		voice    string
		endpoint string
		model    string
	}{
		{"alloy (OpenAI, tts-1)", "alloy", "OpenAI", "tts-1"},
		{"nova (OpenAI, gpt-4o-mini-tts)", "nova", "OpenAI", "gpt-4o-mini-tts"},
		{"af_sky (Local Speaches, kokoro-v1)", "af_sky", "Local Speaches", "kokoro-v1"},
		{"voice with spaces (Endpoint Name, model-name)", "voice with spaces", "Endpoint Name", "model-name"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			v, e, m, ok := ParseCanonicalName(tt.input)
			require.True(t, ok)
			assert.Equal(t, tt.voice, v)
			assert.Equal(t, tt.endpoint, e)
			assert.Equal(t, tt.model, m)
		})
	}
}

func TestParseCanonicalName_Invalid(t *testing.T) {
	tests := []string{
		"simple-voice",
		"no-parens",
		"missing (comma)",
		"(no voice, model)",
		" (, model)",
		"voice (endpoint, )",
		"voice (, )",
		"voice (endpoint, model",  // no closing paren
		"voice endpoint, model)",  // no opening paren
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			_, _, _, ok := ParseCanonicalName(input)
			assert.False(t, ok)
		})
	}
}

// --- CanonicalName tests ---

func TestCanonicalName(t *testing.T) {
	assert.Equal(t, "alloy (OpenAI, tts-1)", CanonicalName("alloy", "OpenAI", "tts-1"))
	assert.Equal(t, "af_sky (Local Speaches, kokoro-v1)", CanonicalName("af_sky", "Local Speaches", "kokoro-v1"))
}

// --- BuildCanonicalList tests ---

func TestBuildCanonicalList(t *testing.T) {
	endpoints := defaultEndpoints()
	voices := map[string][]string{
		"ep-1": {"alloy", "nova", "shimmer"},
		"ep-2": {"af_sky", "af_bella"},
	}

	list := BuildCanonicalList(endpoints, voices)

	// ep-1: 2 models x 3 voices = 6
	// ep-2: 1 model x 2 voices = 2
	assert.Len(t, list, 8)

	// Verify first entry.
	assert.Equal(t, "alloy (OpenAI, tts-1)", list[0].Name)
	assert.Equal(t, "ep-1", list[0].EndpointID)
	assert.Equal(t, "tts-1", list[0].Model)
	assert.Equal(t, "alloy", list[0].Voice)
	assert.False(t, list[0].IsAlias)

	// Verify last entry from ep-2 has endpoint defaults.
	last := list[len(list)-1]
	assert.Equal(t, "af_bella (Local Speaches, kokoro-v1)", last.Name)
	assert.Equal(t, ptrFloat(1.2), last.Speed)
}

func TestBuildCanonicalList_DisabledEndpointsSkipped(t *testing.T) {
	endpoints := []model.Endpoint{
		{ID: "ep-1", Name: "Disabled", Models: model.StringSlice{"m1"}, Enabled: false},
		{ID: "ep-2", Name: "Enabled", Models: model.StringSlice{"m1"}, Enabled: true},
	}
	voices := map[string][]string{
		"ep-1": {"v1"},
		"ep-2": {"v2"},
	}

	list := BuildCanonicalList(endpoints, voices)
	assert.Len(t, list, 1)
	assert.Equal(t, "v2 (Enabled, m1)", list[0].Name)
}

func TestBuildCanonicalList_NoVoicesForEndpoint(t *testing.T) {
	endpoints := defaultEndpoints()
	voices := map[string][]string{} // no voices for any endpoint

	list := BuildCanonicalList(endpoints, voices)
	assert.Empty(t, list)
}

func TestBuildCanonicalList_Empty(t *testing.T) {
	list := BuildCanonicalList(nil, nil)
	assert.Empty(t, list)
}

// --- Resolve empty voice tests ---

func TestResolve_EmptyVoice_FallsBackToDefaultVoice(t *testing.T) {
	eps := []model.Endpoint{
		{
			ID:           "ep-1",
			Name:         "OpenAI",
			Models:       model.StringSlice{"tts-1"},
			DefaultVoice: "alloy",
			Enabled:      true,
		},
	}
	r := NewResolver(
		&mockEndpointLister{endpoints: eps},
		&mockAliasLister{aliases: nil},
	)

	resolved, err := r.Resolve(context.Background(), "")
	require.NoError(t, err)
	require.NotNil(t, resolved)

	assert.Equal(t, "alloy", resolved.Voice)
	assert.Equal(t, "ep-1", resolved.EndpointID)
	assert.Equal(t, "tts-1", resolved.Model)
}

func TestResolve_DefaultVoiceLiteral_FallsBackToDefaultVoice(t *testing.T) {
	eps := []model.Endpoint{
		{
			ID:           "ep-1",
			Name:         "OpenAI",
			Models:       model.StringSlice{"tts-1"},
			DefaultVoice: "alloy",
			Enabled:      true,
		},
	}
	r := NewResolver(
		&mockEndpointLister{endpoints: eps},
		&mockAliasLister{aliases: nil},
	)

	// "default" should resolve the same as empty string
	resolved, err := r.Resolve(context.Background(), "default")
	require.NoError(t, err)
	require.NotNil(t, resolved)

	assert.Equal(t, "alloy", resolved.Voice)
	assert.Equal(t, "ep-1", resolved.EndpointID)
	assert.Equal(t, "tts-1", resolved.Model)
}

func TestResolve_EmptyVoice_NoDefaultVoiceConfigured(t *testing.T) {
	eps := []model.Endpoint{
		{
			ID:      "ep-1",
			Name:    "OpenAI",
			Models:  model.StringSlice{"tts-1"},
			Enabled: true,
			// DefaultVoice is empty
		},
	}
	r := NewResolver(
		&mockEndpointLister{endpoints: eps},
		&mockAliasLister{aliases: nil},
	)

	_, err := r.Resolve(context.Background(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no voice specified and no default voice configured")
}

func TestResolve_EmptyVoice_SkipsDisabledEndpoint(t *testing.T) {
	eps := []model.Endpoint{
		{
			ID:           "ep-1",
			Name:         "Disabled",
			Models:       model.StringSlice{"tts-1"},
			DefaultVoice: "nova",
			Enabled:      false,
		},
		{
			ID:           "ep-2",
			Name:         "Enabled",
			Models:       model.StringSlice{"kokoro"},
			DefaultVoice: "alloy",
			Enabled:      true,
		},
	}
	r := NewResolver(
		&mockEndpointLister{endpoints: eps},
		&mockAliasLister{aliases: nil},
	)

	resolved, err := r.Resolve(context.Background(), "")
	require.NoError(t, err)
	assert.Equal(t, "alloy", resolved.Voice)
	assert.Equal(t, "ep-2", resolved.EndpointID)
}

func TestResolve_EmptyVoice_NoEndpoints(t *testing.T) {
	r := NewResolver(
		&mockEndpointLister{endpoints: nil},
		&mockAliasLister{aliases: nil},
	)

	_, err := r.Resolve(context.Background(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no voice specified and no default voice configured")
}

func TestResolve_EmptyVoice_EndpointListError(t *testing.T) {
	r := NewResolver(
		&mockEndpointLister{err: errors.New("db down")},
		&mockAliasLister{aliases: nil},
	)

	_, err := r.Resolve(context.Background(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db down")
}

func TestResolve_EmptyVoice_SkipsEndpointWithoutDefaultVoice(t *testing.T) {
	// First enabled endpoint has no DefaultVoice; second has one set.
	// The resolver must skip the first and use the second endpoint's
	// default voice, returning that endpoint's ID and model.
	eps := []model.Endpoint{
		{
			ID:      "ep-no-default",
			Name:    "NoDefault",
			Models:  model.StringSlice{"tts-1"},
			Enabled: true,
			// DefaultVoice is empty
		},
		{
			ID:           "ep-has-default",
			Name:         "HasDefault",
			Models:       model.StringSlice{"kokoro-v1"},
			DefaultVoice: "af_sky",
			DefaultSpeed: ptrFloat(1.1),
			Enabled:      true,
		},
	}
	r := NewResolver(
		&mockEndpointLister{endpoints: eps},
		&mockAliasLister{aliases: nil},
	)

	resolved, err := r.Resolve(context.Background(), "")
	require.NoError(t, err)
	require.NotNil(t, resolved)

	// Must come from the second endpoint, not the first.
	assert.Equal(t, "ep-has-default", resolved.EndpointID)
	assert.Equal(t, "kokoro-v1", resolved.Model)
	assert.Equal(t, "af_sky", resolved.Voice)
	assert.Equal(t, ptrFloat(1.1), resolved.Speed)
	assert.False(t, resolved.IsAlias)
}
