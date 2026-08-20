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
)

// Handler processes Wyoming events on a connection. Implementations must
// write response events to the provided writer. The handler is called for
// each event received on the connection.
type Handler interface {
	// HandleEvent processes a single event and writes any response events to w.
	// Returning an error causes an error event to be sent and the connection continues.
	HandleEvent(ctx context.Context, ev *Event, w io.Writer) error
}

// HandlerFunc is an adapter to allow the use of ordinary functions as Handlers.
type HandlerFunc func(ctx context.Context, ev *Event, w io.Writer) error

// HandleEvent calls f(ctx, ev, w).
func (f HandlerFunc) HandleEvent(ctx context.Context, ev *Event, w io.Writer) error {
	return f(ctx, ev, w)
}

// HandlerFactory is an optional interface a Handler may implement to be
// constructed per connection rather than shared process-wide. It exists
// because per-connection state — a streaming synthesis session, for one — has
// nowhere to live in HandleEvent's signature, which carries no connection
// identity.
//
// A Handler that does not implement it, HandlerFunc included, is used as a
// process-wide singleton exactly as before.
type HandlerFactory interface {
	// NewConnHandler returns a fresh Handler for one accepted connection.
	NewConnHandler() Handler
}

// ConnHandler is an optional interface a per-connection Handler may implement
// to release resources when its connection is torn down.
//
// CloseConn must block until the connection's background work has finished, so
// that Shutdown's connection drain genuinely waits for in-flight work rather
// than abandoning it.
type ConnHandler interface {
	// CloseConn releases the handler's resources. It is called exactly once,
	// before the connection goroutine exits.
	CloseConn()
}

// connWriter serializes writes to a connection shared by several goroutines.
//
// It is only half of the atomicity guarantee: WriteEvent issues exactly one
// Write per event, and this mutex makes that Write exclusive. Neither half is
// sufficient alone.
type connWriter struct {
	mu sync.Mutex
	w  io.Writer
}

// Write writes p to the underlying writer, excluding any concurrent write.
func (c *connWriter) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.w.Write(p)
}

// Server is a Wyoming protocol TCP server.
type Server struct {
	addr    string
	handler Handler
	logger  *slog.Logger

	mu       sync.Mutex
	listener net.Listener
	conns    map[net.Conn]struct{}
	wg       sync.WaitGroup
	closed   bool
}

// NewServer creates a new Wyoming TCP server that listens on the given address
// and dispatches events to the given handler.
func NewServer(addr string, handler Handler, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		addr:    addr,
		handler: handler,
		logger:  logger,
		conns:   make(map[net.Conn]struct{}),
	}
}

// ListenAndServe starts the TCP listener and serves connections until
// Shutdown is called or the context is canceled.
func (s *Server) ListenAndServe(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("wyoming: listen on %s: %w", s.addr, err)
	}

	s.mu.Lock()
	s.listener = ln
	s.mu.Unlock()

	s.logger.Info("wyoming server listening", "addr", ln.Addr().String())

	// Close listener when context is canceled.
	go func() {
		<-ctx.Done()
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()
		ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()
			if closed {
				return nil
			}
			s.logger.Error("accept connection", "error", err)
			continue
		}

		// Registration, the shutdown check and the wait-group increment must
		// all happen under one acquisition of s.mu. Splitting them leaves a
		// window in which Shutdown can iterate an empty s.conns and observe a
		// zero wait-group counter, return, and only then have this loop start a
		// connection goroutine that nothing will ever close. Holding the mutex
		// across all three leaves exactly two interleavings: either this
		// connection is registered and counted before Shutdown takes the mutex,
		// or it observes closed and is refused.
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			conn.Close()
			continue
		}
		s.conns[conn] = struct{}{}
		s.wg.Add(1)
		s.mu.Unlock()

		go s.handleConn(ctx, conn)
	}
}

// Addr returns the listener address, or empty string if not yet listening.
func (s *Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return ""
}

// Shutdown gracefully stops the server: closes the listener and drains
// active connections.
func (s *Server) Shutdown() {
	s.mu.Lock()
	s.closed = true
	if s.listener != nil {
		s.listener.Close()
	}
	// Close all active connections to unblock reads.
	for c := range s.conns {
		c.Close()
	}
	s.mu.Unlock()

	// Wait for all connection goroutines to finish.
	s.wg.Wait()
	s.logger.Info("wyoming server stopped")
}

func (s *Server) handleConn(ctx context.Context, conn net.Conn) {
	defer s.wg.Done()

	// Build a per-connection handler when the configured handler is a factory;
	// otherwise keep using the shared singleton.
	handler := s.handler
	var perConn Handler
	if factory, ok := s.handler.(HandlerFactory); ok {
		perConn = factory.NewConnHandler()
		handler = perConn
	}

	defer func() {
		// Teardown is only ever signalled to a handler this connection owns.
		// A shared singleton that happens to implement ConnHandler must not be
		// torn down when one of many connections closes.
		//
		// CloseConn blocks until the connection's background work has
		// finished, which is what makes Shutdown's wg.Wait genuinely wait for
		// in-flight synthesis rather than abandoning it.
		//
		// It must not assume the socket is still writable. This defer runs it
		// before closing the connection itself, but Shutdown closes every
		// connection in s.conns to unblock ReadEvent before it waits — so on
		// that path the socket is already gone by the time CloseConn is
		// called. A teardown path therefore cancels and drains; it does not
		// try to flush anything to the client.
		if ch, ok := perConn.(ConnHandler); ok {
			ch.CloseConn()
		}
		conn.Close()
		s.mu.Lock()
		delete(s.conns, conn)
		s.mu.Unlock()
	}()

	remote := conn.RemoteAddr().String()
	s.logger.Debug("connection accepted", "remote", remote)

	// Every event written to this connection — by the handler, by a session's
	// background goroutine, or by the read loop below — goes through w, so no
	// two writes can interleave.
	w := &connWriter{w: conn}
	reader := bufio.NewReader(conn)

	for {
		ev, err := ReadEvent(reader)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) || isConnReset(err) {
				s.logger.Debug("connection closed", "remote", remote)
				return
			}
			// Check if context was canceled.
			if ctx.Err() != nil {
				return
			}
			s.logger.Error("read event", "remote", remote, "error", err)
			return
		}

		s.logger.Debug("event received", "remote", remote, "type", ev.Type)

		if err := handler.HandleEvent(ctx, ev, w); err != nil {
			s.logger.Error("handle event", "remote", remote, "type", ev.Type, "error", err)
			errEv := &Error{Text: err.Error(), Code: "handler-error"}
			if writeErr := WriteEvent(w, errEv.ToEvent()); writeErr != nil {
				s.logger.Error("write error event", "remote", remote, "error", writeErr)
				return
			}
		}
	}
}

// isConnReset reports whether the error indicates a connection reset.
func isConnReset(err error) bool {
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return opErr.Err.Error() == "read: connection reset by peer" ||
			opErr.Err.Error() == "use of closed network connection"
	}
	// Also check the raw error string for wrapped errors.
	return errors.Is(err, net.ErrClosed)
}
