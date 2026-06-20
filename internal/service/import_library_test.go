package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/fyom/fyom/internal/repository"
)

// ── Test: ImportLibrary returns correct summary ───────────────────────

func TestImportLibrary_ReturnsSummary(t *testing.T) {
	root := buildFixtureLibrary(t)
	db := openImporterTestDB(t)
	lib := createImporterTestLibrary(t, db, root)

	mediaRepo := repository.NewMediaRepository(db)
	jobRepo := repository.NewImportJobRepository(db)

	importer := NewImporter(NewLocalImportFS(), "local", db, mediaRepo, jobRepo)

	ctx := context.Background()
	summary, err := importer.ImportLibrary(ctx, lib.ID)
	if err != nil {
		t.Fatalf("ImportLibrary failed: %v", err)
	}

	// Should have scanned at least 2 directories (Movie + Show)
	if summary.ScannedFiles < 2 {
		t.Errorf("expected ScannedFiles >= 2, got %d", summary.ScannedFiles)
	}

	// Should have imported items: movie + show + episode
	if summary.ImportedItems < 3 {
		t.Errorf("expected at least 3 imported items, got %d", summary.ImportedItems)
	}

	// Duration should be positive
	if summary.Duration <= 0 {
		t.Error("expected positive duration")
	}

	// No parse warnings expected for clean fixtures
	if len(summary.ParseWarnings) != 0 {
		t.Errorf("expected 0 parse warnings, got %d: %v",
			len(summary.ParseWarnings), summary.ParseWarnings)
	}

	// Verify JSON serialization of summary
	data, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("failed to marshal summary: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty JSON")
	}
	t.Logf("summary JSON: %s", string(data))
}

// ── Test: ImportLibrary second run does not lose data ──────────────────

func TestImportLibrary_SecondRunPreservesData(t *testing.T) {
	root := buildFixtureLibrary(t)
	db := openImporterTestDB(t)
	lib := createImporterTestLibrary(t, db, root)

	mediaRepo := repository.NewMediaRepository(db)
	jobRepo := repository.NewImportJobRepository(db)

	importer := NewImporter(NewLocalImportFS(), "local", db, mediaRepo, jobRepo)
	ctx := context.Background()

	// First call
	_, err := importer.ImportLibrary(ctx, lib.ID)
	if err != nil {
		t.Fatalf("first ImportLibrary failed: %v", err)
	}

	var countAfterFirst int
	if err := db.QueryRow(`SELECT COUNT(*) FROM media_items WHERE library_id = ?`, lib.ID).Scan(&countAfterFirst); err != nil {
		t.Fatalf("query count after first: %v", err)
	}
	if countAfterFirst < 3 {
		t.Fatalf("expected at least 3 items after first import, got %d", countAfterFirst)
	}

	// Second call — same directory, same DB
	summary2, err := importer.ImportLibrary(ctx, lib.ID)
	if err != nil {
		t.Fatalf("second ImportLibrary failed: %v", err)
	}

	var countAfterSecond int
	if err := db.QueryRow(`SELECT COUNT(*) FROM media_items WHERE library_id = ?`, lib.ID).Scan(&countAfterSecond); err != nil {
		t.Fatalf("query count after second: %v", err)
	}

	// The second run must NOT delete or lose any items
	if countAfterSecond < countAfterFirst {
		t.Errorf("item count decreased on second run: %d -> %d",
			countAfterFirst, countAfterSecond)
	}

	// Second run should have scanned the same number of directories
	if summary2.ScannedFiles == 0 {
		t.Error("expected ScannedFiles > 0 on second run")
	}

	// No parse warnings on second run for clean fixtures
	if len(summary2.ParseWarnings) != 0 {
		t.Errorf("expected 0 parse warnings on second run, got %d: %v",
			len(summary2.ParseWarnings), summary2.ParseWarnings)
	}
}
