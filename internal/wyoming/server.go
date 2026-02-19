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

		s.mu.Lock()
		s.conns[conn] = struct{}{}
		s.mu.Unlock()

		s.wg.Add(1)
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
	defer func() {
		conn.Close()
		s.mu.Lock()
		delete(s.conns, conn)
		s.mu.Unlock()
	}()

	remote := conn.RemoteAddr().String()
	s.logger.Debug("connection accepted", "remote", remote)

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

		if err := s.handler.HandleEvent(ctx, ev, conn); err != nil {
			s.logger.Error("handle event", "remote", remote, "type", ev.Type, "error", err)
			errEv := &Error{Text: err.Error(), Code: "handler-error"}
			if writeErr := WriteEvent(conn, errEv.ToEvent()); writeErr != nil {
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
