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

type endpointMockStore struct {
	endpoints []model.Endpoint
	aliases   []model.VoiceAlias
	createErr error
	updateErr error
	deleteErr error
}

func (m *endpointMockStore) ListEndpoints(_ context.Context) ([]model.Endpoint, error) {
	return m.endpoints, nil
}
func (m *endpointMockStore) GetEndpoint(_ context.Context, id string) (*model.Endpoint, error) {
	for i := range m.endpoints {
		if m.endpoints[i].ID == id {
			return &m.endpoints[i], nil
		}
	}
	return nil, nil
}
func (m *endpointMockStore) CreateEndpoint(_ context.Context, e *model.Endpoint) error {
	if m.createErr != nil { return m.createErr }
	m.endpoints = append(m.endpoints, *e)
	return nil
}
func (m *endpointMockStore) UpdateEndpoint(_ context.Context, e *model.Endpoint) error {
	if m.updateErr != nil { return m.updateErr }
	for i := range m.endpoints {
		if m.endpoints[i].ID == e.ID { m.endpoints[i] = *e; return nil }
	}
	return fmt.Errorf("store: endpoint %q not found", e.ID)
}
func (m *endpointMockStore) DeleteEndpoint(_ context.Context, id string) error {
	if m.deleteErr != nil { return m.deleteErr }
	for i := range m.endpoints {
		if m.endpoints[i].ID == id { m.endpoints = append(m.endpoints[:i], m.endpoints[i+1:]...); return nil }
	}
	return fmt.Errorf("store: endpoint %q not found", id)
}
func (m *endpointMockStore) ListVoiceAliases(_ context.Context) ([]model.VoiceAlias, error) { return m.aliases, nil }
func (m *endpointMockStore) GetVoiceAlias(_ context.Context, id string) (*model.VoiceAlias, error) {
	for i := range m.aliases { if m.aliases[i].ID == id { return &m.aliases[i], nil } }
	return nil, nil
}
func (m *endpointMockStore) CreateVoiceAlias(_ context.Context, _ *model.VoiceAlias) error { return nil }
func (m *endpointMockStore) UpdateVoiceAlias(_ context.Context, _ *model.VoiceAlias) error { return nil }
func (m *endpointMockStore) DeleteVoiceAlias(_ context.Context, _ string) error { return nil }
func (m *endpointMockStore) Migrate(_ context.Context) error { return nil }
func (m *endpointMockStore) Close() error { return nil }

// noopValidator allows all URLs (used for tests that need loopback mock servers).
func noopValidator(_ context.Context, _ string) error { return nil }

// ssrfValidator uses real SSRF validation with a public DNS resolver.
func ssrfValidator() URLValidator {
	r := publicResolver()
	return func(ctx context.Context, rawURL string) error {
		return validateProbeURL(ctx, rawURL, r)
	}
}

func newEndpointTestServer(ms *endpointMockStore) (*Server, *httptest.Server) {
	srv := &Server{
		store: ms, version: "test", httpPort: 8080, dbDriver: "sqlite", startTime: time.Now(),
		webFS: &fstest.MapFS{"index.html": {Data: []byte("<html></html>")}},
		clientFactory: func(ep *model.Endpoint) *tts.Client { return tts.NewClient(ep.BaseURL, ep.APIKey, nil) },
		urlValidator:  noopValidator,
	}
	return srv, httptest.NewServer(srv.setupRoutes())
}

func TestListEndpoints_Empty(t *testing.T) {
	ms := &endpointMockStore{}
	_, ts := newEndpointTestServer(ms); defer ts.Close()
	resp, err := http.Get(ts.URL + "/api/v1/endpoints")
	require.NoError(t, err); defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var body []model.Endpoint
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Empty(t, body)
}

func TestListEndpoints_WithData(t *testing.T) {
	ms := &endpointMockStore{endpoints: []model.Endpoint{
		{ID: "ep1", Name: "OpenAI", BaseURL: "https://api.openai.com/v1", Models: model.StringSlice{"tts-1"}, Enabled: true},
		{ID: "ep2", Name: "Local", BaseURL: "http://localhost:8000/v1", Models: model.StringSlice{"model-a"}, Enabled: false},
	}}
	_, ts := newEndpointTestServer(ms); defer ts.Close()
	resp, err := http.Get(ts.URL + "/api/v1/endpoints")
	require.NoError(t, err); defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var body []model.Endpoint
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Len(t, body, 2)
	assert.Equal(t, "OpenAI", body[0].Name)
}

