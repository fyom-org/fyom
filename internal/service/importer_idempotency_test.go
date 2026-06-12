package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fyom/fyom/internal/model"
	"github.com/fyom/fyom/internal/repository"
)

// ── NFO fixtures ──────────────────────────────────────────────────────

const movieNFOFixture = `<?xml version="1.0" encoding="UTF-8"?>
<movie>
    <title>Movie A</title>
    <year>2020</year>
    <plot>A test movie.</plot>
    <rating>7.5</rating>
    <genre>Action</genre>
    <genre>Drama</genre>
    <studio>Test Studio</studio>
    <director>Director A</director>
    <credits>Writer A</credits>
    <actor>
        <name>Actor One</name>
        <role>Hero</role>
        <type>Actor</type>
        <sortorder>1</sortorder>
    </actor>
    <actor>
        <name>Actor Two</name>
        <role>Villain</role>
        <type>Actor</type>
        <sortorder>2</sortorder>
    </actor>
    <uniqueid type="imdb">tt0000001</uniqueid>
    <uniqueid type="tmdb">12345</uniqueid>
</movie>`

const showNFOFixture = `<?xml version="1.0" encoding="UTF-8"?>
<tvshow>
    <title>Show A</title>
    <year>2022</year>
    <plot>A test show.</plot>
    <genre>Comedy</genre>
    <studio>Show Studio</studio>
    <rating>8.0</rating>
    <actor>
        <name>Show Actor</name>
        <role>Lead</role>
        <type>Actor</type>
        <sortorder>1</sortorder>
    </actor>
    <uniqueid type="imdb">tt0000002</uniqueid>
    <uniqueid type="tvdb">67890</uniqueid>
</tvshow>`

const episodeNFOFixture = `<?xml version="1.0" encoding="UTF-8"?>
<episodedetails>
    <title>Episode One</title>
    <season>1</season>
    <episode>1</episode>
    <plot>First episode.</plot>
    <rating>7.0</rating>
    <genre>Drama</genre>
    <director>Director A</director>
    <actor>
        <name>Guest Star</name>
        <role>Guest</role>
        <type>GuestStar</type>
        <sortorder>1</sortorder>
    </actor>
    <uniqueid type="imdb">tt0000003</uniqueid>
</episodedetails>`

// ── Helpers ───────────────────────────────────────────────────────────

func writeFileHelper(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func buildFixture_library(t *testing.T) string {
	root := t.TempDir()

	// Movie
	writeFileHelper(t, filepath.Join(root, "Movie (2020)", "movie.nfo"), movieNFOFixture)
	writeFileHelper(t, filepath.Join(root, "Movie (2020)", "movie.mkv"), "")

	// Show with one season, one episode
	showDir := filepath.Join(root, "Show (2022)")
	writeFileHelper(t, filepath.Join(showDir, "tvshow.nfo"), showNFOFixture)
	writeFileHelper(t, filepath.Join(showDir, "Season 01", "S1E1.nfo"), episodeNFOFixture)
	writeFileHelper(t, filepath.Join(showDir, "Season 01", "S1E1.mkv"), "")

	return root
}

func openImporterTestDB(t *testing.T) *repository.DB {
	t.Helper()
	tmpDir := t.TempDir()
	db, err := repository.Open(tmpDir, 5, 2, 60)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
		_ = os.RemoveAll(tmpDir)
	})
	return db
}

func createImporterTestLibrary(t *testing.T, db *repository.DB, sourcePath string) *model.Library {
	t.Helper()
	lib := &model.Library{
		Name:           "Test Library",
		Type:           "movie",
		SourcePath:     sourcePath,
		ProviderID:     "local",
		MetadataSource: "nfo",
	}
	repo := repository.NewLibraryRepository(db)
	if err := repo.Create(context.Background(), lib); err != nil {
		t.Fatalf("failed to create test library: %v", err)
	}
	return lib
}

