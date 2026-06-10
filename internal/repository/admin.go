package repository

import (
	"context"
	"database/sql"
	"strings"

	"github.com/fyom/fyom/internal/model"
)

// AdminStats holds aggregate system statistics for the admin dashboard.
type AdminStats struct {
	Library  LibraryStats  `json:"library"`
	Providers ProviderStats `json:"providers"`
	Users    UserStats    `json:"users"`
	Storage  map[string]int `json:"storage"`
	Imports  ImportStats  `json:"imports"`
}

type LibraryStats struct {
	TotalItems int `json:"total_items"`
	Movies     int `json:"movies"`
	Shows      int `json:"shows"`
	Episodes   int `json:"episodes"`
}

type ProviderStats struct {
	Total   int      `json:"total"`
	Enabled int      `json:"enabled"`
	Types   []string `json:"types"`
}

type UserStats struct {
	Total  int `json:"total"`
	Admins int `json:"admins"`
}

type ImportStats struct {
	Total   int `json:"total"`
	Running int `json:"running"`
	Done    int `json:"done"`
	Error   int `json:"error"`
}

// AdminRepository provides aggregate statistics for the admin dashboard.
type AdminRepository struct {
	db *DB
}

// NewAdminRepository creates a new AdminRepository.
func NewAdminRepository(db *DB) *AdminRepository {
	return &AdminRepository{db: db}
}

// GetStats returns aggregate system statistics.
func (r *AdminRepository) GetStats(ctx context.Context) (*AdminStats, error) {
	stats := &AdminStats{
		Storage: make(map[string]int),
	}

	// Library stats — count by type.
	rows, err := r.db.QueryContext(ctx, "SELECT type, COUNT(*) FROM media_items GROUP BY type")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var total int
	for rows.Next() {
		var itemType string
		var count int
		if err := rows.Scan(&itemType, &count); err != nil {
			return nil, err
		}
		total += count
		switch itemType {
		case "movie":
			stats.Library.Movies = count
		case "show":
			stats.Library.Shows = count
		case "episode":
			stats.Library.Episodes = count
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	stats.Library.TotalItems = total

	// Provider stats — total, enabled, distinct types.
	var enabled int
	var typesSQL sql.NullString
	err = r.db.QueryRowContext(ctx,
		"SELECT COUNT(*), COUNT(CASE WHEN enabled = 1 THEN 1 END), GROUP_CONCAT(DISTINCT type) FROM providers",
	).Scan(&stats.Providers.Total, &enabled, &typesSQL)
	if err != nil {
		return nil, err
	}
	stats.Providers.Enabled = enabled
	if typesSQL.Valid && typesSQL.String != "" {
		stats.Providers.Types = strings.Split(typesSQL.String, ",")
	} else {
		stats.Providers.Types = []string{}
	}

	// User stats — total and admin count.
	err = r.db.QueryRowContext(ctx,
		"SELECT COUNT(*), COUNT(CASE WHEN role = 'admin' THEN 1 END) FROM users",
	).Scan(&stats.Users.Total, &stats.Users.Admins)
	if err != nil {
		return nil, err
	}

	// Storage distribution — count by provider_id.
	srows, err := r.db.QueryContext(ctx, "SELECT provider_id, COUNT(*) FROM media_items GROUP BY provider_id")
	if err != nil {
		return nil, err
	}
	defer func() { _ = srows.Close() }()

	for srows.Next() {
		var providerID string
		var count int
		if err := srows.Scan(&providerID, &count); err != nil {
			return nil, err
		}
		stats.Storage[providerID] = count
	}
	if err := srows.Err(); err != nil {
		return nil, err
	}

	// Import job stats.
	var running, done, importErr int
	err = r.db.QueryRowContext(ctx,
		"SELECT COUNT(*), COUNT(CASE WHEN status = 'running' THEN 1 END), COUNT(CASE WHEN status = 'done' THEN 1 END), COUNT(CASE WHEN status = 'error' THEN 1 END) FROM import_jobs",
	).Scan(&stats.Imports.Total, &running, &done, &importErr)
	if err != nil {
		return nil, err
	}
	stats.Imports.Running = running
	stats.Imports.Done = done
	stats.Imports.Error = importErr

	return stats, nil
}

// ListJobs returns a paginated list of import jobs sorted by created_at DESC.
func (r *AdminRepository) ListJobs(ctx context.Context, page, limit int) ([]model.ImportJob, int, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 10
	}
	offset := (page - 1) * limit

	var total int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM import_jobs").Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.db.QueryContext(ctx,
		"SELECT id, source_path, status, total_items, done_items, error_msg, created_at, updated_at FROM import_jobs ORDER BY created_at DESC LIMIT ? OFFSET ?",
		limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	var jobs []model.ImportJob
	for rows.Next() {
		var j model.ImportJob
		var errorMsg sql.NullString
		if err := rows.Scan(&j.ID, &j.SourcePath, &j.Status, &j.TotalItems, &j.DoneItems, &errorMsg, &j.CreatedAt, &j.UpdatedAt); err != nil {
			return nil, 0, err
		}
		if errorMsg.Valid {
			j.ErrorMsg = errorMsg.String
		}
		jobs = append(jobs, j)
	}
	return jobs, total, rows.Err()
}
