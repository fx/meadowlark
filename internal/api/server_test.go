package api

import (
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

func newTestServer(webFS *fstest.MapFS) *Server {
	return &Server{
		store:        &mockStore{},
		version:      "test",
		httpPort:     8080,
		webFS:        webFS,
		dbDriver:     "sqlite",
		startTime:    time.Now(),
		urlValidator: noopValidator,
		clientFactory: func(ep *model.Endpoint) *tts.Client {
			return tts.NewClient(ep.BaseURL, ep.APIKey, nil)
		},
	}
}

// All API routes are now implemented — no more not-implemented stubs to test.

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
