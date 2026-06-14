package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/fyom/fyom/internal/model"
	"github.com/fyom/fyom/internal/repository"
)

// TestImporter_SourceRootWithOneExtraGroupingDirectory_StillFindsNestedMedia
// verifies that the importer correctly traverses one level of grouping directory
// to find nested media libraries. This is a regression test for the case where
// source path = /root/media (grouping) fails to discover media under /root/media/library1.
func TestImporter_SourceRootWithOneExtraGroupingDirectory_StillFindsNestedMedia(t *testing.T) {
	dir := t.TempDir()

	db, err := repository.Open(filepath.Join(dir, "fyom.db"), 5, 2, 60)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Create a library whose source path is the grouping directory (one level above the actual media)
	groupingDir := filepath.Join(dir, "media")
	lib := &model.Library{
		Name:           "Grouped Library",
		Type:           "mixed",
		SourcePath:     groupingDir,
		ProviderID:     "local",
		MetadataSource: "nfo",
	}
	libRepo := repository.NewLibraryRepository(db)
	if err := libRepo.Create(context.Background(), lib); err != nil {
		t.Fatal(err)
	}

	// Create nested media structure under the grouping directory
	library1Dir := filepath.Join(groupingDir, "library1")
	movieDir := filepath.Join(library1Dir, "movie-a")
	showDir := filepath.Join(library1Dir, "show-b")
	seasonDir := filepath.Join(showDir, "Season 01")

	os.MkdirAll(movieDir, 0755)
	os.MkdirAll(seasonDir, 0755)

	// Movie with NFO
	os.WriteFile(filepath.Join(movieDir, "movie.nfo"), []byte(`<?xml version="1.0" encoding="UTF-8"?>
<movie>
    <title>Movie A</title>
    <year>2020</year>
    <plot>A test movie.</plot>
    <rating>7.5</rating>
</movie>`), 0644)
	os.WriteFile(filepath.Join(movieDir, "movie-a.mkv"), []byte(""), 0644)

	// Show with NFO and one episode
	os.WriteFile(filepath.Join(showDir, "tvshow.nfo"), []byte(`<?xml version="1.0" encoding="UTF-8"?>
<tvshow>
    <title>Show B</title>
    <year>2022</year>
    <plot>A test show.</plot>
    <rating>8.0</rating>
</tvshow>`), 0644)
	os.WriteFile(filepath.Join(seasonDir, "show-b - S01E01.nfo"), []byte(`<?xml version="1.0" encoding="UTF-8"?>
<episodedetails>
    <title>Episode One</title>
    <season>1</season>
    <episode>1</episode>
    <plot>First episode.</plot>
    <rating>7.0</rating>
</episodedetails>`), 0644)
	os.WriteFile(filepath.Join(seasonDir, "show-b - S01E01.mkv"), []byte(""), 0644)

	mediaRepo := repository.NewMediaRepository(db)
	jobRepo := repository.NewImportJobRepository(db)
	importer := NewImporter(NewLocalImportFS(), "local", db, mediaRepo, jobRepo)

	ctx := context.Background()
	summary, err := importer.ImportLibrary(ctx, lib.ID)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Import summary: scanned=%d imported=%d updated=%d warnings=%d",
		summary.ScannedFiles, summary.ImportedItems, summary.UpdatedItems, len(summary.ParseWarnings))

	// Must discover nested media under the grouping directory
	var movieCount int
	db.QueryRow("SELECT COUNT(*) FROM media_items WHERE library_id = ? AND type = 'movie'", lib.ID).Scan(&movieCount)
	if movieCount != 1 {
		t.Errorf("expected 1 movie, got %d", movieCount)
	}

	var showCount int
	db.QueryRow("SELECT COUNT(*) FROM media_items WHERE library_id = ? AND type = 'show'", lib.ID).Scan(&showCount)
	if showCount != 1 {
		t.Errorf("expected 1 show, got %d", showCount)
	}

	var episodeCount int
	db.QueryRow("SELECT COUNT(*) FROM media_items WHERE library_id = ? AND type = 'episode'", lib.ID).Scan(&episodeCount)
	if episodeCount != 1 {
		t.Errorf("expected 1 episode, got %d", episodeCount)
	}

	// No duplicate movie from episode file
	var dupMovieCount int
	db.QueryRow("SELECT COUNT(*) FROM media_items WHERE library_id = ? AND type = 'movie' AND title LIKE '%S01E01%'", lib.ID).Scan(&dupMovieCount)
	if dupMovieCount != 0 {
		t.Errorf("expected 0 duplicate movies from episode files, got %d", dupMovieCount)
	}
}
