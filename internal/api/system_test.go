package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
	"time"

	"github.com/fx/meadowlark/internal/model"
	"github.com/fx/meadowlark/internal/tts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockStore implements store.Store for testing.
type mockStore struct {
	endpoints []model.Endpoint
	aliases   []model.VoiceAlias
}

func (m *mockStore) ListEndpoints(_ context.Context) ([]model.Endpoint, error) {
	return m.endpoints, nil
}

func (m *mockStore) GetEndpoint(_ context.Context, id string) (*model.Endpoint, error) {
	for i := range m.endpoints {
		if m.endpoints[i].ID == id {
			return &m.endpoints[i], nil
		}
	}
	return nil, fmt.Errorf("endpoint %q not found", id)
}

func (m *mockStore) CreateEndpoint(_ context.Context, _ *model.Endpoint) error { return nil }
func (m *mockStore) UpdateEndpoint(_ context.Context, _ *model.Endpoint) error { return nil }
func (m *mockStore) DeleteEndpoint(_ context.Context, _ string) error          { return nil }

func (m *mockStore) ListVoiceAliases(_ context.Context) ([]model.VoiceAlias, error) {
	return m.aliases, nil
}

func (m *mockStore) GetVoiceAlias(_ context.Context, id string) (*model.VoiceAlias, error) {
	for i := range m.aliases {
		if m.aliases[i].ID == id {
			return &m.aliases[i], nil
		}
	}
	return nil, fmt.Errorf("voice alias %q not found", id)
}

func (m *mockStore) CreateVoiceAlias(_ context.Context, _ *model.VoiceAlias) error { return nil }
func (m *mockStore) UpdateVoiceAlias(_ context.Context, _ *model.VoiceAlias) error { return nil }
func (m *mockStore) DeleteVoiceAlias(_ context.Context, _ string) error            { return nil }

func (m *mockStore) Migrate(_ context.Context) error { return nil }
func (m *mockStore) Close() error                    { return nil }

func newSystemTestServer(st *mockStore) *Server {
	return &Server{
		store:       st,
		version:     "1.2.3",
		startTime:   time.Now().Add(-60 * time.Second),
		wyomingPort: 10300,
		httpPort:    8080,
		dbDriver:    "sqlite",
		urlValidator: noopValidator,
		clientFactory: func(ep *model.Endpoint) *tts.Client {
			return tts.NewClient(ep.BaseURL, ep.APIKey, nil)
		},
		webFS: &fstest.MapFS{
			"index.html": {Data: []byte("<html></html>")},
		},
	}
}

func TestGetStatus_ReturnsCorrectFormat(t *testing.T) {
	ms := &mockStore{
		endpoints: []model.Endpoint{
			{ID: "ep1", Name: "OpenAI", Models: model.StringSlice{"tts-1", "tts-1-hd"}, Enabled: true},
			{ID: "ep2", Name: "Local", Models: model.StringSlice{"model-a"}, Enabled: false},
		},
		aliases: []model.VoiceAlias{
			{ID: "a1", Name: "narrator", EndpointID: "ep1", Model: "tts-1", Voice: "nova", Enabled: true},
			{ID: "a2", Name: "disabled-alias", EndpointID: "ep1", Model: "tts-1", Voice: "alloy", Enabled: false},
		},
	}
	srv := newSystemTestServer(ms)
	router := srv.setupRoutes()
	ts := httptest.NewServer(router)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/status")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body statusResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

	assert.Equal(t, "1.2.3", body.Version)
	assert.Equal(t, 10300, body.WyomingPort)
	assert.Equal(t, 8080, body.HTTPPort)
	assert.Equal(t, "sqlite", body.DBDriver)
	assert.Equal(t, 2, body.EndpointCount)
	assert.Equal(t, 2, body.AliasCount)
	// voice_count = 2 models from enabled ep1 + 1 enabled alias = 3
	assert.Equal(t, 3, body.VoiceCount)
}

func TestGetStatus_UptimeIsPositive(t *testing.T) {
	ms := &mockStore{}
	srv := newSystemTestServer(ms)
	router := srv.setupRoutes()
	ts := httptest.NewServer(router)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/status")
	require.NoError(t, err)
	defer resp.Body.Close()

	var body statusResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

	assert.Greater(t, body.UptimeSeconds, 0)
}

