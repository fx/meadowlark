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

// InfoBuilder aggregates canonical voices from all enabled endpoints and
// enabled voice aliases, then builds a complete Info event.
type InfoBuilder struct {
	endpoints EndpointLister
	aliases   AliasLister
	version   string

	mu    sync.RWMutex
	cache *Info
}

// NewInfoBuilder creates a new InfoBuilder.
func NewInfoBuilder(endpoints EndpointLister, aliases AliasLister, version string) *InfoBuilder {
	return &InfoBuilder{
		endpoints: endpoints,
		aliases:   aliases,
		version:   version,
	}
}

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

	// Add canonical voices: endpoint x model x voice.
	// For canonical voices, we use the endpoint's models as the voice list since
	// we don't have a separate voice discovery mechanism yet. The voice names
	// exposed by each endpoint are not stored in the database -- we use model names
	// as placeholder voice names for canonical entries. In practice, users configure
	// voice aliases for the voices they actually want to expose.
	//
	// Note: BuildCanonicalList from the voice package requires a voicesByEndpoint map.
	// Since we don't have voice discovery yet, we build canonical voices from
	// endpoints and their models directly. Each model on each endpoint becomes a
	// canonical voice entry only if there are voice aliases that reference it,
	// OR we simply expose all endpoint x model combinations.
	//
	// Per the spec, canonical voices are formatted as "voice (endpoint, model)".
	// Without voice discovery, there are no canonical voices to expose unless
	// explicitly configured. Voice aliases are the primary mechanism.
	for _, ep := range endpoints {
		if !ep.Enabled {
			continue
		}
		for _, m := range ep.Models {
			voices = append(voices, TtsVoice{
				Name:        voice.CanonicalName(m, ep.Name, m),
				Description: fmt.Sprintf("%s (%s, %s)", m, ep.Name, m),
				Installed:   true,
				Languages:   []string{"en"},
			})
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
