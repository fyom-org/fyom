package repository

import "context"

// SystemSettingRepository provides access to system_settings.
type SystemSettingRepository struct {
	db *DB
}

// NewSystemSettingRepository creates a new SystemSettingRepository.
func NewSystemSettingRepository(db *DB) *SystemSettingRepository {
	return &SystemSettingRepository{db: db}
}

// GetSetting returns the value for a given system setting key.
func (r *SystemSettingRepository) GetSetting(ctx context.Context, key string) (string, error) {
	var value string
	err := r.db.QueryRowContext(ctx, "SELECT value FROM system_settings WHERE key = ?", key).Scan(&value)
	return value, err
}

// SetSetting updates or inserts a system setting.
func (r *SystemSettingRepository) SetSetting(ctx context.Context, key, value string) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO system_settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// List returns all system settings.
func (r *SystemSettingRepository) List(ctx context.Context) ([]struct {
	Key   string
	Value string
}, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT key, value FROM system_settings ORDER BY key ASC")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var results []struct {
		Key   string
		Value string
	}
	for rows.Next() {
		var kv struct {
			Key   string
			Value string
		}
		if err := rows.Scan(&kv.Key, &kv.Value); err != nil {
			return nil, err
		}
		results = append(results, kv)
	}
	return results, rows.Err()
}
