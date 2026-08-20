package main

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/fx/meadowlark/internal/wyoming"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- test harness ----------------------------------------------------------

// testConn is a fake Wyoming client speaking the real protocol over a real TCP
// connection to a running Server.
type testConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *testConn) send(t *testing.T, ev *wyoming.Event) {
	t.Helper()
	require.NoError(t, wyoming.WriteEvent(c.Conn, ev))
}

// drain reads events until one of the given type arrives, or until the read
// deadline expires. A case expecting no reply passes an empty terminal type and
// a short timeout, so the deadline itself is the assertion.
func (c *testConn) drain(t *testing.T, terminal string, timeout time.Duration) []*wyoming.Event {
	t.Helper()
	require.NoError(t, c.SetReadDeadline(time.Now().Add(timeout)))

	var out []*wyoming.Event
	for {
		ev, err := wyoming.ReadEvent(c.r)
		if err != nil {
			return out
		}
		out = append(out, ev)
		if terminal != "" && ev.Type == terminal {
			return out
		}
	}
}

// startTestServer runs the production Wyoming server, with the production
// handler, over a fake store and a fake upstream, and returns a connected
// client.
func startTestServer(t *testing.T, up *fakeUpstream) *testConn {
	t.Helper()

	handler := newTestHandler(t, singleEndpointStore(up.URL), time.Minute)
	srv := wyoming.NewServer("127.0.0.1:0", handler, testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() { served <- srv.ListenAndServe(ctx) }()

	t.Cleanup(func() {
		cancel()
		srv.Shutdown()
		<-served
	})

	var addr string
	require.Eventually(t, func() bool {
		addr = srv.Addr()
		return addr != ""
	}, 3*time.Second, 5*time.Millisecond, "server must start listening")

	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	return &testConn{Conn: conn, r: bufio.NewReader(conn)}
}

// The message Home Assistant would stream for a three-sentence assistant reply,
// split the way an LLM emits it. It segments into three parts under the default
// thresholds: the first sentence clears firstSegmentChars, the second clears
// minSegmentChars, and the trailing question is flushed at synthesize-stop.
var (
	haSentences = []string{
		"The kitchen light is now on. ",
		"I also turned off the porch light and locked the front door. ",
		"Anything else?",
	}
	haMessage  = haSentences[0] + haSentences[1] + haSentences[2]
	haSegments = []string{
		"The kitchen light is now on.",
		"I also turned off the porch light and locked the front door.",
		"Anything else?",
	}
)

// audioGroups is the emitted framing for n successfully synthesized segments.
func audioGroups(n int) []string {
	var out []string
	for range n {
		out = append(out,
			wyoming.TypeAudioStart, wyoming.TypeAudioChunk, wyoming.TypeAudioStop)
	}
	return out
}

// --- conformance table -----------------------------------------------------

// TestEndToEndConformance replays Home Assistant's exact event sequences over a
// real TCP connection and asserts the ordered list of event types Meadowlark
// emits in reply.
func TestEndToEndConformance(t *testing.T) {
	tests := []struct {
		name string
		// respond scripts the fake upstream; nil answers every request with a
		// 24 kHz WAV.
		respond func(n int, w http.ResponseWriter)
		// script is the client's event sequence.
		script []*wyoming.Event
		// terminal is the event type that ends the reply, or "" when the case
		// expects no reply at all.
		terminal string
		timeout  time.Duration
		// want is the ordered reply, with each run of audio-chunk events
		// collapsed to one.
		want []string
		// wantUpstream is the text of every upstream request, in order.
		wantUpstream []string
		// check runs extra assertions on the decoded reply.
		check func(t *testing.T, evs []*wyoming.Event)
	}{
		{
			// The verified Home Assistant sequence: start, the chunks, the
			// compatibility synthesize carrying the whole message, then stop.
			name: "home assistant baseline",
			script: append(append(
				[]*wyoming.Event{(&wyoming.SynthesizeStart{Voice: "alloy"}).ToEvent()},
				chunkEvents(haSentences)...),
				(&wyoming.Synthesize{Text: haMessage, Voice: "alloy"}).ToEvent(),
				(&wyoming.SynthesizeStop{}).ToEvent(),
			),
			terminal:     wyoming.TypeSynthesizeStopped,
			want:         append(audioGroups(3), wyoming.TypeSynthesizeStopped),
			wantUpstream: haSegments,
		},
		{
			// The whole message in one chunk must segment identically: the
			// segmenter's state is the accumulated text, not the arrival
			// boundaries.
			name: "one chunk containing several sentences",
			script: []*wyoming.Event{
				(&wyoming.SynthesizeStart{Voice: "alloy"}).ToEvent(),
				(&wyoming.SynthesizeChunk{Text: haMessage}).ToEvent(),
				(&wyoming.Synthesize{Text: haMessage, Voice: "alloy"}).ToEvent(),
				(&wyoming.SynthesizeStop{}).ToEvent(),
			},
			terminal:     wyoming.TypeSynthesizeStopped,
			want:         append(audioGroups(3), wyoming.TypeSynthesizeStopped),
			wantUpstream: haSegments,
		},
		{
			// A client that opens a session but never chunks: the
			// compatibility synthesize is the session's only content, and it
			// is spoken exactly once.
			name: "zero chunks with only the compatibility synthesize",
			script: []*wyoming.Event{
				(&wyoming.SynthesizeStart{Voice: "alloy"}).ToEvent(),
				(&wyoming.Synthesize{Text: "Only this one.", Voice: "alloy"}).ToEvent(),
				(&wyoming.SynthesizeStop{}).ToEvent(),
			},
			terminal:     wyoming.TypeSynthesizeStopped,
			want:         append(audioGroups(1), wyoming.TypeSynthesizeStopped),
			wantUpstream: []string{"Only this one."},
		},
		{
			// A client that ignores the streaming flag. No session exists, so
			// the event falls through to the whole-message path and gets no
			// synthesize-stopped — that terminator belongs to a session.
			name: "bare synthesize with no session",
			script: []*wyoming.Event{
				(&wyoming.Synthesize{Text: haMessage, Voice: "alloy"}).ToEvent(),
			},
			terminal:     wyoming.TypeAudioStop,
			want:         audioGroups(1),
			wantUpstream: []string{haMessage},
		},
		{
			// Nothing was ever opened, so there is nothing to terminate and
			// nothing to say about it.
			name: "synthesize-stop with no session",
			script: []*wyoming.Event{
				(&wyoming.SynthesizeStop{}).ToEvent(),
			},
			timeout:      300 * time.Millisecond,
			want:         nil,
			wantUpstream: nil,
		},
		{
			// The second segment's upstream fails. Segment 1's group is
			// emitted in full, segments 2 and 3 emit nothing, and the session
			// ends on exactly one error — never both an error and a
			// synthesize-stopped, which would leave an unconsumed terminator
			// for the next stream to read as an immediate end of audio.
			name: "upstream error mid-session",
			respond: func(n int, w http.ResponseWriter) {
				if n == 1 {
					http.Error(w, "upstream exploded", http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "audio/wav")
				_, _ = w.Write(testWAV(24000, 640))
			},
			script: append(append(
				[]*wyoming.Event{(&wyoming.SynthesizeStart{Voice: "alloy"}).ToEvent()},
				chunkEvents(haSentences)...),
				(&wyoming.Synthesize{Text: haMessage, Voice: "alloy"}).ToEvent(),
				(&wyoming.SynthesizeStop{}).ToEvent(),
			),
			terminal: wyoming.TypeError,
			want:     append(audioGroups(1), wyoming.TypeError),
			// Segment 3 is never even requested: the open loop stops at the
			// first failure, so no work is wasted on a session already lost.
			wantUpstream: haSegments[:2],
			check: func(t *testing.T, evs []*wyoming.Event) {
				last, err := wyoming.ErrorFromEvent(evs[len(evs)-1])
				require.NoError(t, err)
				assert.Equal(t, "tts-error", last.Code)
			},
		},
		{
			// The switch that makes Home Assistant take its streaming path.
			name: "describe advertises supports_synthesize_streaming",
			script: []*wyoming.Event{
				(&wyoming.Describe{}).ToEvent(),
			},
			terminal: wyoming.TypeInfo,
			want:     []string{wyoming.TypeInfo},
			check: func(t *testing.T, evs []*wyoming.Event) {
				info, err := wyoming.InfoFromEvent(evs[0])
				require.NoError(t, err)
				require.Len(t, info.Tts, 1)
				assert.True(t, info.Tts[0].SupportsSynthesizeStreaming)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			up := newFakeUpstream(t, tt.respond)
			conn := startTestServer(t, up)

			for _, ev := range tt.script {
				conn.send(t, ev)
			}

			timeout := tt.timeout
			if timeout == 0 {
				timeout = 5 * time.Second
			}
			evs := conn.drain(t, tt.terminal, timeout)

			assert.Equal(t, tt.want, collapseChunks(eventTypes(evs)))
			assert.Equal(t, tt.wantUpstream, up.recorded())
			if tt.check != nil {
				tt.check(t, evs)
			}
		})
	}
}

func chunkEvents(texts []string) []*wyoming.Event {
	out := make([]*wyoming.Event, 0, len(texts))
	for _, text := range texts {
		out = append(out, (&wyoming.SynthesizeChunk{Text: text}).ToEvent())
	}
	return out
}
