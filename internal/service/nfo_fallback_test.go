package service

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fyom/fyom/internal/repository"
)

// ── Additional NFO fixtures for fallback tests ─────────────────────────

const movieNFOFixtureEmptyTitle = `<?xml version="1.0" encoding="UTF-8"?>
<movie>
    <title></title>
    <year>2020</year>
    <plot>A movie with no title in NFO.</plot>
    <genre>Action</genre>
</movie>`

const movieNFOFixtureNewAndOldID = `<?xml version="1.0" encoding="UTF-8"?>
<movie>
    <title>New Format Wins</title>
    <year>2021</year>
    <uniqueid type="imdb">tt0000001</uniqueid>
    <imdb_id>tt9999999</imdb_id>
</movie>`

const movieNFOFixtureOldFormatOnly = `<?xml version="1.0" encoding="UTF-8"?>
<movie>
    <title>Old Format Only</title>
    <year>2021</year>
    <tvdbid>413522</tvdbid>
</movie>`

// ── Test: findMovieNFO prefers movie.nfo ──────────────────────────────

func TestNFOFallback_PrefersMovieNFO(t *testing.T) {
	dir := t.TempDir()
	writeFileHelper(t, filepath.Join(dir, "movie.nfo"), movieNFOFixture)
	writeFileHelper(t, filepath.Join(dir, "MovieFile-S01E01.nfo"), episodeNFOFixture)
	writeFileHelper(t, filepath.Join(dir, "MovieFile.mkv"), "")

	imp := NewImporter(NewLocalImportFS(), "test", nil, nil, nil)
	path := imp.findMovieNFO(context.Background(), dir)
	if path == "" {
		t.Fatal("findMovieNFO returned empty, expected movie.nfo")
	}
	if filepath.Base(path) != "movie.nfo" {
		t.Errorf("expected movie.nfo, got %s", filepath.Base(path))
	}
}

// ── Test: findMovieNFO falls back to per-file NFO ─────────────────────

func TestNFOFallback_FallsBackToPerFileNFO(t *testing.T) {
	dir := t.TempDir()
	writeFileHelper(t, filepath.Join(dir, "MyMovie.nfo"), movieNFOFixture)
	writeFileHelper(t, filepath.Join(dir, "MyMovie.mkv"), "")

	imp := NewImporter(NewLocalImportFS(), "test", nil, nil, nil)
	path := imp.findMovieNFO(context.Background(), dir)
	if path == "" {
		t.Fatal("findMovieNFO returned empty, expected MyMovie.nfo")
	}
	if filepath.Base(path) != "MyMovie.nfo" {
		t.Errorf("expected MyMovie.nfo, got %s", filepath.Base(path))
	}
}

// ── Test: missing title falls back to filename ─────────────────────────

func TestNFOFallback_MissingTitle_FallsBackToFilename(t *testing.T) {
	dir := t.TempDir()
	writeFileHelper(t, filepath.Join(dir, "My Movie (2020)", "movie.nfo"), movieNFOFixtureEmptyTitle)
	writeFileHelper(t, filepath.Join(dir, "My Movie (2020)", "movie.mkv"), "")

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
	waitForJob(t, ctx, jobRepo, job.ID, 5*time.Second)

	jobFinal, _ := jobRepo.Get(ctx, job.ID)
	if jobFinal.Status == "error" {
		t.Fatalf("import ended in error: %s", jobFinal.ErrorMsg)
	}

	var title string
	err = db.QueryRow(`SELECT title FROM media_items WHERE library_id = ? AND type = 'movie'`, lib.ID).Scan(&title)
	if err != nil {
		t.Fatalf("failed to query movie title: %v", err)
	}
	// The title must NOT be empty — the safety guard should prevent
	// an empty <title></title> from overwriting the filename-derived title.
	if title == "" {
		t.Error("title should not be empty — should fall back to NFO filename or directory name")
	}
	// The NFO file is named "movie.nfo", so title = trimExt("movie.nfo") = "movie"
	// This is the filename-derived fallback.
}

// ── Test: new-format uniqueid wins over old-format ─────────────────────

func TestNFOFallback_OldAndNewFormatIDs_NewFormatWins(t *testing.T) {
	dir := t.TempDir()
	writeFileHelper(t, filepath.Join(dir, "IDTest (2021)", "movie.nfo"), movieNFOFixtureNewAndOldID)
	writeFileHelper(t, filepath.Join(dir, "IDTest (2021)", "movie.mkv"), "")

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
	waitForJob(t, ctx, jobRepo, job.ID, 5*time.Second)

	var uniqueIDsJSON string
	err = db.QueryRow(`SELECT unique_ids FROM media_items WHERE library_id = ? AND type = 'movie'`, lib.ID).Scan(&uniqueIDsJSON)
	if err != nil {
		t.Fatalf("failed to query unique_ids: %v", err)
	}

	if !strings.Contains(uniqueIDsJSON, "tt0000001") {
		t.Errorf("expected unique_ids to contain tt0000001 (new format), got %s", uniqueIDsJSON)
	}
	if strings.Contains(uniqueIDsJSON, "tt9999999") {
		t.Errorf("unique_ids should NOT contain tt9999999 (old format), got %s", uniqueIDsJSON)
	}
}

// ── Test: old-format ID fills gap when new-format absent ──────────────

func TestNFOFallback_OldFormatID_FillsGapWhenNewFormatAbsent(t *testing.T) {
	dir := t.TempDir()
	writeFileHelper(t, filepath.Join(dir, "OldID (2021)", "movie.nfo"), movieNFOFixtureOldFormatOnly)
	writeFileHelper(t, filepath.Join(dir, "OldID (2021)", "movie.mkv"), "")

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
	waitForJob(t, ctx, jobRepo, job.ID, 5*time.Second)

	var uniqueIDsJSON string
	err = db.QueryRow(`SELECT unique_ids FROM media_items WHERE library_id = ? AND type = 'movie'`, lib.ID).Scan(&uniqueIDsJSON)
	if err != nil {
		t.Fatalf("failed to query unique_ids: %v", err)
	}

	if !strings.Contains(uniqueIDsJSON, `"tvdb"`) {
		t.Errorf("expected unique_ids to contain tvdb type, got %s", uniqueIDsJSON)
	}
	if !strings.Contains(uniqueIDsJSON, `"413522"`) {
		t.Errorf("expected unique_ids to contain 413522 value, got %s", uniqueIDsJSON)
	}
}
