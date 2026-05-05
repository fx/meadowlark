package wyoming

import (
	"context"
	"fmt"
	"sync"

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

// EndpointVoiceLister provides read access to persisted endpoint_voices rows.
type EndpointVoiceLister interface {
	ListEndpointVoices(ctx context.Context, endpointID string) ([]model.EndpointVoice, error)
}

// VoiceDiscoverer is retained for backwards compatibility but is no longer
// consulted by InfoBuilder. The canonical voice list is built from persisted
// endpoint_voices rows; live probes have moved to the explicit Refresh flow.
type VoiceDiscoverer interface {
	DiscoverVoices(ctx context.Context, ep *model.Endpoint) []string
}

// InfoBuilder aggregates canonical voices from persisted endpoint_voices and
// enabled voice aliases, then builds a complete Info event.
type InfoBuilder struct {
	endpoints      EndpointLister
	aliases        AliasLister
	endpointVoices EndpointVoiceLister
	version        string

	mu    sync.RWMutex
	cache *Info
}

// NewInfoBuilder creates a new InfoBuilder. The endpointVoices argument is the
// persisted-state lister; the legacy VoiceDiscoverer parameter is accepted for
// callers that still pass it but is ignored.
func NewInfoBuilder(endpoints EndpointLister, aliases AliasLister, endpointVoices EndpointVoiceLister, version string) *InfoBuilder {
	return &InfoBuilder{
		endpoints:      endpoints,
		aliases:        aliases,
		endpointVoices: endpointVoices,
		version:        version,
	}
}

// Build rebuilds the Info response from the current state of endpoints,
// endpoint_voices, and aliases. The result is cached until the next call to Build.
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

	for _, ep := range endpoints {
		if !ep.Enabled {
			continue
		}
		var epVoices []model.EndpointVoice
		if b.endpointVoices != nil {
			epVoices, err = b.endpointVoices.ListEndpointVoices(ctx, ep.ID)
			if err != nil {
				return nil, fmt.Errorf("info: list endpoint voices: %w", err)
			}
		}
		for _, m := range ep.Models {
			for _, ev := range epVoices {
				if !ev.Enabled {
					continue
				}
				voices = append(voices, TtsVoice{
					Name:        voice.CanonicalName(ev.VoiceID, ep.Name, m),
					Description: fmt.Sprintf("%s (%s, %s)", ev.VoiceID, ep.Name, m),
					Installed:   true,
					Languages:   []string{"en"},
				})
			}
		}
	}

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
