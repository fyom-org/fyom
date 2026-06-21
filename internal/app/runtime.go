package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"

	"github.com/fyom/fyom/internal/config"
	"github.com/fyom/fyom/internal/repository"
	"github.com/fyom/fyom/internal/server"
	"github.com/fyom/fyom/internal/service"
	"github.com/fyom/fyom/internal/version"
)

// DesktopRuntime holds the initialized application state for desktop mode.
// It exposes Startup/Shutdown lifecycle hooks for the Wails desktop shell.
type DesktopRuntime struct {
	cfg    *config.Config
	db     *repository.DB
	router *server.Server
	logger *slog.Logger
}

// NewDesktopRuntime initializes backend services: config, database, router.
// It does not start any HTTP listener. Call Startup after creation.
func NewDesktopRuntime(ctx context.Context, dbPath string) (*DesktopRuntime, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context canceled before init: %w", err)
	}

	cfg, err := config.Load("")
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	// Resolve database path.
	if dbPath == "" {
		dbPath = cfg.Database.DBPath
	}
	if dbPath == "" {
		exePath, _ := os.Executable()
		dbPath = filepath.Join(filepath.Dir(exePath), "fyom.db")
	}

	slog.Info("database path resolved", "path", dbPath, "mode", "desktop")

	if dir := filepath.Dir(dbPath); dir != "." && dir != "/" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create db parent dir %s: %w", dir, err)
		}
	}

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context canceled before database open: %w", err)
	}

	db, err := repository.Open(
		dbPath,
		cfg.Database.MaxOpenConns,
		cfg.Database.MaxIdleConns,
		cfg.Database.ConnMaxLifetime,
	)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	srv := server.New(
		cfg,
		log,
		db,
		version.Version,
		version.Commit,
		version.BuildTime,
		runtime.Version(),
		service.BootstrapModeDesktop,
	)

	return &DesktopRuntime{
		cfg:    cfg,
		db:     db,
		router: srv,
		logger: log,
	}, nil
}

// Startup performs post-initialization steps: runs bootstrap admin creation
// and logs startup diagnostics. It is safe to call after NewDesktopRuntime
// succeeds.
func (r *DesktopRuntime) Startup(ctx context.Context) error {
	if r == nil {
		return fmt.Errorf("desktop runtime is nil")
	}

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context canceled before startup: %w", err)
	}

	r.logger.Info("desktop runtime starting",
		"version", version.Version,
		"commit", version.Commit,
		"db_path", r.cfg.Database.DBPath,
	)

	// Run bootstrap: auto-create initial admin if zero users.
	bootstrapResult, err := r.router.RunBootstrap(ctx, service.BootstrapModeDesktop)
	if err != nil {
		r.logger.Error("bootstrap failed", "error", err)
		return fmt.Errorf("bootstrap: %w", err)
	}

	if bootstrapResult.Created {
		r.logger.Info("desktop first run — auto-created admin",
			"username", bootstrapResult.Username,
			"user_id", bootstrapResult.UserID,
		)
	}

	r.logger.Info("desktop runtime started")
	return nil
}

// Shutdown gracefully releases resources. It is safe to call multiple times.
// Shutdown does not panic.
func (r *DesktopRuntime) Shutdown(ctx context.Context) error {
	if r == nil {
		return nil
	}

	if err := ctx.Err(); err != nil {
		r.logger.Info("shutdown requested with canceled context", "error", err)
	} else {
		r.logger.Info("desktop runtime shutting down")
	}

	var shutdownErr error

	if r.db != nil {
		if err := r.db.Close(); err != nil {
			r.logger.Error("database close error", "error", err)
			shutdownErr = err
		} else {
			r.logger.Info("database closed")
		}
	}

	return shutdownErr
}

// Router returns the Chi router for use with Wails asset server.
func (r *DesktopRuntime) Router() *server.Server {
	if r == nil {
		return nil
	}
	return r.router
}
