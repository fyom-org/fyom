package repository

import (
	"context"
	"os"
	"testing"

	"github.com/fyom/fyom/internal/model"
)

func setupTestDB(t *testing.T) *DB {
	t.Helper()
	tmpDir := t.TempDir()

	db, err := Open(tmpDir, 5, 2, 60)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
		os.RemoveAll(tmpDir)
	})

	return db
}

func TestMediaRepository_CreateAndGet(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMediaRepository(db)

	ctx := context.Background()
	item := &model.MediaItem{
		Type:     "movie",
		Title:    "Test Movie",
		Year:     2024,
		FilePath: "/movies/test.mkv",
	}

	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	if item.ID == "" {
		t.Error("expected ID to be set after create")
	}

	got, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got == nil {
		t.Fatal("expected item, got nil")
	}
	if got.Title != "Test Movie" {
		t.Errorf("expected title 'Test Movie', got '%s'", got.Title)
	}
	if got.Year != 2024 {
		t.Errorf("expected year 2024, got %d", got.Year)
	}
}

func TestMediaRepository_List(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMediaRepository(db)
	ctx := context.Background()

	items := []*model.MediaItem{
		{Type: "movie", Title: "Movie A", FilePath: "/movies/a.mkv"},
		{Type: "movie", Title: "Movie B", FilePath: "/movies/b.mkv"},
		{Type: "show", Title: "Show A", FilePath: "/shows/a.mkv"},
	}

	for _, item := range items {
		if err := repo.Create(ctx, item); err != nil {
			t.Fatalf("Create() error: %v", err)
		}
	}

	all, err := repo.List(ctx, "")
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3 items, got %d", len(all))
	}

	movies, err := repo.List(ctx, "movie")
	if err != nil {
		t.Fatalf("List(movie) error: %v", err)
	}
	if len(movies) != 2 {
		t.Errorf("expected 2 movies, got %d", len(movies))
	}
}

func TestMediaRepository_Get_NotFound(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMediaRepository(db)
	ctx := context.Background()

	got, err := repo.Get(ctx, "non-existent-id")
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got != nil {
		t.Error("expected nil for non-existent id")
	}
}

func TestMediaRepository_Delete(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMediaRepository(db)
	ctx := context.Background()

	item := &model.MediaItem{
		Type:     "movie",
		Title:    "To Delete",
		FilePath: "/movies/delete.mkv",
	}
	if err := repo.Create(ctx, item); err != nil {
		t.Fatalf("Create() error: %v", err)
	}

	if err := repo.Delete(ctx, item.ID); err != nil {
		t.Fatalf("Delete() error: %v", err)
	}

	got, err := repo.Get(ctx, item.ID)
	if err != nil {
		t.Fatalf("Get() error: %v", err)
	}
	if got != nil {
		t.Error("expected item to be deleted")
	}
}

func TestMediaRepository_Count(t *testing.T) {
	db := setupTestDB(t)
	repo := NewMediaRepository(db)
	ctx := context.Background()

	count, err := repo.Count(ctx)
	if err != nil {
		t.Fatalf("Count() error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 items, got %d", count)
	}

	repo.Create(ctx, &model.MediaItem{Type: "movie", Title: "T", FilePath: "/t.mkv"})

	count, err = repo.Count(ctx)
	if err != nil {
		t.Fatalf("Count() error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 item, got %d", count)
	}
}
