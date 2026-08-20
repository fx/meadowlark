package tts

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"

	"github.com/fx/meadowlark/internal/model"
	"github.com/fx/meadowlark/internal/voice"
	"github.com/fx/meadowlark/internal/wyoming"
)

// chunkSize is the number of PCM bytes per audio-chunk event.
const chunkSize = 2048

// defaultStreamSampleRate is the sample rate assumed for a streaming endpoint
// that does not configure one.
const defaultStreamSampleRate = 24000

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

// synthesisPlan is everything a resolved synthesis request needs in order to
// synthesize text against an endpoint: the endpoint itself, a client bound to
// it, and the merged request parameters shared by every segment of that
// request. Params.Input holds the caller's already-parsed input text; the text
// actually sent upstream is supplied per segment to openSegment.
type synthesisPlan struct {
	Endpoint *model.Endpoint
	Client   *Client
	Params   voice.ParsedInput
}

// OpenSegment is an upstream synthesis response whose audio format is already
// known and whose PCM data has not been read or written anywhere yet. It is an
// io.ReadCloser over the segment's PCM, positioned at the first sample.
//
// The caller owns the segment and MUST Close it — openSegment hands over the
// underlying response body, and emitSegment never closes it.
type OpenSegment struct {
	format    *AudioFormat
	pcm       io.Reader
	body      io.Closer
	closeOnce sync.Once
	closeErr  error
}

// Format returns the segment's audio format. It is known before any byte of
// the segment is written, which is what lets a caller reject a segment whose
// format does not match the rest of its stream without emitting audio-start.
func (s *OpenSegment) Format() *AudioFormat { return s.format }

// Read returns the segment's raw PCM data.
func (s *OpenSegment) Read(p []byte) (int, error) { return s.pcm.Read(p) }

// Close releases the underlying upstream response body. It is safe to call
// more than once; every call returns the first close's result.
func (s *OpenSegment) Close() error {
	s.closeOnce.Do(func() { s.closeErr = s.body.Close() })
	return s.closeErr
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
	// Parse input text for overrides. The caller of resolveSynthesis owns
	// parsing: a whole message can be parsed, a fragment cannot.
	parsed := voice.ParseInput(ev.Text)

	plan, err := p.resolveSynthesis(ctx, ev.Voice, parsed)
	if err != nil {
		return err
	}

	seg, err := p.openSegment(ctx, plan, plan.Params.Input)
	if err != nil {
		return err
	}
	defer seg.Close()

	return p.emitSegment(w, seg)
}

// resolveSynthesis resolves a Wyoming voice name to an endpoint and merges the
// request parameters for it: input overrides beat alias defaults beat endpoint
// defaults.
//
// It deliberately does not call voice.ParseInput. The caller owns parsing,
// because parsing is only meaningful on a complete message: a streaming
// session accumulating text must parse the whole message or not at all, and
// handing ParseInput a fragment would drop every override and speak the
// leftover JSON braces aloud.
func (p *Proxy) resolveSynthesis(ctx context.Context, voiceName string, parsed voice.ParsedInput) (*synthesisPlan, error) {
	// 1. Resolve voice name.
	resolved, err := p.resolver.Resolve(ctx, voiceName)
	if err != nil {
		return nil, fmt.Errorf("resolve voice: %w", err)
	}

	// 2. Build alias and endpoint defaults for merging.
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
		return nil, fmt.Errorf("get endpoint %s: %w", resolved.EndpointID, err)
	}
	if !ep.Enabled {
		return nil, fmt.Errorf("endpoint %s is disabled", ep.ID)
	}

	epDefaults := &voice.ParsedInput{
		Voice:        resolved.Voice,
		Model:        resolved.Model,
		Speed:        ep.DefaultSpeed,
		Instructions: ep.DefaultInstructions,
	}

	// 3. Merge parameters: input overrides > alias defaults > endpoint defaults.
	merged := voice.MergeParams(parsed, aliasDefaults, epDefaults)

	// 4. Build the client for the resolved endpoint.
	return &synthesisPlan{
		Endpoint: ep,
		Client:   p.clientFactory(ep),
		Params:   merged,
	}, nil
}

