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

// ── Test 12: full import regression with DB-level assertions ──

func TestImporter_FullImport_Regression(t *testing.T) {
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

	// Create show fixture with tvshow.nfo, season, episode file + episode NFO
	showDir := filepath.Join(dir, "Show A")
	seasonDir := filepath.Join(showDir, "Season 01")
	if err := os.MkdirAll(seasonDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(showDir, "tvshow.nfo"), []byte(testShowNFO), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seasonDir, "S01E01.mkv"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seasonDir, "S01E01.nfo"), []byte(testEpisodeNFO), 0644); err != nil {
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

	// 1. providers contains exactly one local row
	var providerCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM providers WHERE id='local'").Scan(&providerCount); err != nil && err != sql.ErrNoRows {
		t.Fatal(err)
	}
	if providerCount != 1 {
		t.Errorf("providers.local count = %d, expected 1", providerCount)
	}

	// 2. one show row exists
	var showCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM media_items WHERE library_id = ? AND type='show'", lib.ID).Scan(&showCount); err != nil && err != sql.ErrNoRows {
		t.Fatal(err)
	}
	if showCount != 1 {
		t.Errorf("show count = %d, expected 1", showCount)
	}

	// 3. one episode row exists
	var episodeCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM media_items WHERE library_id = ? AND type='episode'", lib.ID).Scan(&episodeCount); err != nil && err != sql.ErrNoRows {
		t.Fatal(err)
	}
	if episodeCount != 1 {
		t.Errorf("episode count = %d, expected 1", episodeCount)
	}

	// 4. zero movie rows from episode-style files
	var movieCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM media_items WHERE library_id = ? AND type='movie' AND title LIKE '%S01E01%'", lib.ID).Scan(&movieCount); err != nil && err != sql.ErrNoRows {
		t.Fatal(err)
	}
	if movieCount != 0 {
		t.Errorf("duplicate movies from episode files = %d, expected 0", movieCount)
	}

	// 5. episode primary_path points to real existing file
	var epPrimaryPath string
	if err := db.QueryRow("SELECT primary_path FROM media_items WHERE library_id = ? AND type='episode'", lib.ID).Scan(&epPrimaryPath); err != nil && err != sql.ErrNoRows {
		t.Fatal(err)
	}
	if epPrimaryPath == "" {
		t.Error("episode primary_path is empty")
	} else {
		// Verify file exists on disk
		if _, err := os.Stat(epPrimaryPath); os.IsNotExist(err) {
			t.Errorf("episode primary_path does not exist on disk: %s", epPrimaryPath)
		}
	}

	// 6. show primary_path is empty
	var showPrimaryPath string
	if err := db.QueryRow("SELECT primary_path FROM media_items WHERE library_id = ? AND type='show'", lib.ID).Scan(&showPrimaryPath); err != nil && err != sql.ErrNoRows {
		t.Fatal(err)
	}
	if showPrimaryPath != "" {
		t.Errorf("show primary_path = %q, expected empty", showPrimaryPath)
	}

	// 7. show root_path is the show directory
	var showRootPath string
	if err := db.QueryRow("SELECT root_path FROM media_items WHERE library_id = ? AND type='show'", lib.ID).Scan(&showRootPath); err != nil && err != sql.ErrNoRows {
		t.Fatal(err)
	}
	if showRootPath != showDir {
		t.Errorf("show root_path = %q, expected %q", showRootPath, showDir)
	}

	// 8. show nfo_path points to tvshow.nfo
	var showNFOPath string
	if err := db.QueryRow("SELECT nfo_path FROM media_items WHERE library_id = ? AND type='show'", lib.ID).Scan(&showNFOPath); err != nil && err != sql.ErrNoRows {
		t.Fatal(err)
	}
	expectedNFOPath := filepath.Join(showDir, "tvshow.nfo")
	if showNFOPath != expectedNFOPath {
		t.Errorf("show nfo_path = %q, expected %q", showNFOPath, expectedNFOPath)
	}

	// 9. import_jobs total_items == done_items
	var totalItems, doneItems int
	if err := db.QueryRow("SELECT total_items, done_items FROM import_jobs ORDER BY created_at DESC LIMIT 1").Scan(&totalItems, &doneItems); err != nil && err != sql.ErrNoRows {
		t.Fatal(err)
	}
	if totalItems != doneItems {
		t.Errorf("total_items=%d, done_items=%d, should be equal", totalItems, doneItems)
	}

	// 10. done_items <= total_items
	if doneItems > totalItems {
		t.Errorf("done_items=%d > total_items=%d", doneItems, totalItems)
	}

	// Summary checks
	if summary.ImportedItems < 2 {
		t.Errorf("summary.ImportedItems = %d, expected >= 2", summary.ImportedItems)
	}
	if summary.ScannedFiles < 2 {
		t.Errorf("summary.ScannedFiles = %d, expected >= 2", summary.ScannedFiles)
	}
}
