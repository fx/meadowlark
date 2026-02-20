package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/fx/meadowlark/internal/model"
	"github.com/fx/meadowlark/internal/tts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// aliasStore is a mock store for aliases tests with full CRUD support.
type aliasStore struct {
	endpoints []model.Endpoint
	aliases   []model.VoiceAlias
	createErr error
	updateErr error
	deleteErr error
}

func (m *aliasStore) ListEndpoints(_ context.Context) ([]model.Endpoint, error) {
	return m.endpoints, nil
}

func (m *aliasStore) GetEndpoint(_ context.Context, id string) (*model.Endpoint, error) {
	for i := range m.endpoints {
		if m.endpoints[i].ID == id {
			return &m.endpoints[i], nil
		}
	}
	return nil, fmt.Errorf("endpoint %q not found", id)
}

func (m *aliasStore) CreateEndpoint(_ context.Context, _ *model.Endpoint) error { return nil }
func (m *aliasStore) UpdateEndpoint(_ context.Context, _ *model.Endpoint) error { return nil }
func (m *aliasStore) DeleteEndpoint(_ context.Context, _ string) error          { return nil }

func (m *aliasStore) ListVoiceAliases(_ context.Context) ([]model.VoiceAlias, error) {
	return m.aliases, nil
}

func (m *aliasStore) GetVoiceAlias(_ context.Context, id string) (*model.VoiceAlias, error) {
	for i := range m.aliases {
		if m.aliases[i].ID == id {
			return &m.aliases[i], nil
		}
	}
	return nil, fmt.Errorf("alias %q not found", id)
}

func (m *aliasStore) CreateVoiceAlias(_ context.Context, a *model.VoiceAlias) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.aliases = append(m.aliases, *a)
	return nil
}

func (m *aliasStore) UpdateVoiceAlias(_ context.Context, a *model.VoiceAlias) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	for i := range m.aliases {
		if m.aliases[i].ID == a.ID {
			m.aliases[i] = *a
			return nil
		}
	}
	return fmt.Errorf("alias %q not found", a.ID)
}

