// Package main is the entry point for the fyom server.
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"runtime"

	"github.com/fyom/fyom/internal/config"
	"github.com/fyom/fyom/internal/repository"
	fyomserver "github.com/fyom/fyom/internal/server"
	"github.com/fyom/fyom/internal/version"
	"github.com/fyom/fyom/pkg/logger"
)

func main() {
	logLevel := flag.String("log-level", "info", "log level: debug, info, warn, error")
	flag.Parse()

	// Initialize slog with the requested level
	var level slog.Level
	switch *logLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	})))

	// Load configuration
	cfg, err := config.Load("")
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Initialize the structured logger for startup messages
	log := logger.New(cfg.Log.Level, cfg.Log.Format)

	slog.Info("fyom starting",
		"version", version.Version,
		"commit", version.Commit,
		"go", runtime.Version(),
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
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()

	log.Info("database connected", "data_dir", cfg.Database.DataDir)

	// Create and run server
	srv := fyomserver.New(cfg, log, db, version.Version, version.Commit, version.BuildTime, runtime.Version())
	slog.Info("server listening", "addr", cfg.Server.Address())

	if err := srv.Run(); err != nil {
		log.Error("server error", "error", err)
		os.Exit(1)
	}

	fmt.Fprintln(os.Stderr, "server stopped gracefully")
}
