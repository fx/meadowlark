package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/fx/meadowlark/internal/model"
	"github.com/fx/meadowlark/internal/segment"
	"github.com/fx/meadowlark/internal/tts"
	"github.com/fx/meadowlark/internal/voice"
	"github.com/fx/meadowlark/internal/wyoming"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- helpers ---------------------------------------------------------------

// testStore satisfies every read interface the Wyoming handler needs: the
// endpoint and alias listers for voice resolution and info, the endpoint-voice
// lister for the canonical voice list, and the endpoint getter for synthesis.
type testStore struct {
	endpoints []model.Endpoint
	aliases   []model.VoiceAlias
	voices    map[string][]model.EndpointVoice
}

func (s *testStore) ListEndpoints(context.Context) ([]model.Endpoint, error) {
	return s.endpoints, nil
}

func (s *testStore) ListVoiceAliases(context.Context) ([]model.VoiceAlias, error) {
	return s.aliases, nil
}

func (s *testStore) ListEndpointVoices(_ context.Context, endpointID string) ([]model.EndpointVoice, error) {
	return s.voices[endpointID], nil
}

func (s *testStore) GetEndpoint(_ context.Context, id string) (*model.Endpoint, error) {
	for i := range s.endpoints {
		if s.endpoints[i].ID == id {
			return &s.endpoints[i], nil
		}
	}
	return nil, fmt.Errorf("endpoint %q not found", id)
}

// singleEndpointStore is one enabled buffered-WAV endpoint served by baseURL.
func singleEndpointStore(baseURL string) *testStore {
	return &testStore{
		endpoints: []model.Endpoint{{
			ID: "ep-1", Name: "Test", BaseURL: baseURL, APIKey: "k",
			Models: model.StringSlice{"tts-1"}, DefaultVoice: "alloy",
			DefaultResponseFormat: "wav", Enabled: true,
		}},
		voices: map[string][]model.EndpointVoice{
			"ep-1": {{EndpointID: "ep-1", VoiceID: "alloy", Enabled: true}},
		},
	}
}

// testWAV builds a minimal 16-bit mono WAV carrying pcmBytes of silence.
func testWAV(rate, pcmBytes int) []byte {
	const channels, width = 1, 2
	pcm := make([]byte, pcmBytes)
	buf := &bytes.Buffer{}
	buf.WriteString("RIFF")
	_ = binary.Write(buf, binary.LittleEndian, uint32(4+24+8+len(pcm)))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	_ = binary.Write(buf, binary.LittleEndian, uint32(16))
	_ = binary.Write(buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(buf, binary.LittleEndian, uint16(channels))
	_ = binary.Write(buf, binary.LittleEndian, uint32(rate))
	_ = binary.Write(buf, binary.LittleEndian, uint32(rate*channels*width))
	_ = binary.Write(buf, binary.LittleEndian, uint16(channels*width))
	_ = binary.Write(buf, binary.LittleEndian, uint16(width*8))
	buf.WriteString("data")
	_ = binary.Write(buf, binary.LittleEndian, uint32(len(pcm)))
	buf.Write(pcm)
	return buf.Bytes()
}

// fakeUpstream is an OpenAI-compatible speech endpoint recording the input of
// every request it answers. respond defaults to a 24 kHz WAV.
type fakeUpstream struct {
	*httptest.Server

	mu     sync.Mutex
	inputs []string

	respond func(n int, w http.ResponseWriter)
}

func newFakeUpstream(t *testing.T, respond func(n int, w http.ResponseWriter)) *fakeUpstream {
	t.Helper()
	if respond == nil {
		respond = func(_ int, w http.ResponseWriter) {
			w.Header().Set("Content-Type", "audio/wav")
			_, _ = w.Write(testWAV(24000, 640))
		}
	}
	u := &fakeUpstream{respond: respond}
	u.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if !assert.NoError(t, err) {
			return
		}
		var call struct {
			Input string `json:"input"`
		}
		if !assert.NoError(t, json.Unmarshal(body, &call)) {
			return
		}
		u.mu.Lock()
		n := len(u.inputs)
		u.inputs = append(u.inputs, call.Input)
		u.mu.Unlock()
		u.respond(n, w)
	}))
	t.Cleanup(u.Close)
	return u
}

