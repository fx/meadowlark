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
		p.logger.Error("synthesis failed", "error", err, "voice", ev.Voice, "text_len", len(ev.Text))
		errEv := &wyoming.Error{Text: err.Error(), Code: "tts-error"}
		if writeErr := wyoming.WriteEvent(w, errEv.ToEvent()); writeErr != nil {
			p.logger.Error("failed to write TTS error event", "error", writeErr)
		}
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
	if !ep.Enabled {
		return fmt.Errorf("endpoint %s is disabled", ep.ID)
	}

	epDefaults := &voice.ParsedInput{
		Voice:        resolved.Voice,
		Model:        resolved.Model,
		Speed:        ep.DefaultSpeed,
		Instructions: ep.DefaultInstructions,
	}

	// 4. Merge parameters: input overrides > alias defaults > endpoint defaults.
	merged := voice.MergeParams(parsed, aliasDefaults, epDefaults)

	// 5. Call TTS client — streaming or buffered based on endpoint config.
	client := p.clientFactory(ep)

	var pcmReader io.Reader
	var format *AudioFormat

	if ep.StreamingEnabled {
		// Streaming mode: request raw PCM, no WAV header.
		streamReq := &StreamSynthesizeRequest{
			Model:          merged.Model,
			Voice:          merged.Voice,
			Input:          merged.Input,
			ResponseFormat: "pcm",
			Speed:          merged.Speed,
			Instructions:   merged.Instructions,
			Stream:         true,
		}
		body, streamErr := client.SynthesizeStream(ctx, streamReq)
		if streamErr != nil {
			return fmt.Errorf("tts api call (streaming): %w", streamErr)
		}
		defer body.Close()

		sampleRate := ep.StreamSampleRate
		if sampleRate == 0 {
			sampleRate = 24000
		}
		format = &AudioFormat{Rate: sampleRate, Width: 2, Channels: 1}
		pcmReader = body
	} else {
		// Buffered mode: WAV response (existing behavior).
		if ep.DefaultResponseFormat != "" && ep.DefaultResponseFormat != "wav" {
			return fmt.Errorf("unsupported response format %q; only %q is supported by proxy", ep.DefaultResponseFormat, "wav")
		}

		req := &SynthesizeRequest{
			Model:          merged.Model,
			Voice:          merged.Voice,
			Input:          merged.Input,
			ResponseFormat: "wav",
			Speed:          merged.Speed,
			Instructions:   merged.Instructions,
		}
		body, synthErr := client.Synthesize(ctx, req)
		if synthErr != nil {
			return fmt.Errorf("tts api call: %w", synthErr)
		}
		defer body.Close()

		wavReader := NewWAVReader(body)
		wavFormat, fmtErr := wavReader.ReadFormat()
		if fmtErr != nil {
			return fmt.Errorf("parse wav header: %w", fmtErr)
		}
		format = wavFormat
		pcmReader = wavReader
	}

	// 6. Send audio-start event.
	audioStart := &wyoming.AudioStart{
		Rate:     format.Rate,
		Width:    format.Width,
		Channels: format.Channels,
	}
	if err := wyoming.WriteEvent(w, audioStart.ToEvent()); err != nil {
		return fmt.Errorf("write audio-start: %w", err)
	}

	// 7. Read PCM data and send audio-chunk events (2048-byte chunks).
	buf := make([]byte, chunkSize)
	for {
		n, readErr := pcmReader.Read(buf)
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

	// 8. Send audio-stop event.
	audioStop := &wyoming.AudioStop{}
	if err := wyoming.WriteEvent(w, audioStop.ToEvent()); err != nil {
		return fmt.Errorf("write audio-stop: %w", err)
	}

	return nil
}
