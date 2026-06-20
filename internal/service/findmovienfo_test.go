package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testNFOFixtureDir is the repository-local fixture directory for findMovieNFO tests.
var testNFOFixtureDir = filepath.Join("testdata", "nfo_prefer_movie")

// TestFindMovieNFO_PrefersMovieNFO verifies that findMovieNFO selects movie.nfo
// over per-episode NFO files. Uses repository-local fixtures by default.
func TestFindMovieNFO_PrefersMovieNFO(t *testing.T) {
	dir := testNFOFixtureDir

	imp := NewImporter(NewLocalImportFS(), "test", nil, nil, nil)

	nfoPath := imp.findMovieNFO(context.Background(), dir)
	if nfoPath == "" {
		t.Fatal("findMovieNFO returned empty, expected movie.nfo")
	}

	base := filepath.Base(nfoPath)
	if !strings.EqualFold(base, "movie.nfo") {
		t.Errorf("findMovieNFO selected %q, expected movie.nfo", base)
	}

	// Verify the parsed title is correct
	f, err := os.Open(nfoPath)
	if err != nil {
		t.Fatalf("failed to open NFO: %v", err)
	}
	defer func() { _ = f.Close() }()

	movieNFO, err := ParseMovieNFO(f)
	if err != nil {
		t.Fatalf("failed to parse NFO: %v", err)
	}

	if movieNFO.Title != "秒速5厘米" {
		t.Errorf("title = %q, want %q", movieNFO.Title, "秒速5厘米")
	}

	// Verify the Haruhana NFO has a different (wrong) title
	haruPath := filepath.Join(dir, "56eS6YCfNeWOmOexsyAoMjAwNyk - [1080p.BDRip-Haruhana].nfo")
	f2, err := os.Open(haruPath)
	if err != nil {
		t.Fatalf("failed to open Haruhana NFO: %v", err)
	}
	defer func() { _ = f2.Close() }()
	haruNFO, _ := ParseMovieNFO(f2)
	if haruNFO.Title == movieNFO.Title {
		t.Error("Haruhana NFO should have a different title")
	}
	t.Logf("movie.nfo title: %q", movieNFO.Title)
	t.Logf("Haruhana NFO title: %q (should NOT be selected)", haruNFO.Title)
}

// requireExternalMediaRoot returns the value of FYOM_TEST_MEDIA_ROOT, or skips the test.
func requireExternalMediaRoot(t *testing.T) string {
	t.Helper()
	root := os.Getenv("FYOM_TEST_MEDIA_ROOT")
	if root == "" {
		t.Skip("FYOM_TEST_MEDIA_ROOT not set; skipping optional integration test")
	}
	return root
}

// TestFindMovieNFO_PrefersMovieNFO_ExternalMedia is an optional integration test
// that runs against a real media corpus when FYOM_TEST_MEDIA_ROOT is set.
func TestFindMovieNFO_PrefersMovieNFO_ExternalMedia(t *testing.T) {
	root := requireExternalMediaRoot(t)

	// Look for a movie directory with both movie.nfo and per-file NFOs
	movieDir := filepath.Join(root, "library1", "56eS6YCfNeWOmOexsyAoMjAwNyk")
	if _, err := os.Stat(movieDir); os.IsNotExist(err) {
		t.Skipf("external media directory not found: %s", movieDir)
	}

	imp := NewImporter(NewLocalImportFS(), "test", nil, nil, nil)

	nfoPath := imp.findMovieNFO(context.Background(), movieDir)
	if nfoPath == "" {
		t.Fatal("findMovieNFO returned empty, expected movie.nfo")
	}

	base := filepath.Base(nfoPath)
	if !strings.EqualFold(base, "movie.nfo") {
		t.Errorf("findMovieNFO selected %q, expected movie.nfo", base)
	}

	f, err := os.Open(nfoPath)
	if err != nil {
		t.Fatalf("failed to open NFO: %v", err)
	}
	defer func() { _ = f.Close() }()

	movieNFO, err := ParseMovieNFO(f)
	if err != nil {
		t.Fatalf("failed to parse NFO: %v", err)
	}

	if movieNFO.Title != "秒速5厘米" {
		t.Errorf("title = %q, want %q", movieNFO.Title, "秒速5厘米")
	}
	t.Logf("external media movie.nfo title: %q", movieNFO.Title)
}