// openSegment issues the upstream request for one segment of text and
// determines its audio format — from the endpoint's configured sample rate in
// streaming mode, from the WAV header in buffered mode.
//
// It writes nothing. That is the property R8 of change 0006 depends on: Home
// Assistant honours only the first audio-start of a stream, so a segment whose
// format disagrees with the rest of the session must be rejected with zero
// bytes on the wire. It is also what makes prefetching a segment possible —
// a prefetch is an early openSegment, and Close releases a prefetch that is
// never emitted.
//
// On success the caller owns the returned segment and MUST Close it.
func (p *Proxy) openSegment(ctx context.Context, plan *synthesisPlan, text string) (*OpenSegment, error) {
	ep := plan.Endpoint

	if ep.StreamingEnabled {
		// Streaming mode: request raw PCM, no WAV header.
		streamReq := &StreamSynthesizeRequest{
			Model:          plan.Params.Model,
			Voice:          plan.Params.Voice,
			Input:          text,
			ResponseFormat: "pcm",
			Speed:          plan.Params.Speed,
			Instructions:   plan.Params.Instructions,
			Stream:         true,
		}
		body, streamErr := plan.Client.SynthesizeStream(ctx, streamReq)
		if streamErr != nil {
			return nil, fmt.Errorf("tts api call (streaming): %w", streamErr)
		}

		sampleRate := ep.StreamSampleRate
		if sampleRate == 0 {
			sampleRate = defaultStreamSampleRate
		}
		return &OpenSegment{
			format: &AudioFormat{Rate: sampleRate, Width: 2, Channels: 1},
			pcm:    body,
			body:   body,
		}, nil
	}

	// Buffered mode: WAV response.
	if ep.DefaultResponseFormat != "" && ep.DefaultResponseFormat != "wav" {
		return nil, fmt.Errorf("unsupported response format %q; only %q is supported by proxy", ep.DefaultResponseFormat, "wav")
	}

	req := &SynthesizeRequest{
		Model:          plan.Params.Model,
		Voice:          plan.Params.Voice,
		Input:          text,
		ResponseFormat: "wav",
		Speed:          plan.Params.Speed,
		Instructions:   plan.Params.Instructions,
	}
	body, synthErr := plan.Client.Synthesize(ctx, req)
	if synthErr != nil {
		return nil, fmt.Errorf("tts api call: %w", synthErr)
	}

	wavReader := NewWAVReader(body)
	wavFormat, fmtErr := wavReader.ReadFormat()
	if fmtErr != nil {
		// Ownership never transfers on an error return, so close it here.
		_ = body.Close()
		return nil, fmt.Errorf("parse wav header: %w", fmtErr)
	}

	return &OpenSegment{format: wavFormat, pcm: wavReader, body: body}, nil
}

// emitSegment writes an already-opened segment to w as audio-start, one or
// more audio-chunk events, and audio-stop. It does not close the segment; the
// caller that opened it owns it.
func (p *Proxy) emitSegment(w io.Writer, seg *OpenSegment) error {
	format := seg.Format()

	audioStart := &wyoming.AudioStart{
		Rate:     format.Rate,
		Width:    format.Width,
		Channels: format.Channels,
	}
	if err := wyoming.WriteEvent(w, audioStart.ToEvent()); err != nil {
		return fmt.Errorf("write audio-start: %w", err)
	}

	buf := make([]byte, chunkSize)
	for {
		n, readErr := seg.Read(buf)
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

	audioStop := &wyoming.AudioStop{}
	if err := wyoming.WriteEvent(w, audioStop.ToEvent()); err != nil {
		return fmt.Errorf("write audio-stop: %w", err)
	}

	return nil
}
