package server

import (
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/fyom/fyom/internal/config"
	"github.com/fyom/fyom/internal/repository"
	"github.com/fyom/fyom/internal/service"
	"github.com/stretchr/testify/require"
)

// TestServerNew_DoesNotDoubleRegisterLocalProvider verifies that creating a
// new server does not panic due to duplicate in-memory registration of the
// local provider. This was a regression after the DB seed migration added
// the local provider row to the providers table.
func TestServerNew_DoesNotDoubleRegisterLocalProvider(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := repository.Open(filepath.Join(tmpDir, "fyom.db"), 5, 2, 60)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	cfg := &config.Config{
		Server: config.Server{
			Host: "127.0.0.1",
			Port: 0,
		},
		Auth: config.Auth{
			JWTSecret:   "test-secret",
			TokenExpiry: 24,
		},
	}
	logger := slog.Default()

	// This must not panic
	require.NotPanics(t, func() {
		New(cfg, logger, db, "test", "abc123", "now", "go1.26", service.BootstrapModeServer)
	}, "server.New should not panic from duplicate local provider registration")
}
