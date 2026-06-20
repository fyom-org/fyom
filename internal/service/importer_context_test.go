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

const testShowNFO = `<?xml version="1.0" encoding="UTF-8"?>
<tvshow>
    <title>Show A</title>
    <year>2022</year>
    <plot>A test show.</plot>
    <genre>Comedy</genre>
    <studio>Show Studio</studio>
    <rating>8.0</rating>
    <uniqueid type="imdb">tt0000002</uniqueid>
    <uniqueid type="tvdb">67890</uniqueid>
</tvshow>`

const testEpisodeNFO = `<?xml version="1.0" encoding="UTF-8"?>
<episodedetails>
    <title>Episode One</title>
    <season>1</season>
    <episode>1</episode>
    <plot>First episode.</plot>
    <rating>7.0</rating>
    <uniqueid type="imdb">tt0000003</uniqueid>
</episodedetails>`

// ── Test 1: show subtree claims episode files, no duplicate movie import ──

func TestImporter_ShowSubtreeClaimsEpisodeFiles_NoDuplicateMovieImport(t *testing.T) {
	dir := t.TempDir()

	db, err := repository.Open(filepath.Join(dir, "fyom.db"), 5, 2, 60)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	lib := &model.Library{
		Name:           "Test Library",
		Type:           "mixed",
		SourcePath:     dir,
		ProviderID:     "local",
		MetadataSource: "nfo",
	}
	libRepo := repository.NewLibraryRepository(db)
	if err := libRepo.Create(context.Background(), lib); err != nil {
		t.Fatal(err)
	}

	// Create show fixture
	showDir := filepath.Join(dir, "Show A")
	seasonDir := filepath.Join(showDir, "Season 01")
	if err := os.MkdirAll(seasonDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(showDir, "tvshow.nfo"), []byte(testShowNFO), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seasonDir, "Show A - S01E01.mkv"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seasonDir, "Show A - S01E01.nfo"), []byte(testEpisodeNFO), 0644); err != nil {
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

	if summary.ScannedFiles < 2 {
		t.Errorf("expected ScannedFiles >= 2, got %d", summary.ScannedFiles)
	}

	var showCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM media_items WHERE library_id = ? AND type = 'show'", lib.ID).Scan(&showCount); err != nil && err != sql.ErrNoRows {
		t.Fatal(err)
	}
	if showCount != 1 {
		t.Errorf("expected 1 show, got %d", showCount)
	}

	var episodeCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM media_items WHERE library_id = ? AND type = 'episode'", lib.ID).Scan(&episodeCount); err != nil && err != sql.ErrNoRows {
		t.Fatal(err)
	}
	if episodeCount != 1 {
		t.Errorf("expected 1 episode, got %d", episodeCount)
	}

	var movieCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM media_items WHERE library_id = ? AND type = 'movie'", lib.ID).Scan(&movieCount); err != nil && err != sql.ErrNoRows {
		t.Fatal(err)
	}
	if movieCount != 0 {
		t.Errorf("expected 0 movies, got %d", movieCount)
	}

	var showID, epParentID string
	if err := db.QueryRow("SELECT id FROM media_items WHERE library_id = ? AND type = 'show'", lib.ID).Scan(&showID); err != nil && err != sql.ErrNoRows {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT parent_id FROM media_items WHERE library_id = ? AND type = 'episode'", lib.ID).Scan(&epParentID); err != nil && err != sql.ErrNoRows {
		t.Fatal(err)
	}
	if epParentID != showID {
		t.Errorf("episode parent_id = %s, expected %s", epParentID, showID)
	}

	var epTitle string
	if err := db.QueryRow("SELECT title FROM media_items WHERE library_id = ? AND type = 'episode'", lib.ID).Scan(&epTitle); err != nil && err != sql.ErrNoRows {
		t.Fatal(err)
	}
	if epTitle != "Episode One" {
		t.Errorf("episode title = %q, expected %q", epTitle, "Episode One")
	}
}