func TestListVoices_EmptyWhenNoEndpoints(t *testing.T) {
	ms := &mockStore{}
	srv := newSystemTestServer(ms)
	router := srv.setupRoutes()
	ts := httptest.NewServer(router)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/voices")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body []voiceEntry
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Empty(t, body)
}

func TestListVoices_ReturnsCanonicalVoices(t *testing.T) {
	ms := &mockStore{
		endpoints: []model.Endpoint{
			{ID: "ep1", Name: "OpenAI", Models: model.StringSlice{"tts-1", "tts-1-hd"}, Enabled: true},
		},
	}
	srv := newSystemTestServer(ms)
	router := srv.setupRoutes()
	ts := httptest.NewServer(router)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/voices")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body []voiceEntry
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

	require.Len(t, body, 2)
	assert.Equal(t, "tts-1 (OpenAI, tts-1)", body[0].Name)
	assert.Equal(t, "OpenAI", body[0].Endpoint)
	assert.Equal(t, "tts-1", body[0].Model)
	assert.False(t, body[0].IsAlias)

	assert.Equal(t, "tts-1-hd (OpenAI, tts-1-hd)", body[1].Name)
	assert.Equal(t, "OpenAI", body[1].Endpoint)
	assert.Equal(t, "tts-1-hd", body[1].Model)
	assert.False(t, body[1].IsAlias)
}

func TestListVoices_ReturnsAliasVoices(t *testing.T) {
	ms := &mockStore{
		endpoints: []model.Endpoint{
			{ID: "ep1", Name: "OpenAI", Models: model.StringSlice{"tts-1"}, Enabled: true},
		},
		aliases: []model.VoiceAlias{
			{ID: "a1", Name: "my-narrator", EndpointID: "ep1", Model: "gpt-4o-mini-tts", Voice: "nova", Enabled: true},
		},
	}
	srv := newSystemTestServer(ms)
	router := srv.setupRoutes()
	ts := httptest.NewServer(router)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/voices")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body []voiceEntry
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

	require.Len(t, body, 2)

	// Second entry is the alias.
	alias := body[1]
	assert.Equal(t, "my-narrator", alias.Name)
	assert.Equal(t, "OpenAI", alias.Endpoint)
	assert.Equal(t, "gpt-4o-mini-tts", alias.Model)
	assert.Equal(t, "nova", alias.Voice)
	assert.True(t, alias.IsAlias)
}

func TestListVoices_CombinesCanonicalAndAlias(t *testing.T) {
	ms := &mockStore{
		endpoints: []model.Endpoint{
			{ID: "ep1", Name: "OpenAI", Models: model.StringSlice{"tts-1"}, Enabled: true},
			{ID: "ep2", Name: "Local", Models: model.StringSlice{"model-a"}, Enabled: true},
		},
		aliases: []model.VoiceAlias{
			{ID: "a1", Name: "narrator", EndpointID: "ep1", Model: "tts-1", Voice: "nova", Enabled: true},
			{ID: "a2", Name: "disabled", EndpointID: "ep2", Model: "model-a", Voice: "v1", Enabled: false},
		},
	}
	srv := newSystemTestServer(ms)
	router := srv.setupRoutes()
	ts := httptest.NewServer(router)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/voices")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body []voiceEntry
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

	// 2 canonical (one per enabled endpoint model) + 1 enabled alias = 3
	require.Len(t, body, 3)

	// Canonical entries.
	assert.False(t, body[0].IsAlias)
	assert.False(t, body[1].IsAlias)

	// Alias entry.
	assert.True(t, body[2].IsAlias)
	assert.Equal(t, "narrator", body[2].Name)
}

