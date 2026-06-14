package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

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
	defer db.Close()

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
	os.MkdirAll(seasonDir, 0755)
	os.MkdirAll(seasonDir2, 0755)
	os.MkdirAll(seasonDir3, 0755)
	os.WriteFile(filepath.Join(showDir, "tvshow.nfo"), []byte(testShowNFO), 0644)
	os.WriteFile(filepath.Join(seasonDir, "S01E01.mkv"), []byte(""), 0644)
	os.WriteFile(filepath.Join(seasonDir, "S01E01.nfo"), []byte(testEpisodeNFO), 0644)
	os.WriteFile(filepath.Join(seasonDir2, "S02E01.mkv"), []byte(""), 0644)
	os.WriteFile(filepath.Join(seasonDir3, "S03E01.mkv"), []byte(""), 0644)

	mediaRepo := repository.NewMediaRepository(db)
	jobRepo := repository.NewImportJobRepository(db)
	importer := NewImporter(NewLocalImportFS(), "local", db, mediaRepo, jobRepo)

	ctx := context.Background()
	job, err := importer.ImportRequest(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}

	waitForJob(t, ctx, jobRepo, job.ID, 5000000000) // 5s timeout

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