func (u *fakeUpstream) recorded() []string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]string(nil), u.inputs...)
}

// syncWriter stands in for the server's mutex-guarded connection writer: the
// session's emitter goroutine and the caller both write to it.
type syncWriter struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (w *syncWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *syncWriter) events(t *testing.T) []*wyoming.Event {
	t.Helper()
	w.mu.Lock()
	snapshot := append([]byte(nil), w.buf.Bytes()...)
	w.mu.Unlock()
	return decodeEvents(t, snapshot)
}

func (w *syncWriter) types(t *testing.T) []string {
	t.Helper()
	return eventTypes(w.events(t))
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// decodeEvents reads every complete event out of a captured byte stream.
func decodeEvents(t *testing.T, raw []byte) []*wyoming.Event {
	t.Helper()
	r := bufio.NewReader(bytes.NewReader(raw))
	var out []*wyoming.Event
	for {
		ev, err := wyoming.ReadEvent(r)
		if err != nil {
			require.ErrorIs(t, err, io.EOF, "trailing bytes must be a clean end of stream")
			return out
		}
		out = append(out, ev)
	}
}

func eventTypes(evs []*wyoming.Event) []string {
	var out []string
	for _, ev := range evs {
		out = append(out, ev.Type)
	}
	return out
}

// collapseChunks rewrites each run of audio-chunk events as a single
// audio-chunk. How the PCM happens to split across chunk events depends on how
// the upstream body arrives and is not part of the contract; the group framing
// is.
func collapseChunks(types []string) []string {
	var out []string
	for i, ty := range types {
		if ty == wyoming.TypeAudioChunk && i > 0 && types[i-1] == wyoming.TypeAudioChunk {
			continue
		}
		out = append(out, ty)
	}
	return out
}

// newTestHandler builds the production Wyoming handler over a fake store.
func newTestHandler(t *testing.T, st *testStore, idle time.Duration) *wyomingHandler {
	t.Helper()
	resolver := voice.NewResolver(st, st)
	proxy := tts.NewProxy(resolver, st, defaultClientFactory, testLogger())
	info := wyoming.NewInfoBuilder(st, st, st, "test")
	return newWyomingHandler(info, proxy, segment.DefaultConfig(), idle, testLogger())
}

// newTestConnHandler builds a per-connection handler exactly as the server does.
func newTestConnHandler(t *testing.T, st *testStore, idle time.Duration) *connHandler {
	t.Helper()
	h, ok := newTestHandler(t, st, idle).NewConnHandler().(*connHandler)
	require.True(t, ok, "NewConnHandler must return a *connHandler")
	t.Cleanup(h.CloseConn)
	return h
}

// --- configuration ---------------------------------------------------------

func TestSegmentConfig(t *testing.T) {
	def := segment.DefaultConfig()

	tests := []struct {
		name                  string
		first, minimum, maxim int
		want                  segment.Config
	}{
		{
			name:  "coherent thresholds are honoured",
			first: 10, minimum: 20, maxim: 30,
			want: segment.Config{FirstSegmentChars: 10, MinSegmentChars: 20, MaxSegmentChars: 30},
		},
		{
			name:  "equal thresholds are coherent",
			first: 50, minimum: 50, maxim: 50,
			want: segment.Config{FirstSegmentChars: 50, MinSegmentChars: 50, MaxSegmentChars: 50},
		},
		{
			name:  "min above max falls back to all three defaults",
			first: 24, minimum: 600, maxim: 400,
			want: def,
		},
		{
			name:  "first above min falls back to all three defaults",
			first: 100, minimum: 60, maxim: 400,
			want: def,
		},
		{
			name:  "zero threshold falls back to all three defaults",
			first: 0, minimum: 60, maxim: 400,
			want: def,
		},
		{
			name:  "negative threshold falls back to all three defaults",
			first: 24, minimum: 60, maxim: -1,
			want: def,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := segmentConfig(testLogger(), tt.first, tt.minimum, tt.maxim)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSegmentConfigWarnsOnIncoherentThresholds(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	got := segmentConfig(logger, 24, 600, 400)

	assert.Equal(t, segment.DefaultConfig(), got)
	assert.Contains(t, buf.String(), "falling back to defaults")
	assert.Contains(t, buf.String(), "600")
}

func TestSessionTimeout(t *testing.T) {
	tests := []struct {
		name string
		in   time.Duration
		want time.Duration
	}{
		{name: "positive enables the timer at that duration", in: 5 * time.Second, want: 5 * time.Second},
		{name: "zero disables the timer", in: 0, want: 0},
		{name: "negative is rejected in favour of the default", in: -time.Second, want: defaultSessionTimeout},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, sessionTimeout(testLogger(), tt.in))
		})
	}
}

// A negative duration would fire a timer immediately, so it must be named in
// the warning rather than silently treated as "disabled".
func TestSessionTimeoutWarningNamesTheSuppliedValue(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))

	got := sessionTimeout(logger, -1500*time.Millisecond)

	assert.Equal(t, defaultSessionTimeout, got)
	assert.Contains(t, buf.String(), "-1.5s")
	assert.Contains(t, buf.String(), "30s")
}

