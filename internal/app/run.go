package app

import (
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
func Run(opts RunOptions) error {
	// Load koanf config: defaults < file < env.
	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Apply flag overrides on top of koanf-resolved config.
	// Flag > env > default is enforced here: if the user passed a non-empty
	// flag value, it wins over whatever koanf resolved from env/default.
	if opts.DBPath != "" {
		cfg.Database.DBPath = opts.DBPath
	}
	if opts.LogLevel != "" {
		cfg.Log.Level = opts.LogLevel
	}
	if opts.LogFormat != "" {
		cfg.Log.Format = opts.LogFormat
	}

	// For sidecar mode, force host/port.
	if opts.Mode == RunModeSidecar {
		cfg.Server.Host = opts.Host
		cfg.Server.Port = opts.Port
	}

	// Initialize slog with the resolved log level.
	var level slog.Level
	switch cfg.Log.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	var logHandler slog.Handler
	if opts.Mode == RunModeSidecar {
		logHandler = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	} else {
		logHandler = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	}
	slog.SetDefault(slog.New(logHandler))

	// Resolve final DB path.
	dbPath := cfg.Database.DBPath
	dbSource := "default"
	if opts.DBPath != "" {
		dbSource = "flag"
	} else if os.Getenv("FYOM_DB_PATH") != "" {
		dbSource = "env"
	}

	// If still empty (no flag, no env, no config file), default to <binary-dir>/fyom.db.
	if dbPath == "" {
		exePath, exeErr := os.Executable()
		if exeErr != nil {
			return fmt.Errorf("get executable path: %w", exeErr)
		}
		dbPath = filepath.Join(filepath.Dir(exePath), "fyom.db")
		dbSource = "default-binary-dir"
	}

	// Ensure parent directory exists.
	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		return fmt.Errorf("create db parent dir %s: %w", dbDir, err)
	}

	slog.Info("fyom starting",
		"mode", opts.Mode,
		"version", version.Version,
		"commit", version.Commit,
		"db_path", dbPath,
		"db_source", dbSource,
	)

	// Open database.
	db, err := repository.Open(dbPath, cfg.Database.MaxOpenConns, cfg.Database.MaxIdleConns, cfg.Database.ConnMaxLifetime)
	if err != nil {
		slog.Error("failed to open database", "error", err)
		return fmt.Errorf("open database: %w", err)
	}
	defer func() { _ = db.Close() }()

	slog.Info("database connected", "db_path", dbPath)

	// Initialize structured logger.
	log := logger.New(cfg.Log.Level, cfg.Log.Format)

	// Create and run server.
	srv := fyomserver.New(cfg, log, db, version.Version, version.Commit, version.BuildTime, runtime.Version())
	addr := cfg.Server.Address()
	slog.Info("server listening", "addr", addr)

	if opts.Mode == RunModeSidecar {
		time.Sleep(100 * time.Millisecond)
		fmt.Fprintf(os.Stdout, "FYOM_READY http://%s\n", addr)
	}

	return srv.Run()
}
