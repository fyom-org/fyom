package repository

import (
	"context"
	"fmt"

	"github.com/fyom/fyom/internal/model"
)

// ListPaged returns a paginated list of media items and the total count.
func (r *MediaRepository) ListPaged(ctx context.Context, mediaType string, page, pageSize int) ([]model.MediaItem, int, error) {
	where := ""
	args := []interface{}{}

	if mediaType != "" {
		where = " WHERE type = ?"
		args = append(args, mediaType)
	}

	// Get total count
	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM media_items%s", where)
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Get paginated items
	offset := (page - 1) * pageSize
	query := fmt.Sprintf(`SELECT id, type, title, sort_title, year, overview, rating, duration,
		file_path, poster_path, backdrop_path, parent_id, season, episode,
		metadata_source, created_at, updated_at FROM media_items%s
		ORDER BY sort_title ASC, title ASC LIMIT ? OFFSET ?`, where)
	args = append(args, pageSize, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []model.MediaItem
	for rows.Next() {
		var m model.MediaItem
		if err := rows.Scan(&m.ID, &m.Type, &m.Title, &m.SortTitle, &m.Year,
			&m.Overview, &m.Rating, &m.Duration, &m.FilePath, &m.PosterPath,
			&m.BackdropPath, &m.ParentID, &m.Season, &m.Episode,
			&m.MetadataSource, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, 0, err
		}
		items = append(items, m)
	}
	return items, total, rows.Err()
}
