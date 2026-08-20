package main

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	meadowlark "github.com/fx/meadowlark"
	"github.com/fx/meadowlark/internal/api"
	"github.com/fx/meadowlark/internal/model"
	"github.com/fx/meadowlark/internal/segment"
	"github.com/fx/meadowlark/internal/store"
	"github.com/fx/meadowlark/internal/tts"
	"github.com/fx/meadowlark/internal/voice"
	"github.com/fx/meadowlark/internal/wyoming"
)

var (
	version = "dev"
	commit  = "none"
)

// defaultSessionTimeout is how long a streaming synthesis session may sit idle
// before it is abandoned. Zero disables the timer; a negative value is rejected
// in favour of this default.
const defaultSessionTimeout = 30 * time.Second

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "meadowlark",
		Short: "Wyoming to OpenAI-compatible TTS bridge",
		RunE:  run,
	}

	// Wyoming flags
	cmd.Flags().String("wyoming-host", "0.0.0.0", "Wyoming TCP listen address")
	cmd.Flags().Int("wyoming-port", 10300, "Wyoming TCP listen port")

	// HTTP flags
	cmd.Flags().String("http-host", "0.0.0.0", "HTTP server listen address")
	cmd.Flags().Int("http-port", 8080, "HTTP server listen port")

	// Database flags
	cmd.Flags().String("db-driver", "sqlite", "Database driver: sqlite or postgres")
	cmd.Flags().String("db-dsn", "meadowlark.db", "Database connection string")

	// Zeroconf flags
	hostname, _ := os.Hostname()
	cmd.Flags().String("zeroconf-name", hostname, "Zeroconf/mDNS service name")
	cmd.Flags().Bool("no-zeroconf", false, "Disable Zeroconf registration")

	// Streaming synthesis flags
	cmd.Flags().Int("synthesize-first-segment-chars", segment.DefaultFirstSegmentChars,
		"Minimum characters in a streaming session's first segment")
	cmd.Flags().Int("synthesize-min-segment-chars", segment.DefaultMinSegmentChars,
		"Minimum characters in every later streaming segment")
	cmd.Flags().Int("synthesize-max-segment-chars", segment.DefaultMaxSegmentChars,
		"Characters after which a streaming segment break is forced")
	cmd.Flags().Duration("synthesize-session-timeout", defaultSessionTimeout,
		"Idle timeout for a streaming synthesis session (0 disables it)")

	// Logging flags
	cmd.Flags().String("log-level", "info", "Log level: debug, info, warn, error")
	cmd.Flags().String("log-format", "text", "Log format: text, json")

	// Version flag
	cmd.Version = fmt.Sprintf("%s (commit: %s)", version, commit)

	// Bind flags to viper with MEADOWLARK_ prefix
	viper.SetEnvPrefix("MEADOWLARK")
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(dashReplacer())

	_ = viper.BindPFlag("wyoming_host", cmd.Flags().Lookup("wyoming-host"))
	_ = viper.BindPFlag("wyoming_port", cmd.Flags().Lookup("wyoming-port"))
	_ = viper.BindPFlag("http_host", cmd.Flags().Lookup("http-host"))
	_ = viper.BindPFlag("http_port", cmd.Flags().Lookup("http-port"))
	_ = viper.BindPFlag("db_driver", cmd.Flags().Lookup("db-driver"))
	_ = viper.BindPFlag("db_dsn", cmd.Flags().Lookup("db-dsn"))
	_ = viper.BindPFlag("zeroconf_name", cmd.Flags().Lookup("zeroconf-name"))
	_ = viper.BindPFlag("no_zeroconf", cmd.Flags().Lookup("no-zeroconf"))
	_ = viper.BindPFlag("synthesize_first_segment_chars", cmd.Flags().Lookup("synthesize-first-segment-chars"))
	_ = viper.BindPFlag("synthesize_min_segment_chars", cmd.Flags().Lookup("synthesize-min-segment-chars"))
	_ = viper.BindPFlag("synthesize_max_segment_chars", cmd.Flags().Lookup("synthesize-max-segment-chars"))
	_ = viper.BindPFlag("synthesize_session_timeout", cmd.Flags().Lookup("synthesize-session-timeout"))
	_ = viper.BindPFlag("log_level", cmd.Flags().Lookup("log-level"))
	_ = viper.BindPFlag("log_format", cmd.Flags().Lookup("log-format"))

	return cmd
}

