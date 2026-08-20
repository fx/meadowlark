package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/fx/meadowlark/internal/model"
	"github.com/fx/meadowlark/internal/voice"
	"github.com/fx/meadowlark/internal/wyoming"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- helpers ---------------------------------------------------------------

// newSegmentTestProxy builds a Proxy whose resolver and endpoint getter both
// read from eps, and whose clients talk over httpClient.
func newSegmentTestProxy(eps map[string]*model.Endpoint, aliases []model.VoiceAlias, httpClient *http.Client) *Proxy {
	store := &mockEndpointStore{endpoints: eps}
	aliasStore := &mockAliasStore{aliases: aliases}
	resolver := voice.NewResolver(store, aliasStore)
	factory := func(e *model.Endpoint) *Client { return NewClient(e.BaseURL, e.APIKey, httpClient) }
	return NewProxy(resolver, store, factory, slog.Default())
}

// segTrackedBody counts Close calls on an upstream response body.
type segTrackedBody struct {
	io.Reader
	closes atomic.Int32
}

func (b *segTrackedBody) Close() error {
	b.closes.Add(1)
	return nil
}

// segStubTransport serves canned responses without a real network listener,
// so a test can hand back a body whose Close calls it can count.
type segStubTransport struct {
	fn func(*http.Request) (*http.Response, error)
}

func (t *segStubTransport) RoundTrip(r *http.Request) (*http.Response, error) { return t.fn(r) }

// segErrWriter accepts okWrites calls and fails every one after that. Because
// WriteEvent issues exactly one Write per event, that picks which event in an
// audio group fails.
type segErrWriter struct {
	okWrites int
	calls    int
}

func (w *segErrWriter) Write(p []byte) (int, error) {
	w.calls++
	if w.calls > w.okWrites {
		return 0, errors.New("write failed")
	}
	return len(p), nil
}

// segErrReader yields prefix, then a non-EOF read error.
type segErrReader struct {
	prefix []byte
	pos    int
}

func (r *segErrReader) Read(p []byte) (int, error) {
	if r.pos < len(r.prefix) {
		n := copy(p, r.prefix[r.pos:])
		r.pos += n
		return n, nil
	}
	return 0, errors.New("upstream disconnected")
}

// newTestSegment builds an OpenSegment directly, bypassing the upstream call.
func newTestSegment(format *AudioFormat, pcm io.Reader) *OpenSegment {
	return &OpenSegment{format: format, pcm: pcm, body: io.NopCloser(pcm)}
}

func floatPtr(f float64) *float64 { return &f }
func strPtr(s string) *string     { return &s }

// --- resolveSynthesis ------------------------------------------------------

