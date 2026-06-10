package repository

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/fyom/fyom/internal/model"
	"github.com/google/uuid"
)

// LibraryRepository provides access to libraries.
type LibraryRepository struct {
	db *DB
}

// NewLibraryRepository creates a new LibraryRepository.
func NewLibraryRepository(db *DB) *LibraryRepository {
	return &LibraryRepository{db: db}
}

// List returns all libraries ordered by name.
func (r *LibraryRepository) List(ctx context.Context) ([]model.Library, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id, name, type, provider_id, source_path, metadata_source, created_at, updated_at FROM libraries ORDER BY name ASC")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var libs []model.Library
	for rows.Next() {
		var l model.Library
		if err := rows.Scan(&l.ID, &l.Name, &l.Type, &l.ProviderID, &l.SourcePath, &l.MetadataSource, &l.CreatedAt, &l.UpdatedAt); err != nil {
			return nil, err
		}
		libs = append(libs, l)
	}
	return libs, rows.Err()
}

// GetByID returns a single library by ID.
func (r *LibraryRepository) GetByID(ctx context.Context, id string) (*model.Library, error) {
	var l model.Library
	err := r.db.QueryRowContext(ctx, "SELECT id, name, type, provider_id, source_path, metadata_source, created_at, updated_at FROM libraries WHERE id = ?", id).Scan(
		&l.ID, &l.Name, &l.Type, &l.ProviderID, &l.SourcePath, &l.MetadataSource, &l.CreatedAt, &l.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &l, nil
}

// Create inserts a new library.
func (r *LibraryRepository) Create(ctx context.Context, lib *model.Library) error {
	if lib.ID == "" {
		lib.ID = uuid.New().String()
	}
	now := time.Now().UTC().Format(time.RFC3339)
	lib.CreatedAt = now
	lib.UpdatedAt = now

	// Validate provider_id exists (skip for built-in "local" provider).
	if lib.ProviderID != "local" {
		var dummy int
		if err := r.db.QueryRowContext(ctx, "SELECT 1 FROM providers WHERE id = ?", lib.ProviderID).Scan(&dummy); err != nil {
			if err == sql.ErrNoRows {
				return errors.New("provider not found: " + lib.ProviderID)
			}
			return err
		}
	}

	_, err := r.db.ExecContext(ctx,
		"INSERT INTO libraries (id, name, type, provider_id, source_path, metadata_source, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		lib.ID, lib.Name, lib.Type, lib.ProviderID, lib.SourcePath, lib.MetadataSource, lib.CreatedAt, lib.UpdatedAt)
	return err
}

// Update modifies a library.
func (r *LibraryRepository) Update(ctx context.Context, lib *model.Library) error {
	lib.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	// Validate provider_id exists (skip for built-in "local" provider).
	if lib.ProviderID != "local" {
		var dummy int
		if err := r.db.QueryRowContext(ctx, "SELECT 1 FROM providers WHERE id = ?", lib.ProviderID).Scan(&dummy); err != nil {
			if err == sql.ErrNoRows {
				return errors.New("provider not found: " + lib.ProviderID)
			}
			return err
		}
	}

	res, err := r.db.ExecContext(ctx,
		"UPDATE libraries SET name = ?, type = ?, provider_id = ?, source_path = ?, metadata_source = ?, updated_at = ? WHERE id = ?",
		lib.Name, lib.Type, lib.ProviderID, lib.SourcePath, lib.MetadataSource, lib.UpdatedAt, lib.ID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return errors.New("library not found: " + lib.ID)
	}
	return nil
}

// Delete removes a library. Returns error if it has items.
func (r *LibraryRepository) Delete(ctx context.Context, id string) error {
	count, err := r.ItemCount(ctx, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return errors.New("library has " + strconv.Itoa(count) + " items")
	}
	_, err = r.db.ExecContext(ctx, "DELETE FROM libraries WHERE id = ?", id)
	return err
}

// ItemCount returns the number of media items in a library.
func (r *LibraryRepository) ItemCount(ctx context.Context, id string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM media_items WHERE library_id = ?", id).Scan(&count)
	return count, err
}

// MissingCount returns the number of missing items in a library.
func (r *LibraryRepository) MissingCount(ctx context.Context, id string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM media_items WHERE library_id = ? AND status = 'missing'", id).Scan(&count)
	return count, err
}

// DeleteWithItems deletes a library and all its associated data in a transaction.
func (r *LibraryRepository) DeleteWithItems(ctx context.Context, libraryID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Delete watch progress for items in this library.
	if _, err := tx.ExecContext(ctx, "DELETE FROM watch_progress WHERE media_item_id IN (SELECT id FROM media_items WHERE library_id = ?)", libraryID); err != nil {
		return err
	}
	// Delete episodes for shows in this library.
	if _, err := tx.ExecContext(ctx, "DELETE FROM media_items WHERE type = 'episode' AND parent_id IN (SELECT id FROM media_items WHERE library_id = ? AND type = 'show')", libraryID); err != nil {
		return err
	}
	// Delete all remaining media items.
	if _, err := tx.ExecContext(ctx, "DELETE FROM media_items WHERE library_id = ?", libraryID); err != nil {
		return err
	}
	// Delete library permissions.
	if _, err := tx.ExecContext(ctx, "DELETE FROM library_permissions WHERE library_id = ?", libraryID); err != nil {
		return err
	}
	// Delete the library.
	if _, err := tx.ExecContext(ctx, "DELETE FROM libraries WHERE id = ?", libraryID); err != nil {
		return err
	}
	return tx.Commit()
}

// ItemCountsByType returns counts of movies, shows, and episodes in a library.
func (r *LibraryRepository) ItemCountsByType(ctx context.Context, id string) (movies, shows, episodes int, err error) {
	rows, err := r.db.QueryContext(ctx, "SELECT type, COUNT(*) FROM media_items WHERE library_id = ? GROUP BY type", id)
	if err != nil {
		return 0, 0, 0, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var t string
		var c int
		if err := rows.Scan(&t, &c); err != nil {
			return 0, 0, 0, err
		}
		switch t {
		case "movie":
			movies = c
		case "show":
			shows = c
		case "episode":
			episodes = c
		}
	}
	return movies, shows, episodes, rows.Err()
}

// MediaPathCheck holds an item ID and its file path for existence checking.
type MediaPathCheck struct {
	ID       string
	FilePath string
}

// GetLocalItemPaths returns id + file_path for all non-episode items in a library.
func (r *LibraryRepository) GetLocalItemPaths(ctx context.Context, libraryID string) ([]MediaPathCheck, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id, file_path FROM media_items WHERE library_id = ? AND type != 'episode'", libraryID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var results []MediaPathCheck
	for rows.Next() {
		var p MediaPathCheck
		if err := rows.Scan(&p.ID, &p.FilePath); err != nil {
			return nil, err
		}
		results = append(results, p)
	}
	return results, rows.Err()
}

// MarkMissing sets status='missing' for the given item IDs.
func (r *LibraryRepository) MarkMissing(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	query := "UPDATE media_items SET status = 'missing' WHERE id IN (" + strings.Join(placeholders, ",") + ")"
	_, err := r.db.ExecContext(ctx, query, args...)
	return err
}

// MarkAvailableByLibrary sets status='available' for all items in a library
// except those in the excludeIDs list.
func (r *LibraryRepository) MarkAvailableByLibrary(ctx context.Context, libraryID string, excludeIDs []string) error {
	if len(excludeIDs) == 0 {
		_, err := r.db.ExecContext(ctx, "UPDATE media_items SET status = 'available' WHERE library_id = ?", libraryID)
		return err
	}
	placeholders := make([]string, len(excludeIDs))
	args := make([]interface{}, len(excludeIDs)+1)
	args[0] = libraryID
	for i, id := range excludeIDs {
		placeholders[i] = "?"
		args[i+1] = id
	}
	query := "UPDATE media_items SET status = 'available' WHERE library_id = ? AND id NOT IN (" + strings.Join(placeholders, ",") + ")"
	_, err := r.db.ExecContext(ctx, query, args...)
	return err
}
