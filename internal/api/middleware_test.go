package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestLogger(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	slog.SetDefault(logger)
	defer slog.SetDefault(slog.Default())

	handler := requestLogger(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	logOutput := buf.String()
	assert.Contains(t, logOutput, "http request")
	assert.Contains(t, logOutput, "GET")
	assert.Contains(t, logOutput, "/api/v1/status")
}

func TestRecovery(t *testing.T) {
	handler := recovery(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		panic("test panic")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var body map[string]apiError
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "internal_error", body["error"].Code)
}

func TestRecovery_NoPanic(t *testing.T) {
	handler := recovery(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestCORS(t *testing.T) {
	handler := cors(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "GET")
	assert.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "POST")
	assert.Contains(t, w.Header().Get("Access-Control-Allow-Headers"), "Content-Type")
}

func TestCORS_Preflight(t *testing.T) {
	handler := cors(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/endpoints", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
}

func TestJSONContentType(t *testing.T) {
	handler := jsonContentType(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
}

func TestStatusWriter_Unwrap(t *testing.T) {
	inner := httptest.NewRecorder()
	sw := &statusWriter{ResponseWriter: inner, code: http.StatusOK}
	assert.Same(t, inner, sw.Unwrap())
}

func TestRequestLogger_ResponseControllerReachesConnection(t *testing.T) {
	// Through a real server, SetWriteDeadline only succeeds if the request
	// logger's writer unwraps to the connection's writer.
	var deadlineErr error
	srv := httptest.NewServer(requestLogger(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		deadlineErr = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(time.Minute))
		w.WriteHeader(http.StatusNoContent)
	})))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	require.NoError(t, err)
	resp.Body.Close()
	assert.NoError(t, deadlineErr)
}
