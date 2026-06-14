package service

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/fyom/fyom/internal/model"
	"github.com/fyom/fyom/internal/repository"
)

// ── Test 10: library name is preserved, not replaced with source_path ──

func TestLibraryCreate_PreservesProvidedName_AndDoesNotReplaceWithSourcePath(t *testing.T) {
	dir := t.TempDir()

	db, err := repository.Open(filepath.Join(dir, "fyom.db"), 5, 2, 60)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	libRepo := repository.NewLibraryRepository(db)
	sourcePath := filepath.Join(dir, "movies")
	lib := &model.Library{
		Name:           "Anime",
		Type:           "mixed",
		SourcePath:     sourcePath,
		ProviderID:     "local",
		MetadataSource: "nfo",
	}
	err = libRepo.Create(context.Background(), lib)
	if err != nil {
		t.Fatal(err)
	}

	// Verify name is preserved
	var storedName, storedSourcePath string
	err = db.QueryRowContext(context.Background(),
		"SELECT name, source_path FROM libraries WHERE id = ?", lib.ID,
	).Scan(&storedName, &storedSourcePath)
	if err != nil {
		t.Fatal(err)
	}

	if storedName != "Anime" {
		t.Errorf("name = %q, expected %q", storedName, "Anime")
	}
	if storedSourcePath != sourcePath {
		t.Errorf("source_path = %q, expected %q", storedSourcePath, sourcePath)
	}
}

// ── Test 11: empty name does not persist raw source_path as name ──

func TestLibraryCreate_EmptyName_DoesNotPersistRawSourcePathAsName(t *testing.T) {
	dir := t.TempDir()

	db, err := repository.Open(filepath.Join(dir, "fyom.db"), 5, 2, 60)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	libRepo := repository.NewLibraryRepository(db)

	sourcePath := filepath.Join(dir, "movies")
	// The handler requires name to be non-empty, but the repository doesn't enforce it.
	// Test that if an empty name is passed, it doesn't default to source_path.
	lib := &model.Library{
		Name:           "",
		Type:           "mixed",
		SourcePath:     sourcePath,
		ProviderID:     "local",
		MetadataSource: "nfo",
	}
	err = libRepo.Create(context.Background(), lib)
	if err != nil {
		t.Fatal(err)
	}

	var storedName string
	err = db.QueryRowContext(context.Background(),
		"SELECT name FROM libraries WHERE id = ?", lib.ID,
	).Scan(&storedName)
	if err != nil {
		t.Fatal(err)
	}

	// Empty name should remain empty (not replaced with source_path)
	if storedName != "" {
		t.Errorf("name = %q, expected empty (should not be source_path)", storedName)
	}
}
