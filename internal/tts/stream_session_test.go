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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fx/meadowlark/internal/model"
	"github.com/fx/meadowlark/internal/segment"
	"github.com/fx/meadowlark/internal/wyoming"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- helpers ---------------------------------------------------------------

// upstreamCall is one decoded /audio/speech request body. Both the buffered and
// the streaming request shapes fit it.
type upstreamCall struct {
	Model          string   `json:"model"`
	Voice          string   `json:"voice"`
	Input          string   `json:"input"`
	ResponseFormat string   `json:"response_format"`
	Speed          *float64 `json:"speed"`
	Instructions   *string  `json:"instructions"`
	Stream         bool     `json:"stream"`
}

// fakeUpstream is an OpenAI-compatible speech endpoint that records every
// request and answers it from a per-index script, so a test can make the second
// segment fail, change format, or block.
type fakeUpstream struct {
	*httptest.Server

	mu    sync.Mutex
	calls []upstreamCall

	respond func(n int, call upstreamCall, w http.ResponseWriter, r *http.Request)
}

func newFakeUpstream(t *testing.T, respond func(n int, call upstreamCall, w http.ResponseWriter, r *http.Request)) *fakeUpstream {
	t.Helper()
	u := &fakeUpstream{respond: respond}
	u.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if !assert.NoError(t, err) {
			return
		}
		var call upstreamCall
		if !assert.NoError(t, json.Unmarshal(body, &call)) {
			return
		}

		u.mu.Lock()
		n := len(u.calls)
		u.calls = append(u.calls, call)
		u.mu.Unlock()

		u.respond(n, call, w, r)
	}))
	t.Cleanup(u.Close)
	return u
}

// wavResponder answers every request with a WAV of the given format.
func wavResponder(rate, pcmBytes int) func(int, upstreamCall, http.ResponseWriter, *http.Request) {
	return func(_ int, _ upstreamCall, w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write(buildTestWAV(rate, 2, 1, make([]byte, pcmBytes)))
	}
}

func (u *fakeUpstream) recorded() []upstreamCall {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]upstreamCall(nil), u.calls...)
}

func (u *fakeUpstream) inputs() []string {
	var out []string
	for _, c := range u.recorded() {
		out = append(out, c.Input)
	}
	return out
}

func (u *fakeUpstream) count() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.calls)
}

// syncWriter is the connection writer. The real one is the Wyoming server's
// mutex-guarded wrapper; this is the same contract, so the emitter goroutine and
// the test goroutine can both write to it.
type syncWriter struct {
	mu   sync.Mutex
	buf  bytes.Buffer
	fail func(n int) error // optional: fail the nth write
	n    int
}

func (w *syncWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.n++
	if w.fail != nil {
		if err := w.fail(w.n); err != nil {
			return 0, err
		}
	}
	return w.buf.Write(p)
}

func (w *syncWriter) snapshot() []byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]byte(nil), w.buf.Bytes()...)
}

func (w *syncWriter) events(t *testing.T) []*wyoming.Event {
	t.Helper()
	return readAllEvents(t, w.snapshot())
}

func (w *syncWriter) types(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, ev := range w.events(t) {
		out = append(out, ev.Type)
	}
	return out
}

// collapse rewrites each run of audio-chunk events as a single "audio-chunk+"
// token. How the PCM splits across chunk events depends on how the upstream's
// body happens to arrive and is not part of the contract; the group framing is.
func collapse(types []string) []string {
	var out []string
	for i, ty := range types {
		if ty == wyoming.TypeAudioChunk {
			if i > 0 && types[i-1] == wyoming.TypeAudioChunk {
				continue
			}
			out = append(out, wyoming.TypeAudioChunk+"+")
			continue
		}
		out = append(out, ty)
	}
	return out
}

// waitForEvent blocks until at least one event of the given type has been
// written. Every event is a single Write, so a snapshot always ends on an event
// boundary.
func waitForEvent(t *testing.T, w *syncWriter, ty string) {
	t.Helper()
	require.Eventually(t, func() bool {
		return countType(w.types(t), ty) > 0
	}, 3*time.Second, 5*time.Millisecond)
}

// countType counts events of one type in a type list.
func countType(types []string, want string) int {
	n := 0
	for _, ty := range types {
		if ty == want {
			n++
		}
	}
	return n
}