func run(cmd *cobra.Command, args []string) error {
	// Configure slog
	logger := configureLogger(
		viper.GetString("log_level"),
		viper.GetString("log_format"),
	)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// Log configuration summary
	slog.Info("starting meadowlark",
		"version", version,
		"commit", commit,
		"wyoming_host", viper.GetString("wyoming_host"),
		"wyoming_port", viper.GetInt("wyoming_port"),
		"http_host", viper.GetString("http_host"),
		"http_port", viper.GetInt("http_port"),
		"db_driver", viper.GetString("db_driver"),
		"db_dsn", viper.GetString("db_dsn"),
		"zeroconf_name", viper.GetString("zeroconf_name"),
		"no_zeroconf", viper.GetBool("no_zeroconf"),
		"log_level", viper.GetString("log_level"),
		"log_format", viper.GetString("log_format"),
		"synthesize_first_segment_chars", viper.GetInt("synthesize_first_segment_chars"),
		"synthesize_min_segment_chars", viper.GetInt("synthesize_min_segment_chars"),
		"synthesize_max_segment_chars", viper.GetInt("synthesize_max_segment_chars"),
		"synthesize_session_timeout", viper.GetDuration("synthesize_session_timeout").String(),
	)

	// Segmentation and session configuration are validated before anything is
	// built, so a misconfigured process logs its warning at startup rather than
	// on the first synthesis.
	segCfg := segmentConfig(logger,
		viper.GetInt("synthesize_first_segment_chars"),
		viper.GetInt("synthesize_min_segment_chars"),
		viper.GetInt("synthesize_max_segment_chars"),
	)
	idleTimeout := sessionTimeout(logger, viper.GetDuration("synthesize_session_timeout"))

	// 1. Initialize database store.
	db, err := openStore(ctx, viper.GetString("db_driver"), viper.GetString("db_dsn"))
	if err != nil {
		return fmt.Errorf("init database: %w", err)
	}

	// 2. Run migrations.
	if err := db.Migrate(ctx); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	slog.Info("database migrations complete")

	// 3. Create voice resolver and TTS proxy.
	resolver := voice.NewResolver(db, db)
	infoBuilder := wyoming.NewInfoBuilder(db, db, db, version)
	proxy := tts.NewProxy(resolver, db, defaultClientFactory, logger)

	// 4. Build Wyoming handler.
	handler := newWyomingHandler(infoBuilder, proxy, segCfg, idleTimeout, logger)

	// 5. Start Wyoming TCP server.
	wyomingAddr := fmt.Sprintf("%s:%d", viper.GetString("wyoming_host"), viper.GetInt("wyoming_port"))
	srv := wyoming.NewServer(wyomingAddr, handler, logger)

	srvErr := make(chan error, 1)
	go func() {
		srvErr <- srv.ListenAndServe(ctx)
	}()

	// 6. Start HTTP API server.
	httpAddr := fmt.Sprintf("%s:%d", viper.GetString("http_host"), viper.GetInt("http_port"))
	webFS, err := fs.Sub(meadowlark.WebFS, "web/dist")
	if err != nil {
		return fmt.Errorf("embedded web filesystem: %w", err)
	}
	httpSrv := api.NewServer(
		db,
		infoBuilder,
		apiClientFactory,
		httpAddr,
		version,
		viper.GetInt("wyoming_port"),
		viper.GetInt("http_port"),
		viper.GetString("db_driver"),
		webFS,
	)

	httpErr := make(chan error, 1)
	go func() {
		httpErr <- httpSrv.Start(ctx)
	}()

	// 7. Register Zeroconf (unless disabled).
	var zc *wyoming.ZeroconfService
	if !viper.GetBool("no_zeroconf") {
		zc, err = wyoming.RegisterZeroconf(wyoming.ZeroconfConfig{
			ServiceName: viper.GetString("zeroconf_name"),
			Port:        viper.GetInt("wyoming_port"),
		}, logger)
		if err != nil {
			slog.Warn("zeroconf registration failed", "error", err)
		}
	}

	slog.Info("ready")

	// 8. Block until shutdown signal or server error.
	select {
	case <-ctx.Done():
		slog.Info("shutdown signal received")
	case err := <-srvErr:
		if err != nil {
			slog.Error("wyoming server error", "error", err)
		}
	case err := <-httpErr:
		if err != nil {
			slog.Error("http server error", "error", err)
		}
	}

	// 9. Graceful shutdown sequence.
	slog.Info("shutting down")

	// Stop HTTP server first (with timeout via Start's internal shutdown).
	stop()

	// Stop Wyoming server (stop accepting, drain connections).
	srv.Shutdown()

	// Deregister Zeroconf.
	if zc != nil {
		zc.Shutdown()
	}

	// Close database.
	if err := db.Close(); err != nil {
		slog.Error("close database", "error", err)
	}

	slog.Info("shutdown complete")
	return nil
}

