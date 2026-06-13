package app

import (
	"fmt"
	"os"
	"path/filepath"
)

// RunMode represents the application run mode.
type RunMode string

const (
	RunModeServer  RunMode = "server"
	RunModeSidecar RunMode = "sidecar"
)

// RunOptions holds the configuration for running the application.
type RunOptions struct {
	Mode      RunMode
	Host      string
	Port      int
	DBPath    string
	LogLevel  string
	LogFormat string
}

// DefaultRunOptions returns the default run options for server mode.
func DefaultRunOptions() RunOptions {
	return RunOptions{
		Mode:      RunModeServer,
		Host:      "0.0.0.0",
		Port:      8080,
		DBPath:    "",
		LogLevel:  "info",
		LogFormat: "text",
	}
}

// SidecarRunOptions returns the run options for sidecar mode.
func SidecarRunOptions(dbPath, logLevel, logFormat string) RunOptions {
	return RunOptions{
		Mode:      RunModeSidecar,
		Host:      "127.0.0.1",
		Port:      27403,
		DBPath:    dbPath,
		LogLevel:  logLevel,
		LogFormat: logFormat,
	}
}

// ResolveDBPath resolves the final absolute database path.
// Priority: flag > env FYOM_DB_PATH > default (binary-dir/fyom.db)
// Returns: absolute path, source label, error
func ResolveDBPath(flagValue string) (string, string, error) {
	// 1. Flag
	if flagValue != "" {
		abs, err := filepath.Abs(flagValue)
		if err != nil {
			return "", "", fmt.Errorf("resolve db-path: %w", err)
		}
		return abs, "flag", nil
	}

	// 2. Env
	if envVal := os.Getenv("FYOM_DB_PATH"); envVal != "" {
		abs, err := filepath.Abs(envVal)
		if err != nil {
			return "", "", fmt.Errorf("resolve FYOM_DB_PATH: %w", err)
		}
		return abs, "env", nil
	}

	// 3. Default: <binary-dir>/fyom.db
	exePath, err := os.Executable()
	if err != nil {
		return "", "", fmt.Errorf("get executable path: %w", err)
	}
	exeDir := filepath.Dir(exePath)
	defaultPath := filepath.Join(exeDir, "fyom.db")
	return defaultPath, "default-binary-dir", nil
}