// errorEvents extracts every Wyoming error event.
func errorEvents(t *testing.T, w *syncWriter) []*wyoming.Error {
	t.Helper()
	var out []*wyoming.Error
	for _, ev := range w.events(t) {
		if ev.Type == wyoming.TypeError {
			e, err := wyoming.ErrorFromEvent(ev)
			require.NoError(t, err)
			out = append(out, e)
		}
	}
	return out
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// wavEndpoint is a buffered-mode endpoint served by u.
func wavEndpoint(u *fakeUpstream) map[string]*model.Endpoint {
	return map[string]*model.Endpoint{"ep-1": {
		ID: "ep-1", Name: "Test", BaseURL: u.URL, APIKey: "k",
		Models: model.StringSlice{"tts-1"}, DefaultVoice: "alloy",
		DefaultResponseFormat: "wav", Enabled: true,
	}}
}

func newTestStreamSession(t *testing.T, eps map[string]*model.Endpoint, client *http.Client, idle time.Duration) *StreamSession {
	t.Helper()
	proxy := newSegmentTestProxy(eps, nil, client)
	return NewStreamSession(proxy, segment.DefaultConfig(), idle, testLogger())
}

// feed drives a whole Home Assistant message through a session.
func feed(t *testing.T, s *StreamSession, w io.Writer, voiceName string, chunks []string, compat string) {
	t.Helper()
	require.NoError(t, s.Start(context.Background(), w, &wyoming.SynthesizeStart{Voice: voiceName}))
	for _, c := range chunks {
		require.NoError(t, s.Chunk(&wyoming.SynthesizeChunk{Text: c}))
	}
	if compat != "" {
		assert.True(t, s.Compat(&wyoming.Synthesize{Text: compat, Voice: voiceName}))
	}
	require.NoError(t, s.Stop())
}

// --- happy paths -----------------------------------------------------------

func TestStreamSession_OrderedEmission(t *testing.T) {
	// The R6 worked example: two segments, not three. The boundary after
	// "region" is passed over for being below minSegmentChars, and the final
	// period ends the buffer so it never satisfies the trailing-whitespace guard.
	const message = "The weather is sunny today and quite warm. " +
		"Tomorrow will bring rain across the whole region. Bring an umbrella."

	tests := []struct {
		name   string
		chunks []string
	}{
		{name: "one chunk", chunks: []string{message}},
		{name: "token sized chunks", chunks: splitEvery(message, 7)},
		{name: "one rune at a time", chunks: splitEvery(message, 1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			up := newFakeUpstream(t, wavResponder(24000, 4096))
			s := newTestStreamSession(t, wavEndpoint(up), up.Client(), 0)
			w := &syncWriter{}

			feed(t, s, w, "alloy (Test, tts-1)", tt.chunks, message)

			assert.Equal(t, []string{
				"The weather is sunny today and quite warm.",
				"Tomorrow will bring rain across the whole region. Bring an umbrella.",
			}, up.inputs())

			assert.Equal(t, []string{
				wyoming.TypeAudioStart, wyoming.TypeAudioChunk + "+", wyoming.TypeAudioStop,
				wyoming.TypeAudioStart, wyoming.TypeAudioChunk + "+", wyoming.TypeAudioStop,
				wyoming.TypeSynthesizeStopped,
			}, collapse(w.types(t)))
			assert.False(t, s.Active())
		})
	}
}

// splitEvery cuts s into runs of at most n runes.
func splitEvery(s string, n int) []string {
	runes := []rune(s)
	var out []string
	for i := 0; i < len(runes); i += n {
		end := min(i+n, len(runes))
		out = append(out, string(runes[i:end]))
	}
	return out
}

func TestStreamSession_StreamingEndpointMode(t *testing.T) {
	up := newFakeUpstream(t, func(_ int, _ upstreamCall, w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, 2048))
	})
	eps := map[string]*model.Endpoint{"ep-1": {
		ID: "ep-1", Name: "Test", BaseURL: up.URL, Models: model.StringSlice{"tts-1"},
		DefaultVoice: "alloy", Enabled: true, StreamingEnabled: true, StreamSampleRate: 16000,
	}}
	s := newTestStreamSession(t, eps, up.Client(), 0)
	w := &syncWriter{}

	feed(t, s, w, "alloy (Test, tts-1)", []string{"Turning the kitchen lights on for you now."}, "")

	events := w.events(t)
	require.NotEmpty(t, events)
	start, err := wyoming.AudioStartFromEvent(events[0])
	require.NoError(t, err)
	assert.Equal(t, 16000, start.Rate)
	assert.Equal(t, 2, start.Width)
	assert.Equal(t, 1, start.Channels)

	calls := up.recorded()
	require.Len(t, calls, 1)
	assert.True(t, calls[0].Stream)
	assert.Equal(t, "pcm", calls[0].ResponseFormat)
	assert.Equal(t, wyoming.TypeSynthesizeStopped, w.types(t)[len(events)-1])
}

func TestStreamSession_EmptyMessageStopsCleanly(t *testing.T) {
	up := newFakeUpstream(t, wavResponder(24000, 512))
	s := newTestStreamSession(t, wavEndpoint(up), up.Client(), 0)
	w := &syncWriter{}

	feed(t, s, w, "alloy (Test, tts-1)", []string{"   ", "\n"}, "")

	assert.Equal(t, 0, up.count())
	assert.Equal(t, []string{wyoming.TypeSynthesizeStopped}, w.types(t))
}

// --- compatibility synthesize (R7) -----------------------------------------

func TestStreamSession_CompatSynthesize(t *testing.T) {
	tests := []struct {
		name       string
		chunks     []string
		compat     string
		wantInputs []string
		wantSpeed  *float64
	}{
		{
			name:       "chunks present so the compatibility text is discarded",
			chunks:     []string{"The kitchen light is now on and set to warm white."},
			compat:     "The kitchen light is now on and set to warm white.",
			wantInputs: []string{"The kitchen light is now on and set to warm white."},
		},
		{
			name:       "zero chunks fall back to the compatibility text",
			compat:     "Hello world.",
			wantInputs: []string{"Hello world."},
		},
		{
			name:       "zero chunk fallback carrying an override",
			compat:     "[speed: 1.2] Hello.",
			wantInputs: []string{"Hello."},
			wantSpeed:  floatPtr(1.2),
		},
		{
			name:       "zero chunk fallback in json form",
			compat:     `{"voice": "shimmer", "input": "Lights are on."}`,
			wantInputs: []string{"Lights are on."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			up := newFakeUpstream(t, wavResponder(24000, 1024))
			s := newTestStreamSession(t, wavEndpoint(up), up.Client(), 0)
			w := &syncWriter{}

			feed(t, s, w, "alloy (Test, tts-1)", tt.chunks, tt.compat)

			assert.Equal(t, tt.wantInputs, up.inputs())
			assert.Equal(t, 1, countType(w.types(t), wyoming.TypeSynthesizeStopped))
			if tt.wantSpeed != nil {
				calls := up.recorded()
				require.Len(t, calls, 1)
				require.NotNil(t, calls[0].Speed)
				assert.InDelta(t, *tt.wantSpeed, *calls[0].Speed, 1e-9)
			}
		})
	}
}

