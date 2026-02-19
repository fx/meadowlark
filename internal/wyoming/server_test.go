package wyoming

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testHandler is a configurable Handler for testing.
type testHandler struct {
	fn func(ctx context.Context, ev *Event, w io.Writer) error
}

func (h *testHandler) HandleEvent(ctx context.Context, ev *Event, w io.Writer) error {
	return h.fn(ctx, ev, w)
}

// startTestServer starts a Server on a random port and returns a cleanup func.
func startTestServer(t *testing.T, handler Handler) (*Server, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := NewServer("127.0.0.1:0", handler, logger)

	ready := make(chan struct{})
	go func() {
		// Wait for listener to be set, then signal ready.
		go func() {
			for {
				if srv.Addr() != "" {
					close(ready)
					return
				}
				time.Sleep(time.Millisecond)
			}
		}()
		if err := srv.ListenAndServe(ctx); err != nil {
			t.Logf("server error: %v", err)
		}
	}()

	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not start in time")
	}

	return srv, cancel
}

// dialServer connects a raw TCP client to the server.
func dialServer(t *testing.T, srv *Server) net.Conn {
	t.Helper()
	conn, err := net.DialTimeout("tcp", srv.Addr(), 2*time.Second)
	require.NoError(t, err)
	return conn
}

func TestServer_PingPong(t *testing.T) {
	handler := &testHandler{
		fn: func(_ context.Context, ev *Event, w io.Writer) error {
			if ev.Type == TypePing {
				return WriteEvent(w, (&Pong{}).ToEvent())
			}
			return fmt.Errorf("unexpected event type: %s", ev.Type)
		},
	}

	srv, cancel := startTestServer(t, handler)
	defer cancel()
	defer srv.Shutdown()

	conn := dialServer(t, srv)
	defer conn.Close()

	// Send ping.
	require.NoError(t, WriteEvent(conn, (&Ping{}).ToEvent()))

	// Read pong.
	reader := bufio.NewReader(conn)
	ev, err := ReadEvent(reader)
	require.NoError(t, err)
	assert.Equal(t, TypePong, ev.Type)
}

func TestServer_Describe(t *testing.T) {
	info := &Info{
		Tts: []TtsProgram{
			{
				Name:        "meadowlark",
				Description: "Meadowlark TTS Bridge",
				Installed:   true,
				Version:     "0.1.0",
				Voices: []TtsVoice{
					{
						Name:        "test-voice",
						Description: "test-voice",
						Installed:   true,
						Languages:   []string{"en"},
					},
				},
			},
		},
	}

	handler := &testHandler{
		fn: func(_ context.Context, ev *Event, w io.Writer) error {
			if ev.Type == TypeDescribe {
				return WriteEvent(w, info.ToEvent())
			}
			return fmt.Errorf("unexpected event type: %s", ev.Type)
		},
	}

	srv, cancel := startTestServer(t, handler)
	defer cancel()
	defer srv.Shutdown()

	conn := dialServer(t, srv)
	defer conn.Close()

	// Send describe.
	require.NoError(t, WriteEvent(conn, (&Describe{}).ToEvent()))

	// Read info response.
	reader := bufio.NewReader(conn)
	ev, err := ReadEvent(reader)
	require.NoError(t, err)
	assert.Equal(t, TypeInfo, ev.Type)

	parsed, err := InfoFromEvent(ev)
	require.NoError(t, err)
	require.Len(t, parsed.Tts, 1)
	assert.Equal(t, "meadowlark", parsed.Tts[0].Name)
	require.Len(t, parsed.Tts[0].Voices, 1)
	assert.Equal(t, "test-voice", parsed.Tts[0].Voices[0].Name)
}

func TestServer_HandlerError_SendsErrorEvent(t *testing.T) {
	handler := &testHandler{
		fn: func(_ context.Context, ev *Event, w io.Writer) error {
			return errors.New("synthesis failed: voice not found")
		},
	}

	srv, cancel := startTestServer(t, handler)
	defer cancel()
	defer srv.Shutdown()

	conn := dialServer(t, srv)
	defer conn.Close()

	// Send any event.
	require.NoError(t, WriteEvent(conn, (&Ping{}).ToEvent()))

	// Should get back an error event.
	reader := bufio.NewReader(conn)
	ev, err := ReadEvent(reader)
	require.NoError(t, err)
	assert.Equal(t, TypeError, ev.Type)

	errEv, err := ErrorFromEvent(ev)
	require.NoError(t, err)
	assert.Contains(t, errEv.Text, "synthesis failed: voice not found")
	assert.Equal(t, "handler-error", errEv.Code)
}

func TestServer_MultipleClients(t *testing.T) {
	handler := &testHandler{
		fn: func(_ context.Context, ev *Event, w io.Writer) error {
			if ev.Type == TypePing {
				return WriteEvent(w, (&Pong{}).ToEvent())
			}
			return nil
		},
	}

	srv, cancel := startTestServer(t, handler)
	defer cancel()
	defer srv.Shutdown()

	const numClients = 5
	var wg sync.WaitGroup
	wg.Add(numClients)

	for i := 0; i < numClients; i++ {
		go func(id int) {
			defer wg.Done()

			conn := dialServer(t, srv)
			defer conn.Close()

			require.NoError(t, WriteEvent(conn, (&Ping{}).ToEvent()))

			reader := bufio.NewReader(conn)
			ev, err := ReadEvent(reader)
			require.NoError(t, err)
			assert.Equal(t, TypePong, ev.Type)
		}(i)
	}

	wg.Wait()
}

