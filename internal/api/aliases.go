package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/fx/meadowlark/internal/model"
	"github.com/fx/meadowlark/internal/tts"
)

// aliasRequest is the JSON body for create/update alias requests.
type aliasRequest struct {
	Name         string            `json:"name"`
	EndpointID   string            `json:"endpoint_id"`
	Model        string            `json:"model"`
	Voice        string            `json:"voice"`
	Speed        *float64          `json:"speed,omitempty"`
	Instructions *string           `json:"instructions,omitempty"`
	Languages    model.StringSlice `json:"languages,omitempty"`
	Enabled      *bool             `json:"enabled,omitempty"`
}

// testAliasRequest is the JSON body for POST /aliases/{id}/test.
type testAliasRequest struct {
	Text string `json:"text,omitempty"`
}

// ListAliases returns all voice aliases.
func (s *Server) ListAliases(w http.ResponseWriter, r *http.Request) {
	aliases, err := s.store.ListVoiceAliases(r.Context())
	if err != nil {
		slog.Error("list aliases", "error", err)
		respondError(w, http.StatusInternalServerError, "internal_error", "failed to list aliases")
		return
	}
	respondJSON(w, http.StatusOK, aliases)
}

// GetAlias returns a single voice alias by ID.
func (s *Server) GetAlias(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	alias, err := s.store.GetVoiceAlias(r.Context(), id)
	if err != nil {
		slog.Error("get alias", "error", err, "id", id)
		respondError(w, http.StatusNotFound, "not_found", "alias not found")
		return
	}
	if alias == nil {
		respondError(w, http.StatusNotFound, "not_found", "alias not found")
		return
	}
	respondJSON(w, http.StatusOK, alias)
}

// CreateAlias creates a new voice alias.
func (s *Server) CreateAlias(w http.ResponseWriter, r *http.Request) {
	var req aliasRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		respondError(w, http.StatusBadRequest, "bad_request", "name is required")
		return
	}
	if strings.TrimSpace(req.EndpointID) == "" {
		respondError(w, http.StatusBadRequest, "bad_request", "endpoint_id is required")
		return
	}
	if strings.TrimSpace(req.Model) == "" {
		respondError(w, http.StatusBadRequest, "bad_request", "model is required")
		return
	}
	if strings.TrimSpace(req.Voice) == "" {
		respondError(w, http.StatusBadRequest, "bad_request", "voice is required")
		return
	}
	if req.Speed != nil && (*req.Speed < 0.25 || *req.Speed > 4.0) {
		respondError(w, http.StatusBadRequest, "bad_request", "speed must be between 0.25 and 4.0")
		return
	}

	// Verify endpoint exists.
	if _, err := s.store.GetEndpoint(r.Context(), req.EndpointID); err != nil {
		respondError(w, http.StatusBadRequest, "bad_request", "endpoint not found")
		return
	}

	// Check name uniqueness.
	existing, err := s.store.ListVoiceAliases(r.Context())
	if err != nil {
		slog.Error("list aliases for uniqueness check", "error", err)
		respondError(w, http.StatusInternalServerError, "internal_error", "failed to check alias uniqueness")
		return
	}
	for _, a := range existing {
		if strings.EqualFold(a.Name, req.Name) {
			respondError(w, http.StatusConflict, "conflict", "alias with this name already exists")
			return
		}
	}

	now := time.Now()
	alias := &model.VoiceAlias{
		ID:           uuid.New().String(),
		Name:         req.Name,
		EndpointID:   req.EndpointID,
		Model:        req.Model,
		Voice:        req.Voice,
		Speed:        req.Speed,
		Instructions: req.Instructions,
		Languages:    req.Languages,
		Enabled:      true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.store.CreateVoiceAlias(r.Context(), alias); err != nil {
		slog.Error("create alias", "error", err)
		respondError(w, http.StatusInternalServerError, "internal_error", "failed to create alias")
		return
	}

	s.rebuildVoiceList(r.Context())
	respondJSON(w, http.StatusCreated, alias)
}