func TestStreamSession_CompatIsNotSuppressedWhenIdle(t *testing.T) {
	up := newFakeUpstream(t, wavResponder(24000, 512))
	s := newTestStreamSession(t, wavEndpoint(up), up.Client(), 0)

	assert.False(t, s.Active())
	assert.False(t, s.Compat(&wyoming.Synthesize{Text: "Hello."}))
	assert.Equal(t, 0, up.count())
}

// --- override-form input (R6) ----------------------------------------------

func TestStreamSession_OverrideFormInput(t *testing.T) {
	tests := []struct {
		name         string
		chunks       []string
		wantInputs   []string
		wantVoice    string
		wantModel    string
		wantSpeed    *float64
		wantDeferred bool // nothing synthesized before synthesize-stop
	}{
		{
			name:         "json form arriving in fragments",
			chunks:       splitEvery(`{"voice": "alloy", "input": "Turning on the lights now."}`, 9),
			wantInputs:   []string{"Turning on the lights now."},
			wantVoice:    "alloy",
			wantModel:    "tts-1",
			wantDeferred: true,
		},
		{
			name:         "tag form with the bracket split across chunks",
			chunks:       []string{"[speed:", " 1.2] The kitchen light is now on."},
			wantInputs:   []string{"The kitchen light is now on."},
			wantVoice:    "nova",
			wantModel:    "tts-1",
			wantSpeed:    floatPtr(1.2),
			wantDeferred: true,
		},
		{
			name:         "model override survives the session",
			chunks:       splitEvery("[model: gpt-4o-mini-tts] Turning the lights on.", 6),
			wantInputs:   []string{"Turning the lights on."},
			wantVoice:    "nova",
			wantModel:    "gpt-4o-mini-tts",
			wantDeferred: true,
		},
		{
			name:         "leading whitespace does not hide the override marker",
			chunks:       []string{"  ", `{"input": "Done."}`},
			wantInputs:   []string{"Done."},
			wantVoice:    "nova",
			wantModel:    "tts-1",
			wantDeferred: true,
		},
		{
			name: "prose is never override parsed",
			chunks: []string{
				"The kitchen light is now on and set to warm white. ",
				"The hallway light stays off until sunset this evening. ",
			},
			wantInputs: []string{
				"The kitchen light is now on and set to warm white.",
				"The hallway light stays off until sunset this evening.",
			},
			wantVoice: "nova",
			wantModel: "tts-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			up := newFakeUpstream(t, wavResponder(24000, 512))
			eps := map[string]*model.Endpoint{"ep-1": {
				ID: "ep-1", Name: "OpenAI", BaseURL: up.URL,
				Models: model.StringSlice{"tts-1"}, DefaultResponseFormat: "wav", Enabled: true,
			}}
			s := newTestStreamSession(t, eps, up.Client(), 0)
			w := &syncWriter{}

			require.NoError(t, s.Start(context.Background(), w, &wyoming.SynthesizeStart{Voice: "nova (OpenAI, tts-1)"}))
			for _, c := range tt.chunks {
				require.NoError(t, s.Chunk(&wyoming.SynthesizeChunk{Text: c}))
			}

			if tt.wantDeferred {
				assert.Equal(t, 0, up.count(), "override-form input must not flush before synthesize-stop")
				assert.Empty(t, w.types(t), "override-form input must not emit audio before synthesize-stop")
			} else {
				require.Eventually(t, func() bool { return up.count() > 0 },
					3*time.Second, 5*time.Millisecond, "prose must segment incrementally")
			}

			require.NoError(t, s.Stop())

			assert.Equal(t, tt.wantInputs, up.inputs())
			calls := up.recorded()
			require.NotEmpty(t, calls)
			for _, c := range calls {
				assert.Equal(t, tt.wantVoice, c.Voice)
				assert.Equal(t, tt.wantModel, c.Model)
				if tt.wantSpeed != nil {
					require.NotNil(t, c.Speed)
					assert.InDelta(t, *tt.wantSpeed, *c.Speed, 1e-9)
				}
			}
			assert.Equal(t, 1, countType(w.types(t), wyoming.TypeSynthesizeStopped))
		})
	}
}

// TestStreamSession_OverrideDoesNotChangeEndpoint pins R5's split: the start
// voice fixes the endpoint for the whole session, and an input override only
// changes the parameters sent to it.
func TestStreamSession_OverrideDoesNotChangeEndpoint(t *testing.T) {
	chosen := newFakeUpstream(t, wavResponder(24000, 512))
	other := newFakeUpstream(t, wavResponder(24000, 512))
	eps := map[string]*model.Endpoint{
		"ep-a": {
			ID: "ep-a", Name: "Alpha", BaseURL: chosen.URL, Models: model.StringSlice{"tts-1"},
			DefaultResponseFormat: "wav", Enabled: true,
		},
		"ep-b": {
			ID: "ep-b", Name: "Beta", BaseURL: other.URL, Models: model.StringSlice{"tts-1"},
			DefaultResponseFormat: "wav", Enabled: true,
		},
	}
	s := newTestStreamSession(t, eps, chosen.Client(), 0)
	w := &syncWriter{}

	feed(t, s, w, "alloy (Alpha, tts-1)",
		[]string{`{"voice": "echo", "model": "gpt-4o-mini-tts", "input": "Lights on."}`}, "")

	assert.Equal(t, 0, other.count(), "an input override must not move the session to another endpoint")
	calls := chosen.recorded()
	require.Len(t, calls, 1)
	assert.Equal(t, "echo", calls[0].Voice)
	assert.Equal(t, "gpt-4o-mini-tts", calls[0].Model)
	assert.Equal(t, "Lights on.", calls[0].Input)
}

