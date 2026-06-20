package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fyom/fyom/internal/model"
	"github.com/fyom/fyom/internal/repository"
)

// ── Test 4: episode primary_path is real file path, not double-nested ──

func TestImporter_EpisodePrimaryPath_IsRealFilePath_NotDoubleNested(t *testing.T) {
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
	_, err = importer.ImportLibrary(ctx, lib.ID)
	if err != nil {
		t.Fatal(err)
	}

	var primaryPath, filePath string
	err = db.QueryRow("SELECT primary_path, file_path FROM media_items WHERE library_id = ? AND type = 'episode'", lib.ID).Scan(&primaryPath, &filePath)
	if err != nil {
		t.Fatal(err)
	}

	// primary_path must equal the actual .mkv path
	expectedPath := filepath.Join(seasonDir, "S01E01.mkv")
	if primaryPath != expectedPath {
		t.Errorf("primary_path = %s, expected %s", primaryPath, expectedPath)
	}

	// file_path compatibility field must also equal the actual .mkv path
	if filePath != expectedPath {
		t.Errorf("file_path = %s, expected %s", filePath, expectedPath)
	}

	// Must NOT contain .mkv/.mkv double-nesting
	if strings.Contains(primaryPath, ".mkv/.mkv") {
		t.Errorf("primary_path contains double-nested .mkv: %s", primaryPath)
	}
	if strings.Contains(filePath, ".mkv/.mkv") {
		t.Errorf("file_path contains double-nested .mkv: %s", filePath)
	}
}

// ── Test 5: show paths use root_path and nfo_path, not directory as file_path ──

func TestImporter_ShowPaths_UseRootPathAndNFOPath_NotDirectoryAsFilePath(t *testing.T) {
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
	_, err = importer.ImportLibrary(ctx, lib.ID)
	if err != nil {
		t.Fatal(err)
	}

	var rootPath, primaryPath, nfoPath, filePath string
	err = db.QueryRow("SELECT root_path, primary_path, nfo_path, file_path FROM media_items WHERE library_id = ? AND type = 'show'", lib.ID).Scan(&rootPath, &primaryPath, &nfoPath, &filePath)
	if err != nil {
		t.Fatal(err)
	}

	// root_path must be the show directory
	if rootPath != showDir {
		t.Errorf("root_path = %s, expected %s", rootPath, showDir)
	}

	// primary_path must be empty (show has no playable file)
	if primaryPath != "" {
		t.Errorf("primary_path = %q, expected empty", primaryPath)
	}

	// nfo_path must point to tvshow.nfo
	expectedNFO := filepath.Join(showDir, "tvshow.nfo")
	if nfoPath != expectedNFO {
		t.Errorf("nfo_path = %s, expected %s", nfoPath, expectedNFO)
	}

	// file_path must be empty (show is not a playable file)
	if filePath != "" {
		t.Errorf("file_path = %q, expected empty", filePath)
	}
}
