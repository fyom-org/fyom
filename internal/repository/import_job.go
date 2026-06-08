package repository

import (
	"context"
	"database/sql"
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
func (r *ImportJobRepository) Create(ctx context.Context, sourcePath string) (*model.ImportJob, error) {
	job := &model.ImportJob{
		ID:         uuid.New().String(),
		SourcePath: sourcePath,
		Status:     "pending",
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
		UpdatedAt:  time.Now().UTC().Format(time.RFC3339),
	}
	_, err := r.db.ExecContext(ctx,
		"INSERT INTO import_jobs (id, source_path, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		job.ID, job.SourcePath, job.Status, job.CreatedAt, job.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return job, nil
}

// Get returns a single import job by ID.
func (r *ImportJobRepository) Get(ctx context.Context, id string) (*model.ImportJob, error) {
	var j model.ImportJob
	var errorMsg sql.NullString
	err := r.db.QueryRowContext(ctx,
		"SELECT id, source_path, status, total_items, done_items, error_msg, created_at, updated_at FROM import_jobs WHERE id = ?", id,
	).Scan(&j.ID, &j.SourcePath, &j.Status, &j.TotalItems, &j.DoneItems, &errorMsg, &j.CreatedAt, &j.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if errorMsg.Valid {
		j.ErrorMsg = errorMsg.String
	}
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