func (m *aliasStore) DeleteVoiceAlias(_ context.Context, id string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	for i := range m.aliases {
		if m.aliases[i].ID == id {
			m.aliases = append(m.aliases[:i], m.aliases[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("alias %q not found", id)
}

func (m *aliasStore) Migrate(_ context.Context) error { return nil }
func (m *aliasStore) Close() error                    { return nil }

func newAliasTestServer(st *aliasStore) *Server {
	return &Server{
		store:       st,
		version:     "test",
		httpPort:    8080,
		dbDriver:    "sqlite",
		startTime:   time.Now(),
		clientFactory: func(ep *model.Endpoint) *tts.Client {
			return tts.NewClient(ep.BaseURL, ep.APIKey, nil)
		},
		webFS: &fstest.MapFS{
			"index.html": {Data: []byte("<html></html>")},
		},
	}
}

func makeAliasRequest(t *testing.T, ts *httptest.Server, method, path string, body any) *http.Response {
	t.Helper()
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, ts.URL+path, reqBody)
	require.NoError(t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

// --- List Aliases ---

func TestListAliases_Empty(t *testing.T) {
	st := &aliasStore{}
	srv := newAliasTestServer(st)
	ts := httptest.NewServer(srv.setupRoutes())
	defer ts.Close()

	resp := makeAliasRequest(t, ts, http.MethodGet, "/api/v1/aliases", nil)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body []model.VoiceAlias
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Empty(t, body)
}

func TestListAliases_WithData(t *testing.T) {
	st := &aliasStore{
		aliases: []model.VoiceAlias{
			{ID: "a1", Name: "narrator", EndpointID: "ep1", Model: "tts-1", Voice: "nova"},
			{ID: "a2", Name: "reader", EndpointID: "ep1", Model: "tts-1", Voice: "alloy"},
		},
	}
	srv := newAliasTestServer(st)
	ts := httptest.NewServer(srv.setupRoutes())
	defer ts.Close()

	resp := makeAliasRequest(t, ts, http.MethodGet, "/api/v1/aliases", nil)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body []model.VoiceAlias
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Len(t, body, 2)
	assert.Equal(t, "narrator", body[0].Name)
	assert.Equal(t, "reader", body[1].Name)
}

// --- Get Alias ---

func TestGetAlias_Found(t *testing.T) {
	st := &aliasStore{
		aliases: []model.VoiceAlias{
			{ID: "a1", Name: "narrator", EndpointID: "ep1", Model: "tts-1", Voice: "nova"},
		},
	}
	srv := newAliasTestServer(st)
	ts := httptest.NewServer(srv.setupRoutes())
	defer ts.Close()

	resp := makeAliasRequest(t, ts, http.MethodGet, "/api/v1/aliases/a1", nil)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body model.VoiceAlias
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "narrator", body.Name)
}

func TestGetAlias_NotFound(t *testing.T) {
	st := &aliasStore{}
	srv := newAliasTestServer(st)
	ts := httptest.NewServer(srv.setupRoutes())
	defer ts.Close()

	resp := makeAliasRequest(t, ts, http.MethodGet, "/api/v1/aliases/nonexistent", nil)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// --- Create Alias ---

func TestCreateAlias_Valid(t *testing.T) {
	st := &aliasStore{
		endpoints: []model.Endpoint{
			{ID: "ep1", Name: "OpenAI", BaseURL: "https://api.openai.com/v1"},
		},
	}
	srv := newAliasTestServer(st)
	ts := httptest.NewServer(srv.setupRoutes())
	defer ts.Close()

	resp := makeAliasRequest(t, ts, http.MethodPost, "/api/v1/aliases", map[string]any{
		"name":        "narrator",
		"endpoint_id": "ep1",
		"model":       "tts-1",
		"voice":       "nova",
		"speed":       1.5,
	})
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var body model.VoiceAlias
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "narrator", body.Name)
	assert.Equal(t, "ep1", body.EndpointID)
	assert.Equal(t, "tts-1", body.Model)
	assert.Equal(t, "nova", body.Voice)
	assert.NotEmpty(t, body.ID)
	assert.True(t, body.Enabled)
	assert.NotZero(t, body.CreatedAt)
	assert.NotZero(t, body.UpdatedAt)
}

func TestCreateAlias_MissingName(t *testing.T) {
	st := &aliasStore{}
	srv := newAliasTestServer(st)
	ts := httptest.NewServer(srv.setupRoutes())
	defer ts.Close()

	resp := makeAliasRequest(t, ts, http.MethodPost, "/api/v1/aliases", map[string]any{
		"endpoint_id": "ep1",
		"model":       "tts-1",
		"voice":       "nova",
	})
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var body map[string]apiError
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "bad_request", body["error"].Code)
	assert.Contains(t, body["error"].Message, "name")
}

func TestCreateAlias_MissingEndpointID(t *testing.T) {
	st := &aliasStore{}
	srv := newAliasTestServer(st)
	ts := httptest.NewServer(srv.setupRoutes())
	defer ts.Close()

	resp := makeAliasRequest(t, ts, http.MethodPost, "/api/v1/aliases", map[string]any{
		"name":  "narrator",
		"model": "tts-1",
		"voice": "nova",
	})
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var body map[string]apiError
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Contains(t, body["error"].Message, "endpoint_id")
}

func TestCreateAlias_InvalidEndpointID(t *testing.T) {
	st := &aliasStore{} // No endpoints
	srv := newAliasTestServer(st)
	ts := httptest.NewServer(srv.setupRoutes())
	defer ts.Close()

	resp := makeAliasRequest(t, ts, http.MethodPost, "/api/v1/aliases", map[string]any{
		"name":        "narrator",
		"endpoint_id": "nonexistent",
		"model":       "tts-1",
		"voice":       "nova",
	})
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var body map[string]apiError
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Contains(t, body["error"].Message, "endpoint not found")
}

func TestCreateAlias_MissingModel(t *testing.T) {
	st := &aliasStore{}
	srv := newAliasTestServer(st)
	ts := httptest.NewServer(srv.setupRoutes())
	defer ts.Close()

	resp := makeAliasRequest(t, ts, http.MethodPost, "/api/v1/aliases", map[string]any{
		"name":        "narrator",
		"endpoint_id": "ep1",
		"voice":       "nova",
	})
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var body map[string]apiError
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Contains(t, body["error"].Message, "model")
}

func TestCreateAlias_MissingVoice(t *testing.T) {
	st := &aliasStore{}
	srv := newAliasTestServer(st)
	ts := httptest.NewServer(srv.setupRoutes())
	defer ts.Close()

	resp := makeAliasRequest(t, ts, http.MethodPost, "/api/v1/aliases", map[string]any{
		"name":        "narrator",
		"endpoint_id": "ep1",
		"model":       "tts-1",
	})
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var body map[string]apiError
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Contains(t, body["error"].Message, "voice")
}

func TestCreateAlias_InvalidSpeed(t *testing.T) {
	st := &aliasStore{
		endpoints: []model.Endpoint{
			{ID: "ep1", Name: "OpenAI", BaseURL: "https://api.openai.com/v1"},
		},
	}
	srv := newAliasTestServer(st)
	ts := httptest.NewServer(srv.setupRoutes())
	defer ts.Close()

	resp := makeAliasRequest(t, ts, http.MethodPost, "/api/v1/aliases", map[string]any{
		"name":        "narrator",
		"endpoint_id": "ep1",
		"model":       "tts-1",
		"voice":       "nova",
		"speed":       5.0,
	})
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var body map[string]apiError
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Contains(t, body["error"].Message, "speed")
}

func TestCreateAlias_SpeedTooLow(t *testing.T) {
	st := &aliasStore{
		endpoints: []model.Endpoint{
			{ID: "ep1", Name: "OpenAI", BaseURL: "https://api.openai.com/v1"},
		},
	}
	srv := newAliasTestServer(st)
	ts := httptest.NewServer(srv.setupRoutes())
	defer ts.Close()

	resp := makeAliasRequest(t, ts, http.MethodPost, "/api/v1/aliases", map[string]any{
		"name":        "narrator",
		"endpoint_id": "ep1",
		"model":       "tts-1",
		"voice":       "nova",
		"speed":       0.1,
	})
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var body map[string]apiError
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Contains(t, body["error"].Message, "speed")
}

func TestCreateAlias_DuplicateName(t *testing.T) {
	st := &aliasStore{
		endpoints: []model.Endpoint{
			{ID: "ep1", Name: "OpenAI", BaseURL: "https://api.openai.com/v1"},
		},
		aliases: []model.VoiceAlias{
			{ID: "a1", Name: "narrator", EndpointID: "ep1", Model: "tts-1", Voice: "nova"},
		},
	}
	srv := newAliasTestServer(st)
	ts := httptest.NewServer(srv.setupRoutes())
	defer ts.Close()

	resp := makeAliasRequest(t, ts, http.MethodPost, "/api/v1/aliases", map[string]any{
		"name":        "narrator",
		"endpoint_id": "ep1",
		"model":       "tts-1",
		"voice":       "alloy",
	})
	defer resp.Body.Close()

	assert.Equal(t, http.StatusConflict, resp.StatusCode)

	var body map[string]apiError
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "conflict", body["error"].Code)
}

func TestCreateAlias_InvalidJSON(t *testing.T) {
	st := &aliasStore{}
	srv := newAliasTestServer(st)
	ts := httptest.NewServer(srv.setupRoutes())
	defer ts.Close()

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/aliases", strings.NewReader("{invalid"))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// --- Update Alias ---

func TestUpdateAlias_Valid(t *testing.T) {
	st := &aliasStore{
		endpoints: []model.Endpoint{
			{ID: "ep1", Name: "OpenAI", BaseURL: "https://api.openai.com/v1"},
		},
		aliases: []model.VoiceAlias{
			{ID: "a1", Name: "narrator", EndpointID: "ep1", Model: "tts-1", Voice: "nova", Enabled: true},
		},
	}
	srv := newAliasTestServer(st)
	ts := httptest.NewServer(srv.setupRoutes())
	defer ts.Close()

	resp := makeAliasRequest(t, ts, http.MethodPut, "/api/v1/aliases/a1", map[string]any{
		"voice": "alloy",
	})
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body model.VoiceAlias
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "alloy", body.Voice)
	assert.Equal(t, "narrator", body.Name) // Unchanged
}

func TestUpdateAlias_NotFound(t *testing.T) {
	st := &aliasStore{}
	srv := newAliasTestServer(st)
	ts := httptest.NewServer(srv.setupRoutes())
	defer ts.Close()

	resp := makeAliasRequest(t, ts, http.MethodPut, "/api/v1/aliases/nonexistent", map[string]any{
		"voice": "alloy",
	})
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestUpdateAlias_PartialUpdate(t *testing.T) {
	speed := 1.0
	st := &aliasStore{
		endpoints: []model.Endpoint{
			{ID: "ep1", Name: "OpenAI", BaseURL: "https://api.openai.com/v1"},
		},
		aliases: []model.VoiceAlias{
			{
				ID:         "a1",
				Name:       "narrator",
				EndpointID: "ep1",
				Model:      "tts-1",
				Voice:      "nova",
				Speed:      &speed,
				Enabled:    true,
			},
		},
	}
	srv := newAliasTestServer(st)
	ts := httptest.NewServer(srv.setupRoutes())
	defer ts.Close()

	resp := makeAliasRequest(t, ts, http.MethodPut, "/api/v1/aliases/a1", map[string]any{
		"name": "new-narrator",
	})
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body model.VoiceAlias
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "new-narrator", body.Name)
	assert.Equal(t, "nova", body.Voice)        // Unchanged
	assert.Equal(t, "tts-1", body.Model)       // Unchanged
	assert.InDelta(t, 1.0, *body.Speed, 0.001) // Unchanged
}

// --- Delete Alias ---

func TestDeleteAlias_Found(t *testing.T) {
	st := &aliasStore{
		aliases: []model.VoiceAlias{
			{ID: "a1", Name: "narrator", EndpointID: "ep1", Model: "tts-1", Voice: "nova"},
		},
	}
	srv := newAliasTestServer(st)
	ts := httptest.NewServer(srv.setupRoutes())
	defer ts.Close()

	resp := makeAliasRequest(t, ts, http.MethodDelete, "/api/v1/aliases/a1", nil)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Empty(t, st.aliases)
}

func TestDeleteAlias_NotFound(t *testing.T) {
	st := &aliasStore{}
	srv := newAliasTestServer(st)
	ts := httptest.NewServer(srv.setupRoutes())
	defer ts.Close()

	resp := makeAliasRequest(t, ts, http.MethodDelete, "/api/v1/aliases/nonexistent", nil)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// --- Test Alias ---

func TestTestAlias_Success(t *testing.T) {
	mockTTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "audio/wav")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("RIFF\x00\x00\x00\x00WAVEfmt "))
	}))
	defer mockTTS.Close()

	st := &aliasStore{
		endpoints: []model.Endpoint{
			{ID: "ep1", Name: "Mock", BaseURL: mockTTS.URL},
		},
		aliases: []model.VoiceAlias{
			{ID: "a1", Name: "narrator", EndpointID: "ep1", Model: "tts-1", Voice: "nova"},
		},
	}
	srv := newAliasTestServer(st)
	srv.clientFactory = func(ep *model.Endpoint) *tts.Client {
		return tts.NewClient(ep.BaseURL, ep.APIKey, nil)
	}
	ts := httptest.NewServer(srv.setupRoutes())
	defer ts.Close()

	resp := makeAliasRequest(t, ts, http.MethodPost, "/api/v1/aliases/a1/test", nil)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.True(t, body["ok"].(bool))
}

