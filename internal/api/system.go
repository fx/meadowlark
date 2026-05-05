package api

import (
	"net/http"
	"time"
)

// statusResponse is the JSON response for GET /api/v1/status.
type statusResponse struct {
	Version       string `json:"version"`
	UptimeSeconds int    `json:"uptime_seconds"`
	WyomingPort   int    `json:"wyoming_port"`
	HTTPPort      int    `json:"http_port"`
	DBDriver      string `json:"db_driver"`
	VoiceCount    int    `json:"voice_count"`
	EndpointCount int    `json:"endpoint_count"`
	AliasCount    int    `json:"alias_count"`
}

// voiceEntry is a single voice in the GET /api/v1/voices response.
type voiceEntry struct {
	Name     string `json:"name"`
	Endpoint string `json:"endpoint"`
	Model    string `json:"model"`
	Voice    string `json:"voice"`
	IsAlias  bool   `json:"is_alias"`
}

// GetStatus returns system status information.
func (s *Server) GetStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	endpoints, err := s.store.ListEndpoints(ctx)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "internal_error", "failed to list endpoints")
		return
	}

	aliases, err := s.store.ListVoiceAliases(ctx)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "internal_error", "failed to list voice aliases")
		return
	}

	// Count voices: sum of models across enabled endpoints, plus enabled aliases.
	voiceCount := 0
	for _, ep := range endpoints {
		if ep.Enabled {
			voiceCount += len(ep.Models)
		}
	}
	for _, a := range aliases {
		if a.Enabled {
			voiceCount++
		}
	}

	respondJSON(w, http.StatusOK, statusResponse{
		Version:       s.version,
		UptimeSeconds: int(time.Since(s.startTime).Seconds()),
		WyomingPort:   s.wyomingPort,
		HTTPPort:      s.httpPort,
		DBDriver:      s.dbDriver,
		VoiceCount:    voiceCount,
		EndpointCount: len(endpoints),
		AliasCount:    len(aliases),
	})
}

// ListVoices returns all resolved voices (canonical + aliases). Canonical
// voices are sourced from persisted endpoint_voices rows where enabled=true
// for an endpoints.enabled=true endpoint, cross-joined with each model on the
// endpoint. Aliases bypass the enabled filter (resolver Stage 1 invariant).
func (s *Server) ListVoices(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	endpoints, err := s.store.ListEndpoints(ctx)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "internal_error", "failed to list endpoints")
		return
	}

	aliases, err := s.store.ListVoiceAliases(ctx)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "internal_error", "failed to list voice aliases")
		return
	}

	epNames := make(map[string]string, len(endpoints))
	for _, ep := range endpoints {
		epNames[ep.ID] = ep.Name
	}

	var voices []voiceEntry
	for _, ep := range endpoints {
		if !ep.Enabled {
			continue
		}
		epVoices, err := s.store.ListEndpointVoices(ctx, ep.ID)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "internal_error", "failed to list endpoint voices")
			return
		}
		for _, m := range ep.Models {
			for _, ev := range epVoices {
				if !ev.Enabled {
					continue
				}
				voices = append(voices, voiceEntry{
					Name:     ev.VoiceID + " (" + ep.Name + ", " + m + ")",
					Endpoint: ep.Name,
					Model:    m,
					Voice:    ev.VoiceID,
					IsAlias:  false,
				})
			}
		}
	}

	for _, a := range aliases {
		if !a.Enabled {
			continue
		}
		voices = append(voices, voiceEntry{
			Name:     a.Name,
			Endpoint: epNames[a.EndpointID],
			Model:    a.Model,
			Voice:    a.Voice,
			IsAlias:  true,
		})
	}

	if voices == nil {
		voices = []voiceEntry{}
	}

	respondJSON(w, http.StatusOK, voices)
}
