package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fx/meadowlark/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every list response must encode an empty result as [], never null: the UI
// treats a null body as "still loading".
func TestListResponses_EmptyIsArray(t *testing.T) {
	// An upstream with no models or voices: every discovery call 404s.
	upstream := httptest.NewServer(http.NotFoundHandler())
	defer upstream.Close()
	endpointServer := func(ms *endpointMockStore) *Server {
		srv, ts := newEndpointTestServer(ms)
		ts.Close()
		return srv
	}
	withEndpoint := func() *Server {
		return endpointServer(&endpointMockStore{endpoints: []model.Endpoint{{ID: "ep1", Name: "EP", BaseURL: upstream.URL, Enabled: true}}})
	}
	noEndpoints := func() *Server { return endpointServer(&endpointMockStore{}) }

	tests := []struct {
		name   string
		server func() *Server
		method string
		path   string
		body   string
		want   string
	}{
		{
			name:   "aliases",
			server: func() *Server { return newAliasTestServer(&aliasStore{}) },
			method: http.MethodGet, path: "/api/v1/aliases", want: `[]`,
		},
		{
			name:   "endpoints",
			server: noEndpoints,
			method: http.MethodGet, path: "/api/v1/endpoints", want: `[]`,
		},
		{
			name:   "endpoint voices",
			server: withEndpoint,
			method: http.MethodGet, path: "/api/v1/endpoints/ep1/voices", want: `[]`,
		},
		{
			name:   "refreshed endpoint voices",
			server: withEndpoint,
			method: http.MethodPost, path: "/api/v1/endpoints/ep1/voices/refresh", want: `[]`,
		},
		{
			name:   "configured models",
			server: withEndpoint,
			method: http.MethodGet, path: "/api/v1/endpoints/ep1/configured-models", want: `[]`,
		},
		{
			name:   "discovered models",
			server: withEndpoint,
			method: http.MethodGet, path: "/api/v1/endpoints/ep1/models", want: `[]`,
		},
		{
			name:   "discovered remote voices",
			server: withEndpoint,
			method: http.MethodGet, path: "/api/v1/endpoints/ep1/remote-voices", want: `[]`,
		},
		{
			name:   "probe",
			server: noEndpoints,
			method: http.MethodPost, path: "/api/v1/endpoints/probe", body: `{"url":"` + upstream.URL + `"}`,
			want: `{"models":[],"voices":[]}`,
		},
		{
			name:   "voices",
			server: func() *Server { return newSystemTestServer(&mockStore{}) },
			method: http.MethodGet, path: "/api/v1/voices", want: `[]`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := httptest.NewServer(tt.server().setupRoutes())
			defer ts.Close()

			req, err := http.NewRequest(tt.method, ts.URL+tt.path, strings.NewReader(tt.body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			require.Equal(t, http.StatusOK, resp.StatusCode)
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			assert.JSONEq(t, tt.want, string(body))
		})
	}
}

// List fields nested in an object encode as [] too.
func TestListResponses_NestedListsAreArrays(t *testing.T) {
	st := &aliasStore{aliases: []model.VoiceAlias{{ID: "a1", Name: "narrator", EndpointID: "ep1", Model: "tts-1", Voice: "nova"}}}
	ts := httptest.NewServer(newAliasTestServer(st).setupRoutes())
	defer ts.Close()

	resp := makeAliasRequest(t, ts, http.MethodGet, "/api/v1/aliases/a1", nil)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), `"languages":[]`)
}
