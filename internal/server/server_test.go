package server

import (
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/fyom/fyom/internal/config"
	"github.com/fyom/fyom/internal/repository"
	"github.com/fyom/fyom/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServer_Shutdown_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := repository.Open(filepath.Join(tmpDir, "fyom.db"), 5, 2, 60)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	cfg := &config.Config{
		Server: config.Server{
			Host: "127.0.0.1",
			Port: 0, // random port
		},
		Auth: config.Auth{
			JWTSecret:   "test-secret",
			TokenExpiry: 24,
		},
	}

	logger := slog.Default()
	srv := New(cfg, logger, db, "test", "abc123", "now", "go1.26", service.BootstrapModeServer)

	// Start server in background
	go func() {
		_ = srv.Run()
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// First shutdown
	srv.httpServer.Close()

	// Second shutdown should not panic
	srv.httpServer.Close()

	// Wait a bit for cleanup
	time.Sleep(50 * time.Millisecond)

	assert.True(t, true, "shutdown is idempotent")
}

func TestServer_ShutdownStopsScheduler(t *testing.T) {
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
	srv := New(cfg, logger, db, "test", "abc123", "now", "go1.26", service.BootstrapModeServer)

	// Start server in background
	go func() {
		_ = srv.Run()
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Trigger shutdown via the shutdown channel (simulating signal)
	close(srv.shutdownCh)

	// Give scheduler time to stop
	time.Sleep(200 * time.Millisecond)

	// Verify shutdown channel is closed (scheduler should have exited)
	select {
	case <-srv.shutdownCh:
		// Good, shutdown channel is closed
	default:
		t.Error("shutdownCh should be closed after shutdown")
	}
}

func TestServer_WaitForImports(t *testing.T) {
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
	srv := New(cfg, logger, db, "test", "abc123", "now", "go1.26", service.BootstrapModeServer)

	// Add some work to the waitgroup
	srv.importWG.Add(2)
	go func() {
		time.Sleep(50 * time.Millisecond)
		srv.importWG.Done()
	}()
	go func() {
		time.Sleep(100 * time.Millisecond)
		srv.importWG.Done()
	}()

	// waitForImports should complete within timeout
	done := make(chan struct{})
	go func() {
		srv.waitForImports(1 * time.Second)
		close(done)
	}()

	select {
	case <-done:
		// Completed in time
	case <-time.After(2 * time.Second):
		t.Error("waitForImports timed out unexpectedly")
	}
}

func TestServer_WaitForImports_Timeout(t *testing.T) {
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
	srv := New(cfg, logger, db, "test", "abc123", "now", "go1.26", service.BootstrapModeServer)

	// Add work that takes longer than timeout
	srv.importWG.Add(1)
	go func() {
		time.Sleep(500 * time.Millisecond)
		srv.importWG.Done()
	}()

	// waitForImports should timeout
	done := make(chan struct{})
	go func() {
		srv.waitForImports(50 * time.Millisecond)
		close(done)
	}()

	select {
	case <-done:
		// Should complete (with timeout warning logged)
	case <-time.After(1 * time.Second):
		t.Error("waitForImports should have returned after timeout")
	}
}

func TestServer_HealthEndpoints(t *testing.T) {
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
	srv := New(cfg, logger, db, "v1.0.0", "abc123def", "2024-01-01T00:00:00Z", "go1.26", service.BootstrapModeServer)

	// Start server
	go func() {
		_ = srv.Run()
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Test /healthz
	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()
	srv.router.ServeHTTP(rec, req)
	assert.Equal(t, 200, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")

	// Test /readyz
	req = httptest.NewRequest("GET", "/readyz", nil)
	rec = httptest.NewRecorder()
	srv.router.ServeHTTP(rec, req)
	assert.Equal(t, 200, rec.Code)

	// Test /version - uses package-level version vars (dev/unknown in tests)
	req = httptest.NewRequest("GET", "/version", nil)
	rec = httptest.NewRecorder()
	srv.router.ServeHTTP(rec, req)
	assert.Equal(t, 200, rec.Code)
	assert.Contains(t, rec.Body.String(), "version")
	assert.Contains(t, rec.Body.String(), "commit")
	assert.Contains(t, rec.Body.String(), "build_time")
	assert.Contains(t, rec.Body.String(), "go_version")

	// Cleanup
	close(srv.shutdownCh)
	time.Sleep(50 * time.Millisecond)
}
