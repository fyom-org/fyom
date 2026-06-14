package service

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/fyom/fyom/internal/model"
	"github.com/fyom/fyom/internal/repository"
)

// helper to create a minimal movie NFO
func movieNFO(title string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<movie>
    <title>` + title + `</title>
    <year>2020</year>
    <plot>A test movie.</plot>
    <rating>7.5</rating>
</movie>`
}

// helper to create a minimal show NFO
func showNFO(title string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<tvshow>
    <title>` + title + `</title>
    <year>2022</year>
    <plot>A test show.</plot>
    <rating>8.0</rating>
</tvshow>`
}

// helper to create a minimal episode NFO
func episodeNFO(title string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<episodedetails>
    <title>` + title + `</title>
    <season>1</season>
    <episode>1</episode>
    <plot>First episode.</plot>
    <rating>7.0</rating>
</episodedetails>`
}

// runImportTest is a helper that creates a library, runs import, and returns the DB for assertions.
func runImportTest(t *testing.T, sourcePath string, libType string) (*repository.DB, *model.Library) {
	t.Helper()
	dir := filepath.Dir(sourcePath)
	db, err := repository.Open(filepath.Join(dir, "fyom.db"), 5, 2, 60)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	lib := &model.Library{
		Name:           "Test Library",
		Type:           libType,
		SourcePath:     sourcePath,
		ProviderID:     "local",
		MetadataSource: "nfo",
	}
	libRepo := repository.NewLibraryRepository(db)
	if err := libRepo.Create(context.Background(), lib); err != nil {
		t.Fatal(err)
	}

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

	return db, lib
}

// countMediaItems counts media items of a given type for the library.
func countMediaItems(t *testing.T, db *repository.DB, libID string, mediaType string) int {
	t.Helper()
	var count int
	err := db.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM media_items WHERE library_id = ? AND type = ?", libID, mediaType).Scan(&count)
	if err != nil && err != sql.ErrNoRows {
		t.Fatal(err)
	}
	return count
}

// ── Test 1: one extra grouping directory ──────────────────────────────────

// TestImporter_SourceRootWithOneExtraGroupingDirectory_StillFindsNestedMedia
// verifies that the importer correctly traverses one level of grouping directory
// to find nested media libraries.
func TestImporter_SourceRootWithOneExtraGroupingDirectory_StillFindsNestedMedia(t *testing.T) {
	dir := t.TempDir()
	groupingDir := filepath.Join(dir, "media")
	library1Dir := filepath.Join(groupingDir, "library1")
	movieDir := filepath.Join(library1Dir, "movie-a")
	showDir := filepath.Join(library1Dir, "show-b")
	seasonDir := filepath.Join(showDir, "Season 01")

	os.MkdirAll(movieDir, 0755)
	os.MkdirAll(seasonDir, 0755)

	os.WriteFile(filepath.Join(movieDir, "movie.nfo"), []byte(movieNFO("Movie A")), 0644)
	os.WriteFile(filepath.Join(movieDir, "movie-a.mkv"), []byte(""), 0644)
	os.WriteFile(filepath.Join(showDir, "tvshow.nfo"), []byte(showNFO("Show B")), 0644)
	os.WriteFile(filepath.Join(seasonDir, "show-b - S01E01.nfo"), []byte(episodeNFO("Episode One")), 0644)
	os.WriteFile(filepath.Join(seasonDir, "show-b - S01E01.mkv"), []byte(""), 0644)

	db, lib := runImportTest(t, groupingDir, "mixed")

	if got := countMediaItems(t, db, lib.ID, "movie"); got != 1 {
		t.Errorf("expected 1 movie, got %d", got)
	}
	if got := countMediaItems(t, db, lib.ID, "show"); got != 1 {
		t.Errorf("expected 1 show, got %d", got)
	}
	if got := countMediaItems(t, db, lib.ID, "episode"); got != 1 {
		t.Errorf("expected 1 episode, got %d", got)
	}
	if got := countMediaItems(t, db, lib.ID, "movie"); got != 1 {
		// verify no duplicate from episode file
		var dup int
		db.QueryRow("SELECT COUNT(*) FROM media_items WHERE library_id = ? AND type = 'movie' AND title LIKE '%S01E01%'", lib.ID).Scan(&dup)
		if dup != 0 {
			t.Errorf("expected 0 duplicate movies from episode files, got %d", dup)
		}
	}
}

// ── Test 2: multiple nested grouping directories ──────────────────────────

// TestImporter_SourceRootWithMultipleGroupingDirectories_StillFindsNestedMedia
// verifies that the importer correctly traverses multiple levels of grouping
// directories to find nested media.
func TestImporter_SourceRootWithMultipleGroupingDirectories_StillFindsNestedMedia(t *testing.T) {
	dir := t.TempDir()
	groupingDir := filepath.Join(dir, "media", "group-a", "group-b", "library1")
	movieDir := filepath.Join(groupingDir, "movie-a")
	showDir := filepath.Join(groupingDir, "show-b")
	seasonDir := filepath.Join(showDir, "Season 01")

	os.MkdirAll(movieDir, 0755)
	os.MkdirAll(seasonDir, 0755)

	os.WriteFile(filepath.Join(movieDir, "movie.nfo"), []byte(movieNFO("Movie A")), 0644)
	os.WriteFile(filepath.Join(movieDir, "movie-a.mkv"), []byte(""), 0644)
	os.WriteFile(filepath.Join(showDir, "tvshow.nfo"), []byte(showNFO("Show B")), 0644)
	os.WriteFile(filepath.Join(seasonDir, "show-b - S01E01.nfo"), []byte(episodeNFO("Episode One")), 0644)
	os.WriteFile(filepath.Join(seasonDir, "show-b - S01E01.mkv"), []byte(""), 0644)

	db, lib := runImportTest(t, filepath.Join(dir, "media"), "mixed")

	if got := countMediaItems(t, db, lib.ID, "movie"); got != 1 {
		t.Errorf("expected 1 movie, got %d", got)
	}
	if got := countMediaItems(t, db, lib.ID, "show"); got != 1 {
		t.Errorf("expected 1 show, got %d", got)
	}
	if got := countMediaItems(t, db, lib.ID, "episode"); got != 1 {
		t.Errorf("expected 1 episode, got %d", got)
	}

	// No duplicate movie from episode file
	var dup int
	db.QueryRow("SELECT COUNT(*) FROM media_items WHERE library_id = ? AND type = 'movie' AND title LIKE '%S01E01%'", lib.ID).Scan(&dup)
	if dup != 0 {
		t.Errorf("expected 0 duplicate movies from episode files, got %d", dup)
	}
}

// ── Test 3: grouping directory does not become media item ──────────────────

// TestImporter_GroupingDirectory_DoesNotBecomeMediaItem
// verifies that a wrapper/grouping directory is not persisted as a media item.
func TestImporter_GroupingDirectory_DoesNotBecomeMediaItem(t *testing.T) {
	dir := t.TempDir()
	wrapperDir := filepath.Join(dir, "wrapper")
	movieDir := filepath.Join(wrapperDir, "movie-a")

	os.MkdirAll(movieDir, 0755)

	os.WriteFile(filepath.Join(movieDir, "movie.nfo"), []byte(movieNFO("Movie A")), 0644)
	os.WriteFile(filepath.Join(movieDir, "movie-a.mkv"), []byte(""), 0644)

	db, lib := runImportTest(t, wrapperDir, "mixed")

	// Only the actual movie entity should be imported
	if got := countMediaItems(t, db, lib.ID, "movie"); got != 1 {
		t.Errorf("expected 1 movie, got %d", got)
	}
	if got := countMediaItems(t, db, lib.ID, "show"); got != 0 {
		t.Errorf("expected 0 shows, got %d", got)
	}
	if got := countMediaItems(t, db, lib.ID, "episode"); got != 0 {
		t.Errorf("expected 0 episodes, got %d", got)
	}

	// Total media items should be exactly 1 (just the movie)
	var total int
	db.QueryRow("SELECT COUNT(*) FROM media_items WHERE library_id = ?", lib.ID).Scan(&total)
	if total != 1 {
		t.Errorf("expected exactly 1 media item, got %d", total)
	}
}

// ── Test 4: show-only library with grouping directory ──────────────────────

// TestImporter_ShowOnlyLibrary_WithGroupingDirectory_ImportsShowAndRejectsMovie
// verifies that a show-only library correctly imports shows under grouping
// directories while rejecting movies.
func TestImporter_ShowOnlyLibrary_WithGroupingDirectory_ImportsShowAndRejectsMovie(t *testing.T) {
	dir := t.TempDir()
	groupDir := filepath.Join(dir, "group")
	movieDir := filepath.Join(groupDir, "movie-a")
	showDir := filepath.Join(groupDir, "show-b")
	seasonDir := filepath.Join(showDir, "Season 01")

	os.MkdirAll(movieDir, 0755)
	os.MkdirAll(seasonDir, 0755)

	os.WriteFile(filepath.Join(movieDir, "movie.nfo"), []byte(movieNFO("Movie A")), 0644)
	os.WriteFile(filepath.Join(movieDir, "movie-a.mkv"), []byte(""), 0644)
	os.WriteFile(filepath.Join(showDir, "tvshow.nfo"), []byte(showNFO("Show B")), 0644)
	os.WriteFile(filepath.Join(seasonDir, "show-b - S01E01.nfo"), []byte(episodeNFO("Episode One")), 0644)
	os.WriteFile(filepath.Join(seasonDir, "show-b - S01E01.mkv"), []byte(""), 0644)

	db, lib := runImportTest(t, groupDir, "show")

	// Show-only library should import show + episode
	if got := countMediaItems(t, db, lib.ID, "show"); got != 1 {
		t.Errorf("expected 1 show, got %d", got)
	}
	if got := countMediaItems(t, db, lib.ID, "episode"); got != 1 {
		t.Errorf("expected 1 episode, got %d", got)
	}

	// Movie should NOT be imported in show-only library
	if got := countMediaItems(t, db, lib.ID, "movie"); got != 0 {
		t.Errorf("expected 0 movies in show-only library, got %d", got)
	}
}

// ── Test 5: movie-only library with grouping directory ─────────────────────

// TestImporter_MovieOnlyLibrary_WithGroupingDirectory_ImportsMovieAndRejectsShow
// verifies that a movie-only library correctly imports movies under grouping
// directories while rejecting shows.
func TestImporter_MovieOnlyLibrary_WithGroupingDirectory_ImportsMovieAndRejectsShow(t *testing.T) {
	dir := t.TempDir()
	groupDir := filepath.Join(dir, "group")
	movieDir := filepath.Join(groupDir, "movie-a")
	showDir := filepath.Join(groupDir, "show-b")
	seasonDir := filepath.Join(showDir, "Season 01")

	os.MkdirAll(movieDir, 0755)
	os.MkdirAll(seasonDir, 0755)

	os.WriteFile(filepath.Join(movieDir, "movie.nfo"), []byte(movieNFO("Movie A")), 0644)
	os.WriteFile(filepath.Join(movieDir, "movie-a.mkv"), []byte(""), 0644)
	os.WriteFile(filepath.Join(showDir, "tvshow.nfo"), []byte(showNFO("Show B")), 0644)
	os.WriteFile(filepath.Join(seasonDir, "show-b - S01E01.nfo"), []byte(episodeNFO("Episode One")), 0644)
	os.WriteFile(filepath.Join(seasonDir, "show-b - S01E01.mkv"), []byte(""), 0644)

	db, lib := runImportTest(t, groupDir, "movie")

	// Movie-only library should import movie
	if got := countMediaItems(t, db, lib.ID, "movie"); got != 1 {
		t.Errorf("expected 1 movie, got %d", got)
	}

	// Show should NOT be imported in movie-only library
	if got := countMediaItems(t, db, lib.ID, "show"); got != 0 {
		t.Errorf("expected 0 shows in movie-only library, got %d", got)
	}
	if got := countMediaItems(t, db, lib.ID, "episode"); got != 0 {
		t.Errorf("expected 0 episodes in movie-only library, got %d", got)
	}
}