func TestGetEndpoint_Found(t *testing.T) {
	ms := &endpointMockStore{endpoints: []model.Endpoint{{ID: "ep1", Name: "OpenAI", BaseURL: "https://api.openai.com/v1", Models: model.StringSlice{"tts-1"}, Enabled: true}}}
	_, ts := newEndpointTestServer(ms); defer ts.Close()
	resp, err := http.Get(ts.URL + "/api/v1/endpoints/ep1")
	require.NoError(t, err); defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var body model.Endpoint
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "ep1", body.ID)
}

func TestGetEndpoint_NotFound(t *testing.T) {
	ms := &endpointMockStore{}
	_, ts := newEndpointTestServer(ms); defer ts.Close()
	resp, err := http.Get(ts.URL + "/api/v1/endpoints/nonexistent")
	require.NoError(t, err); defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestCreateEndpoint_Valid(t *testing.T) {
	ms := &endpointMockStore{}
	_, ts := newEndpointTestServer(ms); defer ts.Close()
	payload := `{"name":"OpenAI","base_url":"https://api.openai.com/v1","models":["tts-1","tts-1-hd"]}`
	resp, err := http.Post(ts.URL+"/api/v1/endpoints", "application/json", strings.NewReader(payload))
	require.NoError(t, err); defer resp.Body.Close()
	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	var body model.Endpoint
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.NotEmpty(t, body.ID)
	assert.Equal(t, "OpenAI", body.Name)
	assert.True(t, body.Enabled)
	assert.Equal(t, "wav", body.DefaultResponseFormat)
}

func TestCreateEndpoint_WithOptionalFields(t *testing.T) {
	ms := &endpointMockStore{}
	_, ts := newEndpointTestServer(ms); defer ts.Close()
	payload := map[string]any{"name": "Custom", "base_url": "https://custom.example.com/v1", "api_key": "sk-test", "models": []string{"model-1"}, "default_speed": 1.5, "default_response_format": "mp3", "enabled": false}
	b, _ := json.Marshal(payload)
	resp, err := http.Post(ts.URL+"/api/v1/endpoints", "application/json", bytes.NewReader(b))
	require.NoError(t, err); defer resp.Body.Close()
	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	var body model.Endpoint
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "sk-test", body.APIKey)
	assert.InDelta(t, 1.5, *body.DefaultSpeed, 0.001)
	assert.Equal(t, "mp3", body.DefaultResponseFormat)
	assert.False(t, body.Enabled)
}