func TestServer_GracefulShutdown(t *testing.T) {
	handler := &testHandler{
		fn: func(_ context.Context, ev *Event, w io.Writer) error {
			if ev.Type == TypePing {
				return WriteEvent(w, (&Pong{}).ToEvent())
			}
			return nil
		},
	}

	srv, cancel := startTestServer(t, handler)
	defer cancel()

	// Connect a client.
	conn := dialServer(t, srv)

	// Verify connection works.
	require.NoError(t, WriteEvent(conn, (&Ping{}).ToEvent()))
	reader := bufio.NewReader(conn)
	ev, err := ReadEvent(reader)
	require.NoError(t, err)
	assert.Equal(t, TypePong, ev.Type)

	// Shutdown the server.
	srv.Shutdown()

	// Connection should be closed -- read should fail.
	_, err = ReadEvent(reader)
	assert.Error(t, err)
	conn.Close()
}

func TestServer_MultipleEventsOnSameConnection(t *testing.T) {
	handler := &testHandler{
		fn: func(_ context.Context, ev *Event, w io.Writer) error {
			switch ev.Type {
			case TypePing:
				return WriteEvent(w, (&Pong{}).ToEvent())
			case TypeDescribe:
				info := &Info{
					Tts: []TtsProgram{{Name: "test", Installed: true, Version: "1.0"}},
				}
				return WriteEvent(w, info.ToEvent())
			}
			return nil
		},
	}

	srv, cancel := startTestServer(t, handler)
	defer cancel()
	defer srv.Shutdown()

	conn := dialServer(t, srv)
	defer conn.Close()

	reader := bufio.NewReader(conn)

	// Send ping, read pong.
	require.NoError(t, WriteEvent(conn, (&Ping{}).ToEvent()))
	ev, err := ReadEvent(reader)
	require.NoError(t, err)
	assert.Equal(t, TypePong, ev.Type)

	// Send describe, read info.
	require.NoError(t, WriteEvent(conn, (&Describe{}).ToEvent()))
	ev, err = ReadEvent(reader)
	require.NoError(t, err)
	assert.Equal(t, TypeInfo, ev.Type)

	// Send another ping, read pong.
	require.NoError(t, WriteEvent(conn, (&Ping{}).ToEvent()))
	ev, err = ReadEvent(reader)
	require.NoError(t, err)
	assert.Equal(t, TypePong, ev.Type)
}

func TestServer_ClientDisconnect(t *testing.T) {
	var once sync.Once
	connected := make(chan struct{})
	handler := &testHandler{
		fn: func(_ context.Context, ev *Event, w io.Writer) error {
			if ev.Type == TypePing {
				once.Do(func() { close(connected) })
				return WriteEvent(w, (&Pong{}).ToEvent())
			}
			return nil
		},
	}

	srv, cancel := startTestServer(t, handler)
	defer cancel()
	defer srv.Shutdown()

	conn := dialServer(t, srv)

	// Send a ping so the server processes the connection.
	require.NoError(t, WriteEvent(conn, (&Ping{}).ToEvent()))
	<-connected

	// Abruptly close the client connection.
	conn.Close()

	// Give the server a moment to process the disconnect.
	time.Sleep(50 * time.Millisecond)

	// Server should still be accepting new connections.
	conn2 := dialServer(t, srv)
	defer conn2.Close()

	require.NoError(t, WriteEvent(conn2, (&Ping{}).ToEvent()))
	reader := bufio.NewReader(conn2)
	ev, err := ReadEvent(reader)
	require.NoError(t, err)
	assert.Equal(t, TypePong, ev.Type)
}

func TestServer_HandlerFunc(t *testing.T) {
	fn := HandlerFunc(func(_ context.Context, ev *Event, w io.Writer) error {
		if ev.Type == TypePing {
			return WriteEvent(w, (&Pong{}).ToEvent())
		}
		return nil
	})

	srv, cancel := startTestServer(t, fn)
	defer cancel()
	defer srv.Shutdown()

	conn := dialServer(t, srv)
	defer conn.Close()

	require.NoError(t, WriteEvent(conn, (&Ping{}).ToEvent()))
	reader := bufio.NewReader(conn)
	ev, err := ReadEvent(reader)
	require.NoError(t, err)
	assert.Equal(t, TypePong, ev.Type)
}

func TestServer_ContextCancellation(t *testing.T) {
	handler := &testHandler{
		fn: func(_ context.Context, ev *Event, w io.Writer) error {
			return nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := NewServer("127.0.0.1:0", handler, logger)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe(ctx)
	}()

	// Wait for server to start.
	for srv.Addr() == "" {
		time.Sleep(time.Millisecond)
	}

	// Cancel the context.
	cancel()

	// Server should return nil (clean shutdown).
	select {
	case err := <-errCh:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop after context cancellation")
	}
}

func TestServer_Addr_BeforeListening(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := NewServer("127.0.0.1:0", nil, logger)
	assert.Equal(t, "", srv.Addr())
}

func TestNewServer_NilLogger(t *testing.T) {
	srv := NewServer("127.0.0.1:0", nil, nil)
	assert.NotNil(t, srv.logger)
}