func TestStreamingFlagDefaults(t *testing.T) {
	cmd := newRootCmd()

	tests := []struct{ flag, want string }{
		{flag: "synthesize-first-segment-chars", want: "24"},
		{flag: "synthesize-min-segment-chars", want: "60"},
		{flag: "synthesize-max-segment-chars", want: "400"},
		{flag: "synthesize-session-timeout", want: "30s"},
	}

	for _, tt := range tests {
		t.Run(tt.flag, func(t *testing.T) {
			f := cmd.Flags().Lookup(tt.flag)
			require.NotNil(t, f, "flag must be registered")
			assert.Equal(t, tt.want, f.DefValue)
		})
	}
}

// Every flag carries the project's standard MEADOWLARK_ env fallback, derived
// from the bound key with dashes replaced by underscores.
func TestStreamingFlagEnvFallbacks(t *testing.T) {
	newRootCmd() // installs the env prefix, replacer and bindings on viper

	t.Setenv("MEADOWLARK_SYNTHESIZE_FIRST_SEGMENT_CHARS", "12")
	t.Setenv("MEADOWLARK_SYNTHESIZE_MIN_SEGMENT_CHARS", "34")
	t.Setenv("MEADOWLARK_SYNTHESIZE_MAX_SEGMENT_CHARS", "56")
	t.Setenv("MEADOWLARK_SYNTHESIZE_SESSION_TIMEOUT", "7s")

	assert.Equal(t, 12, viper.GetInt("synthesize_first_segment_chars"))
	assert.Equal(t, 34, viper.GetInt("synthesize_min_segment_chars"))
	assert.Equal(t, 56, viper.GetInt("synthesize_max_segment_chars"))
	assert.Equal(t, 7*time.Second, viper.GetDuration("synthesize_session_timeout"))
}

// --- dispatch --------------------------------------------------------------

func TestConnHandlerDescribeAdvertisesStreaming(t *testing.T) {
	c := newTestConnHandler(t, singleEndpointStore(""), time.Minute)
	w := &syncWriter{}

	require.NoError(t, c.HandleEvent(context.Background(), (&wyoming.Describe{}).ToEvent(), w))

	evs := w.events(t)
	require.Len(t, evs, 1)
	info, err := wyoming.InfoFromEvent(evs[0])
	require.NoError(t, err)
	require.Len(t, info.Tts, 1)
	assert.True(t, info.Tts[0].SupportsSynthesizeStreaming)
}

