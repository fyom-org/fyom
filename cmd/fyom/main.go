// Package main is the entry point for the fyom server.
package main

import (
	"log/slog"
	"os"
	"runtime"

	"github.com/fyom/fyom/internal/config"
	"github.com/fyom/fyom/internal/repository"
	fyomserver "github.com/fyom/fyom/internal/server"
	"github.com/fyom/fyom/pkg/logger"
)

// Build-time variables (set via -ldflags).
var (
	version   = "dev"
	gitCommit = "none"
	buildTime = "unknown"
)

func main() {
	// Load configuration
	cfg, err := config.Load("")
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Initialize logger
	log := logger.New(cfg.Log.Level, cfg.Log.Format)
	log.Info("fyom starting",
		"version", version,
		"commit", gitCommit,
		"build_time", buildTime,
		"go", runtime.Version(),
	)

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
	srv := fyomserver.New(cfg, log, db, version, gitCommit, buildTime, runtime.Version())
	if err := srv.Run(); err != nil {
		log.Error("server error", "error", err)
		os.Exit(1)
	}
}