func TestCreateEndpoint_MissingName(t *testing.T) {
	ms := &endpointMockStore{}
	_, ts := newEndpointTestServer(ms); defer ts.Close()
	resp, err := http.Post(ts.URL+"/api/v1/endpoints", "application/json", strings.NewReader(`{"base_url":"https://api.openai.com/v1","models":["tts-1"]}`))
	require.NoError(t, err); defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestCreateEndpoint_MissingBaseURL(t *testing.T) {
	ms := &endpointMockStore{}
	_, ts := newEndpointTestServer(ms); defer ts.Close()
	resp, err := http.Post(ts.URL+"/api/v1/endpoints", "application/json", strings.NewReader(`{"name":"Test","models":["tts-1"]}`))
	require.NoError(t, err); defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestCreateEndpoint_InvalidBaseURL(t *testing.T) {
	ms := &endpointMockStore{}
	_, ts := newEndpointTestServer(ms); defer ts.Close()
	resp, err := http.Post(ts.URL+"/api/v1/endpoints", "application/json", strings.NewReader(`{"name":"Test","base_url":"not a url","models":["tts-1"]}`))
	require.NoError(t, err); defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestCreateEndpoint_MissingModels(t *testing.T) {
	ms := &endpointMockStore{}
	_, ts := newEndpointTestServer(ms); defer ts.Close()
	resp, err := http.Post(ts.URL+"/api/v1/endpoints", "application/json", strings.NewReader(`{"name":"Test","base_url":"https://api.openai.com/v1"}`))
	require.NoError(t, err); defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestCreateEndpoint_EmptyModels(t *testing.T) {
	ms := &endpointMockStore{}
	_, ts := newEndpointTestServer(ms); defer ts.Close()
	resp, err := http.Post(ts.URL+"/api/v1/endpoints", "application/json", strings.NewReader(`{"name":"Test","base_url":"https://api.openai.com/v1","models":[]}`))
	require.NoError(t, err); defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestCreateEndpoint_InvalidSpeed(t *testing.T) {
	ms := &endpointMockStore{}
	_, ts := newEndpointTestServer(ms); defer ts.Close()
	resp, err := http.Post(ts.URL+"/api/v1/endpoints", "application/json", strings.NewReader(`{"name":"Test","base_url":"https://api.openai.com/v1","models":["tts-1"],"default_speed":0.1}`))
	require.NoError(t, err); defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestCreateEndpoint_DuplicateName(t *testing.T) {
	ms := &endpointMockStore{endpoints: []model.Endpoint{{ID: "ep1", Name: "OpenAI", BaseURL: "https://api.openai.com/v1", Models: model.StringSlice{"tts-1"}, Enabled: true}}}
	_, ts := newEndpointTestServer(ms); defer ts.Close()
	resp, err := http.Post(ts.URL+"/api/v1/endpoints", "application/json", strings.NewReader(`{"name":"OpenAI","base_url":"https://other.api.com/v1","models":["tts-2"]}`))
	require.NoError(t, err); defer resp.Body.Close()
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestCreateEndpoint_DuplicateNameCaseInsensitive(t *testing.T) {
	ms := &endpointMockStore{endpoints: []model.Endpoint{{ID: "ep1", Name: "OpenAI", BaseURL: "https://api.openai.com/v1", Models: model.StringSlice{"tts-1"}, Enabled: true}}}
	_, ts := newEndpointTestServer(ms); defer ts.Close()
	resp, err := http.Post(ts.URL+"/api/v1/endpoints", "application/json", strings.NewReader(`{"name":"openai","base_url":"https://other.api.com/v1","models":["tts-2"]}`))
	require.NoError(t, err); defer resp.Body.Close()
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestCreateEndpoint_InvalidJSON(t *testing.T) {
	ms := &endpointMockStore{}
	_, ts := newEndpointTestServer(ms); defer ts.Close()
	resp, err := http.Post(ts.URL+"/api/v1/endpoints", "application/json", strings.NewReader("{invalid"))
	require.NoError(t, err); defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestUpdateEndpoint_Valid(t *testing.T) {
	ms := &endpointMockStore{endpoints: []model.Endpoint{{ID: "ep1", Name: "OpenAI", BaseURL: "https://api.openai.com/v1", Models: model.StringSlice{"tts-1"}, Enabled: true, DefaultResponseFormat: "wav"}}}
	_, ts := newEndpointTestServer(ms); defer ts.Close()
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/endpoints/ep1", strings.NewReader(`{"name":"OpenAI Updated"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err); defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var body model.Endpoint
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "OpenAI Updated", body.Name)
}

func TestUpdateEndpoint_NotFound(t *testing.T) {
	ms := &endpointMockStore{}
	_, ts := newEndpointTestServer(ms); defer ts.Close()
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/endpoints/nonexistent", strings.NewReader(`{"name":"Updated"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err); defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestUpdateEndpoint_PartialUpdate(t *testing.T) {
	speed := 1.0
	ms := &endpointMockStore{endpoints: []model.Endpoint{{ID: "ep1", Name: "OpenAI", BaseURL: "https://api.openai.com/v1", APIKey: "sk-original", Models: model.StringSlice{"tts-1"}, DefaultSpeed: &speed, DefaultResponseFormat: "wav", Enabled: true}}}
	_, ts := newEndpointTestServer(ms); defer ts.Close()
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/endpoints/ep1", strings.NewReader(`{"base_url":"https://new.api.com/v1","enabled":false}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err); defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var body model.Endpoint
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "OpenAI", body.Name)
	assert.Equal(t, "https://new.api.com/v1", body.BaseURL)
	assert.False(t, body.Enabled)
}

func TestUpdateEndpoint_DuplicateName(t *testing.T) {
	ms := &endpointMockStore{endpoints: []model.Endpoint{
		{ID: "ep1", Name: "OpenAI", BaseURL: "https://api.openai.com/v1", Models: model.StringSlice{"tts-1"}, Enabled: true},
		{ID: "ep2", Name: "Local", BaseURL: "http://localhost:8000/v1", Models: model.StringSlice{"model-a"}, Enabled: true},
	}}
	_, ts := newEndpointTestServer(ms); defer ts.Close()
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/endpoints/ep2", strings.NewReader(`{"name":"OpenAI"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err); defer resp.Body.Close()
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

func TestUpdateEndpoint_SameNameSameEndpoint(t *testing.T) {
	ms := &endpointMockStore{endpoints: []model.Endpoint{{ID: "ep1", Name: "OpenAI", BaseURL: "https://api.openai.com/v1", Models: model.StringSlice{"tts-1"}, Enabled: true, DefaultResponseFormat: "wav"}}}
	_, ts := newEndpointTestServer(ms); defer ts.Close()
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/endpoints/ep1", strings.NewReader(`{"name":"OpenAI"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err); defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestUpdateEndpoint_InvalidSpeed(t *testing.T) {
	ms := &endpointMockStore{endpoints: []model.Endpoint{{ID: "ep1", Name: "OpenAI", BaseURL: "https://api.openai.com/v1", Models: model.StringSlice{"tts-1"}, Enabled: true}}}
	_, ts := newEndpointTestServer(ms); defer ts.Close()
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/endpoints/ep1", strings.NewReader(`{"default_speed":10.0}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err); defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestUpdateEndpoint_EmptyName(t *testing.T) {
	ms := &endpointMockStore{endpoints: []model.Endpoint{{ID: "ep1", Name: "OpenAI", BaseURL: "https://api.openai.com/v1", Models: model.StringSlice{"tts-1"}, Enabled: true}}}
	_, ts := newEndpointTestServer(ms); defer ts.Close()
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/endpoints/ep1", strings.NewReader(`{"name":""}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err); defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestUpdateEndpoint_EmptyModels(t *testing.T) {
	ms := &endpointMockStore{endpoints: []model.Endpoint{{ID: "ep1", Name: "OpenAI", BaseURL: "https://api.openai.com/v1", Models: model.StringSlice{"tts-1"}, Enabled: true}}}
	_, ts := newEndpointTestServer(ms); defer ts.Close()
	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/endpoints/ep1", strings.NewReader(`{"models":[]}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err); defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestDeleteEndpoint_Found(t *testing.T) {
	ms := &endpointMockStore{endpoints: []model.Endpoint{{ID: "ep1", Name: "OpenAI", BaseURL: "https://api.openai.com/v1", Models: model.StringSlice{"tts-1"}, Enabled: true}}}
	_, ts := newEndpointTestServer(ms); defer ts.Close()
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/endpoints/ep1", nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err); defer resp.Body.Close()
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Empty(t, ms.endpoints)
}

func TestDeleteEndpoint_NotFound(t *testing.T) {
	ms := &endpointMockStore{}
	_, ts := newEndpointTestServer(ms); defer ts.Close()
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/endpoints/nonexistent", nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err); defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestTestEndpoint_Success(t *testing.T) {
	mockTTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "audio/wav"); w.WriteHeader(http.StatusOK); _, _ = w.Write([]byte("RIFF....WAVEfmt"))
	}))
	defer mockTTS.Close()
	ms := &endpointMockStore{endpoints: []model.Endpoint{{ID: "ep1", Name: "Mock", BaseURL: mockTTS.URL, Models: model.StringSlice{"tts-1"}, Enabled: true}}}
	_, ts := newEndpointTestServer(ms); defer ts.Close()
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/endpoints/ep1/test", nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err); defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var body testEndpointResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.True(t, body.OK)
	assert.Empty(t, body.Error)
}

func TestTestEndpoint_Failure(t *testing.T) {
	mockTTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError); _, _ = w.Write([]byte("error"))
	}))
	defer mockTTS.Close()
	ms := &endpointMockStore{endpoints: []model.Endpoint{{ID: "ep1", Name: "Mock", BaseURL: mockTTS.URL, Models: model.StringSlice{"tts-1"}, Enabled: true}}}
	_, ts := newEndpointTestServer(ms); defer ts.Close()
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/endpoints/ep1/test", nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err); defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var body testEndpointResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.False(t, body.OK)
	assert.NotEmpty(t, body.Error)
}

