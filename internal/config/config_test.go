package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoad_Defaults(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { require.NoError(t, os.Chdir(origDir)) }()

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	require.Equal(t, "0.0.0.0", cfg.Server.Host)
	require.Equal(t, 8080, cfg.Server.Port)
	require.Equal(t, "", cfg.Database.DBPath) // empty = use default-binary-dir
	require.Equal(t, 24, cfg.Auth.TokenExpiry)
	require.Equal(t, "info", cfg.Log.Level)
	require.Equal(t, "json", cfg.Log.Format)
}

func TestLoad_FromFile(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { require.NoError(t, os.Chdir(origDir)) }()

	cfgContent := `
server:
  port: 9090
database:
  db_path: "./test_data/foo.db"
log:
  level: "debug"
  format: "text"
`
	require.NoError(t, os.WriteFile("fyom.yaml", []byte(cfgContent), 0644))

	cfg, err := Load("")
	require.NoError(t, err)
	require.Equal(t, 9090, cfg.Server.Port)
	require.Equal(t, "./test_data/foo.db", cfg.Database.DBPath)
	require.Equal(t, "debug", cfg.Log.Level)
	require.Equal(t, "text", cfg.Log.Format)
}

func TestLoad_EnvOverride(t *testing.T) {
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	require.NoError(t, os.Chdir(tmpDir))
	defer func() { require.NoError(t, os.Chdir(origDir)) }()

	t.Setenv("FYOM_SERVER_PORT", "7777")
	t.Setenv("FYOM_LOG_LEVEL", "warn")
	t.Setenv("FYOM_DB_PATH", "/tmp/env.db")
	defer os.Unsetenv("FYOM_DB_PATH")

	cfg, err := Load("")
	require.NoError(t, err)
	require.Equal(t, 7777, cfg.Server.Port)
	require.Equal(t, "warn", cfg.Log.Level)
	require.Equal(t, "/tmp/env.db", cfg.Database.DBPath)
}

func TestLoad_ExplicitPath(t *testing.T) {
	tmpDir := t.TempDir()

	cfgPath := filepath.Join(tmpDir, "custom.yaml")
	cfgContent := "server:\n  port: 3000\n"
	require.NoError(t, os.WriteFile(cfgPath, []byte(cfgContent), 0644))

	cfg, err := Load(cfgPath)
	_ = cfg
	_ = err
}

func TestServer_Address(t *testing.T) {
	s := Server{Host: "127.0.0.1", Port: 8080}
	require.Equal(t, "127.0.0.1:8080", s.Address())
}