// openStore initializes the correct store backend based on the driver flag.
func openStore(ctx context.Context, driver, dsn string) (store.Store, error) {
	switch driver {
	case "sqlite":
		return store.NewSQLiteStore(dsn)
	case "postgres":
		return store.NewPostgresStore(ctx, dsn)
	default:
		return nil, fmt.Errorf("unsupported database driver: %q (use sqlite or postgres)", driver)
	}
}

// defaultClientFactory creates a TTS client from an endpoint configuration.
func defaultClientFactory(ep *model.Endpoint) *tts.Client {
	return tts.NewClient(ep.BaseURL, ep.APIKey, nil)
}

// apiClientFactory adapts defaultClientFactory to the api.ClientFactory type.
func apiClientFactory(ep *model.Endpoint) *tts.Client {
	return defaultClientFactory(ep)
}

// segmentConfig builds the segmentation configuration from the three character
// thresholds, falling back to all three defaults when they are incoherent.
//
// The fallback is deliberately all-or-nothing. Honouring the coherent subset of
// an invalid set would start the process with a segmenter the operator never
// asked for and cannot infer from the flags they passed; using the documented
// defaults for all three at least matches what the flag help says.
func segmentConfig(logger *slog.Logger, first, minimum, maximum int) segment.Config {
	cfg := segment.Config{
		FirstSegmentChars: first,
		MinSegmentChars:   minimum,
		MaxSegmentChars:   maximum,
	}
	if err := cfg.Validate(); err != nil {
		def := segment.DefaultConfig()
		logger.Warn("invalid segmentation thresholds; falling back to defaults",
			"error", err,
			"first_segment_chars", first,
			"min_segment_chars", minimum,
			"max_segment_chars", maximum,
			"default_first_segment_chars", def.FirstSegmentChars,
			"default_min_segment_chars", def.MinSegmentChars,
			"default_max_segment_chars", def.MaxSegmentChars,
		)
		return def
	}
	return cfg
}

// sessionTimeout validates the streaming session idle timeout.
//
// A duration flag accepts values the character thresholds cannot, so it is
// validated separately: a positive value enables the timer, zero disables it
// entirely, and a negative value is rejected. Cobra and Viper both accept
// "-1s" without complaint, and a negative duration fires a timer immediately —
// every session would fail with synthesize-timeout the instant it opened.
// Treating a negative as "disabled" would hide the operator's mistake instead.
func sessionTimeout(logger *slog.Logger, d time.Duration) time.Duration {
	if d < 0 {
		logger.Warn("negative synthesize session timeout; falling back to default",
			"synthesize_session_timeout", d.String(),
			"default", defaultSessionTimeout.String(),
		)
		return defaultSessionTimeout
	}
	return d
}

// wyomingHandler dispatches Wyoming protocol events.
//
// It is both the process-wide handler and a wyoming.HandlerFactory: the server
// builds one connHandler per connection so that each gets a private streaming
// session, and HandleEvent below is the sessionless dispatch that a connHandler
// falls back to for every event no session owns.
type wyomingHandler struct {
	info        *wyoming.InfoBuilder
	proxy       *tts.Proxy
	segCfg      segment.Config
	idleTimeout time.Duration
	logger      *slog.Logger
}

