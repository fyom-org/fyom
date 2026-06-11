package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindMovieNFO_PrefersMovieNFO(t *testing.T) {
	dir := "/root/media/library1/56eS6YCfNeWOmOexsyAoMjAwNyk"

	// Use NewImporter with nil DB/repos since findMovieNFO only uses fs
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
	defer f.Close()

	movieNFO, err := ParseMovieNFO(f)
	if err != nil {
		t.Fatalf("failed to parse NFO: %v", err)
	}

	if movieNFO.Title != "秒速5厘米" {
		t.Errorf("title = %q, want %q", movieNFO.Title, "秒速5厘米")
	}

	// Verify the Haruhana NFO has a different (wrong) title
	haruPath := filepath.Join(dir, "56eS6YCfNeWOmOexsyAoMjAwNyk - [1080p.BDRip-Haruhana].nfo")
	f2, _ := os.Open(haruPath)
	defer f2.Close()
	haruNFO, _ := ParseMovieNFO(f2)
	if haruNFO.Title == movieNFO.Title {
		t.Error("Haruhana NFO should have a different title")
	}
	t.Logf("movie.nfo title: %q", movieNFO.Title)
	t.Logf("Haruhana NFO title: %q (should NOT be selected)", haruNFO.Title)
}
