package api

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/fx/meadowlark/internal/model"
	"github.com/fx/meadowlark/internal/store"
	"github.com/fx/meadowlark/internal/tts"
	"github.com/fx/meadowlark/internal/wyoming"
)

// ClientFactory creates a TTS Client for a given endpoint.
type ClientFactory func(ep *model.Endpoint) *tts.Client

// Server is the HTTP API server for Meadowlark.
type Server struct {
	store         store.Store
	infoBuilder   *wyoming.InfoBuilder
	clientFactory ClientFactory
	listenAddr    string
	startTime     time.Time
	version       string
	wyomingPort   int
	httpPort      int
	dbDriver      string
	webFS         fs.FS
}

// NewServer creates a new HTTP API server.
func NewServer(
	st store.Store,
	ib *wyoming.InfoBuilder,
	cf ClientFactory,
	listenAddr string,
	version string,
	wyomingPort int,
	httpPort int,
	dbDriver string,
	webFS fs.FS,
) *Server {
	return &Server{
		store:         st,
		infoBuilder:   ib,
		clientFactory: cf,
		listenAddr:    listenAddr,
		startTime:     time.Now(),
		version:       version,
		wyomingPort:   wyomingPort,
		httpPort:      httpPort,
		dbDriver:      dbDriver,
		webFS:         webFS,
	}
}

// Start starts the HTTP server and blocks until ctx is cancelled, then
// gracefully shuts down with a 10-second timeout.
func (s *Server) Start(ctx context.Context) error {
	router := s.setupRoutes()

	srv := &http.Server{
		Addr:              s.listenAddr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	srvErr := make(chan error, 1)
	go func() {
		slog.Info("http server listening", "addr", s.listenAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			srvErr <- err
		}
		close(srvErr)
	}()

	select {
	case <-ctx.Done():
	case err := <-srvErr:
		return fmt.Errorf("http server: %w", err)
	}

	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutCtx); err != nil {
		return fmt.Errorf("http server shutdown: %w", err)
	}
	return nil
}

func (s *Server) setupRoutes() *chi.Mux {
	r := chi.NewRouter()

	// Middleware stack
	r.Use(requestLogger)
	r.Use(recovery)
	r.Use(cors)

	// API routes
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(jsonContentType)

		// Endpoints
		r.Get("/endpoints", s.notImplemented)
		r.Post("/endpoints", s.notImplemented)
		r.Get("/endpoints/{id}", s.notImplemented)
		r.Put("/endpoints/{id}", s.notImplemented)
		r.Delete("/endpoints/{id}", s.notImplemented)
		r.Post("/endpoints/{id}/test", s.notImplemented)
		r.Get("/endpoints/{id}/voices", s.notImplemented)

		// Aliases
		r.Get("/aliases", s.ListAliases)
		r.Post("/aliases", s.CreateAlias)
		r.Get("/aliases/{id}", s.GetAlias)
		r.Put("/aliases/{id}", s.UpdateAlias)
		r.Delete("/aliases/{id}", s.DeleteAlias)
		r.Post("/aliases/{id}/test", s.TestAlias)

		// System
		r.Get("/status", s.GetStatus)
		r.Get("/voices", s.ListVoices)
	})

	// Static file serving with SPA fallback
	s.mountSPA(r)

	return r
}

func (s *Server) notImplemented(w http.ResponseWriter, _ *http.Request) {
	respondError(w, http.StatusNotImplemented, "not_implemented", "this endpoint is not yet implemented")
}

// mountSPA serves the embedded frontend and falls back to index.html for
// any path that doesn't match a real file or the /api/ prefix.
func (s *Server) mountSPA(r *chi.Mux) {
	fileServer := http.FileServer(http.FS(s.webFS))

	r.HandleFunc("/*", func(w http.ResponseWriter, req *http.Request) {
		path := req.URL.Path

		// Skip /api/ routes (already handled above).
		if strings.HasPrefix(path, "/api/") {
			http.NotFound(w, req)
			return
		}

		// Try to serve the requested file.
		cleanPath := strings.TrimPrefix(path, "/")
		if cleanPath == "" {
			cleanPath = "index.html"
		}
		if _, err := fs.Stat(s.webFS, cleanPath); err == nil {
			fileServer.ServeHTTP(w, req)
			return
		}

		// SPA fallback: serve index.html for all other paths.
		req.URL.Path = "/"
		fileServer.ServeHTTP(w, req)
	})
}
