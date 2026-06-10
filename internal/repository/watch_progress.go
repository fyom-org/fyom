package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/fyom/fyom/internal/model"
)

// WatchProgress represents a user's playback position for a media item.
type WatchProgress struct {
	UserID      string `db:"user_id"`
	MediaItemID string `db:"media_item_id"`
	Position    int    `db:"position"`
	Duration    int    `db:"duration"`
	Finished    bool   `db:"finished"`
	UpdatedAt   string `db:"updated_at"`
}

// MediaItemWithProgress joins media_items with watch_progress.
type MediaItemWithProgress struct {
	model.MediaItem
	Position int  `db:"position"`
	Duration int  `db:"duration"`
	Finished bool `db:"finished"`
}

// UpsertProgress inserts or updates watch progress for a user/media pair.
func (r *MediaRepository) UpsertProgress(ctx context.Context, userID, mediaItemID string, position, duration int, finished bool) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := r.db.ExecContext(ctx, `INSERT INTO watch_progress
		(user_id, media_item_id, position, duration, finished, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, media_item_id) DO UPDATE SET
			position = excluded.position,
			duration = excluded.duration,
			finished = excluded.finished,
			updated_at = excluded.updated_at`,
		userID, mediaItemID, position, duration, boolToInt(finished), now)
	return err
}

// GetProgress returns watch progress for a user/media pair.
func (r *MediaRepository) GetProgress(ctx context.Context, userID, mediaItemID string) (*WatchProgress, error) {
	var wp WatchProgress
	err := r.db.QueryRowContext(ctx, `SELECT user_id, media_item_id, position, duration, finished, updated_at
		FROM watch_progress WHERE user_id = ? AND media_item_id = ?`, userID, mediaItemID,
	).Scan(&wp.UserID, &wp.MediaItemID, &wp.Position, &wp.Duration, &wp.Finished, &wp.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &wp, nil
}

// GetContinueWatching returns media items with unfinished progress for a user.
func (r *MediaRepository) GetContinueWatching(ctx context.Context, userID string, limit int) ([]MediaItemWithProgress, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT m.id, m.type, m.title, m.sort_title, m.year, m.overview,
		m.rating, m.duration, m.file_path, m.poster_path, m.backdrop_path, m.parent_id,
		m.season, m.episode, m.metadata_source, m.provider_id, m.library_id, m.created_at, m.updated_at,
		w.position, w.duration, w.finished
		FROM watch_progress w
		JOIN media_items m ON m.id = w.media_item_id
		WHERE w.user_id = ? AND w.finished = 0 AND w.position > 0
		ORDER BY w.updated_at DESC
		LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var items []MediaItemWithProgress
	for rows.Next() {
		var item MediaItemWithProgress
		var season, episode int
		if err := rows.Scan(
			&item.ID, &item.Type, &item.Title, &item.SortTitle, &item.Year, &item.Overview,
			&item.Rating, &item.Duration, &item.FilePath, &item.PosterPath, &item.BackdropPath,
			&item.ParentID, &season, &episode, &item.MetadataSource, &item.ProviderID,
			&item.LibraryID, &item.CreatedAt, &item.UpdatedAt,
			&item.Position, &item.Duration, &item.Finished,
		); err != nil {
			return nil, err
		}
		item.Season = IntPtr(season)
		item.Episode = IntPtr(episode)
		items = append(items, item)
	}
	return items, rows.Err()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