func TestProxy_ResolveSynthesis(t *testing.T) {
	tests := []struct {
		name      string
		endpoints map[string]*model.Endpoint
		aliases   []model.VoiceAlias
		voiceName string
		parsed    voice.ParsedInput
		wantErr   string
		check     func(t *testing.T, plan *synthesisPlan)
	}{
		{
			name: "endpoint defaults fill unset params",
			endpoints: map[string]*model.Endpoint{"ep-1": {
				ID: "ep-1", Name: "Test", BaseURL: "http://upstream",
				Models: model.StringSlice{"tts-1"}, Enabled: true,
				DefaultSpeed: floatPtr(0.9), DefaultInstructions: strPtr("calm"),
			}},
			voiceName: "alloy (Test, tts-1)",
			parsed:    voice.ParsedInput{Input: "Hello"},
			check: func(t *testing.T, plan *synthesisPlan) {
				t.Helper()
				assert.Equal(t, "ep-1", plan.Endpoint.ID)
				require.NotNil(t, plan.Client)
				assert.Equal(t, "Hello", plan.Params.Input)
				assert.Equal(t, "alloy", plan.Params.Voice)
				assert.Equal(t, "tts-1", plan.Params.Model)
				require.NotNil(t, plan.Params.Speed)
				assert.InDelta(t, 0.9, *plan.Params.Speed, 1e-9)
				require.NotNil(t, plan.Params.Instructions)
				assert.Equal(t, "calm", *plan.Params.Instructions)
			},
		},
		{
			name: "alias defaults beat endpoint defaults",
			endpoints: map[string]*model.Endpoint{"ep-1": {
				ID: "ep-1", Name: "Test", BaseURL: "http://upstream",
				Models: model.StringSlice{"tts-1"}, Enabled: true,
				DefaultSpeed: floatPtr(0.9), DefaultInstructions: strPtr("calm"),
			}},
			aliases: []model.VoiceAlias{{
				Name: "narrator", EndpointID: "ep-1", Model: "tts-1-hd", Voice: "nova",
				Speed: floatPtr(1.1), Instructions: strPtr("bright"), Enabled: true,
			}},
			voiceName: "narrator",
			parsed:    voice.ParsedInput{Input: "Hello"},
			check: func(t *testing.T, plan *synthesisPlan) {
				t.Helper()
				assert.Equal(t, "nova", plan.Params.Voice)
				assert.Equal(t, "tts-1-hd", plan.Params.Model)
				require.NotNil(t, plan.Params.Speed)
				assert.InDelta(t, 1.1, *plan.Params.Speed, 1e-9)
				require.NotNil(t, plan.Params.Instructions)
				assert.Equal(t, "bright", *plan.Params.Instructions)
			},
		},
		{
			name: "input overrides beat alias defaults",
			endpoints: map[string]*model.Endpoint{"ep-1": {
				ID: "ep-1", Name: "Test", BaseURL: "http://upstream",
				Models: model.StringSlice{"tts-1"}, Enabled: true,
			}},
			aliases: []model.VoiceAlias{{
				Name: "narrator", EndpointID: "ep-1", Model: "tts-1", Voice: "nova",
				Speed: floatPtr(1.1), Enabled: true,
			}},
			voiceName: "narrator",
			parsed: voice.ParsedInput{
				Input: "Hello", Voice: "shimmer", Model: "gpt-4o-mini-tts", Speed: floatPtr(1.5),
			},
			check: func(t *testing.T, plan *synthesisPlan) {
				t.Helper()
				assert.Equal(t, "ep-1", plan.Endpoint.ID, "an override must not change the endpoint")
				assert.Equal(t, "shimmer", plan.Params.Voice)
				assert.Equal(t, "gpt-4o-mini-tts", plan.Params.Model)
				require.NotNil(t, plan.Params.Speed)
				assert.InDelta(t, 1.5, *plan.Params.Speed, 1e-9)
			},
		},
		{
			name:      "voice resolution failure",
			endpoints: map[string]*model.Endpoint{},
			voiceName: "alloy",
			parsed:    voice.ParsedInput{Input: "Hello"},
			wantErr:   "resolve voice",
		},
		{
			name: "disabled endpoint",
			endpoints: map[string]*model.Endpoint{"ep-1": {
				ID: "ep-1", Name: "Test", BaseURL: "http://upstream",
				Models: model.StringSlice{"tts-1"}, Enabled: false,
			}},
			aliases: []model.VoiceAlias{{
				Name: "narrator", EndpointID: "ep-1", Model: "tts-1", Voice: "nova", Enabled: true,
			}},
			voiceName: "narrator",
			parsed:    voice.ParsedInput{Input: "Hello"},
			wantErr:   "endpoint ep-1 is disabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proxy := newSegmentTestProxy(tt.endpoints, tt.aliases, nil)

			plan, err := proxy.resolveSynthesis(context.Background(), tt.voiceName, tt.parsed)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, plan)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, plan)
			tt.check(t, plan)
		})
	}
}

