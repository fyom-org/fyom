package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/fyom/fyom/internal/model"
	"github.com/google/uuid"
)

// IntPtr returns a pointer to the given int value.
func IntPtr(v int) *int {
	return &v
}

// MediaRepository provides access to media_items.
type MediaRepository struct {
	db *DB
}

// NewMediaRepository creates a new MediaRepository.
func NewMediaRepository(db *DB) *MediaRepository {
	return &MediaRepository{db: db}
}

// List returns all media items, optionally filtered by type.
func (r *MediaRepository) List(ctx context.Context, mediaType string) ([]model.MediaItem, error) {
	query := `SELECT id, type, title, sort_title, year, overview, rating, duration,
	          file_path, poster_path, backdrop_path, parent_id, season, episode,
	          metadata_source, provider_id, library_id, status, created_at, updated_at FROM media_items`
	args := []interface{}{}

	if mediaType != "" {
		query += " WHERE type = ?"
		args = append(args, mediaType)
	}
	query += " ORDER BY sort_title ASC, title ASC"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var items []model.MediaItem
	for rows.Next() {
		var m model.MediaItem
		var season, episode int
		if err := rows.Scan(&m.ID, &m.Type, &m.Title, &m.SortTitle, &m.Year,
			&m.Overview, &m.Rating, &m.Duration, &m.FilePath, &m.PosterPath,
			&m.BackdropPath, &m.ParentID, &season, &episode,
			&m.MetadataSource, &m.ProviderID, &m.LibraryID, &m.Status, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		m.Season = IntPtr(season)
		m.Episode = IntPtr(episode)
		items = append(items, m)
	}
	return items, rows.Err()
}

// Get returns a single media item by ID.
func (r *MediaRepository) Get(ctx context.Context, id string) (*model.MediaItem, error) {
	var m model.MediaItem
	var season, episode int
	err := r.db.QueryRowContext(ctx, `SELECT id, type, title, sort_title, year, overview,
		rating, duration, file_path, poster_path, backdrop_path, parent_id, season,
		episode, metadata_source, provider_id, library_id, status, created_at, updated_at FROM media_items WHERE id = ?`, id,
	).Scan(&m.ID, &m.Type, &m.Title, &m.SortTitle, &m.Year, &m.Overview,
		&m.Rating, &m.Duration, &m.FilePath, &m.PosterPath, &m.BackdropPath,
		&m.ParentID, &season, &episode, &m.MetadataSource, &m.ProviderID, &m.LibraryID, &m.Status, &m.CreatedAt, &m.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	m.Season = IntPtr(season)
	m.Episode = IntPtr(episode)
	return &m, nil
}

// Create inserts a new media item.
func (r *MediaRepository) Create(ctx context.Context, m *model.MediaItem) error {
	if m.ID == "" {
		m.ID = uuid.New().String()
	}
	now := time.Now().UTC().Format(time.RFC3339)
	m.CreatedAt = now
	m.UpdatedAt = now
	if m.Status == "" {
		m.Status = "available"
	}

	season := 0
	if m.Season != nil {
		season = *m.Season
	}
	episode := 0
	if m.Episode != nil {
		episode = *m.Episode
	}

	_, err := r.db.ExecContext(ctx, `INSERT OR IGNORE INTO media_items
		(id, type, title, sort_title, year, overview, rating, duration, file_path,
		 poster_path, backdrop_path, parent_id, season, episode, metadata_source,
		 provider_id, library_id, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.Type, m.Title, m.SortTitle, m.Year, m.Overview, m.Rating,
		m.Duration, m.FilePath, m.PosterPath, m.BackdropPath, m.ParentID,
		season, episode, m.MetadataSource, m.ProviderID, m.LibraryID, m.Status, m.CreatedAt, m.UpdatedAt)
	return err
}

// Delete removes a media item by ID.
func (r *MediaRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM media_items WHERE id = ?", id)
	return err
}

// DeleteShowWithEpisodes deletes a show and all its episodes + watch progress in a transaction.
func (r *MediaRepository) DeleteShowWithEpisodes(ctx context.Context, showID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Delete watch progress for episodes of this show.
	if _, err := tx.ExecContext(ctx, "DELETE FROM watch_progress WHERE media_item_id IN (SELECT id FROM media_items WHERE parent_id = ?)", showID); err != nil {
		return err
	}
	// Delete watch progress for the show itself.
	if _, err := tx.ExecContext(ctx, "DELETE FROM watch_progress WHERE media_item_id = ?", showID); err != nil {
		return err
	}
	// Delete episodes.
	if _, err := tx.ExecContext(ctx, "DELETE FROM media_items WHERE parent_id = ?", showID); err != nil {
		return err
	}
	// Delete the show.
	if _, err := tx.ExecContext(ctx, "DELETE FROM media_items WHERE id = ?", showID); err != nil {
		return err
	}
	return tx.Commit()
}

// Count returns the total number of media items.
func (r *MediaRepository) Count(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM media_items").Scan(&count)
	return count, err
}

// GetEpisodesByShowID returns all episodes for a given show, sorted by season and episode.
func (r *MediaRepository) GetEpisodesByShowID(ctx context.Context, showID string) ([]model.MediaItem, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, type, title, season, episode, duration, overview, poster_path, provider_id, library_id, status
		FROM media_items WHERE parent_id = ? AND type = 'episode'
		ORDER BY season ASC, episode ASC`, showID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var items []model.MediaItem
	for rows.Next() {
		var m model.MediaItem
		var season, episode int
		if err := rows.Scan(&m.ID, &m.Type, &m.Title, &season, &episode, &m.Duration, &m.Overview, &m.PosterPath, &m.ProviderID, &m.LibraryID, &m.Status); err != nil {
			return nil, err
		}
		m.Season = IntPtr(season)
		m.Episode = IntPtr(episode)
		items = append(items, m)
	}
	return items, rows.Err()
}