// --- format consistency (R8) -----------------------------------------------

func TestStreamSession_FormatMismatchAbortsSession(t *testing.T) {
	up := newFakeUpstream(t, func(n int, _ upstreamCall, w http.ResponseWriter, _ *http.Request) {
		rate := 24000
		if n > 0 {
			rate = 16000
		}
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write(buildTestWAV(rate, 2, 1, make([]byte, 512)))
	})
	s := newTestStreamSession(t, wavEndpoint(up), up.Client(), 0)
	w := &syncWriter{}

	feed(t, s, w, "alloy (Test, tts-1)", []string{
		"The kitchen light is now on and set to warm white. ",
		"The hallway light stays off until sunset this evening. ",
	}, "")

	types := w.types(t)
	assert.Equal(t, 1, countType(types, wyoming.TypeAudioStart), "no audio-start for the mismatched segment")
	assert.Equal(t, 0, countType(types, wyoming.TypeSynthesizeStopped))
	assert.Equal(t, wyoming.TypeError, types[len(types)-1])

	errs := errorEvents(t, w)
	require.Len(t, errs, 1)
	assert.Equal(t, "tts-error", errs[0].Code)
	assert.Contains(t, errs[0].Text, "24000 Hz")
	assert.Contains(t, errs[0].Text, "16000 Hz")
}

// --- failures and terminator discipline (R10) ------------------------------

func TestStreamSession_UpstreamFailureBeforeAudio(t *testing.T) {
	up := newFakeUpstream(t, func(n int, _ upstreamCall, w http.ResponseWriter, _ *http.Request) {
		if n > 0 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write(buildTestWAV(24000, 2, 1, make([]byte, 512)))
	})
	s := newTestStreamSession(t, wavEndpoint(up), up.Client(), 0)
	w := &syncWriter{}

	require.NoError(t, s.Start(context.Background(), w, &wyoming.SynthesizeStart{Voice: "alloy (Test, tts-1)"}))
	require.NoError(t, s.Chunk(&wyoming.SynthesizeChunk{
		Text: "The kitchen light is now on and set to warm white. " +
			"The hallway light stays off until sunset this evening. " +
			"The bedroom lamp is dimmed to forty percent brightness. ",
	}))
	require.NoError(t, s.Stop())

	assert.Equal(t, []string{
		wyoming.TypeAudioStart, wyoming.TypeAudioChunk + "+", wyoming.TypeAudioStop, wyoming.TypeError,
	}, collapse(w.types(t)), "segment 1 in full, then one error, and no audio-start for segment 2")

	errs := errorEvents(t, w)
	require.Len(t, errs, 1)
	assert.Equal(t, "tts-error", errs[0].Code)
}

func TestStreamSession_UpstreamFailureAfterAudioStart(t *testing.T) {
	up := newFakeUpstream(t, func(_ int, _ upstreamCall, w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, 4096))
		w.(http.Flusher).Flush()
		// Abort the response mid-body: the client sees a non-EOF read error.
		panic(http.ErrAbortHandler)
	})
	eps := map[string]*model.Endpoint{"ep-1": {
		ID: "ep-1", Name: "Test", BaseURL: up.URL, Models: model.StringSlice{"tts-1"},
		Enabled: true, StreamingEnabled: true, StreamSampleRate: 24000,
	}}
	s := newTestStreamSession(t, eps, up.Client(), 0)
	w := &syncWriter{}

	feed(t, s, w, "alloy (Test, tts-1)", []string{"The kitchen light is now on and set to warm white."}, "")

	types := w.types(t)
	require.GreaterOrEqual(t, len(types), 3)
	assert.Equal(t, wyoming.TypeAudioStart, types[0])
	assert.Equal(t, wyoming.TypeAudioStop, types[len(types)-2], "the group is closed before the error")
	assert.Equal(t, wyoming.TypeError, types[len(types)-1])
	assert.Equal(t, 0, countType(types, wyoming.TypeSynthesizeStopped))
	require.Len(t, errorEvents(t, w), 1)
}

func TestStreamSession_ResolveFailureTerminatesOnce(t *testing.T) {
	up := newFakeUpstream(t, wavResponder(24000, 512))
	s := newTestStreamSession(t, map[string]*model.Endpoint{}, up.Client(), 0)
	w := &syncWriter{}

	feed(t, s, w, "", []string{"The kitchen light is now on and set to warm white. "}, "")

	assert.Equal(t, 0, up.count())
	assert.Equal(t, []string{wyoming.TypeError}, w.types(t))
	errs := errorEvents(t, w)
	require.Len(t, errs, 1)
	assert.Equal(t, "tts-error", errs[0].Code)
}