func TestConnHandlerPing(t *testing.T) {
	c := newTestConnHandler(t, singleEndpointStore(""), time.Minute)
	w := &syncWriter{}

	require.NoError(t, c.HandleEvent(context.Background(), (&wyoming.Ping{}).ToEvent(), w))

	assert.Equal(t, []string{wyoming.TypePong}, w.types(t))
}

// A stray event is logged at debug and ignored: it must not reach the session,
// must write nothing, and must not fail the connection.
func TestConnHandlerIgnoresUnknownEvent(t *testing.T) {
	c := newTestConnHandler(t, singleEndpointStore(""), time.Minute)
	w := &syncWriter{}

	err := c.HandleEvent(context.Background(), &wyoming.Event{Type: "not-a-real-event"}, w)

	require.NoError(t, err)
	assert.Empty(t, w.events(t))
	assert.False(t, c.session.Active())
}

// A bare synthesize with no session open is the ordinary whole-message path,
// untouched by this change.
func TestConnHandlerBareSynthesizeFallsThroughToTheProxy(t *testing.T) {
	up := newFakeUpstream(t, nil)
	c := newTestConnHandler(t, singleEndpointStore(up.URL), time.Minute)
	w := &syncWriter{}

	ev := (&wyoming.Synthesize{Text: "Hello there.", Voice: "alloy"}).ToEvent()
	require.NoError(t, c.HandleEvent(context.Background(), ev, w))

	assert.Equal(t, []string{"Hello there."}, up.recorded())
	assert.Equal(t, []string{
		wyoming.TypeAudioStart, wyoming.TypeAudioChunk, wyoming.TypeAudioStop,
	}, collapseChunks(w.types(t)))
}

// synthesize-stop with no session emits nothing at all — there is no session to
// terminate, so there is no terminator to write.
func TestConnHandlerStopWithoutSession(t *testing.T) {
	c := newTestConnHandler(t, singleEndpointStore(""), time.Minute)
	w := &syncWriter{}

	require.NoError(t, c.HandleEvent(context.Background(), (&wyoming.SynthesizeStop{}).ToEvent(), w))

	assert.Empty(t, w.events(t))
}

func TestConnHandlerStreamingDispatch(t *testing.T) {
	up := newFakeUpstream(t, nil)
	c := newTestConnHandler(t, singleEndpointStore(up.URL), time.Minute)
	w := &syncWriter{}
	ctx := context.Background()

	start := (&wyoming.SynthesizeStart{Voice: "alloy"}).ToEvent()
	require.NoError(t, c.HandleEvent(ctx, start, w))
	assert.True(t, c.session.Active(), "synthesize-start must open the session")

	for _, text := range []string{"The kitchen light is on. ", "The porch light is off."} {
		chunk := (&wyoming.SynthesizeChunk{Text: text}).ToEvent()
		require.NoError(t, c.HandleEvent(ctx, chunk, w))
	}

	// Home Assistant's compatibility synthesize repeats the whole message. The
	// session absorbs it, so the proxy must not see it.
	compat := (&wyoming.Synthesize{
		Text:  "The kitchen light is on. The porch light is off.",
		Voice: "alloy",
	}).ToEvent()
	require.NoError(t, c.HandleEvent(ctx, compat, w))

	require.NoError(t, c.HandleEvent(ctx, (&wyoming.SynthesizeStop{}).ToEvent(), w))
	assert.False(t, c.session.Active(), "synthesize-stop must close the session")

	assert.Equal(t, []string{
		"The kitchen light is on.",
		"The porch light is off.",
	}, up.recorded(), "each segment is one upstream request; the compat event adds none")

	assert.Equal(t, []string{
		wyoming.TypeAudioStart, wyoming.TypeAudioChunk, wyoming.TypeAudioStop,
		wyoming.TypeAudioStart, wyoming.TypeAudioChunk, wyoming.TypeAudioStop,
		wyoming.TypeSynthesizeStopped,
	}, collapseChunks(w.types(t)))
}