func TestTestEndpoint_NotFound(t *testing.T) {
	ms := &endpointMockStore{}
	_, ts := newEndpointTestServer(ms); defer ts.Close()
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/endpoints/nonexistent/test", nil)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err); defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestDiscoverVoices_ReturnsModels(t *testing.T) {
	ms := &endpointMockStore{endpoints: []model.Endpoint{{ID: "ep1", Name: "OpenAI", BaseURL: "https://api.openai.com/v1", Models: model.StringSlice{"tts-1", "tts-1-hd", "gpt-4o-mini-tts"}, Enabled: true}}}
	_, ts := newEndpointTestServer(ms); defer ts.Close()
	resp, err := http.Get(ts.URL + "/api/v1/endpoints/ep1/voices")
	require.NoError(t, err); defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var body []string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, []string{"tts-1", "tts-1-hd", "gpt-4o-mini-tts"}, body)
}

func TestDiscoverVoices_NotFound(t *testing.T) {
	ms := &endpointMockStore{}
	_, ts := newEndpointTestServer(ms); defer ts.Close()
	resp, err := http.Get(ts.URL + "/api/v1/endpoints/nonexistent/voices")
	require.NoError(t, err); defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestCreateEndpoint_NilInfoBuilder(t *testing.T) {
	ms := &endpointMockStore{}
	_, ts := newEndpointTestServer(ms); defer ts.Close()
	resp, err := http.Post(ts.URL+"/api/v1/endpoints", "application/json", strings.NewReader(`{"name":"Test","base_url":"https://api.openai.com/v1","models":["tts-1"]}`))
	require.NoError(t, err); defer resp.Body.Close()
	assert.Equal(t, http.StatusCreated, resp.StatusCode)
}

func TestListEndpoints_ReturnsEmptyArray(t *testing.T) {
	ms := &endpointMockStore{}
	_, ts := newEndpointTestServer(ms); defer ts.Close()
	resp, err := http.Get(ts.URL + "/api/v1/endpoints")
	require.NoError(t, err); defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "[]")
}