func TestProxy_ResolveSynthesis_EndpointFetchFails(t *testing.T) {
	// The resolver sees the endpoint, but the proxy's getter does not — the
	// GetEndpoint failure must be wrapped rather than panicking on a nil plan.
	ep := &model.Endpoint{
		ID: "ep-1", Name: "Test", BaseURL: "http://upstream",
		Models: model.StringSlice{"tts-1"}, Enabled: true,
	}
	resolver := voice.NewResolver(
		&mockEndpointStore{endpoints: map[string]*model.Endpoint{"ep-1": ep}},
		&mockAliasStore{},
	)
	empty := &mockEndpointStore{endpoints: map[string]*model.Endpoint{}}
	factory := func(e *model.Endpoint) *Client { return NewClient(e.BaseURL, e.APIKey, nil) }
	proxy := NewProxy(resolver, empty, factory, slog.Default())

	plan, err := proxy.resolveSynthesis(context.Background(), "alloy (Test, tts-1)", voice.ParsedInput{Input: "Hi"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "get endpoint ep-1")
	assert.Nil(t, plan)
}

// --- openSegment -----------------------------------------------------------

func TestProxy_OpenSegment(t *testing.T) {
	pcm := make([]byte, 512)
	for i := range pcm {
		pcm[i] = byte(i % 256)
	}

	tests := []struct {
		name       string
		handler    http.HandlerFunc
		endpoint   func(baseURL string) *model.Endpoint
		text       string
		wantErr    string
		wantFormat *AudioFormat
		wantPCM    []byte
	}{
		{
			name: "buffered mode reads the format from the wav header",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write(buildTestWAV(16000, 2, 1, pcm))
			},
			endpoint: func(baseURL string) *model.Endpoint {
				return &model.Endpoint{
					ID: "ep-1", Name: "Test", BaseURL: baseURL,
					Models: model.StringSlice{"tts-1"}, DefaultResponseFormat: "wav", Enabled: true,
				}
			},
			text:       "Hello",
			wantFormat: &AudioFormat{Rate: 16000, Width: 2, Channels: 1},
			wantPCM:    pcm,
		},
		{
			name: "streaming mode takes the configured sample rate",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write(pcm)
			},
			endpoint: func(baseURL string) *model.Endpoint {
				return &model.Endpoint{
					ID: "ep-1", Name: "Test", BaseURL: baseURL,
					Models: model.StringSlice{"tts-1"}, Enabled: true,
					StreamingEnabled: true, StreamSampleRate: 48000,
				}
			},
			text:       "Hello",
			wantFormat: &AudioFormat{Rate: 48000, Width: 2, Channels: 1},
			wantPCM:    pcm,
		},
		{
			name: "streaming mode falls back to 24000 Hz",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write(pcm)
			},
			endpoint: func(baseURL string) *model.Endpoint {
				return &model.Endpoint{
					ID: "ep-1", Name: "Test", BaseURL: baseURL,
					Models: model.StringSlice{"tts-1"}, Enabled: true,
					StreamingEnabled: true,
				}
			},
			text:       "Hello",
			wantFormat: &AudioFormat{Rate: 24000, Width: 2, Channels: 1},
			wantPCM:    pcm,
		},
		{
			name: "buffered upstream error",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			endpoint: func(baseURL string) *model.Endpoint {
				return &model.Endpoint{
					ID: "ep-1", Name: "Test", BaseURL: baseURL,
					Models: model.StringSlice{"tts-1"}, Enabled: true,
				}
			},
			text:    "Hello",
			wantErr: "tts api call: ",
		},
		{
			name: "streaming upstream error",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			endpoint: func(baseURL string) *model.Endpoint {
				return &model.Endpoint{
					ID: "ep-1", Name: "Test", BaseURL: baseURL,
					Models: model.StringSlice{"tts-1"}, Enabled: true, StreamingEnabled: true,
				}
			},
			text:    "Hello",
			wantErr: "tts api call (streaming): ",
		},
		{
			// The client rejects a body with no RIFF magic before openSegment
			// sees it, so reaching ReadFormat needs a RIFF that is not a WAVE.
			name: "unparseable wav header",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte("RIFF\x00\x00\x00\x00NOPE"))
			},
			endpoint: func(baseURL string) *model.Endpoint {
				return &model.Endpoint{
					ID: "ep-1", Name: "Test", BaseURL: baseURL,
					Models: model.StringSlice{"tts-1"}, Enabled: true,
				}
			},
			text:    "Hello",
			wantErr: "parse wav header: ",
		},
		{
			name:    "unsupported response format never reaches the upstream",
			handler: func(_ http.ResponseWriter, _ *http.Request) { t.Error("upstream must not be called") },
			endpoint: func(baseURL string) *model.Endpoint {
				return &model.Endpoint{
					ID: "ep-1", Name: "Test", BaseURL: baseURL,
					Models: model.StringSlice{"tts-1"}, DefaultResponseFormat: "mp3", Enabled: true,
				}
			},
			text:    "Hello",
			wantErr: `unsupported response format "mp3"; only "wav" is supported by proxy`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(tt.handler)
			defer server.Close()

			ep := tt.endpoint(server.URL)
			proxy := newSegmentTestProxy(map[string]*model.Endpoint{ep.ID: ep}, nil, server.Client())
			plan, err := proxy.resolveSynthesis(context.Background(), "alloy (Test, tts-1)", voice.ParsedInput{Input: tt.text})
			require.NoError(t, err)

			seg, err := proxy.openSegment(context.Background(), plan, tt.text)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, seg)
				return
			}
			require.NoError(t, err)
			defer seg.Close()

			assert.Equal(t, tt.wantFormat, seg.Format())
			got, readErr := io.ReadAll(seg)
			require.NoError(t, readErr)
			assert.Equal(t, tt.wantPCM, got)
		})
	}
}

