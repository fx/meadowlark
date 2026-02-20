package api

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/fx/meadowlark/internal/model"
	"github.com/fx/meadowlark/internal/tts"
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

// probeTimeout is the per-endpoint timeout for voice discovery.
const probeTimeout = 5 * time.Second

// endpointVoices holds the result of a single endpoint voice discovery.
type endpointVoices struct {
	endpoint *model.Endpoint
	voices   []tts.Voice
}

// discoverEndpointVoices queries enabled endpoints for voices in parallel
// with a per-probe timeout. Results preserve endpoint order.
func (s *Server) discoverEndpointVoices(ctx context.Context, endpoints []model.Endpoint) []endpointVoices {
	// Collect enabled endpoints.
	var enabled []*model.Endpoint
	for i := range endpoints {
		if endpoints[i].Enabled {
			enabled = append(enabled, &endpoints[i])
		}
	}
	if len(enabled) == 0 {
		return nil
	}

	results := make([]endpointVoices, len(enabled))
	var wg sync.WaitGroup
	wg.Add(len(enabled))

	for i, ep := range enabled {
		go func(idx int, ep *model.Endpoint) {
			defer wg.Done()
			probeCtx, cancel := context.WithTimeout(ctx, probeTimeout)
			defer cancel()
			client := s.clientFactory(ep)
			voices, _ := client.ListVoices(probeCtx)
			results[idx] = endpointVoices{endpoint: ep, voices: voices}
		}(i, ep)
	}

	wg.Wait()
	return results
}

// ListVoices returns all resolved voices (canonical + aliases).
// For canonical voices it queries each endpoint for real voice names
// via the TTS client in parallel with a per-probe timeout. If discovery
// fails, the endpoint falls back to using model names as voices.
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

	// Discover voices from all enabled endpoints in parallel.
	discovered := s.discoverEndpointVoices(ctx, endpoints)
	for _, d := range discovered {
		ep := d.endpoint
		if len(d.voices) > 0 {
			// Use discovered voice names.
			for _, m := range ep.Models {
				for _, rv := range d.voices {
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
