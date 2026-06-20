package app

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fyom/fyom/internal/config"
)

func TestRun_EmitsStartupDiagnostics(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "db", "fyom.db")
	cfg := &config.Config{
		Server: config.Server{Host: "127.0.0.1", Port: 27402},
	}

	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	emitStartupDiagnostics(log, newStartupDiagnostics(dbPath, cfg))

	out := buf.String()
	for _, want := range []string{
		`"msg":"startup diagnostics"`,
		`"data_dir"`,
		`"db_path"`,
		`"web_asset_mode"`,
		`"listen_addr"`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("startup diagnostics log missing %s: %s", want, out)
		}
	}
}

func TestStartupDiagnostics_DoNotIncludeSecrets(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "fyom-diag-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
	dbPath := filepath.Join(tmpDir, "fyom.db")
	cfg := &config.Config{
		Server: config.Server{Host: "127.0.0.1", Port: 27402},
		Auth: config.Auth{
			JWTSecret:     "dummy-jwt-secret-token",
			TokenExpiry:   24,
			RefreshExpiry: 168,
		},
	}

	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	emitStartupDiagnostics(log, newStartupDiagnostics(dbPath, cfg))

	out := strings.ToLower(buf.String())
	for _, forbidden := range []string{
		cfg.Auth.JWTSecret,
		"password",
		"token",
		"secret",
		"access_key",
		"access-key",
		"accesskey",
	} {
		if strings.Contains(out, strings.ToLower(forbidden)) {
			t.Fatalf("startup diagnostics log included sensitive value %q: %s", forbidden, buf.String())
		}
	}
}