func TestProxy_OpenSegment_SendsSegmentTextAndMergedParams(t *testing.T) {
	// The plan is resolved once for a whole message; openSegment must send the
	// per-segment text it is given, not the plan's own input.
	tests := []struct {
		name      string
		streaming bool
	}{
		{name: "buffered", streaming: false},
		{name: "streaming", streaming: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
				if tt.streaming {
					_, _ = w.Write(make([]byte, 16))
					return
				}
				_, _ = w.Write(buildTestWAV(24000, 2, 1, make([]byte, 16)))
			}))
			defer server.Close()

			ep := &model.Endpoint{
				ID: "ep-1", Name: "Test", BaseURL: server.URL,
				Models: model.StringSlice{"tts-1"}, Enabled: true,
				StreamingEnabled: tt.streaming,
				DefaultSpeed:     floatPtr(0.9),
			}
			proxy := newSegmentTestProxy(map[string]*model.Endpoint{"ep-1": ep}, nil, server.Client())
			plan, err := proxy.resolveSynthesis(context.Background(), "alloy (Test, tts-1)",
				voice.ParsedInput{Input: "the whole message", Model: "gpt-4o-mini-tts"})
			require.NoError(t, err)

			seg, err := proxy.openSegment(context.Background(), plan, "just this segment")
			require.NoError(t, err)
			defer seg.Close()

			assert.Equal(t, "just this segment", body["input"])
			assert.Equal(t, "gpt-4o-mini-tts", body["model"], "an input model override must reach the upstream")
			assert.Equal(t, "alloy", body["voice"])
			assert.InDelta(t, 0.9, body["speed"], 1e-9)
		})
	}
}

func TestProxy_OpenSegment_WritesNothing(t *testing.T) {
	// R8: a segment's format must be known with zero bytes on the wire, so a
	// mismatched segment can be rejected before any audio-start is emitted.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(buildTestWAV(24000, 2, 1, make([]byte, 512)))
	}))
	defer server.Close()

	ep := &model.Endpoint{
		ID: "ep-1", Name: "Test", BaseURL: server.URL,
		Models: model.StringSlice{"tts-1"}, Enabled: true,
	}
	proxy := newSegmentTestProxy(map[string]*model.Endpoint{"ep-1": ep}, nil, server.Client())
	plan, err := proxy.resolveSynthesis(context.Background(), "alloy (Test, tts-1)", voice.ParsedInput{Input: "Hello"})
	require.NoError(t, err)

	var buf bytes.Buffer
	seg, err := proxy.openSegment(context.Background(), plan, "Hello")
	require.NoError(t, err)
	defer seg.Close()

	require.NotNil(t, seg.Format(), "the format must be known after openSegment")
	assert.Zero(t, buf.Len(), "openSegment must write nothing")

	require.NoError(t, proxy.emitSegment(&buf, seg))
	assert.NotZero(t, buf.Len(), "emitSegment is what writes")
}

