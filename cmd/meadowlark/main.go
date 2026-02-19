package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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

	// Wait for shutdown signal
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	slog.Info("ready, waiting for signal")
	<-ctx.Done()
	slog.Info("shutting down")

	return nil
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
