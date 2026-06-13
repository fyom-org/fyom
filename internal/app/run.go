package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
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

	// Resolve DB path with strict priority: flag > env > default
	dbPath, dbSource, err := ResolveDBPath(opts.DBPath)
	if err != nil {
		slog.Error("failed to resolve db path", "error", err)
		return fmt.Errorf("resolve db path: %w", err)
	}

	// Ensure parent directory exists before opening SQLite
	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		slog.Error("failed to create db parent directory", "dir", dbDir, "error", err)
		return fmt.Errorf("create db dir %s: %w", dbDir, err)
	}

	// Load koanf config (server, auth, log settings)
	cfg, err := config.Load("")
	if err != nil {
		slog.Error("failed to load config", "error", err)
		return fmt.Errorf("load config: %w", err)
	}

	// Override DB path with resolved value
	cfg.Database.DBPath = dbPath

	// Override server host/port for sidecar mode
	if opts.Mode == RunModeSidecar {
		cfg.Server.Host = opts.Host
		cfg.Server.Port = opts.Port
	}

	slog.Info("fyom starting",
		"mode", opts.Mode,
		"version", version.Version,
		"commit", version.Commit,
		"db_path", dbPath,
		"db_source", dbSource,
	)

	// Open database
	db, err := repository.Open(
		cfg.Database.DBPath,
		cfg.Database.MaxOpenConns,
		cfg.Database.MaxIdleConns,
		cfg.Database.ConnMaxLifetime,
	)
	if err != nil {
		slog.Error("failed to open database", "error", err)
		return fmt.Errorf("open database: %w", err)
	}
	defer func() { _ = db.Close() }()

	slog.Info("database connected", "db_path", dbPath)

	// Build server config for sidecar mode
	// (host/port already set in cfg above)

	// Initialize the structured logger for startup messages
	log := logger.New(cfg.Log.Level, cfg.Log.Format)

	// Create and run server
	srv := fyomserver.New(cfg, log, db, version.Version, version.Commit, version.BuildTime, runtime.Version())
	addr := cfg.Server.Address()
	slog.Info("server listening", "addr", addr)

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

	// Resolve DB path
	dbPath, dbSource, err := ResolveDBPath(opts.DBPath)
	if err != nil {
		return fmt.Errorf("resolve db path: %w", err)
	}

	// Ensure parent directory
	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		return fmt.Errorf("create db dir %s: %w", dbDir, err)
	}

	// Load koanf config
	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	cfg.Database.DBPath = dbPath
	if opts.Mode == RunModeSidecar {
		cfg.Server.Host = opts.Host
		cfg.Server.Port = opts.Port
	}

	slog.Info("fyom starting",
		"mode", opts.Mode,
		"db_path", dbPath,
		"db_source", dbSource,
	)

	db, err := repository.Open(cfg.Database.DBPath, cfg.Database.MaxOpenConns, cfg.Database.MaxIdleConns, cfg.Database.ConnMaxLifetime)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() { _ = db.Close() }()

	log := logger.New(cfg.Log.Level, cfg.Log.Format)
	srv := fyomserver.New(cfg, log, db, version.Version, version.Commit, version.BuildTime, runtime.Version())

	// Start server in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Run()
	}()

	// Wait for context cancellation or server error
	select {
	case <-ctx.Done():
		return nil
	case err := <-errCh:
		return err
	}
}
