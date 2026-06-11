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

	if query != "" {
		pattern := "%" + query + "%"
		whereClauses = append(whereClauses, "(title LIKE ? OR sort_title LIKE ?)")
		whereArgs = append(whereArgs, pattern, pattern)
	}

	if allowedLibraryIDs != nil {
		placeholders := make([]string, len(allowedLibraryIDs))
		for i, id := range allowedLibraryIDs {
			placeholders[i] = "?"
			whereArgs = append(whereArgs, id)
		}
		whereClauses = append(whereClauses, fmt.Sprintf("library_id IN (%s)", strings.Join(placeholders, ",")))
	}

	if hideMissing {
		whereClauses = append(whereClauses, "status = 'available'")
	}

	var where string
	if len(whereClauses) > 0 {
		where = " WHERE " + strings.Join(whereClauses, " AND ")
	}

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

	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM media_items%s", where)
	if err := r.db.QueryRowContext(ctx, countQuery, whereArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	dataQuery := fmt.Sprintf(`SELECT %s FROM media_items%s
		ORDER BY %s LIMIT ? OFFSET ?`, mediaColumns, where, orderBy)
	dataArgs := append(whereArgs, limit, offset)

	rows, err := r.db.QueryContext(ctx, dataQuery, dataArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	var items []model.MediaItem
	for rows.Next() {
		var m model.MediaItem
		if err := scanMediaItem(rows, &m); err != nil {
			return nil, 0, err
		}
		items = append(items, m)
	}
	return items, total, rows.Err()
}
