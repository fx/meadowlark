package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/fx/meadowlark/internal/model"
	"github.com/fx/meadowlark/internal/tts"
)

// ListEndpoints returns all endpoints.
func (s *Server) ListEndpoints(w http.ResponseWriter, r *http.Request) {
	endpoints, err := s.store.ListEndpoints(r.Context())
	if err != nil {
		slog.Error("list endpoints", "error", err)
		respondError(w, http.StatusInternalServerError, "internal_error", "failed to list endpoints")
		return
	}
	if endpoints == nil {
		endpoints = []model.Endpoint{}
	}
	respondJSON(w, http.StatusOK, endpoints)
}

// GetEndpoint returns a single endpoint by ID.
func (s *Server) GetEndpoint(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ep, err := s.store.GetEndpoint(r.Context(), id)
	if err != nil {
		slog.Error("get endpoint", "error", err)
		respondError(w, http.StatusInternalServerError, "internal_error", "failed to get endpoint")
		return
	}
	if ep == nil {
		respondError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	respondJSON(w, http.StatusOK, ep)
}

type createEndpointRequest struct {
	Name                  string            `json:"name"`
	BaseURL               string            `json:"base_url"`
	APIKey                string            `json:"api_key"`
	Models                model.StringSlice `json:"models"`
	DefaultModel          string            `json:"default_model"`
	DefaultVoice          string            `json:"default_voice"`
	DefaultSpeed          *float64          `json:"default_speed"`
	DefaultInstructions   *string           `json:"default_instructions"`
	DefaultResponseFormat string            `json:"default_response_format"`
	Enabled               *bool             `json:"enabled"`
	StreamingEnabled      *bool             `json:"streaming_enabled,omitempty"`
	StreamSampleRate      *int              `json:"stream_sample_rate,omitempty"`
}

// CreateEndpoint creates a new endpoint.
func (s *Server) CreateEndpoint(w http.ResponseWriter, r *http.Request) {
	var req createEndpointRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		respondError(w, http.StatusBadRequest, "bad_request", "name is required")
		return
	}
	if strings.TrimSpace(req.BaseURL) == "" {
		respondError(w, http.StatusBadRequest, "bad_request", "base_url is required")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.BaseURL = strings.TrimSpace(req.BaseURL)
	if _, err := url.ParseRequestURI(req.BaseURL); err != nil {
		respondError(w, http.StatusBadRequest, "bad_request", "base_url is not a valid URL")
		return
	}
	if len(req.Models) == 0 {
		respondError(w, http.StatusBadRequest, "bad_request", "models is required and must be non-empty")
		return
	}
	if req.DefaultModel != "" && !containsString(req.Models, req.DefaultModel) {
		respondError(w, http.StatusBadRequest, "invalid_default_model", "default_model must be one of the configured models")
		return
	}
	if req.DefaultSpeed != nil && (*req.DefaultSpeed < 0.25 || *req.DefaultSpeed > 4.0) {
		respondError(w, http.StatusBadRequest, "bad_request", "default_speed must be between 0.25 and 4.0")
		return
	}
	if req.StreamSampleRate != nil && (*req.StreamSampleRate < 8000 || *req.StreamSampleRate > 48000) {
		respondError(w, http.StatusBadRequest, "bad_request", "stream_sample_rate must be between 8000 and 48000")
		return
	}
	existing, err := s.store.ListEndpoints(r.Context())
	if err != nil {
		slog.Error("create endpoint: list for duplicate check", "error", err)
		respondError(w, http.StatusInternalServerError, "internal_error", "failed to check for duplicate name")
		return
	}
	for _, ep := range existing {
		if strings.EqualFold(ep.Name, req.Name) {
			respondError(w, http.StatusConflict, "conflict", "an endpoint with this name already exists")
			return
		}
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	responseFormat := "wav"
	if req.DefaultResponseFormat != "" {
		responseFormat = req.DefaultResponseFormat
	}
	now := time.Now().UTC()
	ep := &model.Endpoint{
		ID: uuid.New().String(), Name: req.Name, BaseURL: req.BaseURL, APIKey: req.APIKey,
		Models: req.Models, DefaultModel: req.DefaultModel, DefaultVoice: req.DefaultVoice, DefaultSpeed: req.DefaultSpeed, DefaultInstructions: req.DefaultInstructions,
		DefaultResponseFormat: responseFormat, Enabled: enabled, CreatedAt: now, UpdatedAt: now,
	}
	if req.StreamingEnabled != nil {
		ep.StreamingEnabled = *req.StreamingEnabled
	}
	if req.StreamSampleRate != nil {
		ep.StreamSampleRate = *req.StreamSampleRate
	}
	if err := s.store.CreateEndpoint(r.Context(), ep); err != nil {
		slog.Error("create endpoint", "error", err)
		respondError(w, http.StatusInternalServerError, "internal_error", "failed to create endpoint")
		return
	}
	s.rebuildVoiceList(r.Context())
	respondJSON(w, http.StatusCreated, ep)
}

type updateEndpointRequest struct {
	Name                  *string            `json:"name"`
	BaseURL               *string            `json:"base_url"`
	APIKey                *string            `json:"api_key"`
	Models                *model.StringSlice `json:"models"`
	DefaultModel          *string            `json:"default_model"`
	DefaultVoice          *string            `json:"default_voice"`
	DefaultSpeed          *float64           `json:"default_speed"`
	DefaultInstructions   *string            `json:"default_instructions"`
	DefaultResponseFormat *string            `json:"default_response_format"`
	Enabled               *bool              `json:"enabled"`
	StreamingEnabled      *bool              `json:"streaming_enabled,omitempty"`
	StreamSampleRate      *int               `json:"stream_sample_rate,omitempty"`
}

// UpdateEndpoint updates an existing endpoint.
func (s *Server) UpdateEndpoint(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ep, err := s.store.GetEndpoint(r.Context(), id)
	if err != nil {
		slog.Error("update endpoint: get", "error", err)
		respondError(w, http.StatusInternalServerError, "internal_error", "failed to get endpoint")
		return
	}
	if ep == nil {
		respondError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	var req updateEndpointRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	if req.Name != nil {
		trimmedName := strings.TrimSpace(*req.Name)
		if trimmedName == "" {
			respondError(w, http.StatusBadRequest, "bad_request", "name cannot be empty")
			return
		}
		existing, err := s.store.ListEndpoints(r.Context())
		if err != nil {
			slog.Error("update endpoint: list for duplicate check", "error", err)
			respondError(w, http.StatusInternalServerError, "internal_error", "failed to check for duplicate name")
			return
		}
		for _, e := range existing {
			if strings.EqualFold(e.Name, trimmedName) && e.ID != id {
				respondError(w, http.StatusConflict, "conflict", "an endpoint with this name already exists")
				return
			}
		}
		ep.Name = strings.TrimSpace(*req.Name)
	}
	if req.BaseURL != nil {
		trimmedURL := strings.TrimSpace(*req.BaseURL)
		if trimmedURL == "" {
			respondError(w, http.StatusBadRequest, "bad_request", "base_url cannot be empty")
			return
		}
		if _, err := url.ParseRequestURI(trimmedURL); err != nil {
			respondError(w, http.StatusBadRequest, "bad_request", "base_url is not a valid URL")
			return
		}
		ep.BaseURL = trimmedURL
	}
	if req.APIKey != nil {
		ep.APIKey = *req.APIKey
	}
	if req.Models != nil {
		if len(*req.Models) == 0 {
			respondError(w, http.StatusBadRequest, "bad_request", "models must be non-empty")
			return
		}
		ep.Models = *req.Models
	}
	if req.DefaultModel != nil {
		ep.DefaultModel = *req.DefaultModel
	}
	if ep.DefaultModel != "" && !containsString(ep.Models, ep.DefaultModel) {
		respondError(w, http.StatusBadRequest, "invalid_default_model", "default_model must be one of the configured models")
		return
	}
	if req.DefaultVoice != nil {
		ep.DefaultVoice = *req.DefaultVoice
	}
	if req.DefaultSpeed != nil {
		if *req.DefaultSpeed < 0.25 || *req.DefaultSpeed > 4.0 {
			respondError(w, http.StatusBadRequest, "bad_request", "default_speed must be between 0.25 and 4.0")
			return
		}
		ep.DefaultSpeed = req.DefaultSpeed
	}
	if req.DefaultInstructions != nil {
		ep.DefaultInstructions = req.DefaultInstructions
	}
	if req.DefaultResponseFormat != nil {
		ep.DefaultResponseFormat = *req.DefaultResponseFormat
	}
	if req.Enabled != nil {
		ep.Enabled = *req.Enabled
	}
	if req.StreamSampleRate != nil && (*req.StreamSampleRate < 8000 || *req.StreamSampleRate > 48000) {
		respondError(w, http.StatusBadRequest, "bad_request", "stream_sample_rate must be between 8000 and 48000")
		return
	}
	if req.StreamingEnabled != nil {
		ep.StreamingEnabled = *req.StreamingEnabled
	}
	if req.StreamSampleRate != nil {
		ep.StreamSampleRate = *req.StreamSampleRate
	}
	if err := s.store.UpdateEndpoint(r.Context(), ep); err != nil {
		slog.Error("update endpoint", "error", err)
		respondError(w, http.StatusInternalServerError, "internal_error", "failed to update endpoint")
		return
	}
	s.rebuildVoiceList(r.Context())
	respondJSON(w, http.StatusOK, ep)
}

// DeleteEndpoint deletes an endpoint by ID.
func (s *Server) DeleteEndpoint(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ep, err := s.store.GetEndpoint(r.Context(), id)
	if err != nil {
		slog.Error("delete endpoint: get", "error", err)
		respondError(w, http.StatusInternalServerError, "internal_error", "failed to get endpoint")
		return
	}
	if ep == nil {
		respondError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	if err := s.store.DeleteEndpoint(r.Context(), id); err != nil {
		slog.Error("delete endpoint", "error", err)
		respondError(w, http.StatusInternalServerError, "internal_error", "failed to delete endpoint")
		return
	}
	s.rebuildVoiceList(r.Context())
	respondNoContent(w)
}

type testEndpointResponse struct {
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
	LatencyMS int64  `json:"latency_ms"`
}

// TestEndpoint tests connectivity to an endpoint by making a small TTS request.
func (s *Server) TestEndpoint(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ep, err := s.store.GetEndpoint(r.Context(), id)
	if err != nil {
		slog.Error("test endpoint: get", "error", err)
		respondError(w, http.StatusInternalServerError, "internal_error", "failed to get endpoint")
		return
	}
	if ep == nil {
		respondError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	client := s.clientFactory(ep)
	start := time.Now()
	body, synthErr := client.Synthesize(r.Context(), &tts.SynthesizeRequest{
		Model: ep.EffectiveDefaultModel(), Voice: "test", Input: "test", ResponseFormat: "wav",
	})
	latency := time.Since(start).Milliseconds()
	if synthErr != nil {
		respondJSON(w, http.StatusOK, testEndpointResponse{OK: false, Error: synthErr.Error(), LatencyMS: latency})
		return
	}
	body.Close()
	respondJSON(w, http.StatusOK, testEndpointResponse{OK: true, LatencyMS: latency})
}

// ListEndpointConfiguredModels returns the configured models list for an endpoint.
func (s *Server) ListEndpointConfiguredModels(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ep, err := s.store.GetEndpoint(r.Context(), id)
	if err != nil {
		slog.Error("list endpoint configured models: get", "error", err)
		respondError(w, http.StatusInternalServerError, "internal_error", "failed to get endpoint")
		return
	}
	if ep == nil {
		respondError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	respondJSON(w, http.StatusOK, []string(ep.Models))
}

// DiscoverModels queries a saved endpoint for available models.
func (s *Server) DiscoverModels(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ep, err := s.store.GetEndpoint(r.Context(), id)
	if err != nil {
		slog.Error("discover models: get", "error", err)
		respondError(w, http.StatusInternalServerError, "internal_error", "failed to get endpoint")
		return
	}
	if ep == nil {
		respondError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	client := s.clientFactory(ep)
	models, _ := client.ListModels(r.Context())
	respondJSON(w, http.StatusOK, models)
}

// DiscoverRemoteVoices queries a saved endpoint for available voices.
func (s *Server) DiscoverRemoteVoices(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ep, err := s.store.GetEndpoint(r.Context(), id)
	if err != nil {
		slog.Error("discover remote voices: get", "error", err)
		respondError(w, http.StatusInternalServerError, "internal_error", "failed to get endpoint")
		return
	}
	if ep == nil {
		respondError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	client := s.clientFactory(ep)
	voices, _ := client.ListVoices(r.Context())
	respondJSON(w, http.StatusOK, voices)
}

type probeRequest struct {
	URL    string `json:"url"`
	APIKey string `json:"api_key"`
}

type probeResponse struct {
	Models []tts.Model `json:"models"`
	Voices []tts.Voice `json:"voices"`
}

// containsString reports whether ss contains s.
func containsString(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

// ProbeEndpoint probes a remote endpoint for models and voices without saving it.
func (s *Server) ProbeEndpoint(w http.ResponseWriter, r *http.Request) {
	var req probeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}
	trimmedURL := strings.TrimSpace(req.URL)
	if trimmedURL == "" {
		respondError(w, http.StatusBadRequest, "bad_request", "url is required")
		return
	}

	client := tts.NewClient(trimmedURL, req.APIKey, nil)
	models, _ := client.ListModels(r.Context())
	voices, _ := client.ListVoices(r.Context())

	respondJSON(w, http.StatusOK, probeResponse{Models: models, Voices: voices})
}

