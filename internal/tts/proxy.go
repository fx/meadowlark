package tts

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/fx/meadowlark/internal/model"
	"github.com/fx/meadowlark/internal/voice"
	"github.com/fx/meadowlark/internal/wyoming"
)

// chunkSize is the number of PCM bytes per audio-chunk event.
const chunkSize = 2048

// EndpointGetter retrieves an endpoint by ID.
type EndpointGetter interface {
	GetEndpoint(ctx context.Context, id string) (*model.Endpoint, error)
}

// ClientFactory creates a TTS Client for a given endpoint.
type ClientFactory func(ep *model.Endpoint) *Client

// Proxy orchestrates synthesis: resolve voice, parse input, merge params,
// call the TTS API, and stream audio events back to the Wyoming client.
type Proxy struct {
	resolver      *voice.Resolver
	endpoints     EndpointGetter
	clientFactory ClientFactory
	logger        *slog.Logger
}

// NewProxy creates a new synthesis proxy.
func NewProxy(resolver *voice.Resolver, endpoints EndpointGetter, factory ClientFactory, logger *slog.Logger) *Proxy {
	if logger == nil {
		logger = slog.Default()
	}
	return &Proxy{
		resolver:      resolver,
		endpoints:     endpoints,
		clientFactory: factory,
		logger:        logger,
	}
}

// HandleSynthesize processes a synthesize event and writes audio events to w.
// On failure it writes an error event instead of crashing.
func (p *Proxy) HandleSynthesize(ctx context.Context, ev *wyoming.Synthesize, w io.Writer) {
	if err := p.doSynthesize(ctx, ev, w); err != nil {
		p.logger.Error("synthesis failed", "error", err, "voice", ev.Voice, "text", ev.Text)
		errEv := &wyoming.Error{Text: err.Error(), Code: "tts-error"}
		_ = wyoming.WriteEvent(w, errEv.ToEvent())
	}
}

func (p *Proxy) doSynthesize(ctx context.Context, ev *wyoming.Synthesize, w io.Writer) error {
	// 1. Resolve voice name.
	resolved, err := p.resolver.Resolve(ctx, ev.Voice)
	if err != nil {
		return fmt.Errorf("resolve voice: %w", err)
	}

	// 2. Parse input text for overrides.
	parsed := voice.ParseInput(ev.Text)

	// 3. Build alias and endpoint defaults for merging.
	var aliasDefaults *voice.ParsedInput
	if resolved.IsAlias {
		aliasDefaults = &voice.ParsedInput{
			Voice:        resolved.Voice,
			Model:        resolved.Model,
			Speed:        resolved.Speed,
			Instructions: resolved.Instructions,
		}
	}

	ep, err := p.endpoints.GetEndpoint(ctx, resolved.EndpointID)
	if err != nil {
		return fmt.Errorf("get endpoint %s: %w", resolved.EndpointID, err)
	}

	epDefaults := &voice.ParsedInput{
		Voice:        resolved.Voice,
		Model:        resolved.Model,
		Speed:        ep.DefaultSpeed,
		Instructions: ep.DefaultInstructions,
	}

	// 4. Merge parameters: input overrides > alias defaults > endpoint defaults.
	merged := voice.MergeParams(parsed, aliasDefaults, epDefaults)

	// Determine response format.
	responseFormat := ep.DefaultResponseFormat
	if responseFormat == "" {
		responseFormat = "wav"
	}

	// 5. Call TTS client.
	client := p.clientFactory(ep)
	req := &SynthesizeRequest{
		Model:          merged.Model,
		Voice:          merged.Voice,
		Input:          merged.Input,
		ResponseFormat: responseFormat,
		Speed:          merged.Speed,
		Instructions:   merged.Instructions,
	}

	body, err := client.Synthesize(ctx, req)
	if err != nil {
		return fmt.Errorf("tts api call: %w", err)
	}
	defer body.Close()

	// 6. Parse WAV header to get audio format.
	wavReader := NewWAVReader(body)
	format, err := wavReader.ReadFormat()
	if err != nil {
		return fmt.Errorf("parse wav header: %w", err)
	}

	// 7. Send audio-start event.
	audioStart := &wyoming.AudioStart{
		Rate:     format.Rate,
		Width:    format.Width,
		Channels: format.Channels,
	}
	if err := wyoming.WriteEvent(w, audioStart.ToEvent()); err != nil {
		return fmt.Errorf("write audio-start: %w", err)
	}

	// 8. Read PCM data and send audio-chunk events (2048-byte chunks).
	buf := make([]byte, chunkSize)
	for {
		n, readErr := wavReader.Read(buf)
		if n > 0 {
			chunk := &wyoming.AudioChunk{
				Rate:     format.Rate,
				Width:    format.Width,
				Channels: format.Channels,
				Audio:    make([]byte, n),
			}
			copy(chunk.Audio, buf[:n])
			if err := wyoming.WriteEvent(w, chunk.ToEvent()); err != nil {
				return fmt.Errorf("write audio-chunk: %w", err)
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return fmt.Errorf("read pcm data: %w", readErr)
		}
	}

	// 9. Send audio-stop event.
	audioStop := &wyoming.AudioStop{}
	if err := wyoming.WriteEvent(w, audioStop.ToEvent()); err != nil {
		return fmt.Errorf("write audio-stop: %w", err)
	}

	return nil
}
