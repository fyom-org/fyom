package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/fyom/fyom/internal/model"
	"github.com/google/uuid"
)

// ImportJobRepository provides access to import_jobs.
type ImportJobRepository struct {
	db *DB
}

// NewImportJobRepository creates a new ImportJobRepository.
func NewImportJobRepository(db *DB) *ImportJobRepository {
	return &ImportJobRepository{db: db}
}

// Create inserts a new import job.
func (r *ImportJobRepository) Create(ctx context.Context, sourcePath, libraryID string) (*model.ImportJob, error) {
	job := &model.ImportJob{
		ID:            uuid.New().String(),
		SourcePath:    sourcePath,
		Status:        "pending",
		LibraryID:     libraryID,
		ParseWarnings: []string{},
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
	}
	_, err := r.db.ExecContext(ctx,
		"INSERT INTO import_jobs (id, source_path, status, library_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		job.ID, job.SourcePath, job.Status, job.LibraryID, job.CreatedAt, job.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return job, nil
}

// Get returns a single import job by ID.
func (r *ImportJobRepository) Get(ctx context.Context, id string) (*model.ImportJob, error) {
	var j model.ImportJob
	var errorMsg sql.NullString
	var parseWarnings sql.NullString
	err := r.db.QueryRowContext(ctx,
		"SELECT id, source_path, status, total_items, done_items, library_id, error_msg, scanned_files, imported_items, updated_items, skipped_files, parse_warnings, duration_ms, created_at, updated_at FROM import_jobs WHERE id = ?", id,
	).Scan(
		&j.ID,
		&j.SourcePath,
		&j.Status,
		&j.TotalItems,
		&j.DoneItems,
		&j.LibraryID,
		&errorMsg,
		&j.ScannedFiles,
		&j.ImportedItems,
		&j.UpdatedItems,
		&j.SkippedFiles,
		&parseWarnings,
		&j.DurationMS,
		&j.CreatedAt,
		&j.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if errorMsg.Valid {
		j.ErrorMsg = errorMsg.String
	}
	j.ParseWarnings = parseWarningsFromDB(parseWarnings)
	return &j, nil
}

// UpdateProgress updates the job's progress counters and status.
func (r *ImportJobRepository) UpdateProgress(ctx context.Context, id string, total, done int, status string) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE import_jobs SET total_items = ?, done_items = ?, status = ?, updated_at = ? WHERE id = ?",
		total, done, status, time.Now().UTC().Format(time.RFC3339), id)
	return err
}

// UpdateError sets the job status to error with a message.
func (r *ImportJobRepository) UpdateError(ctx context.Context, id, msg string) error {
	_, err := r.db.ExecContext(ctx,
		"UPDATE import_jobs SET status = 'error', error_msg = ?, updated_at = ? WHERE id = ?",
		msg, time.Now().UTC().Format(time.RFC3339), id)
	return err
}

// UpdateSummary persists the final import summary on a completed job.
func (r *ImportJobRepository) UpdateSummary(ctx context.Context, id string, summary *model.ImportSummary) error {
	if summary == nil {
		return nil
	}

	warnings := summary.ParseWarnings
	if warnings == nil {
		warnings = []string{}
	}
	warningsJSON, err := json.Marshal(warnings)
	if err != nil {
		return err
	}
	if warningsJSON == nil {
		warningsJSON = []byte("[]")
	}

	_, err = r.db.ExecContext(ctx,
		"UPDATE import_jobs SET scanned_files = ?, imported_items = ?, updated_items = ?, skipped_files = ?, parse_warnings = ?, duration_ms = ?, updated_at = ? WHERE id = ?",
		summary.ScannedFiles,
		summary.ImportedItems,
		summary.UpdatedItems,
		summary.SkippedFiles,
		string(warningsJSON),
		summary.Duration.Milliseconds(),
		time.Now().UTC().Format(time.RFC3339),
		id,
	)
	return err
}

func parseWarningsFromDB(warnings sql.NullString) []string {
	if !warnings.Valid || warnings.String == "" {
		return []string{}
	}

	var parsed []string
	if err := json.Unmarshal([]byte(warnings.String), &parsed); err != nil {
		return []string{warnings.String}
	}
	if parsed == nil {
		return []string{}
	}
	return parsed
}
