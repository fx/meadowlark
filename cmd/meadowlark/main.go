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

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	meadowlark "github.com/fx/meadowlark"
	"github.com/fx/meadowlark/internal/api"
	"github.com/fx/meadowlark/internal/model"
	"github.com/fx/meadowlark/internal/store"
	"github.com/fx/meadowlark/internal/tts"
	"github.com/fx/meadowlark/internal/voice"
	"github.com/fx/meadowlark/internal/wyoming"
)

var (
	version = "dev"
	commit  = "none"
)

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
	)

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
	handler := newWyomingHandler(infoBuilder, proxy, logger)

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

// wyomingHandler dispatches Wyoming protocol events.
type wyomingHandler struct {
	info   *wyoming.InfoBuilder
	proxy  *tts.Proxy
	logger *slog.Logger
}

func newWyomingHandler(info *wyoming.InfoBuilder, proxy *tts.Proxy, logger *slog.Logger) *wyomingHandler {
	return &wyomingHandler{info: info, proxy: proxy, logger: logger}
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