func TestProxy_OpenSegment_FormatMismatchRejectedWithNoBytesWritten(t *testing.T) {
	// Two buffered responses with different sample rates. The second segment is
	// opened, found to disagree, and closed — the writer must be untouched by it.
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		rate := 24000
		if calls.Add(1) > 1 {
			rate = 16000
		}
		_, _ = w.Write(buildTestWAV(rate, 2, 1, make([]byte, 512)))
	}))
	defer server.Close()

	ep := &model.Endpoint{
		ID: "ep-1", Name: "Test", BaseURL: server.URL,
		Models: model.StringSlice{"tts-1"}, Enabled: true,
	}
	proxy := newSegmentTestProxy(map[string]*model.Endpoint{"ep-1": ep}, nil, server.Client())
	plan, err := proxy.resolveSynthesis(context.Background(), "alloy (Test, tts-1)", voice.ParsedInput{Input: "Hello"})
	require.NoError(t, err)

	var buf bytes.Buffer

	first, err := proxy.openSegment(context.Background(), plan, "first")
	require.NoError(t, err)
	require.NoError(t, proxy.emitSegment(&buf, first))
	require.NoError(t, first.Close())
	afterFirst := buf.Len()

	second, err := proxy.openSegment(context.Background(), plan, "second")
	require.NoError(t, err)
	assert.NotEqual(t, first.Format(), second.Format())
	require.NoError(t, second.Close())

	assert.Equal(t, afterFirst, buf.Len(), "a rejected segment must put no bytes on the wire")
	events := readAllEvents(t, buf.Bytes())
	require.Len(t, events, 3) // audio-start + 1 chunk + audio-stop for segment 1 only
	assert.Equal(t, wyoming.TypeAudioStart, events[0].Type)
	assert.Equal(t, wyoming.TypeAudioStop, events[2].Type)
}

func TestProxy_OpenSegment_CloseReleasesBody(t *testing.T) {
	tests := []struct {
		name      string
		streaming bool
		payload   func() []byte
	}{
		{name: "buffered", streaming: false, payload: func() []byte { return buildTestWAV(24000, 2, 1, make([]byte, 64)) }},
		{name: "streaming", streaming: true, payload: func() []byte { return make([]byte, 64) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := &segTrackedBody{Reader: bytes.NewReader(tt.payload())}
			httpClient := &http.Client{Transport: &segStubTransport{fn: func(_ *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Body: body, Header: make(http.Header)}, nil
			}}}

			ep := &model.Endpoint{
				ID: "ep-1", Name: "Test", BaseURL: "http://upstream",
				Models: model.StringSlice{"tts-1"}, Enabled: true, StreamingEnabled: tt.streaming,
			}
			proxy := newSegmentTestProxy(map[string]*model.Endpoint{"ep-1": ep}, nil, httpClient)
			plan, err := proxy.resolveSynthesis(context.Background(), "alloy (Test, tts-1)", voice.ParsedInput{Input: "Hi"})
			require.NoError(t, err)

			seg, err := proxy.openSegment(context.Background(), plan, "Hi")
			require.NoError(t, err)
			assert.Zero(t, body.closes.Load(), "an open segment must keep its body open")

			// Discarded without ever being emitted, as a cancelled prefetch is.
			require.NoError(t, seg.Close())
			assert.Equal(t, int32(1), body.closes.Load())

			require.NoError(t, seg.Close())
			assert.Equal(t, int32(1), body.closes.Load(), "Close must be idempotent")
		})
	}
}

func TestProxy_OpenSegment_ClosesBodyOnHeaderFailure(t *testing.T) {
	body := &segTrackedBody{Reader: bytes.NewReader([]byte("RIFF\x00\x00\x00\x00NOPE"))}
	httpClient := &http.Client{Transport: &segStubTransport{fn: func(_ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: body, Header: make(http.Header)}, nil
	}}}

	ep := &model.Endpoint{
		ID: "ep-1", Name: "Test", BaseURL: "http://upstream",
		Models: model.StringSlice{"tts-1"}, Enabled: true,
	}
	proxy := newSegmentTestProxy(map[string]*model.Endpoint{"ep-1": ep}, nil, httpClient)
	plan, err := proxy.resolveSynthesis(context.Background(), "alloy (Test, tts-1)", voice.ParsedInput{Input: "Hi"})
	require.NoError(t, err)

	seg, err := proxy.openSegment(context.Background(), plan, "Hi")

	require.Error(t, err)
	assert.Nil(t, seg)
	assert.Equal(t, int32(1), body.closes.Load(), "a failed open must not leak the body")
}

// --- emitSegment -----------------------------------------------------------

