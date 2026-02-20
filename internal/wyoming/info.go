package wyoming

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/fx/meadowlark/internal/model"
	"github.com/fx/meadowlark/internal/voice"
)

// EndpointLister provides read access to endpoints.
type EndpointLister interface {
	ListEndpoints(ctx context.Context) ([]model.Endpoint, error)
}

// AliasLister provides read access to voice aliases.
type AliasLister interface {
	ListVoiceAliases(ctx context.Context) ([]model.VoiceAlias, error)
}

// VoiceDiscoverer discovers available voices for an endpoint.
type VoiceDiscoverer interface {
	// DiscoverVoices returns voice IDs for the given endpoint.
	// Returns nil on failure (should not block the info response).
	DiscoverVoices(ctx context.Context, ep *model.Endpoint) []string
}

// InfoBuilder aggregates canonical voices from all enabled endpoints and
// enabled voice aliases, then builds a complete Info event.
type InfoBuilder struct {
	endpoints EndpointLister
	aliases   AliasLister
	discoverer VoiceDiscoverer
	version   string

	mu    sync.RWMutex
	cache *Info
}

// NewInfoBuilder creates a new InfoBuilder.
func NewInfoBuilder(endpoints EndpointLister, aliases AliasLister, discoverer VoiceDiscoverer, version string) *InfoBuilder {
	return &InfoBuilder{
		endpoints:  endpoints,
		aliases:    aliases,
		discoverer: discoverer,
		version:    version,
	}
}

// discoverTimeout is the per-endpoint timeout for voice discovery during info build.
const discoverTimeout = 5 * time.Second

// Build rebuilds the Info response from the current state of endpoints and aliases.
// The result is cached until the next call to Build.
func (b *InfoBuilder) Build(ctx context.Context) (*Info, error) {
	endpoints, err := b.endpoints.ListEndpoints(ctx)
	if err != nil {
		return nil, fmt.Errorf("info: list endpoints: %w", err)
	}

	aliases, err := b.aliases.ListVoiceAliases(ctx)
	if err != nil {
		return nil, fmt.Errorf("info: list voice aliases: %w", err)
	}

	var voices []TtsVoice

	// Discover voices from all enabled endpoints in parallel.
	var enabled []model.Endpoint
	for _, ep := range endpoints {
		if ep.Enabled {
			enabled = append(enabled, ep)
		}
	}

	discovered := make([][]string, len(enabled))
	if b.discoverer != nil && len(enabled) > 0 {
		var wg sync.WaitGroup
		wg.Add(len(enabled))
		for i, ep := range enabled {
			go func(idx int, ep model.Endpoint) {
				defer wg.Done()
				dctx, cancel := context.WithTimeout(ctx, discoverTimeout)
				defer cancel()
				discovered[idx] = b.discoverer.DiscoverVoices(dctx, &ep)
			}(i, ep)
		}
		wg.Wait()
	}

	// Build canonical voice entries from discovery results.
	for i, ep := range enabled {
		voiceNames := discovered[i]
		if len(voiceNames) > 0 {
			for _, m := range ep.Models {
				for _, v := range voiceNames {
					voices = append(voices, TtsVoice{
						Name:        voice.CanonicalName(v, ep.Name, m),
						Description: fmt.Sprintf("%s (%s, %s)", v, ep.Name, m),
						Installed:   true,
						Languages:   []string{"en"},
					})
				}
			}
		} else {
			// Fallback: use model names when voice discovery fails.
			for _, m := range ep.Models {
				voices = append(voices, TtsVoice{
					Name:        voice.CanonicalName(m, ep.Name, m),
					Description: fmt.Sprintf("%s (%s, %s)", m, ep.Name, m),
					Installed:   true,
					Languages:   []string{"en"},
				})
			}
		}
	}

	// Add enabled voice aliases.
	for _, a := range aliases {
		if !a.Enabled {
			continue
		}
		langs := a.Languages
		if len(langs) == 0 {
			langs = model.StringSlice{"en"}
		}
		voices = append(voices, TtsVoice{
			Name:        a.Name,
			Description: a.Name,
			Installed:   true,
			Languages:   []string(langs),
		})
	}

	info := &Info{
		Tts: []TtsProgram{
			{
				Name:        "meadowlark",
				Description: "Meadowlark TTS Bridge",
				Installed:   true,
				Version:     b.version,
				Voices:      voices,
			},
		},
	}

	b.mu.Lock()
	b.cache = info
	b.mu.Unlock()

	return info, nil
}

// Cached returns the last built Info response, or nil if Build has not been called.
func (b *InfoBuilder) Cached() *Info {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.cache
}
