package desktop

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLoadConfig_MissingFile(t *testing.T) {
	// A non-existent path should return default config without error.
	cfg, err := LoadConfig("/nonexistent/path/fyom-desktop.json")
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if cfg.Player.Command != "" {
		t.Errorf("expected empty command, got %q", cfg.Player.Command)
	}
}

func TestLoadConfig_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "bad.json")
	if err := os.WriteFile(path, []byte("{invalid json"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "parse desktop config") {
		t.Errorf("expected parse error, got: %v", err)
	}
}

func TestLoadConfig_ValidFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "fyom-desktop.json")

	content := `{
		"player": {
			"command": "mpv",
			"args": ["--resume-playback"],
			"allowedRoots": ["/media", "/mnt/library"]
		}
	}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Player.Command != "mpv" {
		t.Errorf("expected command mpv, got %q", cfg.Player.Command)
	}
	if len(cfg.Player.Args) != 1 || cfg.Player.Args[0] != "--resume-playback" {
		t.Errorf("unexpected args: %v", cfg.Player.Args)
	}
	if len(cfg.Player.AllowedRoots) != 2 {
		t.Fatalf("expected 2 allowed roots, got %d", len(cfg.Player.AllowedRoots))
	}
	if cfg.Player.AllowedRoots[0] != "/media" {
		t.Errorf("expected /media, got %q", cfg.Player.AllowedRoots[0])
	}
}

func TestLoadConfig_RelativePath(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "fyom-desktop.json")

	content := `{
		"player": {
			"command": "vlc",
			"allowedRoots": ["relative/path"]
		}
	}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Relative path should be converted to absolute.
	if !filepath.IsAbs(cfg.Player.AllowedRoots[0]) {
		t.Errorf("expected absolute path, got %q", cfg.Player.AllowedRoots[0])
	}
	if !strings.HasSuffix(cfg.Player.AllowedRoots[0], "relative/path") {
		t.Errorf("expected path ending with relative/path, got %q", cfg.Player.AllowedRoots[0])
	}
}

func TestApplyDesktopEnvOverrides_Command(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Player.Command = "mpv"

	result := ApplyDesktopEnvOverrides(cfg, func(key string) (string, bool) {
		if key == "FYOM_EXTERNAL_PLAYER" {
			return "vlc", true
		}
		return "", false
	})

	if result.Player.Command != "vlc" {
		t.Errorf("expected vlc, got %q", result.Player.Command)
	}
}

func TestApplyDesktopEnvOverrides_EmptyEnvDoesNotOverride(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Player.Command = "mpv"

	result := ApplyDesktopEnvOverrides(cfg, func(key string) (string, bool) {
		if key == "FYOM_EXTERNAL_PLAYER" {
			return "", true // empty value present
		}
		return "", false
	})

	if result.Player.Command != "mpv" {
		t.Errorf("expected mpv (not overridden by empty env), got %q", result.Player.Command)
	}
}

func TestApplyDesktopEnvOverrides_PlayerArgs(t *testing.T) {
	cfg := DefaultConfig()

	result := ApplyDesktopEnvOverrides(cfg, func(key string) (string, bool) {
		if key == "FYOM_EXTERNAL_PLAYER_ARGS" {
			return "--fullscreen, --loop", true
		}
		return "", false
	})

	if len(result.Player.Args) != 2 {
		t.Fatalf("expected 2 args, got %d: %v", len(result.Player.Args), result.Player.Args)
	}
	if result.Player.Args[0] != "--fullscreen" {
		t.Errorf("expected --fullscreen, got %q", result.Player.Args[0])
	}
	if result.Player.Args[1] != "--loop" {
		t.Errorf("expected --loop, got %q", result.Player.Args[1])
	}
}

func TestApplyDesktopEnvOverrides_AllowedRoots(t *testing.T) {
	cfg := DefaultConfig()

	sep := string(os.PathListSeparator)
	input := "/media" + sep + "/mnt/library"

	result := ApplyDesktopEnvOverrides(cfg, func(key string) (string, bool) {
		if key == "FYOM_ALLOWED_ROOTS" {
			return input, true
		}
		return "", false
	})

	if len(result.Player.AllowedRoots) != 2 {
		t.Fatalf("expected 2 roots, got %d: %v", len(result.Player.AllowedRoots), result.Player.AllowedRoots)
	}
	if result.Player.AllowedRoots[0] != "/media" {
		t.Errorf("expected /media, got %q", result.Player.AllowedRoots[0])
	}
	if result.Player.AllowedRoots[1] != "/mnt/library" {
		t.Errorf("expected /mnt/library, got %q", result.Player.AllowedRoots[1])
	}
}

func TestApplyDesktopEnvOverrides_RelativeAllowedRoot(t *testing.T) {
	cfg := DefaultConfig()

	result := ApplyDesktopEnvOverrides(cfg, func(key string) (string, bool) {
		if key == "FYOM_ALLOWED_ROOTS" {
			return "relative/path", true
		}
		return "", false
	})

	if len(result.Player.AllowedRoots) != 1 {
		t.Fatalf("expected 1 root, got %d", len(result.Player.AllowedRoots))
	}
	if !filepath.IsAbs(result.Player.AllowedRoots[0]) {
		t.Errorf("expected absolute path, got %q", result.Player.AllowedRoots[0])
	}
}

func TestPlatformConfigPath(t *testing.T) {
	path := PlatformConfigPath()

	switch runtime.GOOS {
	case "windows":
		if !strings.Contains(path, "fyom-desktop.json") {
			t.Errorf("expected fyom-desktop.json in path, got %q", path)
		}
	case "darwin":
		if !strings.Contains(path, "Library") || !strings.Contains(path, "fyom-desktop.json") {
			t.Errorf("expected macOS path with Library, got %q", path)
		}
	default:
		if !strings.Contains(path, ".config") || !strings.Contains(path, "fyom-desktop.json") {
			t.Errorf("expected linux path with .config, got %q", path)
		}
	}
}

func TestParseCommaList(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"a,b,c", 3},
		{"a, b, c", 3},
		{"single", 1},
		{"", 0},
		{"a,,b", 2},
		{"  ", 0},
	}

	for _, tt := range tests {
		result := parseCommaList(tt.input)
		if len(result) != tt.want {
			t.Errorf("parseCommaList(%q) = %d elements, want %d", tt.input, len(result), tt.want)
		}
	}
}
