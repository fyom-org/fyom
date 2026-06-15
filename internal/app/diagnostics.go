package app

import (
	"log/slog"
	"path/filepath"

	"github.com/fyom/fyom/internal/config"
)

// StartupDiagnostics is the minimum runtime context emitted during startup.
type StartupDiagnostics struct {
	DataDir      string
	DBPath       string
	WebAssetMode string
	ListenAddr   string
}

// webAssetMode is the frontend asset source used by the current server startup path.
// server.New and staticFileHandler serve web.Dist, which is backed by //go:embed dist.
const webAssetMode = "embed"

func newStartupDiagnostics(dbPath string, cfg *config.Config) StartupDiagnostics {
	return StartupDiagnostics{
		DataDir:      filepath.Dir(dbPath),
		DBPath:       dbPath,
		WebAssetMode: webAssetMode,
		ListenAddr:   cfg.Server.Address(),
	}
}

func emitStartupDiagnostics(log *slog.Logger, d StartupDiagnostics) {
	log.Info("startup diagnostics",
		"data_dir", d.DataDir,
		"db_path", d.DBPath,
		"web_asset_mode", d.WebAssetMode,
		"listen_addr", d.ListenAddr,
	)
}
