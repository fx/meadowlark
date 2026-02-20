package voice

import (
	"context"
	"fmt"
	"strings"

	"github.com/fx/meadowlark/internal/model"
)

// EndpointLister provides read access to endpoints.
type EndpointLister interface {
	ListEndpoints(ctx context.Context) ([]model.Endpoint, error)
}

// AliasLister provides read access to voice aliases.
type AliasLister interface {
	ListVoiceAliases(ctx context.Context) ([]model.VoiceAlias, error)
}

// Resolver resolves voice names to ResolvedVoice structs.
type Resolver struct {
	endpoints EndpointLister
	aliases   AliasLister
}

// NewResolver creates a new voice resolver.
func NewResolver(endpoints EndpointLister, aliases AliasLister) *Resolver {
	return &Resolver{endpoints: endpoints, aliases: aliases}
}

// Resolve takes a voice name string and returns a ResolvedVoice.
//
// Resolution priority:
//  0. If name is empty, resolve directly from the first enabled endpoint's DefaultVoice
//     using that endpoint's own ID and first model for consistency
//  1. Voice alias (by name, must be enabled)
//  2. Canonical name ("voice (endpoint, model)" format)
//  3. Fallback to first enabled endpoint's first model and the voice name as-is
func (r *Resolver) Resolve(ctx context.Context, name string) (*model.ResolvedVoice, error) {
	// 0. If voice is empty, resolve directly to ensure endpoint+voice consistency.
	if name == "" {
		return r.resolveDefaultVoice(ctx)
	}

	// 1. Try alias resolution.
	resolved, err := r.resolveAlias(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("voice: resolve alias: %w", err)
	}
	if resolved != nil {
		return resolved, nil
	}

	// 2. Try canonical name resolution.
	resolved, err = r.resolveCanonical(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("voice: resolve canonical: %w", err)
	}
	if resolved != nil {
		return resolved, nil
	}

	// 3. Fallback to first enabled endpoint.
	return r.resolveFallback(ctx, name)
}

// resolveDefaultVoice resolves directly from the first enabled endpoint that
// has a DefaultVoice set, returning a ResolvedVoice tied to that endpoint's
// ID and first model. This avoids the inconsistency of substituting the voice
// name and then falling back to a different endpoint via resolveFallback.
func (r *Resolver) resolveDefaultVoice(ctx context.Context) (*model.ResolvedVoice, error) {
	endpoints, err := r.endpoints.ListEndpoints(ctx)
	if err != nil {
		return nil, fmt.Errorf("voice: resolve default voice: %w", err)
	}
	for _, ep := range endpoints {
		if !ep.Enabled || ep.DefaultVoice == "" {
			continue
		}
		m := ""
		if len(ep.Models) > 0 {
			m = ep.Models[0]
		}
		return &model.ResolvedVoice{
			Name:         ep.DefaultVoice,
			EndpointID:   ep.ID,
			Model:        m,
			Voice:        ep.DefaultVoice,
			Speed:        ep.DefaultSpeed,
			Instructions: ep.DefaultInstructions,
			Languages:    nil,
			IsAlias:      false,
		}, nil
	}
	return nil, fmt.Errorf("voice: no voice specified and no default voice configured")
}

// resolveAlias looks up the name in voice aliases.
func (r *Resolver) resolveAlias(ctx context.Context, name string) (*model.ResolvedVoice, error) {
	aliases, err := r.aliases.ListVoiceAliases(ctx)
	if err != nil {
		return nil, err
	}
	for _, a := range aliases {
		if a.Enabled && a.Name == name {
			return &model.ResolvedVoice{
				Name:         a.Name,
				EndpointID:   a.EndpointID,
				Model:        a.Model,
				Voice:        a.Voice,
				Speed:        a.Speed,
				Instructions: a.Instructions,
				Languages:    a.Languages,
				IsAlias:      true,
			}, nil
		}
	}
	return nil, nil
}

