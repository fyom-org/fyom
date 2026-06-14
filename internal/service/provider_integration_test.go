package service

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/fyom/fyom/internal/model"
	"github.com/fyom/fyom/internal/repository"
)

// ── Test 8: migration/bootstrap ensures local provider with FK enabled ──

func TestMigrationOrBootstrap_EnsuresLocalProvider_WithForeignKeysEnabled(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "fyom.db")

	db, err := repository.Open(dbPath, 5, 2, 60)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Enable foreign keys
	_, err = db.ExecContext(context.Background(), "PRAGMA foreign_keys = ON")
	if err != nil {
		t.Fatal(err)
	}

	// Verify providers contains local
	var providerCount int
	err = db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM providers WHERE id = 'local'").Scan(&providerCount)
	if err != nil {
		t.Fatal(err)
	}
	if providerCount != 1 {
		t.Errorf("expected 1 local provider, got %d", providerCount)
	}

	// Insert a library with provider_id='local' — should succeed
	libRepo := repository.NewLibraryRepository(db)
	lib := &model.Library{
		Name:           "Test Library",
		Type:           "mixed",
		SourcePath:     "/test",
		ProviderID:     "local",
		MetadataSource: "nfo",
	}
	err = libRepo.Create(context.Background(), lib)
	if err != nil {
		t.Fatalf("creating library with local provider failed: %v", err)
	}

	// Insert a media row with provider_id='local' — should succeed
	_, err = db.ExecContext(context.Background(),
		"INSERT INTO media_items (id, type, title, file_path, provider_id, library_id, status) VALUES (?, ?, ?, ?, ?, ?, ?)",
		"test-movie", "movie", "Test", "/test.mkv", "local", lib.ID, "available",
	)
	if err != nil {
		t.Fatalf("inserting media item with local provider failed: %v", err)
	}

	// Verify the media item was inserted
	var mediaCount int
	err = db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM media_items WHERE provider_id = 'local'").Scan(&mediaCount)
	if err != nil && err != sql.ErrNoRows {
		t.Fatal(err)
	}
	if mediaCount != 1 {
		t.Errorf("expected 1 media item with local provider, got %d", mediaCount)
	}
}
