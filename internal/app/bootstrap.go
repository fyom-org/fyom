package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"github.com/fyom/fyom/internal/config"
	"github.com/fyom/fyom/internal/repository"
	"github.com/fyom/fyom/internal/server"
)

// Runtime holds the initialized application state shared between
// server and desktop modes.
type Runtime struct {
	Router   http.Handler
	Shutdown func(ctx context.Context) error
}

// Options for Bootstrap.
type Options struct {
	Mode string
}

// Bootstrap initializes config, database, repositories, services,
// and builds the Chi router. It returns a Runtime that can be used
// by either server mode (http.ListenAndServe) or desktop mode
// (in-process http.Handler).
func Bootstrap(ctx context.Context, opts Options) (*Runtime, error) {
	cfg, err := config.Load("")
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	dbPath := cfg.Database.DBPath
	if dbPath == "" {
		exePath, _ := os.Executable()
		dbPath = filepath.Join(filepath.Dir(exePath), "fyom.db")
	}

	slog.Info("database path resolved", "path", dbPath)

	if dir := filepath.Dir(dbPath); dir != "." && dir != "/" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create db parent dir %s: %w", dir, err)
		}
	}

	db, err := repository.Open(dbPath, cfg.Database.MaxOpenConns, cfg.Database.MaxIdleConns, cfg.Database.ConnMaxLifetime)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	log := slog.Default()

	srv := server.New(cfg, log, db, "dev", "unknown", "unknown", runtime.Version(), "")

	return &Runtime{
		Router: srv.Router(),
		Shutdown: func(ctx context.Context) error {
			slog.Info("runtime shutdown requested")
			return db.Close()
		},
	}, nil
}
