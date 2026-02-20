package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/fx/meadowlark/internal/model"
	"github.com/fx/meadowlark/internal/store"
	"github.com/fx/meadowlark/internal/tts"
	"github.com/fx/meadowlark/internal/wyoming"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupIntegrationTest creates a test server backed by a real in-memory SQLite
// store. Returns the server URL and a cleanup function.
func setupIntegrationTest(t *testing.T) (string, func()) {
	t.Helper()
	db, err := store.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	require.NoError(t, db.Migrate(context.Background()))

	ib := wyoming.NewInfoBuilder(db, db, nil, "test")
	webFS := fstest.MapFS{"index.html": {Data: []byte("<html>test</html>")}}

	srv := &Server{
		store:       db,
		infoBuilder: ib,
		clientFactory: func(ep *model.Endpoint) *tts.Client {
			return tts.NewClient("http://localhost:0", "", nil)
		},
		listenAddr:   ":0",
		startTime:    time.Now(),
		version:      "test",
		wyomingPort:  10300,
		httpPort:     8080,
		dbDriver:     "sqlite",
		webFS:        &webFS,
	}

	ts := httptest.NewServer(srv.setupRoutes())
	return ts.URL, func() {
		ts.Close()
		db.Close()
	}
}

// doJSON performs an HTTP request with an optional JSON body and returns the response.
func doJSON(t *testing.T, method, url string, body any) *http.Response {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, r)
	require.NoError(t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

// decodeJSON reads a JSON response body into v.
func decodeJSON(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	defer resp.Body.Close()
	require.NoError(t, json.NewDecoder(resp.Body).Decode(v))
}

func TestIntegration_EndpointCRUD(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	baseURL, cleanup := setupIntegrationTest(t)
	defer cleanup()

	// CREATE
	resp := doJSON(t, http.MethodPost, baseURL+"/api/v1/endpoints", map[string]any{
		"name":     "TestEndpoint",
		"base_url": "https://api.example.com/v1",
		"models":   []string{"tts-1", "tts-1-hd"},
	})
	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	var created model.Endpoint
	decodeJSON(t, resp, &created)
	assert.NotEmpty(t, created.ID)
	assert.Equal(t, "TestEndpoint", created.Name)
	assert.True(t, created.Enabled)

	epID := created.ID

	// LIST
	resp = doJSON(t, http.MethodGet, baseURL+"/api/v1/endpoints", nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var listed []model.Endpoint
	decodeJSON(t, resp, &listed)
	require.Len(t, listed, 1)
	assert.Equal(t, epID, listed[0].ID)

	// GET
	resp = doJSON(t, http.MethodGet, baseURL+"/api/v1/endpoints/"+epID, nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var fetched model.Endpoint
	decodeJSON(t, resp, &fetched)
	assert.Equal(t, "TestEndpoint", fetched.Name)
	assert.Equal(t, "https://api.example.com/v1", fetched.BaseURL)

	// UPDATE
	resp = doJSON(t, http.MethodPut, baseURL+"/api/v1/endpoints/"+epID, map[string]any{
		"name": "UpdatedEndpoint",
	})
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var updated model.Endpoint
	decodeJSON(t, resp, &updated)
	assert.Equal(t, "UpdatedEndpoint", updated.Name)
	assert.Equal(t, "https://api.example.com/v1", updated.BaseURL) // unchanged

	// DELETE
	resp = doJSON(t, http.MethodDelete, baseURL+"/api/v1/endpoints/"+epID, nil)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()

	// CONFIRM DELETED
	resp = doJSON(t, http.MethodGet, baseURL+"/api/v1/endpoints/"+epID, nil)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
}

func TestIntegration_AliasCRUD(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	baseURL, cleanup := setupIntegrationTest(t)
	defer cleanup()

	// Create an endpoint first (dependency for alias).
	resp := doJSON(t, http.MethodPost, baseURL+"/api/v1/endpoints", map[string]any{
		"name":     "AliasTestEP",
		"base_url": "https://api.example.com/v1",
		"models":   []string{"tts-1"},
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var ep model.Endpoint
	decodeJSON(t, resp, &ep)

	// CREATE ALIAS
	resp = doJSON(t, http.MethodPost, baseURL+"/api/v1/aliases", map[string]any{
		"name":        "narrator",
		"endpoint_id": ep.ID,
		"model":       "tts-1",
		"voice":       "nova",
	})
	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	var createdAlias model.VoiceAlias
	decodeJSON(t, resp, &createdAlias)
	assert.NotEmpty(t, createdAlias.ID)
	assert.Equal(t, "narrator", createdAlias.Name)
	assert.True(t, createdAlias.Enabled)

	aliasID := createdAlias.ID

	// LIST ALIASES
	resp = doJSON(t, http.MethodGet, baseURL+"/api/v1/aliases", nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var aliases []model.VoiceAlias
	decodeJSON(t, resp, &aliases)
	require.Len(t, aliases, 1)
	assert.Equal(t, aliasID, aliases[0].ID)

	// GET ALIAS
	resp = doJSON(t, http.MethodGet, baseURL+"/api/v1/aliases/"+aliasID, nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var fetchedAlias model.VoiceAlias
	decodeJSON(t, resp, &fetchedAlias)
	assert.Equal(t, "narrator", fetchedAlias.Name)
	assert.Equal(t, "nova", fetchedAlias.Voice)

	// UPDATE ALIAS
	resp = doJSON(t, http.MethodPut, baseURL+"/api/v1/aliases/"+aliasID, map[string]any{
		"voice": "alloy",
	})
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var updatedAlias model.VoiceAlias
	decodeJSON(t, resp, &updatedAlias)
	assert.Equal(t, "alloy", updatedAlias.Voice)
	assert.Equal(t, "narrator", updatedAlias.Name) // unchanged

	// DELETE ALIAS
	resp = doJSON(t, http.MethodDelete, baseURL+"/api/v1/aliases/"+aliasID, nil)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()

	// CONFIRM DELETED
	resp = doJSON(t, http.MethodGet, baseURL+"/api/v1/aliases/"+aliasID, nil)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
}

func TestIntegration_VoiceListRebuild(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	baseURL, cleanup := setupIntegrationTest(t)
	defer cleanup()

	// Create an endpoint.
	resp := doJSON(t, http.MethodPost, baseURL+"/api/v1/endpoints", map[string]any{
		"name":     "VoiceEP",
		"base_url": "https://api.example.com/v1",
		"models":   []string{"tts-1", "tts-1-hd"},
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var ep model.Endpoint
	decodeJSON(t, resp, &ep)

	// GET voices -- should contain canonical entries for the endpoint's models.
	resp = doJSON(t, http.MethodGet, baseURL+"/api/v1/voices", nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var voices []voiceEntry
	decodeJSON(t, resp, &voices)
	require.Len(t, voices, 2)
	assert.False(t, voices[0].IsAlias)
	assert.False(t, voices[1].IsAlias)

	// Create an alias.
	resp = doJSON(t, http.MethodPost, baseURL+"/api/v1/aliases", map[string]any{
		"name":        "my-narrator",
		"endpoint_id": ep.ID,
		"model":       "tts-1",
		"voice":       "nova",
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var alias model.VoiceAlias
	decodeJSON(t, resp, &alias)

	// GET voices -- should now include the alias.
	resp = doJSON(t, http.MethodGet, baseURL+"/api/v1/voices", nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	decodeJSON(t, resp, &voices)
	require.Len(t, voices, 3)

	// Find the alias entry.
	var found bool
	for _, v := range voices {
		if v.Name == "my-narrator" && v.IsAlias {
			found = true
			break
		}
	}
	assert.True(t, found, "alias should appear in voices list")

	// Delete the alias.
	resp = doJSON(t, http.MethodDelete, baseURL+"/api/v1/aliases/"+alias.ID, nil)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	resp.Body.Close()

	// GET voices -- alias should be gone.
	resp = doJSON(t, http.MethodGet, baseURL+"/api/v1/voices", nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	decodeJSON(t, resp, &voices)
	require.Len(t, voices, 2)
	for _, v := range voices {
		assert.False(t, v.IsAlias, "no aliases should remain")
	}
}

func TestIntegration_ConcurrentEndpointCreation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	baseURL, cleanup := setupIntegrationTest(t)
	defer cleanup()

	const n = 10
	var wg sync.WaitGroup
	errs := make(chan error, n)

	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			payload := map[string]any{
				"name":     "Endpoint-" + string(rune('A'+idx)),
				"base_url": "https://api.example.com/v1",
				"models":   []string{"tts-1"},
			}
			resp := doJSON(t, http.MethodPost, baseURL+"/api/v1/endpoints", payload)
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusCreated {
				body, _ := io.ReadAll(resp.Body)
				errs <- &httpError{status: resp.StatusCode, body: string(body)}
			}
		}(i)
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent create failed: %v", err)
	}

	// Verify all endpoints were created.
	resp := doJSON(t, http.MethodGet, baseURL+"/api/v1/endpoints", nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var endpoints []model.Endpoint
	decodeJSON(t, resp, &endpoints)
	assert.Len(t, endpoints, n)
}

type httpError struct {
	status int
	body   string
}

func (e *httpError) Error() string {
	return "HTTP " + http.StatusText(e.status) + ": " + e.body
}

func TestIntegration_SPAFallback(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	baseURL, cleanup := setupIntegrationTest(t)
	defer cleanup()

	// Root serves HTML.
	resp, err := http.Get(baseURL + "/")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body), "<html>")

	// SPA fallback for unknown frontend route.
	resp2, err := http.Get(baseURL + "/settings/general")
	require.NoError(t, err)
	defer resp2.Body.Close()
	assert.Equal(t, http.StatusOK, resp2.StatusCode)
	body2, err := io.ReadAll(resp2.Body)
	require.NoError(t, err)
	assert.Contains(t, string(body2), "<html>")

	// Non-existent asset still gets SPA fallback (since no real assets dir exists).
	resp3, err := http.Get(baseURL + "/assets/nonexistent.js")
	require.NoError(t, err)
	defer resp3.Body.Close()
	// SPA fallback serves index.html for unknown paths.
	assert.Equal(t, http.StatusOK, resp3.StatusCode)
}