func TestListVoices_UsesDiscoveredVoices(t *testing.T) {
	// Create a mock TTS server that returns real voices.
	ttsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/audio/voices" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"voices": []map[string]string{
					{"voice_id": "alloy", "name": "Alloy"},
					{"voice_id": "nova", "name": "Nova"},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer ttsServer.Close()

	ms := &mockStore{
		endpoints: []model.Endpoint{
			{ID: "ep1", Name: "OpenAI", BaseURL: ttsServer.URL, Models: model.StringSlice{"tts-1"}, Enabled: true},
		},
	}
	srv := newSystemTestServer(ms)
	router := srv.setupRoutes()
	ts := httptest.NewServer(router)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/voices")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body []voiceEntry
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

	// Should have 2 canonical voices (alloy, nova) × 1 model (tts-1) = 2 entries.
	require.Len(t, body, 2)

	assert.Equal(t, "alloy (OpenAI, tts-1)", body[0].Name)
	assert.Equal(t, "alloy", body[0].Voice)
	assert.Equal(t, "tts-1", body[0].Model)
	assert.Equal(t, "OpenAI", body[0].Endpoint)
	assert.False(t, body[0].IsAlias)

	assert.Equal(t, "nova (OpenAI, tts-1)", body[1].Name)
	assert.Equal(t, "nova", body[1].Voice)
	assert.Equal(t, "tts-1", body[1].Model)
	assert.False(t, body[1].IsAlias)
}

func TestListVoices_SkipsDisabledEndpoints(t *testing.T) {
	ms := &mockStore{
		endpoints: []model.Endpoint{
			{ID: "ep1", Name: "Disabled", Models: model.StringSlice{"tts-1"}, Enabled: false},
		},
	}
	srv := newSystemTestServer(ms)
	router := srv.setupRoutes()
	ts := httptest.NewServer(router)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/voices")
	require.NoError(t, err)
	defer resp.Body.Close()

	var body []voiceEntry
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

	assert.Empty(t, body)
}

func TestListVoices_SlowEndpointFallsBackToModels(t *testing.T) {
	// Create a TTS server that never responds (blocks until context cancelled).
	slowServer := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer slowServer.Close()

	// Fast server returns real voices.
	fastServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/audio/voices" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"voices": []map[string]string{
					{"voice_id": "alloy", "name": "Alloy"},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer fastServer.Close()

	ms := &mockStore{
		endpoints: []model.Endpoint{
			{ID: "ep1", Name: "FastEP", BaseURL: fastServer.URL, Models: model.StringSlice{"tts-1"}, Enabled: true},
			{ID: "ep2", Name: "SlowEP", BaseURL: slowServer.URL, Models: model.StringSlice{"slow-model"}, Enabled: true},
		},
	}
	srv := newSystemTestServer(ms)
	router := srv.setupRoutes()
	ts := httptest.NewServer(router)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/voices")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body []voiceEntry
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

	// FastEP: discovered voice "alloy" × 1 model = 1 entry
	// SlowEP: timeout → fallback to model name = 1 entry
	require.Len(t, body, 2)

	assert.Equal(t, "alloy (FastEP, tts-1)", body[0].Name)
	assert.Equal(t, "alloy", body[0].Voice)

	assert.Equal(t, "slow-model (SlowEP, slow-model)", body[1].Name)
	assert.Equal(t, "slow-model", body[1].Voice)
}

func TestListVoices_ParallelDiscovery(t *testing.T) {
	// Verify endpoints are queried in parallel by checking that two servers
	// with a delay complete faster than sequential execution would.
	const delay = 200 * time.Millisecond

	makeServer := func(voiceID string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/audio/voices" {
				time.Sleep(delay)
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{
					"data": []map[string]string{
						{"id": voiceID, "name": voiceID},
					},
				})
				return
			}
			http.NotFound(w, r)
		}))
	}

	srv1 := makeServer("voice-a")
	defer srv1.Close()
	srv2 := makeServer("voice-b")
	defer srv2.Close()

	ms := &mockStore{
		endpoints: []model.Endpoint{
			{ID: "ep1", Name: "EP1", BaseURL: srv1.URL, Models: model.StringSlice{"m1"}, Enabled: true},
			{ID: "ep2", Name: "EP2", BaseURL: srv2.URL, Models: model.StringSlice{"m2"}, Enabled: true},
		},
	}
	srv := newSystemTestServer(ms)
	router := srv.setupRoutes()
	ts := httptest.NewServer(router)
	defer ts.Close()

	start := time.Now()
	resp, err := http.Get(ts.URL + "/api/v1/voices")
	elapsed := time.Since(start)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body []voiceEntry
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Len(t, body, 2)

	// If parallel, should take ~delay. If sequential, ~2*delay.
	// Use 1.5*delay as threshold.
	assert.Less(t, elapsed, time.Duration(float64(delay)*1.8),
		"parallel discovery should complete faster than sequential; took %v", elapsed)
}
