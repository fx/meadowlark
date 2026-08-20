package wyoming

import (
	"bufio"
	"bytes"
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

// factoryHandler implements HandlerFactory and ConnHandler, recording how many
// per-connection handlers it built and closed.
type factoryHandler struct {
	mu     sync.Mutex
	built  []*perConnHandler
	closes int
}

func (f *factoryHandler) HandleEvent(context.Context, *Event, io.Writer) error {
	return errors.New("shared handler must not receive events when a factory is available")
}

func (f *factoryHandler) NewConnHandler() Handler {
	f.mu.Lock()
	defer f.mu.Unlock()
	h := &perConnHandler{parent: f}
	f.built = append(f.built, h)
	return h
}

func (f *factoryHandler) builtCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.built)
}

func (f *factoryHandler) closeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closes
}

// distinctHandlers reports whether every built handler is a distinct instance.
func (f *factoryHandler) distinctHandlers() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	seen := make(map[*perConnHandler]struct{}, len(f.built))
	for _, h := range f.built {
		if _, dup := seen[h]; dup {
			return false
		}
		seen[h] = struct{}{}
	}
	return true
}

type perConnHandler struct {
	parent *factoryHandler

	mu     sync.Mutex
	events int
	closed int
}

func (h *perConnHandler) HandleEvent(_ context.Context, _ *Event, w io.Writer) error {
	h.mu.Lock()
	h.events++
	h.mu.Unlock()
	return WriteEvent(w, (&Pong{}).ToEvent())
}

func (h *perConnHandler) CloseConn() {
	h.mu.Lock()
	h.closed++
	h.mu.Unlock()

	h.parent.mu.Lock()
	h.parent.closes++
	h.parent.mu.Unlock()
}

func (h *perConnHandler) closedCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.closed
}

// R3 scenario: one handler per connection.
func TestServer_HandlerFactoryPerConnection(t *testing.T) {
	factory := &factoryHandler{}
	srv, cancel := startTestServer(t, factory)
	defer cancel()
	defer srv.Shutdown()

	for i := 0; i < 3; i++ {
		conn := dialServer(t, srv)
		require.NoError(t, WriteEvent(conn, (&Ping{}).ToEvent()))

		ev, err := ReadEvent(bufio.NewReader(conn))
		require.NoError(t, err)
		assert.Equal(t, TypePong, ev.Type)
		conn.Close()
	}

	require.Eventually(t, func() bool { return factory.closeCount() == 3 }, 2*time.Second, 5*time.Millisecond)
	assert.Equal(t, 3, factory.builtCount())
	assert.True(t, factory.distinctHandlers(), "no two connections may share a handler instance")

	factory.mu.Lock()
	built := append([]*perConnHandler(nil), factory.built...)
	factory.mu.Unlock()
	for _, h := range built {
		assert.Equal(t, 1, h.closedCount(), "CloseConn must be called exactly once")
	}
}

// R3 scenario: teardown notification on client disconnect.
func TestServer_CloseConnOnDisconnect(t *testing.T) {
	factory := &factoryHandler{}
	srv, cancel := startTestServer(t, factory)
	defer cancel()
	defer srv.Shutdown()

	conn := dialServer(t, srv)
	require.NoError(t, WriteEvent(conn, (&Ping{}).ToEvent()))
	_, err := ReadEvent(bufio.NewReader(conn))
	require.NoError(t, err)
	require.NoError(t, conn.Close())

	require.Eventually(t, func() bool { return factory.closeCount() == 1 }, 2*time.Second, 5*time.Millisecond)
}

// R3 scenario: teardown notification on Shutdown, and CloseConn blocking means
// Shutdown genuinely drains the connection's background work.
func TestServer_CloseConnOnShutdown(t *testing.T) {
	factory := &factoryHandler{}
	srv, cancel := startTestServer(t, factory)
	defer cancel()

	conn := dialServer(t, srv)
	defer conn.Close()
	require.NoError(t, WriteEvent(conn, (&Ping{}).ToEvent()))
	_, err := ReadEvent(bufio.NewReader(conn))
	require.NoError(t, err)

	srv.Shutdown()

	// Shutdown waits for connection goroutines, and CloseConn runs before they
	// exit, so the count is already final with no polling.
	assert.Equal(t, 1, factory.closeCount())
}

// R3: a Handler that implements neither optional interface is used as a
// process-wide singleton exactly as before.
func TestServer_NonFactoryHandlerIsShared(t *testing.T) {
	var mu sync.Mutex
	writers := 0
	handler := HandlerFunc(func(_ context.Context, _ *Event, w io.Writer) error {
		mu.Lock()
		writers++
		mu.Unlock()
		return WriteEvent(w, (&Pong{}).ToEvent())
	})

	srv, cancel := startTestServer(t, handler)
	defer cancel()
	defer srv.Shutdown()

	for i := 0; i < 2; i++ {
		conn := dialServer(t, srv)
		require.NoError(t, WriteEvent(conn, (&Ping{}).ToEvent()))
		ev, err := ReadEvent(bufio.NewReader(conn))
		require.NoError(t, err)
		assert.Equal(t, TypePong, ev.Type)
		conn.Close()
	}

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 2, writers)
}

