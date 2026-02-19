package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestServer(webFS *fstest.MapFS) *Server {
	return &Server{
		store:     &mockStore{},
		version:   "test",
		httpPort:  8080,
		webFS:     webFS,
		dbDriver:  "sqlite",
		startTime: time.Now(),
	}
}

// TestAPIRoutes_NotImplemented verifies all stub routes return 501.
func TestAPIRoutes_NotImplemented(t *testing.T) {
	webFS := &fstest.MapFS{
		"index.html": {Data: []byte("<html></html>")},
	}
	srv := newTestServer(webFS)
	router := srv.setupRoutes()
	ts := httptest.NewServer(router)
	defer ts.Close()

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/endpoints"},
		{http.MethodPost, "/api/v1/endpoints"},
		{http.MethodGet, "/api/v1/endpoints/123"},
		{http.MethodPut, "/api/v1/endpoints/123"},
		{http.MethodDelete, "/api/v1/endpoints/123"},
		{http.MethodPost, "/api/v1/endpoints/123/test"},
		{http.MethodGet, "/api/v1/endpoints/123/voices"},
		{http.MethodGet, "/api/v1/aliases"},
		{http.MethodPost, "/api/v1/aliases"},
		{http.MethodGet, "/api/v1/aliases/456"},
		{http.MethodPut, "/api/v1/aliases/456"},
		{http.MethodDelete, "/api/v1/aliases/456"},
		{http.MethodPost, "/api/v1/aliases/456/test"},
	}

	for _, rt := range routes {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			req, err := http.NewRequest(rt.method, ts.URL+rt.path, nil)
			require.NoError(t, err)

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusNotImplemented, resp.StatusCode)

			var body map[string]apiError
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
			assert.Equal(t, "not_implemented", body["error"].Code)
		})
	}
}

// TestSPAFallback_IndexHTML verifies root serves index.html.
func TestSPAFallback_IndexHTML(t *testing.T) {
	webFS := &fstest.MapFS{
		"index.html": {Data: []byte("<html>SPA</html>")},
	}
	srv := newTestServer(webFS)
	router := srv.setupRoutes()
	ts := httptest.NewServer(router)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestSPAFallback_RealFile serves a real static file.
func TestSPAFallback_RealFile(t *testing.T) {
	webFS := &fstest.MapFS{
		"index.html":          {Data: []byte("<html>SPA</html>")},
		"assets/index-abc.js": {Data: []byte("console.log('app')")},
	}
	srv := newTestServer(webFS)
	router := srv.setupRoutes()
	ts := httptest.NewServer(router)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/assets/index-abc.js")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestSPAFallback_UnknownPath falls back to index.html.
func TestSPAFallback_UnknownPath(t *testing.T) {
	webFS := &fstest.MapFS{
		"index.html": {Data: []byte("<html>SPA</html>")},
	}
	srv := newTestServer(webFS)
	router := srv.setupRoutes()
	ts := httptest.NewServer(router)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/settings/general")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestCORSPreflight verifies OPTIONS returns 204 with CORS headers.
func TestCORSPreflight(t *testing.T) {
	webFS := &fstest.MapFS{
		"index.html": {Data: []byte("<html></html>")},
	}
	srv := newTestServer(webFS)
	router := srv.setupRoutes()
	ts := httptest.NewServer(router)
	defer ts.Close()

	req, err := http.NewRequest(http.MethodOptions, ts.URL+"/api/v1/endpoints", nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	assert.Equal(t, "*", resp.Header.Get("Access-Control-Allow-Origin"))
}
