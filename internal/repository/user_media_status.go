package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/fyom/fyom/internal/model"
)

// UserMediaStatusRepository manages user media status (watching, want_to_watch, etc.).
type UserMediaStatusRepository struct {
	db *DB
}

// NewUserMediaStatusRepository creates a new UserMediaStatusRepository.
func NewUserMediaStatusRepository(db *DB) *UserMediaStatusRepository {
	return &UserMediaStatusRepository{db: db}
}

var validStatuses = map[string]bool{
	"none":           true,
	"want_to_watch":  true,
	"watching":       true,
	"watched":        true,
	"dropped":        true,
}

// SetStatus sets the user's status for a media item.
// Validates status is one of: none, want_to_watch, watching, watched, dropped.
func (r *UserMediaStatusRepository) SetStatus(ctx context.Context, userID, mediaItemID, status string) error {
	if !validStatuses[status] {
		return fmt.Errorf("invalid status: %s", status)
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO user_media_status (user_id, media_item_id, status, created_at, updated_at)
		VALUES (?, ?, ?, datetime('now'), datetime('now'))
		ON CONFLICT(user_id, media_item_id) DO UPDATE SET
			status = excluded.status,
			updated_at = datetime('now')
	`, userID, mediaItemID, status)
	return err
}

// GetStatus returns the user's status for a media item.
// Returns "none" if no row found (not an error).
func (r *UserMediaStatusRepository) GetStatus(ctx context.Context, userID, mediaItemID string) (string, error) {
	var status string
	err := r.db.QueryRowContext(ctx,
		"SELECT status FROM user_media_status WHERE user_id = ? AND media_item_id = ?",
		userID, mediaItemID,
	).Scan(&status)
	if err != nil {
		return "none", nil
	}
	return status, nil
}

// GetItemsByStatus returns media items with the given status for the user.
// Joins with media_items to get full item data, filtered to available items only.
func (r *UserMediaStatusRepository) GetItemsByStatus(ctx context.Context, userID, status string, limit int) ([]model.MediaItem, error) {
	query := `SELECT m.id, m.type, m.title, m.sort_title, m.year, m.overview, m.rating, m.duration,
		m.file_path, m.poster_path, m.backdrop_path, m.parent_id, m.season, m.episode,
		m.metadata_source, m.provider_id, m.library_id, m.status, m.created_at, m.updated_at
		FROM media_items m
		JOIN user_media_status s ON s.media_item_id = m.id
		WHERE s.user_id = ? AND s.status = ? AND m.status = 'available'
		ORDER BY s.updated_at DESC
		LIMIT ?`
	rows, err := r.db.QueryContext(ctx, query, userID, status, limit)
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

// GetStatusesForItems returns a map of media_item_id -> status for the given item IDs.
// Items not in the map have status "none".
func (r *UserMediaStatusRepository) GetStatusesForItems(ctx context.Context, userID string, itemIDs []string) (map[string]string, error) {
	if len(itemIDs) == 0 {
		return map[string]string{}, nil
	}
	placeholders := make([]string, len(itemIDs))
	args := make([]interface{}, 0, len(itemIDs)+1)
	args = append(args, userID)
	for i, id := range itemIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	query := fmt.Sprintf(
		"SELECT media_item_id, status FROM user_media_status WHERE user_id = ? AND media_item_id IN (%s)",
		strings.Join(placeholders, ","),
	)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	result := make(map[string]string, len(itemIDs))
	for rows.Next() {
		var itemID, status string
		if err := rows.Scan(&itemID, &status); err != nil {
			return nil, err
		}
		result[itemID] = status
	}
	return result, rows.Err()
}