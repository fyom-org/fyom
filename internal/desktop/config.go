package desktop

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Config holds desktop-local configuration for external player
// behavior. It is intentionally separate from the Go backend fyom.yaml.
type Config struct {
	Player PlayerConfig `json:"player"`
}

// PlayerConfig holds external player launch settings.
type PlayerConfig struct {
	Command      string   `json:"command"`
	Args         []string `json:"args"`
	AllowedRoots []string `json:"allowedRoots"`
}

// DefaultConfig returns a sensible default configuration.
func DefaultConfig() Config {
	return Config{
		Player: PlayerConfig{
			Command:      "",
			Args:         nil,
			AllowedRoots: nil,
		},
	}
}

// LoadConfig loads desktop config from the given path.
// If path is empty, the platform-specific user config path is used.
// If the file does not exist, DefaultConfig is returned without error.
// If the file exists but contains invalid JSON, an error is returned.
func LoadConfig(path string) (Config, error) {
	if path == "" {
		path = PlatformConfigPath()
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return Config{}, fmt.Errorf("read desktop config %s: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse desktop config %s: %w", path, err)
	}

	// Normalize allowed roots: convert to absolute paths.
	for i, root := range cfg.Player.AllowedRoots {
		if !filepath.IsAbs(root) {
			abs, err := filepath.Abs(root)
			if err == nil {
				cfg.Player.AllowedRoots[i] = abs
			}
		}
	}

	return cfg, nil
}

// PlatformConfigPath returns the platform-specific path for
// fyom-desktop.json.
func PlatformConfigPath() string {
	switch runtime.GOOS {
	case "windows":
		if appdata := os.Getenv("APPDATA"); appdata != "" {
			return filepath.Join(appdata, "fyom", "fyom-desktop.json")
		}
		return filepath.Join("~", "AppData", "Roaming", "fyom", "fyom-desktop.json")

	case "darwin":
		if home := os.Getenv("HOME"); home != "" {
			return filepath.Join(home, "Library", "Application Support", "fyom", "fyom-desktop.json")
		}
		return filepath.Join("~", "Library", "Application Support", "fyom", "fyom-desktop.json")

	default: // linux and other unix
		configHome := os.Getenv("XDG_CONFIG_HOME")
		if configHome == "" {
			if home := os.Getenv("HOME"); home != "" {
				configHome = filepath.Join(home, ".config")
			} else {
				configHome = "~/.config"
			}
		}
		return filepath.Join(configHome, "fyom", "fyom-desktop.json")
	}
}

// ApplyDesktopEnvOverrides applies environment variable overrides to the
// desktop config. The lookup function retrieves environment variables,
// making the function deterministic and testable.
//
// Supported overrides:
//   - FYOM_EXTERNAL_PLAYER: overrides player command
//   - FYOM_EXTERNAL_PLAYER_ARGS: overrides player args (comma-separated)
//   - FYOM_ALLOWED_ROOTS: overrides allowed roots (OS path-list separator)
//
// Empty environment variables do not overwrite valid file config.
func ApplyDesktopEnvOverrides(cfg Config, lookup func(string) (string, bool)) Config {
	if v, ok := lookup("FYOM_EXTERNAL_PLAYER"); ok && v != "" {
		cfg.Player.Command = v
	}

	if v, ok := lookup("FYOM_EXTERNAL_PLAYER_ARGS"); ok && v != "" {
		// Comma-separated args. Whitespace around commas is trimmed.
		cfg.Player.Args = parseCommaList(v)
	}

	if v, ok := lookup("FYOM_ALLOWED_ROOTS"); ok && v != "" {
		// OS-specific path-list separator (":" on unix, ";" on windows).
		// Fall back to comma if the OS separator yields a single element.
		sep := string(os.PathListSeparator)
		roots := strings.Split(v, sep)
		if len(roots) <= 1 {
			roots = parseCommaList(v)
		}
		// Normalize to absolute paths.
		for i, root := range roots {
			root = strings.TrimSpace(root)
			if root == "" {
				continue
			}
			if !filepath.IsAbs(root) {
				if abs, err := filepath.Abs(root); err == nil {
					root = abs
				}
			}
			roots[i] = root
		}
		cfg.Player.AllowedRoots = roots
	}

	return cfg
}

// parseCommaList splits a comma-separated string and trims whitespace
// from each element. Empty elements are omitted.
func parseCommaList(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
