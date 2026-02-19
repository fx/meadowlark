package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRespondJSON(t *testing.T) {
	w := httptest.NewRecorder()
	respondJSON(w, http.StatusOK, map[string]string{"hello": "world"})

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var body map[string]string
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "world", body["hello"])
}

func TestRespondJSON_Created(t *testing.T) {
	w := httptest.NewRecorder()
	respondJSON(w, http.StatusCreated, map[string]int{"id": 1})

	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestRespondError(t *testing.T) {
	w := httptest.NewRecorder()
	respondError(w, http.StatusNotFound, "not_found", "item not found")

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var body map[string]apiError
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "not_found", body["error"].Code)
	assert.Equal(t, "item not found", body["error"].Message)
}

func TestRespondError_BadRequest(t *testing.T) {
	w := httptest.NewRecorder()
	respondError(w, http.StatusBadRequest, "bad_request", "invalid input")

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var body map[string]apiError
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "bad_request", body["error"].Code)
}

func TestRespondError_Conflict(t *testing.T) {
	w := httptest.NewRecorder()
	respondError(w, http.StatusConflict, "conflict", "already exists")

	assert.Equal(t, http.StatusConflict, w.Code)

	var body map[string]apiError
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "conflict", body["error"].Code)
}

func TestRespondError_InternalError(t *testing.T) {
	w := httptest.NewRecorder()
	respondError(w, http.StatusInternalServerError, "internal_error", "something broke")

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var body map[string]apiError
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "internal_error", body["error"].Code)
}

func TestRespondNoContent(t *testing.T) {
	w := httptest.NewRecorder()
	respondNoContent(w)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Empty(t, w.Body.Bytes())
}
