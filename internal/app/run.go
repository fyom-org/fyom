package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"time"

	"github.com/fyom/fyom/internal/config"
	"github.com/fyom/fyom/internal/repository"
	fyomserver "github.com/fyom/fyom/internal/server"
	"github.com/fyom/fyom/internal/version"
	"github.com/fyom/fyom/pkg/logger"
)

// Run executes the application with the given options.
// It returns an error if the application fails to start or run.
func Run(opts RunOptions) error {
	// Initialize slog with the requested level
	var level slog.Level
	switch opts.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	// In sidecar mode, route routine logs to stderr and reserve stdout for readiness signal
	var logHandler slog.Handler
	if opts.Mode == RunModeSidecar {
		logHandler = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	} else {
		logHandler = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	}
	slog.SetDefault(slog.New(logHandler))

	// Load configuration
	cfg, err := config.Load(opts.DataDir)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		return fmt.Errorf("load config: %w", err)
	}

	// Override config with CLI options if provided
	if opts.Mode == RunModeSidecar {
		cfg.Server.Host = opts.Host
		cfg.Server.Port = opts.Port
	}

	// Initialize the structured logger for startup messages
	log := logger.New(cfg.Log.Level, cfg.Log.Format)

	slog.Info("fyom starting",
		"mode", opts.Mode,
		"version", version.Version,
		"commit", version.Commit,
		"go", version.BuildTime,
	)

	// Debug: data directory permissions
	if info, err := os.Stat(cfg.Database.DataDir); err == nil {
		slog.Debug("data directory", "path", cfg.Database.DataDir, "mode", info.Mode().String())
	}

	// Open database
	db, err := repository.Open(
		cfg.Database.DataDir,
		cfg.Database.MaxOpenConns,
		cfg.Database.MaxIdleConns,
		cfg.Database.ConnMaxLifetime,
	)
	if err != nil {
		log.Error("failed to open database", "error", err)
		return fmt.Errorf("open database: %w", err)
	}
	defer func() { _ = db.Close() }()

	log.Info("database connected", "data_dir", cfg.Database.DataDir)

	// Create and run server
	srv := fyomserver.New(cfg, log, db, version.Version, version.Commit, version.BuildTime, runtime.Version())
	addr := cfg.Server.Address()
	log.Info("server listening", "addr", addr)

	// In sidecar mode, emit readiness signal after server is fully initialized
	if opts.Mode == RunModeSidecar {
		// Give the server a moment to fully start listening
		time.Sleep(100 * time.Millisecond)
		// Emit readiness signal to stdout (machine-readable)
		fmt.Fprintf(os.Stdout, "FYOM_READY http://%s\n", addr)
	}

	// Run the server (handles signals and shutdown)
	return srv.Run()
}

// RunWithContext runs the application with a context for testing.
func RunWithContext(ctx context.Context, opts RunOptions) error {
	// Initialize slog with the requested level
	var level slog.Level
	switch opts.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	logHandler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(logHandler))

	cfg, err := config.Load(opts.DataDir)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if opts.Mode == RunModeSidecar {
		cfg.Server.Host = opts.Host
		cfg.Server.Port = opts.Port
	}

	log := logger.New(cfg.Log.Level, cfg.Log.Format)

	db, err := repository.Open(
		cfg.Database.DataDir,
		cfg.Database.MaxOpenConns,
		cfg.Database.MaxIdleConns,
		cfg.Database.ConnMaxLifetime,
	)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() { _ = db.Close() }()

	srv := fyomserver.New(cfg, log, db, version.Version, version.Commit, version.BuildTime, runtime.Version())

	// Start server in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Run()
	}()

	// Wait for context cancellation or server error
	select {
	case <-ctx.Done():
		// Trigger shutdown via signal
		return nil
	case err := <-errCh:
		return err
	}
}