// ParseCanonicalName parses a canonical voice name in the format "voice (endpoint, model)".
// Returns the voice, endpoint name, and model, or empty strings if the format doesn't match.
func ParseCanonicalName(name string) (voice, endpointName, modelName string, ok bool) {
	// Find the last " (" to split voice from the rest.
	idx := strings.LastIndex(name, " (")
	if idx < 0 {
		return "", "", "", false
	}
	voice = name[:idx]
	rest := name[idx+2:] // skip " ("

	// Must end with ")".
	if !strings.HasSuffix(rest, ")") {
		return "", "", "", false
	}
	rest = rest[:len(rest)-1] // strip trailing ")"

	// Split on ", " to get endpoint and model.
	parts := strings.SplitN(rest, ", ", 2)
	if len(parts) != 2 {
		return "", "", "", false
	}

	endpointName = parts[0]
	modelName = parts[1]

	if voice == "" || endpointName == "" || modelName == "" {
		return "", "", "", false
	}

	return voice, endpointName, modelName, true
}

// resolveCanonical parses the canonical format and looks up the endpoint.
func (r *Resolver) resolveCanonical(ctx context.Context, name string) (*model.ResolvedVoice, error) {
	voiceName, endpointName, modelName, ok := ParseCanonicalName(name)
	if !ok {
		return nil, nil
	}

	endpoints, err := r.endpoints.ListEndpoints(ctx)
	if err != nil {
		return nil, err
	}

	for _, ep := range endpoints {
		if !ep.Enabled {
			continue
		}
		if ep.Name != endpointName {
			continue
		}
		// Verify the model exists on this endpoint.
		for _, m := range ep.Models {
			if m == modelName {
				return &model.ResolvedVoice{
					Name:         name,
					EndpointID:   ep.ID,
					Model:        modelName,
					Voice:        voiceName,
					Speed:        ep.DefaultSpeed,
					Instructions: ep.DefaultInstructions,
					Languages:    nil,
					IsAlias:      false,
				}, nil
			}
		}
	}

	return nil, nil
}

// resolveFallback returns the first enabled endpoint's first model with the given voice name.
func (r *Resolver) resolveFallback(ctx context.Context, voiceName string) (*model.ResolvedVoice, error) {
	endpoints, err := r.endpoints.ListEndpoints(ctx)
	if err != nil {
		return nil, fmt.Errorf("voice: resolve fallback: %w", err)
	}

	for _, ep := range endpoints {
		if !ep.Enabled || len(ep.Models) == 0 {
			continue
		}
		return &model.ResolvedVoice{
			Name:         voiceName,
			EndpointID:   ep.ID,
			Model:        ep.Models[0],
			Voice:        voiceName,
			Speed:        ep.DefaultSpeed,
			Instructions: ep.DefaultInstructions,
			Languages:    nil,
			IsAlias:      false,
		}, nil
	}

	return nil, fmt.Errorf("voice: no enabled endpoints available")
}

// CanonicalName builds a canonical voice name from its components.
func CanonicalName(voice, endpointName, modelName string) string {
	return fmt.Sprintf("%s (%s, %s)", voice, endpointName, modelName)
}

// BuildCanonicalList builds the full list of canonical voices from all enabled endpoints.
// Each enabled endpoint x model x voice combination produces one ResolvedVoice.
// The voices parameter provides known voice names per endpoint ID.
func BuildCanonicalList(endpoints []model.Endpoint, voicesByEndpoint map[string][]string) []model.ResolvedVoice {
	var result []model.ResolvedVoice
	for _, ep := range endpoints {
		if !ep.Enabled {
			continue
		}
		voices := voicesByEndpoint[ep.ID]
		for _, m := range ep.Models {
			for _, v := range voices {
				result = append(result, model.ResolvedVoice{
					Name:         CanonicalName(v, ep.Name, m),
					EndpointID:   ep.ID,
					Model:        m,
					Voice:        v,
					Speed:        ep.DefaultSpeed,
					Instructions: ep.DefaultInstructions,
					Languages:    nil,
					IsAlias:      false,
				})
			}
		}
	}
	return result
}