// TestStreamSession_TombstoneAbsorbsCompat covers R10's reason for the third
// state: the failure happens before Home Assistant sends its compatibility
// synthesize, so without a tombstone the whole message would be spoken again.
func TestStreamSession_TombstoneAbsorbsCompat(t *testing.T) {
	const message = "The kitchen light is now on and set to warm white. "
	up := newFakeUpstream(t, func(_ int, _ upstreamCall, w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	s := newTestStreamSession(t, wavEndpoint(up), up.Client(), 0)
	w := &syncWriter{}

	require.NoError(t, s.Start(context.Background(), w, &wyoming.SynthesizeStart{Voice: "alloy (Test, tts-1)"}))
	require.NoError(t, s.Chunk(&wyoming.SynthesizeChunk{Text: message}))

	require.Eventually(t, func() bool { return len(errorEvents(t, w)) == 1 }, time.Second, 5*time.Millisecond)
	assert.True(t, s.Active(), "a terminated session stays active so it keeps absorbing")

	// Everything after the failure is absorbed: no second synthesis, no second
	// terminator.
	assert.True(t, s.Compat(&wyoming.Synthesize{Text: message, Voice: "alloy (Test, tts-1)"}))
	require.NoError(t, s.Chunk(&wyoming.SynthesizeChunk{Text: "more text"}))
	require.NoError(t, s.Stop())

	assert.False(t, s.Active())
	assert.Equal(t, 1, up.count(), "the failed request only")
	assert.Equal(t, []string{wyoming.TypeError}, w.types(t))
}

func TestStreamSession_ConnectionWriteFailureIsReturned(t *testing.T) {
	up := newFakeUpstream(t, wavResponder(24000, 512))
	s := newTestStreamSession(t, wavEndpoint(up), up.Client(), 0)
	wantErr := errors.New("connection gone")
	w := &syncWriter{fail: func(n int) error {
		if n > 1 {
			return wantErr
		}
		return nil
	}}

	require.NoError(t, s.Start(context.Background(), w, &wyoming.SynthesizeStart{Voice: "alloy (Test, tts-1)"}))
	require.NoError(t, s.Chunk(&wyoming.SynthesizeChunk{Text: "The kitchen light is now on and set to warm white."}))
	err := s.Stop()

	require.Error(t, err, "only a connection write failure is reported to the handler")
	assert.ErrorIs(t, err, wantErr)
}

// --- session lifecycle (R5) ------------------------------------------------

func TestStreamSession_IdleStateIgnoresStrayEvents(t *testing.T) {
	up := newFakeUpstream(t, wavResponder(24000, 512))
	s := newTestStreamSession(t, wavEndpoint(up), up.Client(), 0)
	w := &syncWriter{}

	require.NoError(t, s.Chunk(&wyoming.SynthesizeChunk{Text: "stray"}))
	require.NoError(t, s.Stop())

	assert.False(t, s.Active())
	assert.Empty(t, w.snapshot())
	assert.Equal(t, 0, up.count())
}

func TestStreamSession_RestartDiscardsBufferedText(t *testing.T) {
	up := newFakeUpstream(t, wavResponder(24000, 512))
	s := newTestStreamSession(t, wavEndpoint(up), up.Client(), 0)
	w := &syncWriter{}

	require.NoError(t, s.Start(context.Background(), w, &wyoming.SynthesizeStart{Voice: "alloy (Test, tts-1)"}))
	require.NoError(t, s.Chunk(&wyoming.SynthesizeChunk{Text: "Sure."}))
	require.NoError(t, s.Start(context.Background(), w, &wyoming.SynthesizeStart{Voice: "alloy (Test, tts-1)"}))

	assert.Equal(t, 0, up.count(), "the abandoned buffer must not be synthesized")
	assert.Equal(t, []string{wyoming.TypeSynthesizeStopped}, w.types(t))

	require.NoError(t, s.Chunk(&wyoming.SynthesizeChunk{Text: "The kitchen light is now on."}))
	require.NoError(t, s.Stop())

	assert.Equal(t, []string{"The kitchen light is now on."}, up.inputs())
	assert.Equal(t, 2, countType(w.types(t), wyoming.TypeSynthesizeStopped))
}

// TestStreamSession_RestartMidAudio is the ordering test R5 demands: the
// abandoned session's group is closed by the emitter, synthesize-stopped
// follows, and nothing of the old session appears after it or interleaves with
// the new one. Run under -race, where the two sessions' writes would otherwise
// collide.
func TestStreamSession_RestartMidAudio(t *testing.T) {
	release := make(chan struct{})
	up := newFakeUpstream(t, func(n int, _ upstreamCall, w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(make([]byte, 4096))
		w.(http.Flusher).Flush()
		if n == 0 {
			// Hold the first session's body open until it is cancelled.
			select {
			case <-r.Context().Done():
			case <-release:
			case <-time.After(5 * time.Second):
			}
			return
		}
		_, _ = w.Write(make([]byte, 4096))
	})
	defer close(release)

	eps := map[string]*model.Endpoint{"ep-1": {
		ID: "ep-1", Name: "Test", BaseURL: up.URL, Models: model.StringSlice{"tts-1"},
		Enabled: true, StreamingEnabled: true, StreamSampleRate: 24000,
	}}
	s := newTestStreamSession(t, eps, up.Client(), 0)
	w := &syncWriter{}

	require.NoError(t, s.Start(context.Background(), w, &wyoming.SynthesizeStart{Voice: "alloy (Test, tts-1)"}))
	require.NoError(t, s.Chunk(&wyoming.SynthesizeChunk{Text: "The kitchen light is now on and set to warm white. "}))
	waitForEvent(t, w, wyoming.TypeAudioChunk)

	// Restart while the first session is mid-group.
	require.NoError(t, s.Start(context.Background(), w, &wyoming.SynthesizeStart{Voice: "alloy (Test, tts-1)"}))

	firstTypes := w.types(t)
	assert.Equal(t, []string{
		wyoming.TypeAudioStart, wyoming.TypeAudioChunk + "+",
		wyoming.TypeAudioStop, wyoming.TypeSynthesizeStopped,
	}, collapse(firstTypes), "the emitter closes its own group, then the terminator follows")

	require.NoError(t, s.Chunk(&wyoming.SynthesizeChunk{Text: "The hallway light stays off until sunset."}))
	require.NoError(t, s.Stop())

	all := w.types(t)
	assert.Equal(t, firstTypes, all[:len(firstTypes)], "no old-session event may appear after the terminator")
	assert.Equal(t, []string{
		wyoming.TypeAudioStart, wyoming.TypeAudioChunk + "+",
		wyoming.TypeAudioStop, wyoming.TypeSynthesizeStopped,
	}, collapse(all[len(firstTypes):]))
}

func TestStreamSession_CloseTearsDownWithoutWriting(t *testing.T) {
	aborted := make(chan struct{}, 1)
	up := newFakeUpstream(t, func(_ int, _ upstreamCall, w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(make([]byte, 4096))
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
			aborted <- struct{}{}
		case <-time.After(5 * time.Second):
		}
	})
	eps := map[string]*model.Endpoint{"ep-1": {
		ID: "ep-1", Name: "Test", BaseURL: up.URL, Models: model.StringSlice{"tts-1"},
		Enabled: true, StreamingEnabled: true, StreamSampleRate: 24000,
	}}
	s := newTestStreamSession(t, eps, up.Client(), 0)
	w := &syncWriter{}

	require.NoError(t, s.Start(context.Background(), w, &wyoming.SynthesizeStart{Voice: "alloy (Test, tts-1)"}))
	require.NoError(t, s.Chunk(&wyoming.SynthesizeChunk{Text: "The kitchen light is now on and set to warm white. "}))
	// Wait until the emitter is inside the group, so teardown really does catch
	// it mid-audio.
	waitForEvent(t, w, wyoming.TypeAudioChunk)

	s.Close()

	assert.False(t, s.Active())
	select {
	case <-aborted:
	case <-time.After(2 * time.Second):
		t.Fatal("Close must cancel the in-flight upstream request")
	}

	types := w.types(t)
	assert.Equal(t, 0, countType(types, wyoming.TypeSynthesizeStopped), "nothing may be written to a gone connection")
	assert.Equal(t, 0, countType(types, wyoming.TypeError))
	assert.Equal(t, 0, countType(types, wyoming.TypeAudioStop), "the emitter skips its closing write on teardown")

	// Close is idempotent and a closed session ignores further events.
	s.Close()
	require.NoError(t, s.Start(context.Background(), w, &wyoming.SynthesizeStart{Voice: "alloy (Test, tts-1)"}))
	assert.False(t, s.Active())
}

// --- idle timeout ----------------------------------------------------------

func TestStreamSession_IdleTimeout(t *testing.T) {
	up := newFakeUpstream(t, wavResponder(24000, 512))
	s := newTestStreamSession(t, wavEndpoint(up), up.Client(), 60*time.Millisecond)
	w := &syncWriter{}

	require.NoError(t, s.Start(context.Background(), w, &wyoming.SynthesizeStart{Voice: "alloy (Test, tts-1)"}))
	require.NoError(t, s.Chunk(&wyoming.SynthesizeChunk{Text: "Sure."}))

	require.Eventually(t, func() bool { return len(errorEvents(t, w)) == 1 }, 2*time.Second, 5*time.Millisecond)
	errs := errorEvents(t, w)
	assert.Equal(t, "synthesize-timeout", errs[0].Code)
	assert.True(t, s.Active(), "the timed-out session leaves a tombstone")
	assert.Equal(t, 0, up.count())

	// The late compatibility synthesize is absorbed rather than synthesized.
	assert.True(t, s.Compat(&wyoming.Synthesize{Text: "Sure.", Voice: "alloy (Test, tts-1)"}))
	require.NoError(t, s.Stop())
	assert.Equal(t, []string{wyoming.TypeError}, w.types(t))
	assert.Equal(t, 0, up.count())
}

func TestStreamSession_IdleTimeoutIsResetByEvents(t *testing.T) {
	up := newFakeUpstream(t, wavResponder(24000, 512))
	s := newTestStreamSession(t, wavEndpoint(up), up.Client(), 150*time.Millisecond)
	w := &syncWriter{}

	require.NoError(t, s.Start(context.Background(), w, &wyoming.SynthesizeStart{Voice: "alloy (Test, tts-1)"}))
	for range 6 {
		time.Sleep(50 * time.Millisecond)
		require.NoError(t, s.Chunk(&wyoming.SynthesizeChunk{Text: "word "}))
	}
	assert.Empty(t, errorEvents(t, w), "a session fed every half-timeout must not time out")

	require.NoError(t, s.Stop())
	assert.Equal(t, []string{"word word word word word word"}, up.inputs())
	assert.Equal(t, 1, countType(w.types(t), wyoming.TypeSynthesizeStopped))
}

func TestStreamSession_IdleTimeoutDisabled(t *testing.T) {
	tests := []struct {
		name    string
		timeout time.Duration
	}{
		{name: "zero disables the timer", timeout: 0},
		{name: "negative is treated as disabled", timeout: -time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			up := newFakeUpstream(t, wavResponder(24000, 512))
			s := newTestStreamSession(t, wavEndpoint(up), up.Client(), tt.timeout)
			w := &syncWriter{}

			require.NoError(t, s.Start(context.Background(), w, &wyoming.SynthesizeStart{Voice: "alloy (Test, tts-1)"}))
			require.NoError(t, s.Chunk(&wyoming.SynthesizeChunk{Text: "Sure."}))
			time.Sleep(120 * time.Millisecond)

			assert.Empty(t, errorEvents(t, w))
			assert.True(t, s.Active())
			require.NoError(t, s.Stop())
			assert.Equal(t, 1, countType(w.types(t), wyoming.TypeSynthesizeStopped))
		})
	}
}