// waitForJob polls the job status until it reaches a terminal state or timeout.
func waitForJob(t *testing.T, ctx context.Context, jobRepo *repository.ImportJobRepository, jobID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		j, _ := jobRepo.Get(ctx, jobID)
		if j != nil && (j.Status == "done" || j.Status == "error") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// ── Test ──────────────────────────────────────────────────────────────
//
// Uses the synchronous ImportLibrary method instead of the async
// ImportRequest path. This eliminates all goroutine timing races —
// ImportLibrary returns only when the scan is fully complete and all
// DB writes are committed.

func TestImport_Idempotent(t *testing.T) {
	root := buildFixture_library(t)
	db := openImporterTestDB(t)
	lib := createImporterTestLibrary(t, db, root)

	mediaRepo := repository.NewMediaRepository(db)
	jobRepo := repository.NewImportJobRepository(db)

	importer := NewImporter(NewLocalImportFS(), "local", db, mediaRepo, jobRepo)

	ctx := context.Background()

	// First import — synchronous, returns when fully done
	summary1, err := importer.ImportLibrary(ctx, lib.ID)
	if err != nil {
		t.Fatalf("first ImportLibrary failed: %v", err)
	}

	var countAfterFirst int
	db.QueryRow(`SELECT COUNT(*) FROM media_items WHERE library_id = ?`, lib.ID).Scan(&countAfterFirst)

	var actorsJSON1, uniqueIDsJSON1, genresJSON1 string
	db.QueryRow(`SELECT actors, unique_ids, genres FROM media_items
	             WHERE library_id = ? AND type = 'movie'`, lib.ID).
		Scan(&actorsJSON1, &uniqueIDsJSON1, &genresJSON1)

	// Capture the show's ID on run 1 for Task 3 verification
	var showID1 string
	db.QueryRow(`SELECT id FROM media_items WHERE library_id = ? AND type = 'show'`, lib.ID).Scan(&showID1)

	// Second import — re-scan same directory, nothing changed on disk
	summary2, err := importer.ImportLibrary(ctx, lib.ID)
	if err != nil {
		t.Fatalf("second ImportLibrary failed: %v", err)
	}

	var countAfterSecond int
	db.QueryRow(`SELECT COUNT(*) FROM media_items WHERE library_id = ?`, lib.ID).Scan(&countAfterSecond)

	// Core idempotency assertion: item count must not change
	if countAfterSecond != countAfterFirst {
		t.Errorf("item count changed on re-import: %d -> %d", countAfterFirst, countAfterSecond)
	}

	var actorsJSON2, uniqueIDsJSON2, genresJSON2 string
	db.QueryRow(`SELECT actors, unique_ids, genres FROM media_items
	             WHERE library_id = ? AND type = 'movie'`, lib.ID).
		Scan(&actorsJSON2, &uniqueIDsJSON2, &genresJSON2)

	if actorsJSON1 != actorsJSON2 {
		t.Errorf("actors JSON changed on re-import:\n  first:  %s\n  second: %s", actorsJSON1, actorsJSON2)
	}
	if uniqueIDsJSON1 != uniqueIDsJSON2 {
		t.Errorf("unique_ids JSON changed on re-import:\n  first:  %s\n  second: %s", uniqueIDsJSON1, uniqueIDsJSON2)
	}
	if genresJSON1 != genresJSON2 {
		t.Errorf("genres JSON changed on re-import:\n  first:  %s\n  second: %s", genresJSON1, genresJSON2)
	}

	// Task 3: show ID must be stable across runs
	var showID2 string
	db.QueryRow(`SELECT id FROM media_items WHERE library_id = ? AND type = 'show'`, lib.ID).Scan(&showID2)
	if showID1 != showID2 {
		t.Errorf("show ID changed between runs: %s -> %s", showID1, showID2)
	}

	// Task 3: exactly 1 show row per show directory (no orphans)
	var showCount int
	db.QueryRow(`SELECT COUNT(*) FROM media_items WHERE library_id = ? AND type = 'show'`, lib.ID).Scan(&showCount)
	if showCount != 1 {
		t.Errorf("expected exactly 1 show row, got %d", showCount)
	}

	// Task 3: episode's parent_id must point to the stable show ID
	var episodeParentID string
	db.QueryRow(`SELECT parent_id FROM media_items WHERE library_id = ? AND type = 'episode'`, lib.ID).Scan(&episodeParentID)
	if episodeParentID != showID1 {
		t.Errorf("episode parent_id = %q, expected %q (the stable show ID)", episodeParentID, showID1)
	}

	// Second run should report 0 new imports
	if summary2.ImportedItems != 0 {
		t.Errorf("expected 0 newly imported items on re-import, got %d", summary2.ImportedItems)
	}

	_ = summary1
}