// UpdateAlias updates an existing voice alias.
func (s *Server) UpdateAlias(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	alias, err := s.store.GetVoiceAlias(r.Context(), id)
	if err != nil {
		respondError(w, http.StatusNotFound, "not_found", "alias not found")
		return
	}

	var req aliasRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	if req.Speed != nil && (*req.Speed < 0.25 || *req.Speed > 4.0) {
		respondError(w, http.StatusBadRequest, "bad_request", "speed must be between 0.25 and 4.0")
		return
	}

	if req.Name != "" {
		// Check name uniqueness (excluding self).
		existing, err := s.store.ListVoiceAliases(r.Context())
		if err != nil {
			slog.Error("list aliases for uniqueness check", "error", err)
			respondError(w, http.StatusInternalServerError, "internal_error", "failed to check alias uniqueness")
			return
		}
		for _, a := range existing {
			if strings.EqualFold(a.Name, req.Name) && a.ID != id {
				respondError(w, http.StatusConflict, "conflict", "alias with this name already exists")
				return
			}
		}
		alias.Name = req.Name
	}
	if req.EndpointID != "" {
		if _, err := s.store.GetEndpoint(r.Context(), req.EndpointID); err != nil {
			respondError(w, http.StatusBadRequest, "bad_request", "endpoint not found")
			return
		}
		alias.EndpointID = req.EndpointID
	}
	if req.Model != "" {
		alias.Model = req.Model
	}
	if req.Voice != "" {
		alias.Voice = req.Voice
	}
	if req.Speed != nil {
		alias.Speed = req.Speed
	}
	if req.Instructions != nil {
		alias.Instructions = req.Instructions
	}
	if req.Languages != nil {
		alias.Languages = req.Languages
	}
	if req.Enabled != nil {
		alias.Enabled = *req.Enabled
	}

	alias.UpdatedAt = time.Now()

	if err := s.store.UpdateVoiceAlias(r.Context(), alias); err != nil {
		slog.Error("update alias", "error", err)
		respondError(w, http.StatusInternalServerError, "internal_error", "failed to update alias")
		return
	}

	s.rebuildVoiceList(r.Context())
	respondJSON(w, http.StatusOK, alias)
}

// DeleteAlias deletes a voice alias.
func (s *Server) DeleteAlias(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := s.store.GetVoiceAlias(r.Context(), id); err != nil {
		respondError(w, http.StatusNotFound, "not_found", "alias not found")
		return
	}

	if err := s.store.DeleteVoiceAlias(r.Context(), id); err != nil {
		slog.Error("delete alias", "error", err)
		respondError(w, http.StatusInternalServerError, "internal_error", "failed to delete alias")
		return
	}

	s.rebuildVoiceList(r.Context())
	respondNoContent(w)
}

// TestAlias synthesizes sample text using an alias's configuration.
func (s *Server) TestAlias(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	alias, err := s.store.GetVoiceAlias(r.Context(), id)
	if err != nil {
		respondError(w, http.StatusNotFound, "not_found", "alias not found")
		return
	}

	ep, err := s.store.GetEndpoint(r.Context(), alias.EndpointID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "internal_error", "referenced endpoint not found")
		return
	}

	text := "Hello, this is a test."
	var req testAliasRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil && req.Text != "" {
			text = req.Text
		}
	}

	client := s.clientFactory(ep)
	body, err := client.Synthesize(r.Context(), &tts.SynthesizeRequest{
		Model:        alias.Model,
		Voice:        alias.Voice,
		Input:        text,
		Speed:        alias.Speed,
		Instructions: alias.Instructions,
	})
	if err != nil {
		respondJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer body.Close()

	respondJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// rebuildVoiceList triggers a Wyoming voice list rebuild, logging any error.
func (s *Server) rebuildVoiceList(ctx context.Context) {
	if s.infoBuilder == nil {
		return
	}
	if _, err := s.infoBuilder.Build(ctx); err != nil {
		slog.Error("failed to rebuild voice list", "error", err)
	}
}
