package service

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/fyom/fyom/internal/repository"
)

// ── Additional failing fixture ─────────────────────────────────────────

const movieNFOFixture2 = `<?xml version="1.0" encoding="UTF-8"?>
<movie>
    <title>Another Good Movie</title>
    <year>2022</year>
    <plot>Another good movie.</plot>
    <genre>Comedy</genre>
    <actor>
        <name>Comic Actor</name>
        <role>Lead</role>
        <type>Actor</type>
        <sortorder>1</sortorder>
    </actor>
    <uniqueid type="imdb">tt0000004</uniqueid>
</movie>`

// ── Test: one bad NFO does not abort library import ───────────────────
//
// BUG FOUND: when a directory has a malformed NFO, findMovieNFO correctly
// rejects it (XML parse fails). But the directory then falls through to
// processDirAsMovie, which imports the video file with a filename-derived
// title. The import does NOT abort — it continues scanning all directories.
// The job ends in "done" state (not "error") because no error propagates
// to the top level.
//
// The real issue is: there's no record that the NFO file was unreadable.
// The admin has no way to know that "Bad Movie (2021)" used a filename-
// derived title instead of NFO metadata. This is what ImportSummary.ParseWarnings
// is designed to address.

func TestImport_OneBadNFO_DoesNotAbortLibrary(t *testing.T) {
	dir := t.TempDir()

	// Item 1: valid
	writeFileHelper(t, filepath.Join(dir, "Good Movie (2020)", "movie.nfo"), movieNFOFixture)
	writeFileHelper(t, filepath.Join(dir, "Good Movie (2020)", "movie.mkv"), "")

	// Item 2: malformed XML — no valid NFO, will fall through to filename-derived
	writeFileHelper(t, filepath.Join(dir, "Bad Movie (2021)", "movie.nfo"), "<movie><title>Unclosed")
	writeFileHelper(t, filepath.Join(dir, "Bad Movie (2021)", "movie.mkv"), "")

	// Item 3: valid
	writeFileHelper(t, filepath.Join(dir, "Another Good Movie (2022)", "movie.nfo"), movieNFOFixture2)
	writeFileHelper(t, filepath.Join(dir, "Another Good Movie (2022)", "movie.mkv"), "")

	db := openImporterTestDB(t)
	lib := createImporterTestLibrary(t, db, dir)
	mediaRepo := repository.NewMediaRepository(db)
	jobRepo := repository.NewImportJobRepository(db)

	importer := NewImporter(NewLocalImportFS(), "local", db, mediaRepo, jobRepo)
	importer.SetLibraryID(lib.ID)

	ctx := context.Background()
	job, err := importer.ImportRequest(ctx, dir)
	if err != nil {
		t.Fatalf("ImportRequest failed: %v", err)
	}

	// Wait for async import to complete
	waitForJob(t, jobRepo, job.ID)

	jobFinal, _ := jobRepo.Get(ctx, job.ID)

	// The library-level call must NOT return an error — the bad NFO should
	// be handled gracefully, not abort the entire scan.
	if jobFinal.Status == "error" {
		t.Fatalf("import ended in error: %s", jobFinal.ErrorMsg)
	}

	// All 3 directories produce movie items:
	// - Good Movie (2020): NFO-parsed
	// - Bad Movie (2021): filename-derived (NFO parse failed, fell through)
	// - Another Good Movie (2022): NFO-parsed
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM media_items WHERE library_id = ? AND type = 'movie'`, lib.ID).Scan(&count); err != nil {
		t.Fatalf("query count: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 imported items (2 NFO + 1 filename-derived), got %d", count)
	}

	// Verify the good items have proper NFO metadata
	var goodTitle, anotherTitle string
	if err := db.QueryRow(`SELECT title FROM media_items WHERE library_id = ? AND type = 'movie' AND title = 'Movie A'`, lib.ID).Scan(&goodTitle); err != nil {
		t.Fatalf("query good title: %v", err)
	}
	if err := db.QueryRow(`SELECT title FROM media_items WHERE library_id = ? AND type = 'movie' AND title = 'Another Good Movie'`, lib.ID).Scan(&anotherTitle); err != nil {
		t.Fatalf("query another title: %v", err)
	}

	if goodTitle != "Movie A" {
		t.Errorf("first good movie title = %q, want %q", goodTitle, "Movie A")
	}
	if anotherTitle != "Another Good Movie" {
		t.Errorf("third movie title = %q, want %q", anotherTitle, "Another Good Movie")
	}

	// The bad movie should have a filename-derived title (not empty, not NFO title)
	var badTitle string
	if err := db.QueryRow(`SELECT title FROM media_items WHERE library_id = ? AND type = 'movie' AND title NOT IN ('Movie A', 'Another Good Movie')`, lib.ID).Scan(&badTitle); err != nil {
		t.Fatalf("query bad title: %v", err)
	}
	if badTitle == "" {
		t.Error("bad movie should have a non-empty filename-derived title")
	}
	t.Logf("bad movie title (filename-derived): %q", badTitle)
}
