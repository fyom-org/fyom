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

// ── Test 9: job counts are candidate-based and never exceed total ──

func TestImporter_JobCounts_AreCandidateBased_AndNeverExceedTotal(t *testing.T) {
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

	// Create show with 3 episodes
	showDir := filepath.Join(dir, "Show A")
	seasonDir := filepath.Join(showDir, "Season 01")
	seasonDir2 := filepath.Join(showDir, "Season 02")
	seasonDir3 := filepath.Join(showDir, "Season 03")
	if err := os.MkdirAll(seasonDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(seasonDir2, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(seasonDir3, 0755); err != nil {
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
	if err := os.WriteFile(filepath.Join(seasonDir2, "S02E01.mkv"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seasonDir3, "S03E01.mkv"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	mediaRepo := repository.NewMediaRepository(db)
	jobRepo := repository.NewImportJobRepository(db)
	importer := NewImporter(NewLocalImportFS(), "local", db, mediaRepo, jobRepo)

	ctx := context.Background()
	job, err := importer.ImportRequest(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}

	waitForJob(t, jobRepo, job.ID)

	jobFinal, _ := jobRepo.Get(ctx, job.ID)
	if jobFinal.Status == "error" {
		t.Fatalf("import ended in error: %s", jobFinal.ErrorMsg)
	}

	// done_items must equal total_items
	if jobFinal.TotalItems != jobFinal.DoneItems {
		t.Errorf("total_items=%d, done_items=%d, should be equal", jobFinal.TotalItems, jobFinal.DoneItems)
	}

	// done_items must not exceed total_items
	if jobFinal.DoneItems > jobFinal.TotalItems {
		t.Errorf("done_items=%d > total_items=%d", jobFinal.DoneItems, jobFinal.TotalItems)
	}

	// Total should be 4 (1 show + 3 episodes)
	if jobFinal.TotalItems != 4 {
		t.Errorf("expected total_items=4 (1 show + 3 episodes), got %d", jobFinal.TotalItems)
	}
}

func TestAsyncImport_PersistsImportSummaryToJob(t *testing.T) {
	root := buildFixtureLibrary(t)
	db := openImporterTestDB(t)
	lib := createImporterTestLibrary(t, db, root)
	mediaRepo := repository.NewMediaRepository(db)
	jobRepo := repository.NewImportJobRepository(db)
	importer := NewImporter(NewLocalImportFS(), "local", db, mediaRepo, jobRepo)
	importer.SetLibraryID(lib.ID)

	ctx := context.Background()
	job, err := importer.ImportRequest(ctx, root)
	if err != nil {
		t.Fatalf("ImportRequest failed: %v", err)
	}

	waitForJob(t, jobRepo, job.ID)
	time.Sleep(50 * time.Millisecond)

	jobFinal, err := jobRepo.Get(ctx, job.ID)
	if err != nil {
		t.Fatalf("Get job failed: %v", err)
	}
	if jobFinal.Status == "error" {
		t.Fatalf("import ended in error: %s", jobFinal.ErrorMsg)
	}
	if jobFinal.ScannedFiles <= 0 {
		t.Fatalf("expected scanned_files > 0, got %d", jobFinal.ScannedFiles)
	}
	if jobFinal.ImportedItems <= 0 {
		t.Fatalf("expected imported_items > 0, got %d", jobFinal.ImportedItems)
	}
	if jobFinal.UpdatedItems != 0 {
		t.Fatalf("expected updated_items=0 on first import, got %d", jobFinal.UpdatedItems)
	}
	if jobFinal.SkippedFiles < 0 {
		t.Fatalf("expected non-negative skipped_files, got %d", jobFinal.SkippedFiles)
	}
	if jobFinal.DurationMS < 0 {
		t.Fatalf("expected non-negative duration_ms, got %d", jobFinal.DurationMS)
	}
	if jobFinal.TotalItems != jobFinal.DoneItems {
		t.Fatalf("progress counters changed unexpectedly: total_items=%d done_items=%d", jobFinal.TotalItems, jobFinal.DoneItems)
	}
}