func TestProbeEndpoint_Success(t *testing.T) {
	mockTTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/models":
			w.Write([]byte(`{"data":[{"id":"tts-1"},{"id":"tts-1-hd"}]}`))
		case "/audio/voices":
			w.Write([]byte(`{"data":[{"id":"alloy","name":"Alloy"},{"id":"nova","name":"Nova"}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockTTS.Close()
	ms := &endpointMockStore{}
	_, ts := newEndpointTestServer(ms); defer ts.Close()
	payload := fmt.Sprintf(`{"url":%q,"api_key":"sk-test"}`, mockTTS.URL)
	resp, err := http.Post(ts.URL+"/api/v1/endpoints/probe", "application/json", strings.NewReader(payload))
	require.NoError(t, err); defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var body struct {
		Models []tts.Model `json:"models"`
		Voices []tts.Voice `json:"voices"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Len(t, body.Models, 2)
	assert.Equal(t, "tts-1", body.Models[0].ID)
	require.Len(t, body.Voices, 2)
	assert.Equal(t, "alloy", body.Voices[0].ID)
}

func TestProbeEndpoint_MissingURL(t *testing.T) {
	ms := &endpointMockStore{}
	_, ts := newEndpointTestServer(ms); defer ts.Close()
	resp, err := http.Post(ts.URL+"/api/v1/endpoints/probe", "application/json", strings.NewReader(`{"api_key":"sk-test"}`))
	require.NoError(t, err); defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestProbeEndpoint_InvalidJSON(t *testing.T) {
	ms := &endpointMockStore{}
	_, ts := newEndpointTestServer(ms); defer ts.Close()
	resp, err := http.Post(ts.URL+"/api/v1/endpoints/probe", "application/json", strings.NewReader("{invalid"))
	require.NoError(t, err); defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestProbeEndpoint_UnreachableURL(t *testing.T) {
	ms := &endpointMockStore{}
	_, ts := newEndpointTestServer(ms); defer ts.Close()
	resp, err := http.Post(ts.URL+"/api/v1/endpoints/probe", "application/json", strings.NewReader(`{"url":"https://unreachable.example.com:0"}`))
	require.NoError(t, err); defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var body struct {
		Models []tts.Model `json:"models"`
		Voices []tts.Voice `json:"voices"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Empty(t, body.Models)
	assert.Empty(t, body.Voices)
}

func newSSRFTestServer(ms *endpointMockStore) (*Server, *httptest.Server) {
	srv := &Server{
		store: ms, version: "test", httpPort: 8080, dbDriver: "sqlite", startTime: time.Now(),
		webFS: &fstest.MapFS{"index.html": {Data: []byte("<html></html>")}},
		clientFactory: func(ep *model.Endpoint) *tts.Client { return tts.NewClient(ep.BaseURL, ep.APIKey, nil) },
		urlValidator:  ssrfValidator(),
	}
	return srv, httptest.NewServer(srv.setupRoutes())
}

func TestProbeEndpoint_SSRFBlocksPrivateIP(t *testing.T) {
	ms := &endpointMockStore{}
	_, ts := newSSRFTestServer(ms); defer ts.Close()
	resp, err := http.Post(ts.URL+"/api/v1/endpoints/probe", "application/json", strings.NewReader(`{"url":"http://127.0.0.1:8080/v1"}`))
	require.NoError(t, err); defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestProbeEndpoint_SSRFBlocksNonHTTPScheme(t *testing.T) {
	ms := &endpointMockStore{}
	_, ts := newSSRFTestServer(ms); defer ts.Close()
	resp, err := http.Post(ts.URL+"/api/v1/endpoints/probe", "application/json", strings.NewReader(`{"url":"ftp://files.example.com/data"}`))
	require.NoError(t, err); defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestProbeEndpoint_SSRFBlocksMetadataEndpoint(t *testing.T) {
	ms := &endpointMockStore{}
	_, ts := newSSRFTestServer(ms); defer ts.Close()
	resp, err := http.Post(ts.URL+"/api/v1/endpoints/probe", "application/json", strings.NewReader(`{"url":"http://169.254.169.254/latest/meta-data/"}`))
	require.NoError(t, err); defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestDiscoverModels_Success(t *testing.T) {
	mockTTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			w.Write([]byte(`{"data":[{"id":"tts-1"},{"id":"tts-1-hd"}]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockTTS.Close()
	ms := &endpointMockStore{endpoints: []model.Endpoint{{ID: "ep1", Name: "Mock", BaseURL: mockTTS.URL, Models: model.StringSlice{"tts-1"}, Enabled: true}}}
	_, ts := newEndpointTestServer(ms); defer ts.Close()
	resp, err := http.Get(ts.URL + "/api/v1/endpoints/ep1/models")
	require.NoError(t, err); defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var body []tts.Model
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Len(t, body, 2)
	assert.Equal(t, "tts-1", body[0].ID)
}

func TestDiscoverModels_NotFound(t *testing.T) {
	ms := &endpointMockStore{}
	_, ts := newEndpointTestServer(ms); defer ts.Close()
	resp, err := http.Get(ts.URL + "/api/v1/endpoints/nonexistent/models")
	require.NoError(t, err); defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestDiscoverRemoteVoices_Success(t *testing.T) {
	mockTTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/audio/voices" {
			w.Write([]byte(`{"data":[{"id":"alloy","name":"Alloy"}]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockTTS.Close()
	ms := &endpointMockStore{endpoints: []model.Endpoint{{ID: "ep1", Name: "Mock", BaseURL: mockTTS.URL, Models: model.StringSlice{"tts-1"}, Enabled: true}}}
	_, ts := newEndpointTestServer(ms); defer ts.Close()
	resp, err := http.Get(ts.URL + "/api/v1/endpoints/ep1/remote-voices")
	require.NoError(t, err); defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var body []tts.Voice
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Len(t, body, 1)
	assert.Equal(t, "alloy", body[0].ID)
	assert.Equal(t, "Alloy", body[0].Name)
}

func TestDiscoverRemoteVoices_NotFound(t *testing.T) {
	ms := &endpointMockStore{}
	_, ts := newEndpointTestServer(ms); defer ts.Close()
	resp, err := http.Get(ts.URL + "/api/v1/endpoints/nonexistent/remote-voices")
	require.NoError(t, err); defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}
