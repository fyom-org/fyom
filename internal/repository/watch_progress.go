package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
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
// allowedLibraryIDs: nil means no filter (admin), non-empty filters by m.library_id IN (...).
func (r *MediaRepository) GetContinueWatching(ctx context.Context, userID string, limit int, allowedLibraryIDs []string) ([]MediaItemWithProgress, error) {
	query := `SELECT m.id, m.type, m.title, m.sort_title, m.year, m.overview,
		m.rating, m.duration, m.file_path, m.poster_path, m.backdrop_path, m.parent_id,
		m.season, m.episode, m.metadata_source, m.provider_id, m.library_id, m.status, m.created_at, m.updated_at,
		m.mpaa, m.genres, m.studios, m.actors, m.unique_ids, m.premiered, m.outline, m.tagline,
		m.countries, m.directors, m.credits, m.tags, m.set_name, m.video_codec, m.video_width,
		m.video_height, m.video_duration_seconds, m.audio_codec, m.audio_channels,
		m.subtitle_languages,
		w.position, w.duration, w.finished
		FROM watch_progress w
		JOIN media_items m ON m.id = w.media_item_id
		WHERE w.user_id = ? AND w.finished = 0 AND w.position > 0`
	args := []interface{}{userID}

	if allowedLibraryIDs != nil {
		placeholders := make([]string, len(allowedLibraryIDs))
		for i, id := range allowedLibraryIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		query += fmt.Sprintf(" AND m.library_id IN (%s)", strings.Join(placeholders, ","))
	}

	query += " AND m.status = 'available'"
	query += " ORDER BY w.updated_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := r.db.QueryContext(ctx, query, args...)
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
			&item.LibraryID, &item.Status, &item.CreatedAt, &item.UpdatedAt,
			&item.MPAA, &item.Genres, &item.Studios, &item.Actors, &item.UniqueIDs,
			&item.Premiered, &item.Outline, &item.Tagline, &item.Countries, &item.Directors,
			&item.Credits, &item.Tags, &item.SetName, &item.VideoCodec, &item.VideoWidth,
			&item.VideoHeight, &item.VideoDurationSeconds, &item.AudioCodec, &item.AudioChannels,
			&item.SubtitleLanguages,
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
