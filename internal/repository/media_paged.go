package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/fyom/fyom/internal/model"
)

// ListPaged returns a paginated, filtered, and sorted list of media items
// and the total count matching the filters.
// allowedLibraryIDs: nil means no filter (admin), empty means no results, non-empty filters by library_id IN (...).
func (r *MediaRepository) ListPaged(ctx context.Context, page, limit int, mediaType string, query string, sort string, allowedLibraryIDs []string, hideMissing bool) ([]model.MediaItem, int, error) {
	var whereClauses []string
	var whereArgs []interface{}

	// Type filter.
	if mediaType != "" {
		if strings.Contains(mediaType, ",") {
			types := strings.Split(mediaType, ",")
			placeholders := make([]string, len(types))
			for i, t := range types {
				placeholders[i] = "?"
				whereArgs = append(whereArgs, strings.TrimSpace(t))
			}
			whereClauses = append(whereClauses, fmt.Sprintf("type IN (%s)", strings.Join(placeholders, ",")))
		} else {
			whereClauses = append(whereClauses, "type = ?")
			whereArgs = append(whereArgs, mediaType)
		}
	}

	// Search filter (case-insensitive LIKE on title and sort_title).
	if query != "" {
		pattern := "%" + query + "%"
		whereClauses = append(whereClauses, "(title LIKE ? OR sort_title LIKE ?)")
		whereArgs = append(whereArgs, pattern, pattern)
	}

	// Library access filter.
	if allowedLibraryIDs != nil {
		placeholders := make([]string, len(allowedLibraryIDs))
		for i, id := range allowedLibraryIDs {
			placeholders[i] = "?"
			whereArgs = append(whereArgs, id)
		}
		whereClauses = append(whereClauses, fmt.Sprintf("library_id IN (%s)", strings.Join(placeholders, ",")))
	}

	// Status filter: hide missing items from user-facing endpoints.
	if hideMissing {
		whereClauses = append(whereClauses, "status = 'available'")
	}

	// Build WHERE string.
	var where string
	if len(whereClauses) > 0 {
		where = " WHERE " + strings.Join(whereClauses, " AND ")
	}

	// ORDER BY — CASE WHEN pushes NULL/zero/empty to the bottom.
	orderBy := "CASE WHEN sort_title IS NULL OR sort_title = '' THEN title ELSE sort_title END ASC, title ASC"
	switch sort {
	case "title_desc":
		orderBy = "sort_title DESC, title DESC"
	case "year_asc":
		orderBy = "CASE WHEN year IS NULL OR year = 0 THEN 1 ELSE 0 END, year ASC, title ASC"
	case "year_desc":
		orderBy = "CASE WHEN year IS NULL OR year = 0 THEN 1 ELSE 0 END, year DESC, title ASC"
	case "rating_desc":
		orderBy = "CASE WHEN rating IS NULL OR rating = 0 THEN 1 ELSE 0 END, rating DESC, title ASC"
	case "created_desc":
		orderBy = "created_at DESC, title ASC"
	}

	// Count query — uses identical WHERE clause.
	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM media_items%s", where)
	if err := r.db.QueryRowContext(ctx, countQuery, whereArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Data query — same WHERE, with ORDER BY and pagination.
	offset := (page - 1) * limit
	dataQuery := fmt.Sprintf(`SELECT id, type, title, sort_title, year, overview, rating, duration,
		file_path, poster_path, backdrop_path, parent_id, season, episode,
		metadata_source, provider_id, library_id, status, created_at, updated_at FROM media_items%s
		ORDER BY %s LIMIT ? OFFSET ?`, where, orderBy)
	dataArgs := append(whereArgs, limit, offset)

	rows, err := r.db.QueryContext(ctx, dataQuery, dataArgs...)
	if err != nil {
		return nil, 0, err
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
			return nil, 0, err
		}
		m.Season = IntPtr(season)
		m.Episode = IntPtr(episode)
		items = append(items, m)
	}
	return items, total, rows.Err()
}