func TestTestAlias_Failure(t *testing.T) {
	mockTTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("server error"))
	}))
	defer mockTTS.Close()

	st := &aliasStore{
		endpoints: []model.Endpoint{
			{ID: "ep1", Name: "Mock", BaseURL: mockTTS.URL},
		},
		aliases: []model.VoiceAlias{
			{ID: "a1", Name: "narrator", EndpointID: "ep1", Model: "tts-1", Voice: "nova"},
		},
	}
	srv := newAliasTestServer(st)
	srv.clientFactory = func(ep *model.Endpoint) *tts.Client {
		return tts.NewClient(ep.BaseURL, ep.APIKey, nil)
	}
	ts := httptest.NewServer(srv.setupRoutes())
	defer ts.Close()

	resp := makeAliasRequest(t, ts, http.MethodPost, "/api/v1/aliases/a1/test", nil)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.False(t, body["ok"].(bool))
	assert.NotEmpty(t, body["error"])
}

func TestTestAlias_CustomText(t *testing.T) {
	var receivedInput string
	mockTTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req tts.SynthesizeRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		receivedInput = req.Input
		w.Header().Set("Content-Type", "audio/wav")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("RIFF\x00\x00\x00\x00WAVEfmt "))
	}))
	defer mockTTS.Close()

	st := &aliasStore{
		endpoints: []model.Endpoint{
			{ID: "ep1", Name: "Mock", BaseURL: mockTTS.URL},
		},
		aliases: []model.VoiceAlias{
			{ID: "a1", Name: "narrator", EndpointID: "ep1", Model: "tts-1", Voice: "nova"},
		},
	}
	srv := newAliasTestServer(st)
	srv.clientFactory = func(ep *model.Endpoint) *tts.Client {
		return tts.NewClient(ep.BaseURL, ep.APIKey, nil)
	}
	ts := httptest.NewServer(srv.setupRoutes())
	defer ts.Close()

	resp := makeAliasRequest(t, ts, http.MethodPost, "/api/v1/aliases/a1/test", map[string]any{
		"text": "Custom test message",
	})
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "Custom test message", receivedInput)
}

func TestTestAlias_NotFound(t *testing.T) {
	st := &aliasStore{}
	srv := newAliasTestServer(st)
	ts := httptest.NewServer(srv.setupRoutes())
	defer ts.Close()

	resp := makeAliasRequest(t, ts, http.MethodPost, "/api/v1/aliases/nonexistent/test", nil)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}
