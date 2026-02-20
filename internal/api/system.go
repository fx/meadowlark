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

// ListVoices returns all resolved voices (canonical + aliases).
// For canonical voices it queries each endpoint for real voice names
// via the TTS client. If discovery fails, the endpoint is skipped.
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

	// Build endpoint ID -> name lookup.
	epNames := make(map[string]string, len(endpoints))
	for _, ep := range endpoints {
		epNames[ep.ID] = ep.Name
	}

	var voices []voiceEntry

	// Canonical voices: each enabled endpoint × model × discovered voice.
	// Query the remote endpoint for real voice names via the TTS client.
	for i := range endpoints {
		ep := &endpoints[i]
		if !ep.Enabled {
			continue
		}
		client := s.clientFactory(ep)
		remoteVoices, _ := client.ListVoices(ctx)

		if len(remoteVoices) > 0 {
			// Use discovered voice names.
			for _, m := range ep.Models {
				for _, rv := range remoteVoices {
					voices = append(voices, voiceEntry{
						Name:     rv.ID + " (" + ep.Name + ", " + m + ")",
						Endpoint: ep.Name,
						Model:    m,
						Voice:    rv.ID,
						IsAlias:  false,
					})
				}
			}
		} else {
			// Fallback: use model names when voice discovery fails.
			for _, m := range ep.Models {
				voices = append(voices, voiceEntry{
					Name:     m + " (" + ep.Name + ", " + m + ")",
					Endpoint: ep.Name,
					Model:    m,
					Voice:    m,
					IsAlias:  false,
				})
			}
		}
	}

	// Alias voices.
	for _, a := range aliases {
		if !a.Enabled {
			continue
		}
		epName := epNames[a.EndpointID]
		voices = append(voices, voiceEntry{
			Name:     a.Name,
			Endpoint: epName,
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
