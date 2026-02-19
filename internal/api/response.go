package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// apiError is the JSON error envelope returned by the API.
type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// respondJSON writes v as JSON with the given HTTP status code.
func respondJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to encode JSON response", "error", err)
	}
}

// respondError writes a JSON error response.
func respondError(w http.ResponseWriter, status int, code, message string) {
	respondJSON(w, status, map[string]apiError{
		"error": {Code: code, Message: message},
	})
}

// respondNoContent writes a 204 No Content response.
func respondNoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}