// --- pipelining ------------------------------------------------------------

// TestStreamSession_PrefetchIsBounded checks both halves of the maxInFlight
// contract: the next segment's upstream request is issued while the current one
// is still being emitted, so its time-to-first-byte is spent during playback,
// and no more than maxInFlight are ever open at once.
//
// Every response stalls after its WAV header, so the emitter is stuck inside
// segment 1 while the opener runs ahead.
func TestStreamSession_PrefetchIsBounded(t *testing.T) {
	const sentence = "The kitchen light is now on and set to warm white for the evening. "
	pcm := make([]byte, 512)
	wav := buildTestWAV(24000, 2, 1, pcm)
	header, data := wav[:len(wav)-len(pcm)], wav[len(wav)-len(pcm):]

	gate := make(chan struct{})
	up := newFakeUpstream(t, func(_ int, _ upstreamCall, w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write(header)
		w.(http.Flusher).Flush()
		select {
		case <-gate:
		case <-r.Context().Done():
			return
		case <-time.After(5 * time.Second):
			return
		}
		_, _ = w.Write(data)
	})
	s := newTestStreamSession(t, wavEndpoint(up), up.Client(), 0)
	w := &syncWriter{}

	require.NoError(t, s.Start(context.Background(), w, &wyoming.SynthesizeStart{Voice: "alloy (Test, tts-1)"}))
	for range 4 {
		require.NoError(t, s.Chunk(&wyoming.SynthesizeChunk{Text: sentence}))
	}

	// Segment 2 is fetched while segment 1 is still being emitted, and segment 3
	// waits for a slot however long segment 1 takes.
	require.Eventually(t, func() bool { return up.count() == maxInFlight },
		3*time.Second, 5*time.Millisecond, "the emitting segment plus exactly one prefetch")
	assert.Never(t, func() bool { return up.count() > maxInFlight },
		200*time.Millisecond, 10*time.Millisecond, "prefetch is bounded at maxInFlight")

	close(gate)
	require.NoError(t, s.Stop())

	assert.Len(t, up.inputs(), 4)
	types := w.types(t)
	assert.Equal(t, 4, countType(types, wyoming.TypeAudioStart))
	assert.Equal(t, 4, countType(types, wyoming.TypeAudioStop))
	assert.Equal(t, 1, countType(types, wyoming.TypeSynthesizeStopped))
	assert.Equal(t, wyoming.TypeSynthesizeStopped, types[len(types)-1])
}