// With no chunk ever sent, the compatibility synthesize is the session's only
// content — absorbed, then spoken once when the session stops.
func TestConnHandlerZeroChunkFallback(t *testing.T) {
	up := newFakeUpstream(t, nil)
	c := newTestConnHandler(t, singleEndpointStore(up.URL), time.Minute)
	w := &syncWriter{}
	ctx := context.Background()

	require.NoError(t, c.HandleEvent(ctx, (&wyoming.SynthesizeStart{Voice: "alloy"}).ToEvent(), w))
	compat := (&wyoming.Synthesize{Text: "Only this.", Voice: "alloy"}).ToEvent()
	require.NoError(t, c.HandleEvent(ctx, compat, w))
	require.NoError(t, c.HandleEvent(ctx, (&wyoming.SynthesizeStop{}).ToEvent(), w))

	assert.Equal(t, []string{"Only this."}, up.recorded())
	assert.Equal(t, []string{
		wyoming.TypeAudioStart, wyoming.TypeAudioChunk, wyoming.TypeAudioStop,
		wyoming.TypeSynthesizeStopped,
	}, collapseChunks(w.types(t)))
}

// CloseConn is the server's teardown hook. It must be safe to call more than
// once and must leave no session behind.
func TestConnHandlerCloseConn(t *testing.T) {
	up := newFakeUpstream(t, nil)
	c := newTestConnHandler(t, singleEndpointStore(up.URL), time.Minute)
	w := &syncWriter{}
	ctx := context.Background()

	require.NoError(t, c.HandleEvent(ctx, (&wyoming.SynthesizeStart{Voice: "alloy"}).ToEvent(), w))
	require.NoError(t, c.HandleEvent(ctx, (&wyoming.SynthesizeChunk{Text: "Anything at all."}).ToEvent(), w))

	c.CloseConn()
	c.CloseConn()

	assert.False(t, c.session.Active())
	assert.NotContains(t, w.types(t), wyoming.TypeSynthesizeStopped,
		"a torn-down connection is not written to")
}

// Each connection gets its own handler, and with it its own session, so one
// connection's streaming state can never be observed by another.
func TestNewConnHandlerIsPerConnection(t *testing.T) {
	h := newTestHandler(t, singleEndpointStore(""), time.Minute)

	a, aOK := h.NewConnHandler().(*connHandler)
	b, bOK := h.NewConnHandler().(*connHandler)
	require.True(t, aOK)
	require.True(t, bOK)
	t.Cleanup(a.CloseConn)
	t.Cleanup(b.CloseConn)

	assert.NotSame(t, a, b)
	assert.NotSame(t, a.session, b.session)

	var w syncWriter
	require.NoError(t, a.HandleEvent(context.Background(), (&wyoming.SynthesizeStart{Voice: "alloy"}).ToEvent(), &w))
	assert.True(t, a.session.Active())
	assert.False(t, b.session.Active())
}

// The handler is handed to wyoming.NewServer as a Handler and used by it as a
// factory; both must hold.
func TestWyomingHandlerImplementsFactoryAndHandler(t *testing.T) {
	h := newTestHandler(t, singleEndpointStore(""), time.Minute)

	var _ wyoming.Handler = h
	var _ wyoming.HandlerFactory = h

	c, ok := h.NewConnHandler().(*connHandler)
	require.True(t, ok)
	t.Cleanup(c.CloseConn)
	var _ wyoming.Handler = c
	var _ wyoming.ConnHandler = c
}

// The sessionless dispatch on wyomingHandler is what a connHandler delegates
// to; it must keep answering describe, ping, synthesize and unknown by itself.
func TestWyomingHandlerSessionlessDispatch(t *testing.T) {
	up := newFakeUpstream(t, nil)
	h := newTestHandler(t, singleEndpointStore(up.URL), time.Minute)
	ctx := context.Background()

	tests := []struct {
		name  string
		event *wyoming.Event
		want  []string
	}{
		{name: "describe", event: (&wyoming.Describe{}).ToEvent(), want: []string{wyoming.TypeInfo}},
		{name: "ping", event: (&wyoming.Ping{}).ToEvent(), want: []string{wyoming.TypePong}},
		{
			name:  "synthesize",
			event: (&wyoming.Synthesize{Text: "Hi.", Voice: "alloy"}).ToEvent(),
			want:  []string{wyoming.TypeAudioStart, wyoming.TypeAudioChunk, wyoming.TypeAudioStop},
		},
		{name: "unknown", event: &wyoming.Event{Type: "nonsense"}, want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &syncWriter{}
			require.NoError(t, h.HandleEvent(ctx, tt.event, w))
			assert.Equal(t, tt.want, collapseChunks(w.types(t)))
		})
	}
}