func newWyomingHandler(
	info *wyoming.InfoBuilder,
	proxy *tts.Proxy,
	segCfg segment.Config,
	idleTimeout time.Duration,
	logger *slog.Logger,
) *wyomingHandler {
	return &wyomingHandler{
		info:        info,
		proxy:       proxy,
		segCfg:      segCfg,
		idleTimeout: idleTimeout,
		logger:      logger,
	}
}

// NewConnHandler gives one accepted connection its own handler, and with it its
// own streaming synthesis session. Session state is then an ordinary field of a
// private struct: no map keyed by connection, no lock around it, and a teardown
// hook that actually runs when the connection drops.
func (h *wyomingHandler) NewConnHandler() wyoming.Handler {
	return &connHandler{
		h:       h,
		session: tts.NewStreamSession(h.proxy, h.segCfg, h.idleTimeout, h.logger),
	}
}

func (h *wyomingHandler) HandleEvent(ctx context.Context, ev *wyoming.Event, w io.Writer) error {
	switch ev.Type {
	case wyoming.TypeDescribe:
		info, err := h.info.Build(ctx)
		if err != nil {
			return fmt.Errorf("build info: %w", err)
		}
		return wyoming.WriteEvent(w, info.ToEvent())

	case wyoming.TypeSynthesize:
		synth, err := wyoming.SynthesizeFromEvent(ev)
		if err != nil {
			return fmt.Errorf("parse synthesize: %w", err)
		}
		h.proxy.HandleSynthesize(ctx, synth, w)
		return nil

	case wyoming.TypePing:
		pong := &wyoming.Pong{}
		return wyoming.WriteEvent(w, pong.ToEvent())

	default:
		h.logger.Debug("ignoring unknown event type", "type", ev.Type)
		return nil
	}
}

// connHandler serves one connection. It owns that connection's streaming
// synthesis session and dispatches the streaming events to it, delegating
// everything else to the sessionless dispatch on wyomingHandler.
//
// Every error returned from here is a connection-write failure, never a
// synthesis failure: a session reports its own failures as Wyoming error events
// and returns nil, because the server turns a returned error into a second
// error event with code handler-error.
type connHandler struct {
	h       *wyomingHandler
	session *tts.StreamSession
}

func (c *connHandler) HandleEvent(ctx context.Context, ev *wyoming.Event, w io.Writer) error {
	switch ev.Type {
	case wyoming.TypeSynthesizeStart:
		start, err := wyoming.SynthesizeStartFromEvent(ev)
		if err != nil {
			return fmt.Errorf("parse synthesize-start: %w", err)
		}
		return c.session.Start(ctx, w, start)

	case wyoming.TypeSynthesizeChunk:
		chunk, err := wyoming.SynthesizeChunkFromEvent(ev)
		if err != nil {
			return fmt.Errorf("parse synthesize-chunk: %w", err)
		}
		return c.session.Chunk(chunk)

	case wyoming.TypeSynthesizeStop:
		// synthesize-stop carries no payload, so there is nothing to parse.
		return c.session.Stop()

	case wyoming.TypeSynthesize:
		synth, err := wyoming.SynthesizeFromEvent(ev)
		if err != nil {
			return fmt.Errorf("parse synthesize: %w", err)
		}
		if c.session.Compat(synth) {
			// Home Assistant's compatibility event, carrying a message this
			// session is already synthesizing from its chunks — or one whose
			// session failed, in which case speaking it now would speak it a
			// second time.
			return nil
		}
		// No session owns it: an ordinary whole-message synthesize from a
		// client that does not stream its input. Hand it to the sessionless
		// dispatch, which is the single definition of what that means.
		return c.h.HandleEvent(ctx, ev, w)

	default:
		return c.h.HandleEvent(ctx, ev, w)
	}
}

// CloseConn tears the session down when the connection goes away. It blocks
// until the session's goroutines have exited, which is what makes the server's
// shutdown drain wait for in-flight synthesis rather than abandoning it.
func (c *connHandler) CloseConn() {
	c.session.Close()
}

func configureLogger(level, format string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: lvl}

	var handler slog.Handler
	if format == "json" {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		handler = slog.NewTextHandler(os.Stderr, opts)
	}

	return slog.New(handler)
}

// dashReplacer returns a replacer that converts dashes to underscores for env var matching.
func dashReplacer() *strings.Replacer {
	return strings.NewReplacer("-", "_")
}