func TestProxy_EmitSegment(t *testing.T) {
	tests := []struct {
		name       string
		format     *AudioFormat
		pcm        []byte
		wantChunks []int // length of each expected audio-chunk
	}{
		{
			name:       "two full chunks",
			format:     &AudioFormat{Rate: 24000, Width: 2, Channels: 1},
			pcm:        make([]byte, 2*chunkSize),
			wantChunks: []int{chunkSize, chunkSize},
		},
		{
			name:       "partial chunk",
			format:     &AudioFormat{Rate: 22050, Width: 2, Channels: 1},
			pcm:        make([]byte, 100),
			wantChunks: []int{100},
		},
		{
			name:       "exact chunk boundary",
			format:     &AudioFormat{Rate: 24000, Width: 2, Channels: 1},
			pcm:        make([]byte, chunkSize),
			wantChunks: []int{chunkSize},
		},
		{
			name:       "empty pcm still frames the group",
			format:     &AudioFormat{Rate: 24000, Width: 2, Channels: 1},
			pcm:        nil,
			wantChunks: nil,
		},
	}

	proxy := NewProxy(nil, nil, nil, slog.Default())

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seg := newTestSegment(tt.format, bytes.NewReader(tt.pcm))

			var buf bytes.Buffer
			require.NoError(t, proxy.emitSegment(&buf, seg))

			events := readAllEvents(t, buf.Bytes())
			require.Len(t, events, len(tt.wantChunks)+2)

			assert.Equal(t, wyoming.TypeAudioStart, events[0].Type)
			start, err := wyoming.AudioStartFromEvent(events[0])
			require.NoError(t, err)
			assert.Equal(t, tt.format.Rate, start.Rate)
			assert.Equal(t, tt.format.Width, start.Width)
			assert.Equal(t, tt.format.Channels, start.Channels)

			for i, want := range tt.wantChunks {
				ev := events[i+1]
				assert.Equal(t, wyoming.TypeAudioChunk, ev.Type)
				chunk, chunkErr := wyoming.AudioChunkFromEvent(ev)
				require.NoError(t, chunkErr)
				assert.Len(t, chunk.Audio, want)
				assert.Equal(t, tt.format.Rate, chunk.Rate)
			}

			assert.Equal(t, wyoming.TypeAudioStop, events[len(events)-1].Type)
		})
	}
}

func TestProxy_EmitSegment_DoesNotCloseTheSegment(t *testing.T) {
	body := &segTrackedBody{Reader: bytes.NewReader(make([]byte, 64))}
	seg := &OpenSegment{format: &AudioFormat{Rate: 24000, Width: 2, Channels: 1}, pcm: body, body: body}

	var buf bytes.Buffer
	proxy := NewProxy(nil, nil, nil, slog.Default())
	require.NoError(t, proxy.emitSegment(&buf, seg))

	assert.Zero(t, body.closes.Load(), "emitSegment must leave ownership with the opener")
	require.NoError(t, seg.Close())
	assert.Equal(t, int32(1), body.closes.Load())
}

func TestProxy_EmitSegment_Errors(t *testing.T) {
	format := &AudioFormat{Rate: 24000, Width: 2, Channels: 1}

	tests := []struct {
		name    string
		pcm     io.Reader
		writer  io.Writer
		wantErr string
	}{
		{
			name:    "audio-start write fails",
			pcm:     bytes.NewReader(make([]byte, 64)),
			writer:  &segErrWriter{okWrites: 0},
			wantErr: "write audio-start",
		},
		{
			name:    "audio-chunk write fails",
			pcm:     bytes.NewReader(make([]byte, 64)),
			writer:  &segErrWriter{okWrites: 1},
			wantErr: "write audio-chunk",
		},
		{
			name:    "audio-stop write fails",
			pcm:     bytes.NewReader(nil),
			writer:  &segErrWriter{okWrites: 1},
			wantErr: "write audio-stop",
		},
		{
			name:    "pcm read fails",
			pcm:     &segErrReader{prefix: make([]byte, 64)},
			writer:  &bytes.Buffer{},
			wantErr: "read pcm data",
		},
	}

	proxy := NewProxy(nil, nil, nil, slog.Default())

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := proxy.emitSegment(tt.writer, newTestSegment(format, tt.pcm))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
