package wyoming

import (
	"context"
	"errors"
	"testing"

	"github.com/fx/meadowlark/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockEndpointLister implements EndpointLister for testing.
type mockEndpointLister struct {
	endpoints []model.Endpoint
	err       error
}

func (m *mockEndpointLister) ListEndpoints(_ context.Context) ([]model.Endpoint, error) {
	return m.endpoints, m.err
}

// mockAliasLister implements AliasLister for testing.
type mockAliasLister struct {
	aliases []model.VoiceAlias
	err     error
}

func (m *mockAliasLister) ListVoiceAliases(_ context.Context) ([]model.VoiceAlias, error) {
	return m.aliases, m.err
}

// mockEndpointVoiceLister implements EndpointVoiceLister for testing.
type mockEndpointVoiceLister struct {
	byEndpoint map[string][]model.EndpointVoice
	err        error
}

func (m *mockEndpointVoiceLister) ListEndpointVoices(_ context.Context, endpointID string) ([]model.EndpointVoice, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.byEndpoint[endpointID], nil
}

func TestInfoBuilder_Build_EmptyState(t *testing.T) {
	builder := NewInfoBuilder(&mockEndpointLister{}, &mockAliasLister{}, nil, "1.0.0")
	info, err := builder.Build(context.Background())
	require.NoError(t, err)

	require.Len(t, info.Tts, 1)
	assert.Equal(t, "meadowlark", info.Tts[0].Name)
	assert.Equal(t, "Meadowlark TTS Bridge", info.Tts[0].Description)
	assert.True(t, info.Tts[0].Installed)
	assert.Equal(t, "1.0.0", info.Tts[0].Version)
	assert.Empty(t, info.Tts[0].Voices)
}

func TestInfoBuilder_Build_WithEndpointsAndAliases(t *testing.T) {
	speed := 1.5
	endpoints := &mockEndpointLister{
		endpoints: []model.Endpoint{
			{
				ID:      "ep1",
				Name:    "OpenAI",
				Enabled: true,
				Models:  model.StringSlice{"tts-1", "gpt-4o-mini-tts"},
			},
			{
				ID:      "ep2",
				Name:    "Disabled",
				Enabled: false,
				Models:  model.StringSlice{"model-x"},
			},
		},
	}

	aliases := &mockAliasLister{
		aliases: []model.VoiceAlias{
			{
				ID:        "a1",
				Name:      "my-voice",
				Enabled:   true,
				Languages: model.StringSlice{"en", "fr"},
				Speed:     &speed,
			},
			{
				ID:      "a2",
				Name:    "disabled-alias",
				Enabled: false,
			},
		},
	}

	endpointVoices := &mockEndpointVoiceLister{
		byEndpoint: map[string][]model.EndpointVoice{
			"ep1": {
				{EndpointID: "ep1", VoiceID: "alloy", Enabled: true},
			},
			// ep2 is disabled, but even if it had rows they should be ignored.
			"ep2": {
				{EndpointID: "ep2", VoiceID: "ignored", Enabled: true},
			},
		},
	}

	builder := NewInfoBuilder(endpoints, aliases, endpointVoices, "0.2.0")
	info, err := builder.Build(context.Background())
	require.NoError(t, err)

	require.Len(t, info.Tts, 1)
	prog := info.Tts[0]
	assert.Equal(t, "0.2.0", prog.Version)

	// 2 canonical voices (ep1 has 1 enabled voice x 2 models) + 1 enabled alias
	require.Len(t, prog.Voices, 3)

	assert.Equal(t, "alloy (OpenAI, tts-1)", prog.Voices[0].Name)
	assert.True(t, prog.Voices[0].Installed)
	assert.Equal(t, []string{"en"}, prog.Voices[0].Languages)

	assert.Equal(t, "alloy (OpenAI, gpt-4o-mini-tts)", prog.Voices[1].Name)

	assert.Equal(t, "my-voice", prog.Voices[2].Name)
	assert.Equal(t, "my-voice", prog.Voices[2].Description)
	assert.True(t, prog.Voices[2].Installed)
	assert.Equal(t, []string{"en", "fr"}, prog.Voices[2].Languages)
}

// Disabled voices on an enabled endpoint MUST NOT appear in the canonical list.
func TestInfoBuilder_Build_FiltersDisabledVoices(t *testing.T) {
	endpoints := &mockEndpointLister{
		endpoints: []model.Endpoint{
			{
				ID:      "ep1",
				Name:    "ep",
				Enabled: true,
				Models:  model.StringSlice{"tts-1"},
			},
		},
	}

	endpointVoices := &mockEndpointVoiceLister{
		byEndpoint: map[string][]model.EndpointVoice{
			"ep1": {
				{EndpointID: "ep1", VoiceID: "alloy", Enabled: true},
				{EndpointID: "ep1", VoiceID: "echo", Enabled: false},
			},
		},
	}

	builder := NewInfoBuilder(endpoints, &mockAliasLister{}, endpointVoices, "1.0.0")
	info, err := builder.Build(context.Background())
	require.NoError(t, err)

	prog := info.Tts[0]
	require.Len(t, prog.Voices, 1)
	assert.Equal(t, "alloy (ep, tts-1)", prog.Voices[0].Name)
}

// An endpoint with no endpoint_voices rows MUST emit no canonical voices —
// no live-probe fallback.
func TestInfoBuilder_Build_NoVoicesNoFallback(t *testing.T) {
	endpoints := &mockEndpointLister{
		endpoints: []model.Endpoint{
			{
				ID:      "ep1",
				Name:    "ep",
				Enabled: true,
				Models:  model.StringSlice{"tts-1"},
			},
		},
	}

	endpointVoices := &mockEndpointVoiceLister{
		byEndpoint: map[string][]model.EndpointVoice{},
	}

	builder := NewInfoBuilder(endpoints, &mockAliasLister{}, endpointVoices, "1.0.0")
	info, err := builder.Build(context.Background())
	require.NoError(t, err)

	assert.Empty(t, info.Tts[0].Voices)
}

// Multiple models cross enabled voices to form the full Cartesian list.
func TestInfoBuilder_Build_VoicesCrossModels(t *testing.T) {
	endpoints := &mockEndpointLister{
		endpoints: []model.Endpoint{
			{
				ID:      "ep1",
				Name:    "ep",
				Enabled: true,
				Models:  model.StringSlice{"model-a", "model-b"},
			},
		},
	}

	endpointVoices := &mockEndpointVoiceLister{
		byEndpoint: map[string][]model.EndpointVoice{
			"ep1": {
				{EndpointID: "ep1", VoiceID: "alice", Enabled: true},
				{EndpointID: "ep1", VoiceID: "bob", Enabled: true},
			},
		},
	}

	builder := NewInfoBuilder(endpoints, &mockAliasLister{}, endpointVoices, "1.0.0")
	info, err := builder.Build(context.Background())
	require.NoError(t, err)

	prog := info.Tts[0]
	require.Len(t, prog.Voices, 4)
	assert.Equal(t, "alice (ep, model-a)", prog.Voices[0].Name)
	assert.Equal(t, "bob (ep, model-a)", prog.Voices[1].Name)
	assert.Equal(t, "alice (ep, model-b)", prog.Voices[2].Name)
	assert.Equal(t, "bob (ep, model-b)", prog.Voices[3].Name)
}

func TestInfoBuilder_Build_AliasDefaultLanguages(t *testing.T) {
	aliases := &mockAliasLister{
		aliases: []model.VoiceAlias{
			{
				ID:      "a1",
				Name:    "no-lang",
				Enabled: true,
				// Languages is nil/empty.
			},
		},
	}

	builder := NewInfoBuilder(&mockEndpointLister{}, aliases, nil, "1.0.0")
	info, err := builder.Build(context.Background())
	require.NoError(t, err)

	require.Len(t, info.Tts[0].Voices, 1)
	assert.Equal(t, []string{"en"}, info.Tts[0].Voices[0].Languages)
}

func TestInfoBuilder_Build_EndpointListError(t *testing.T) {
	endpoints := &mockEndpointLister{err: errors.New("db error")}
	builder := NewInfoBuilder(endpoints, &mockAliasLister{}, nil, "1.0.0")

	_, err := builder.Build(context.Background())
	assert.ErrorContains(t, err, "db error")
}

func TestInfoBuilder_Build_AliasListError(t *testing.T) {
	aliases := &mockAliasLister{err: errors.New("alias error")}
	builder := NewInfoBuilder(&mockEndpointLister{}, aliases, nil, "1.0.0")

	_, err := builder.Build(context.Background())
	assert.ErrorContains(t, err, "alias error")
}

func TestInfoBuilder_Build_EndpointVoiceListError(t *testing.T) {
	endpoints := &mockEndpointLister{
		endpoints: []model.Endpoint{
			{ID: "ep1", Name: "ep", Enabled: true, Models: model.StringSlice{"tts-1"}},
		},
	}
	endpointVoices := &mockEndpointVoiceLister{err: errors.New("voice error")}
	builder := NewInfoBuilder(endpoints, &mockAliasLister{}, endpointVoices, "1.0.0")

	_, err := builder.Build(context.Background())
	assert.ErrorContains(t, err, "voice error")
}

func TestInfoBuilder_Cached(t *testing.T) {
	builder := NewInfoBuilder(&mockEndpointLister{}, &mockAliasLister{}, nil, "1.0.0")

	// Before Build, Cached returns nil.
	assert.Nil(t, builder.Cached())

	info, err := builder.Build(context.Background())
	require.NoError(t, err)

	// After Build, Cached returns the same info.
	cached := builder.Cached()
	assert.Equal(t, info, cached)
}

func TestInfoBuilder_Build_RebuildUpdatesCache(t *testing.T) {
	aliases := &mockAliasLister{}
	builder := NewInfoBuilder(&mockEndpointLister{}, aliases, nil, "1.0.0")

	info1, err := builder.Build(context.Background())
	require.NoError(t, err)
	assert.Empty(t, info1.Tts[0].Voices)

	// Add an alias and rebuild.
	aliases.aliases = []model.VoiceAlias{
		{ID: "a1", Name: "new-voice", Enabled: true},
	}
	info2, err := builder.Build(context.Background())
	require.NoError(t, err)
	require.Len(t, info2.Tts[0].Voices, 1)

	// Cached should return the updated info.
	cached := builder.Cached()
	assert.Len(t, cached.Tts[0].Voices, 1)
}
