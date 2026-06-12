package service

import (
	"path/filepath"
	"testing"
)

// ── Test: logo.png found ──────────────────────────────────────────────

func TestFindLogoPath_LogoPNG(t *testing.T) {
	dir := t.TempDir()
	writeFileHelper(t, filepath.Join(dir, "logo.png"), "fake-png")
	path := FindLogoPath(dir)
	if path == "" {
		t.Fatal("expected logo path, got empty")
	}
	if filepath.Base(path) != "logo.png" {
		t.Errorf("expected logo.png, got %s", filepath.Base(path))
	}
}

// ── Test: clearlogo.png fallback ──────────────────────────────────────

func TestFindLogoPath_ClearLogoFallback(t *testing.T) {
	dir := t.TempDir()
	writeFileHelper(t, filepath.Join(dir, "clearlogo.png"), "fake-png")
	// no logo.png
	path := FindLogoPath(dir)
	if path == "" {
		t.Fatal("expected clearlogo.png to be found")
	}
	if filepath.Base(path) != "clearlogo.png" {
		t.Errorf("expected clearlogo.png, got %s", filepath.Base(path))
	}
}

// ── Test: logo.png preferred over clearlogo.png ───────────────────────

func TestFindLogoPath_PrefersLogoOverClearLogo(t *testing.T) {
	dir := t.TempDir()
	writeFileHelper(t, filepath.Join(dir, "logo.png"), "fake-png-1")
	writeFileHelper(t, filepath.Join(dir, "clearlogo.png"), "fake-png-2")
	path := FindLogoPath(dir)
	if filepath.Base(path) != "logo.png" {
		t.Errorf("expected logo.png preferred, got %s", filepath.Base(path))
	}
}

// ── Test: no logo found ───────────────────────────────────────────────

func TestFindLogoPath_NoneFound(t *testing.T) {
	dir := t.TempDir()
	path := FindLogoPath(dir)
	if path != "" {
		t.Errorf("expected empty path when no logo present, got %s", path)
	}
}