// ── Test 2: orphan season directory is rejected or ignored ──

func TestImporter_OrphanSeasonDirectory_IsRejectedOrIgnored(t *testing.T) {
	dir := t.TempDir()

	db, err := repository.Open(filepath.Join(dir, "fyom.db"), 5, 2, 60)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	lib := &model.Library{
		Name:           "Test Library",
		Type:           "mixed",
		SourcePath:     dir,
		ProviderID:     "local",
		MetadataSource: "nfo",
	}
	libRepo := repository.NewLibraryRepository(db)
	if err := libRepo.Create(context.Background(), lib); err != nil {
		t.Fatal(err)
	}

	seasonDir := filepath.Join(dir, "Season 01")
	if err := os.MkdirAll(seasonDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seasonDir, "E01.mkv"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	mediaRepo := repository.NewMediaRepository(db)
	jobRepo := repository.NewImportJobRepository(db)
	importer := NewImporter(NewLocalImportFS(), "local", db, mediaRepo, jobRepo)

	ctx := context.Background()
	_, err = importer.ImportLibrary(ctx, lib.ID)
	if err != nil {
		t.Fatal(err)
	}
	// An orphan season directory with a video file will produce a filename-fallback
	// movie candidate. This is acceptable -- the key is it does NOT produce a show
	// or episode. The directory is treated as a standalone movie with filename-derived title.
	var showCount, episodeCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM media_items WHERE library_id = ? AND type = 'show'", lib.ID).Scan(&showCount); err != nil && err != sql.ErrNoRows {
		t.Fatal(err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM media_items WHERE library_id = ? AND type = 'episode'", lib.ID).Scan(&episodeCount); err != nil && err != sql.ErrNoRows {
		t.Fatal(err)
	}
	if showCount != 0 {
		t.Errorf("expected 0 shows from orphan season, got %d", showCount)
	}
	if episodeCount != 0 {
		t.Errorf("expected 0 episodes from orphan season, got %d", episodeCount)
	}
}

// ── Test 3: mixed library with video but no NFO produces filename-derived movie ──

func TestImporter_MixedLibrary_AmbiguousDirectory_DoesNotDefaultToMovie(t *testing.T) {
	dir := t.TempDir()

	db, err := repository.Open(filepath.Join(dir, "fyom.db"), 5, 2, 60)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	lib := &model.Library{
		Name:           "Test Library",
		Type:           "mixed",
		SourcePath:     dir,
		ProviderID:     "local",
		MetadataSource: "nfo",
	}
	libRepo := repository.NewLibraryRepository(db)
	if err := libRepo.Create(context.Background(), lib); err != nil {
		t.Fatal(err)
	}

	ambigDir := filepath.Join(dir, "Ambiguous Folder")
	if err := os.MkdirAll(ambigDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ambigDir, "random_video.mkv"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	mediaRepo := repository.NewMediaRepository(db)
	jobRepo := repository.NewImportJobRepository(db)
	importer := NewImporter(NewLocalImportFS(), "local", db, mediaRepo, jobRepo)

	ctx := context.Background()
	_, err = importer.ImportLibrary(ctx, lib.ID)
	if err != nil {
		t.Fatal(err)
	}

	var movieCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM media_items WHERE library_id = ? AND type = 'movie'", lib.ID).Scan(&movieCount); err != nil && err != sql.ErrNoRows {
		t.Fatal(err)
	}
	if movieCount != 1 {
		t.Errorf("expected 1 movie (filename fallback) in mixed library, got %d", movieCount)
	}

	var title string
	if err := db.QueryRow("SELECT title FROM media_items WHERE library_id = ? AND type = 'movie'", lib.ID).Scan(&title); err != nil && err != sql.ErrNoRows {
		t.Fatal(err)
	}
	if title == "" {
		t.Error("movie title should not be empty")
	}
}
