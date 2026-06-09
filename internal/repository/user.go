package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/fyom/fyom/internal/model"
	"github.com/google/uuid"
)

// UserRepository provides access to users.
type UserRepository struct {
	db *DB
}

// NewUserRepository creates a new UserRepository.
func NewUserRepository(db *DB) *UserRepository {
	return &UserRepository{db: db}
}

// GetByUsername finds a user by username.
func (r *UserRepository) GetByUsername(ctx context.Context, username string) (*model.User, error) {
	var u model.User
	err := r.db.QueryRowContext(ctx,
		"SELECT id, username, password, role, created_at, updated_at FROM users WHERE username = ?",
		username,
	).Scan(&u.ID, &u.Username, &u.Password, &u.Role, &u.CreatedAt, &u.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// Create inserts a new user.
func (r *UserRepository) Create(ctx context.Context, u *model.User) error {
	if u.ID == "" {
		u.ID = uuid.New().String()
	}
	now := time.Now().UTC().Format(time.RFC3339)
	u.CreatedAt = now
	u.UpdatedAt = now

	_, err := r.db.ExecContext(ctx,
		"INSERT INTO users (id, username, password, role, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		u.ID, u.Username, u.Password, u.Role, u.CreatedAt, u.UpdatedAt)
	return err
}

// Count returns the total number of users.
func (r *UserRepository) Count(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&count)
	return count, err
}

// GetByID finds a user by ID.
func (r *UserRepository) GetByID(ctx context.Context, id string) (*model.User, error) {
	var u model.User
	err := r.db.QueryRowContext(ctx,
		"SELECT id, username, password, role, created_at, updated_at FROM users WHERE id = ?",
		id,
	).Scan(&u.ID, &u.Username, &u.Password, &u.Role, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// UpdatePassword updates a user's password hash.
func (r *UserRepository) UpdatePassword(ctx context.Context, id, hashedPassword string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE users SET password = ? WHERE id = ?", hashedPassword, id)
	return err
}
