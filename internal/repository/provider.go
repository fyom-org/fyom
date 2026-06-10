package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/fyom/fyom/internal/model"
)

// ProviderRepository handles CRUD for provider configuration records.
type ProviderRepository struct{ db *DB }

// NewProviderRepository creates a new ProviderRepository.
func NewProviderRepository(db *DB) *ProviderRepository {
	return &ProviderRepository{db: db}
}

// ListEnabled returns all providers where enabled = 1, ordered by created_at ASC.
func (r *ProviderRepository) ListEnabled(ctx context.Context) ([]model.ProviderRecord, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, type, display_name, config, enabled, created_at, updated_at
		FROM providers WHERE enabled = 1 ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var records []model.ProviderRecord
	for rows.Next() {
		var p model.ProviderRecord
		if err := rows.Scan(&p.ID, &p.Type, &p.DisplayName, &p.Config, &p.Enabled, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		records = append(records, p)
	}
	return records, rows.Err()
}

// List returns all providers (enabled and disabled), ordered by created_at ASC.
func (r *ProviderRepository) List(ctx context.Context) ([]model.ProviderRecord, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, type, display_name, config, enabled, created_at, updated_at
		FROM providers ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var records []model.ProviderRecord
	for rows.Next() {
		var p model.ProviderRecord
		if err := rows.Scan(&p.ID, &p.Type, &p.DisplayName, &p.Config, &p.Enabled, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		records = append(records, p)
	}
	return records, rows.Err()
}

// GetByID returns a single provider. Returns (nil, nil) if not found.
func (r *ProviderRepository) GetByID(ctx context.Context, id string) (*model.ProviderRecord, error) {
	var p model.ProviderRecord
	err := r.db.QueryRowContext(ctx, `SELECT id, type, display_name, config, enabled, created_at, updated_at
		FROM providers WHERE id = ?`, id,
	).Scan(&p.ID, &p.Type, &p.DisplayName, &p.Config, &p.Enabled, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// Create inserts a new provider record.
func (r *ProviderRepository) Create(ctx context.Context, p *model.ProviderRecord) error {
	now := time.Now().UTC().Format(time.RFC3339)
	p.CreatedAt = now
	p.UpdatedAt = now

	_, err := r.db.ExecContext(ctx, `INSERT INTO providers
		(id, type, display_name, config, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Type, p.DisplayName, p.Config, p.Enabled, p.CreatedAt, p.UpdatedAt)
	return err
}

// Update replaces display_name, config, and enabled for an existing provider.
// Sets updated_at = now().
func (r *ProviderRepository) Update(ctx context.Context, p *model.ProviderRecord) error {
	now := time.Now().UTC().Format(time.RFC3339)
	p.UpdatedAt = now

	res, err := r.db.ExecContext(ctx, `UPDATE providers
		SET display_name = ?, config = ?, enabled = ?, updated_at = ?
		WHERE id = ?`,
		p.DisplayName, p.Config, p.Enabled, p.UpdatedAt, p.ID)
	if err != nil {
		return err
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("provider not found: %s", p.ID)
	}
	return nil
}

// Delete removes a provider record.
// Before deleting, checks that no media_items rows reference this provider_id.
// If any exist, returns a descriptive error.
func (r *ProviderRepository) Delete(ctx context.Context, id string) error {
	var count int
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM media_items WHERE provider_id = ?", id).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("cannot delete provider %q: %d media item(s) still reference it", id, count)
	}

	res, err := r.db.ExecContext(ctx, "DELETE FROM providers WHERE id = ?", id)
	if err != nil {
		return err
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return fmt.Errorf("provider not found: %s", id)
	}
	return nil
}