// A describe whose info cannot be built is a handler error, which the server
// reports as a handler-error event; it must not be swallowed.
func TestWyomingHandlerDescribeErrorPropagates(t *testing.T) {
	st := &failingStore{}
	resolver := voice.NewResolver(st, st)
	proxy := tts.NewProxy(resolver, st, defaultClientFactory, testLogger())
	info := wyoming.NewInfoBuilder(st, st, st, "test")
	h := newWyomingHandler(info, proxy, segment.DefaultConfig(), time.Minute, testLogger())

	err := h.HandleEvent(context.Background(), (&wyoming.Describe{}).ToEvent(), &syncWriter{})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "build info")
}

// failingStore fails every read, so info cannot be built.
type failingStore struct{}

func (failingStore) ListEndpoints(context.Context) ([]model.Endpoint, error) {
	return nil, fmt.Errorf("store is down")
}

func (failingStore) ListVoiceAliases(context.Context) ([]model.VoiceAlias, error) {
	return nil, fmt.Errorf("store is down")
}

func (failingStore) ListEndpointVoices(context.Context, string) ([]model.EndpointVoice, error) {
	return nil, fmt.Errorf("store is down")
}

func (failingStore) GetEndpoint(context.Context, string) (*model.Endpoint, error) {
	return nil, fmt.Errorf("store is down")
}

// --- process plumbing ------------------------------------------------------

func TestOpenStore(t *testing.T) {
	t.Run("unsupported driver", func(t *testing.T) {
		db, err := openStore(context.Background(), "mysql", "dsn")
		require.Error(t, err)
		assert.Nil(t, db)
		assert.Contains(t, err.Error(), "unsupported database driver")
	})

	t.Run("sqlite", func(t *testing.T) {
		db, err := openStore(context.Background(), "sqlite", t.TempDir()+"/test.db")
		require.NoError(t, err)
		require.NotNil(t, db)
		require.NoError(t, db.Close())
	})
}

func TestClientFactories(t *testing.T) {
	ep := &model.Endpoint{ID: "ep-1", BaseURL: "http://example.invalid", APIKey: "k"}
	assert.NotNil(t, defaultClientFactory(ep))
	assert.NotNil(t, apiClientFactory(ep))
}

func TestConfigureLogger(t *testing.T) {
	tests := []struct {
		level  string
		format string
		enable slog.Level
		reject slog.Level
	}{
		{level: "debug", format: "text", enable: slog.LevelDebug},
		{level: "info", format: "json", enable: slog.LevelInfo, reject: slog.LevelDebug},
		{level: "warn", format: "text", enable: slog.LevelWarn, reject: slog.LevelInfo},
		{level: "error", format: "text", enable: slog.LevelError, reject: slog.LevelWarn},
		{level: "nonsense", format: "nonsense", enable: slog.LevelInfo, reject: slog.LevelDebug},
	}

	for _, tt := range tests {
		t.Run(tt.level+"/"+tt.format, func(t *testing.T) {
			logger := configureLogger(tt.level, tt.format)
			require.NotNil(t, logger)
			assert.True(t, logger.Enabled(context.Background(), tt.enable))
			if tt.reject != 0 {
				assert.False(t, logger.Enabled(context.Background(), tt.reject))
			}
		})
	}
}

func TestDashReplacer(t *testing.T) {
	assert.Equal(t, "synthesize_min_segment_chars",
		dashReplacer().Replace("synthesize-min-segment-chars"))
}