// TestStreamSession_CloseDiscardsPrefetchedSegments covers the third step of
// quiesce: a segment that was fetched ahead but never emitted must have its
// upstream body closed, or the connection leaks.
func TestStreamSession_CloseDiscardsPrefetchedSegments(t *testing.T) {
	const sentence = "The kitchen light is now on and set to warm white for the evening. "
	pcm := make([]byte, 512)
	wav := buildTestWAV(24000, 2, 1, pcm)
	header, data := wav[:len(wav)-len(pcm)], wav[len(wav)-len(pcm):]

	var aborted atomic.Int32
	up := newFakeUpstream(t, func(_ int, _ upstreamCall, w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write(header)
		w.(http.Flusher).Flush()
		select {
		case <-r.Context().Done():
			aborted.Add(1)
		case <-time.After(5 * time.Second):
			_, _ = w.Write(data)
		}
	})
	s := newTestStreamSession(t, wavEndpoint(up), up.Client(), 0)
	w := &syncWriter{}

	require.NoError(t, s.Start(context.Background(), w, &wyoming.SynthesizeStart{Voice: "alloy (Test, tts-1)"}))
	for range 4 {
		require.NoError(t, s.Chunk(&wyoming.SynthesizeChunk{Text: sentence}))
	}
	// Segment 1 is stuck mid-body and segment 2 is prefetched; segments 3 and 4
	// are still waiting for a slot.
	require.Eventually(t, func() bool { return up.count() == maxInFlight },
		3*time.Second, 5*time.Millisecond)
	waitForEvent(t, w, wyoming.TypeAudioStart)

	s.Close()

	require.Eventually(t, func() bool { return aborted.Load() == int32(maxInFlight) },
		3*time.Second, 5*time.Millisecond, "both the emitting and the prefetched body must be closed")
	assert.Equal(t, maxInFlight, up.count(), "no further request may be issued after teardown")
	assert.Equal(t, 0, countType(w.types(t), wyoming.TypeSynthesizeStopped))
}

func TestStreamSession_ConstructionAndStrayInput(t *testing.T) {
	up := newFakeUpstream(t, wavResponder(24000, 512))
	proxy := newSegmentTestProxy(wavEndpoint(up), nil, up.Client())
	s := NewStreamSession(proxy, segment.DefaultConfig(), 0, nil) // nil logger falls back
	w := &syncWriter{}

	require.NoError(t, s.Start(context.Background(), w, &wyoming.SynthesizeStart{
		Voice: "alloy (Test, tts-1)", TextFormat: "ssml", Language: "en", Speaker: "s1",
	}))
	require.NoError(t, s.Chunk(&wyoming.SynthesizeChunk{Text: ""}))
	require.NoError(t, s.Chunk(&wyoming.SynthesizeChunk{Text: "Lights on."}))
	require.NoError(t, s.Stop())

	assert.Equal(t, []string{"Lights on."}, up.inputs(), "ssml is accepted and treated as plain text")
}