// R3: a per-connection handler that does not implement ConnHandler is simply
// not notified of teardown.
func TestServer_FactoryWithoutConnHandler(t *testing.T) {
	factory := plainFactory{}
	srv, cancel := startTestServer(t, factory)
	defer cancel()
	defer srv.Shutdown()

	conn := dialServer(t, srv)
	defer conn.Close()
	require.NoError(t, WriteEvent(conn, (&Ping{}).ToEvent()))

	ev, err := ReadEvent(bufio.NewReader(conn))
	require.NoError(t, err)
	assert.Equal(t, TypePong, ev.Type)
}

// plainFactory builds per-connection handlers that do not implement ConnHandler.
type plainFactory struct{}

func (plainFactory) HandleEvent(context.Context, *Event, io.Writer) error {
	return errors.New("shared handler must not be used")
}

func (plainFactory) NewConnHandler() Handler {
	return HandlerFunc(func(_ context.Context, _ *Event, w io.Writer) error {
		return WriteEvent(w, (&Pong{}).ToEvent())
	})
}

// R4 scenario: concurrent writers do not interleave.
func TestConnWriterSerializesConcurrentEvents(t *testing.T) {
	var sink bytes.Buffer
	w := &connWriter{w: &sink}

	const goroutines, perGoroutine = 2, 100
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				ev := &Event{
					Type:    TypeAudioChunk,
					Data:    map[string]any{"rate": 24000, "width": 2, "channels": 1, "g": g},
					Payload: bytes.Repeat([]byte{byte(g)}, 2048),
				}
				if err := WriteEvent(w, ev); err != nil {
					t.Errorf("write event: %v", err)
					return
				}
			}
		}(g)
	}
	wg.Wait()

	reader := bufio.NewReader(&sink)
	counts := map[int]int{}
	for i := 0; i < goroutines*perGoroutine; i++ {
		ev, err := ReadEvent(reader)
		require.NoErrorf(t, err, "event %d must parse without a framing error", i)
		require.Equal(t, TypeAudioChunk, ev.Type)
		require.Len(t, ev.Payload, 2048)

		g := intFromAny(ev.Data["g"])
		// A payload is written by exactly one goroutine, so every byte of it
		// must match that goroutine's marker.
		require.Equal(t, bytes.Repeat([]byte{byte(g)}, 2048), ev.Payload)
		counts[g]++
	}
	assert.Equal(t, map[int]int{0: perGoroutine, 1: perGoroutine}, counts)

	_, err := ReadEvent(reader)
	assert.Error(t, err, "stream must hold exactly 200 events")
}

// R4: the connection writer the server hands to a handler is the same one it
// uses for its own error writes, so a handler goroutine and the read loop
// cannot interleave.
func TestServer_HandlerReceivesSerializedWriter(t *testing.T) {
	handlerErr := errors.New("boom")
	writerCh := make(chan io.Writer, 1)
	handler := HandlerFunc(func(_ context.Context, _ *Event, w io.Writer) error {
		writerCh <- w
		return handlerErr
	})

	srv, cancel := startTestServer(t, handler)
	defer cancel()
	defer srv.Shutdown()

	conn := dialServer(t, srv)
	defer conn.Close()
	require.NoError(t, WriteEvent(conn, (&Ping{}).ToEvent()))

	ev, err := ReadEvent(bufio.NewReader(conn))
	require.NoError(t, err)
	assert.Equal(t, TypeError, ev.Type)

	errEv, err := ErrorFromEvent(ev)
	require.NoError(t, err)
	assert.Equal(t, "handler-error", errEv.Code)
	assert.Equal(t, "boom", errEv.Text)

	select {
	case got := <-writerCh:
		_, ok := got.(*connWriter)
		assert.True(t, ok, "handler must receive the mutex-guarded connection writer")
	case <-time.After(2 * time.Second):
		t.Fatal("handler was not called")
	}
}

// sharedConnHandler implements ConnHandler but NOT HandlerFactory, so it stays
// the process-wide singleton and must never be told a connection closed.
type sharedConnHandler struct {
	mu     sync.Mutex
	closes int
}

func (h *sharedConnHandler) HandleEvent(_ context.Context, _ *Event, w io.Writer) error {
	return WriteEvent(w, (&Pong{}).ToEvent())
}

func (h *sharedConnHandler) CloseConn() {
	h.mu.Lock()
	h.closes++
	h.mu.Unlock()
}

func (h *sharedConnHandler) closeCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.closes
}

// R3: CloseConn is for the handler a connection owns. A shared singleton that
// happens to implement ConnHandler must behave exactly as before, which means
// never being torn down when one of many connections closes.
func TestServer_SharedHandlerIsNotTornDown(t *testing.T) {
	handler := &sharedConnHandler{}
	srv, cancel := startTestServer(t, handler)
	defer cancel()

	for i := 0; i < 2; i++ {
		conn := dialServer(t, srv)
		require.NoError(t, WriteEvent(conn, (&Ping{}).ToEvent()))
		ev, err := ReadEvent(bufio.NewReader(conn))
		require.NoError(t, err)
		assert.Equal(t, TypePong, ev.Type)
		conn.Close()
	}

	srv.Shutdown()
	assert.Zero(t, handler.closeCount(), "a shared handler must not receive CloseConn")
}