// TestStreamSession_TerminatorWriteFailures pins the other half of the error
// split: a failed write to the connection is reported, and a failed terminator
// does not become a second terminator.
func TestStreamSession_TerminatorWriteFailures(t *testing.T) {
	t.Run("synthesize-stopped", func(t *testing.T) {
		up := newFakeUpstream(t, wavResponder(24000, 512))
		s := newTestStreamSession(t, wavEndpoint(up), up.Client(), 0)
		wantErr := errors.New("connection gone")
		w := &syncWriter{fail: func(int) error { return wantErr }}

		// An empty message writes exactly one event: the terminator.
		require.NoError(t, s.Start(context.Background(), w, &wyoming.SynthesizeStart{Voice: "alloy (Test, tts-1)"}))
		err := s.Stop()

		require.Error(t, err)
		assert.ErrorIs(t, err, wantErr)
		assert.Empty(t, w.snapshot())
	})

	t.Run("synthesize-timeout", func(t *testing.T) {
		up := newFakeUpstream(t, wavResponder(24000, 512))
		s := newTestStreamSession(t, wavEndpoint(up), up.Client(), 40*time.Millisecond)
		w := &syncWriter{fail: func(int) error { return errors.New("connection gone") }}

		require.NoError(t, s.Start(context.Background(), w, &wyoming.SynthesizeStart{Voice: "alloy (Test, tts-1)"}))
		require.Eventually(t, func() bool { return !s.Active() || s.run.failed.Load() },
			2*time.Second, 5*time.Millisecond)

		assert.Empty(t, w.snapshot())
		require.Error(t, s.Stop(), "the failed terminator write is reported to the handler")
	})
}

// TestStreamSession_StaleIdleTimeout covers the timer generation check: an
// expiry that lost the race with an event arriving must not abandon the session.
func TestStreamSession_StaleIdleTimeout(t *testing.T) {
	up := newFakeUpstream(t, wavResponder(24000, 512))
	s := newTestStreamSession(t, wavEndpoint(up), up.Client(), time.Hour)
	w := &syncWriter{}

	require.NoError(t, s.Start(context.Background(), w, &wyoming.SynthesizeStart{Voice: "alloy (Test, tts-1)"}))
	r := s.run

	s.onIdleTimeout(r, r.timerGen-1) // stale generation
	assert.Empty(t, w.snapshot())

	require.NoError(t, s.Stop())
	s.onIdleTimeout(r, r.timerGen) // the session it belonged to is gone
	assert.Equal(t, []string{wyoming.TypeSynthesizeStopped}, w.types(t))
}

// TestStreamSession_EmitterTeardownPaths exercises the emitter's own exit paths
// directly, because which of them a cancelled session takes depends on where the
// emitter happened to be when the cancellation landed.
func TestStreamSession_EmitterTeardownPaths(t *testing.T) {
	newRun := func(w io.Writer) *streamRun {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		r := &streamRun{
			logger: testLogger(), w: w, ctx: ctx, cancel: cancel,
			sem: make(chan struct{}, maxInFlight), ready: make(chan *segJob, maxInFlight),
		}
		r.qcond = sync.NewCond(&r.qmu)
		return r
	}
	openSeg := func() (*OpenSegment, *segTrackedBody) {
		body := &segTrackedBody{Reader: bytes.NewReader(make([]byte, 16))}
		return &OpenSegment{format: &AudioFormat{Rate: 24000, Width: 2, Channels: 1}, pcm: body, body: body}, body
	}

	t.Run("a prefetched segment taken after cancellation is closed unsynthesized", func(t *testing.T) {
		w := &syncWriter{}
		r := newRun(w)
		seg, body := openSeg()
		r.cancel()

		assert.False(t, r.emitOne(&segJob{seg: seg}))
		assert.Equal(t, int32(1), body.closes.Load())
		assert.Empty(t, w.snapshot())
	})

	t.Run("teardown writes neither the closing audio-stop nor an error", func(t *testing.T) {
		w := &syncWriter{}
		r := newRun(w)
		r.teardown.Store(true)

		r.closeGroup(true)
		r.fail(errors.New("upstream exploded"), "tts-error")

		assert.Empty(t, w.snapshot())
		assert.True(t, r.failed.Load())
	})

	t.Run("a failed closing audio-stop is recorded", func(t *testing.T) {
		wantErr := errors.New("connection gone")
		w := &syncWriter{fail: func(int) error { return wantErr }}
		r := newRun(w)

		r.closeGroup(true)
		assert.ErrorIs(t, r.writeErr, wantErr)

		r.closeGroup(false) // no group open: nothing to close
		assert.Empty(t, w.snapshot())
	})

	t.Run("a failed error event is recorded rather than retried", func(t *testing.T) {
		wantErr := errors.New("connection gone")
		w := &syncWriter{fail: func(int) error { return wantErr }}
		r := newRun(w)

		r.fail(errors.New("upstream exploded"), "tts-error")

		assert.ErrorIs(t, r.writeErr, wantErr)
		assert.True(t, r.failed.Load())
		assert.Empty(t, w.snapshot())
	})

	t.Run("a segment that cannot be handed to the emitter is not delivered", func(t *testing.T) {
		r := newRun(&syncWriter{})
		seg, body := openSeg()
		// The emitter is gone and the queue is full, so the send can only lose to
		// the cancellation.
		for range maxInFlight {
			r.ready <- &segJob{}
		}
		r.cancel()

		assert.False(t, r.send(&segJob{seg: seg}))
		_ = seg.Close()
		assert.Equal(t, int32(1), body.closes.Load())
	})

	t.Run("the writer refuses to write once the session is cancelled", func(t *testing.T) {
		w := &syncWriter{}
		r := newRun(w)
		cw := &cancelWriter{w: r.w, ctx: r.ctx}

		n, err := cw.Write([]byte("one"))
		require.NoError(t, err)
		assert.Equal(t, 3, n)
		assert.Equal(t, 1, cw.ok)

		r.cancel()
		_, err = cw.Write([]byte("two"))
		assert.ErrorIs(t, err, errSessionCanceled)
		assert.Equal(t, 1, cw.ok)
		assert.NoError(t, cw.err, "a cancelled write is not a connection failure")
	})
}
