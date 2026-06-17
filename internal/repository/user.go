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
	return &UserRepository{
		db: db,
	}
}

// GetByUsername finds a user by username.
func (r *UserRepository) GetByUsername(ctx context.Context, username string) (*model.User, error) {
	row := r.db.QueryRowContext(
		ctx,
		`
        SELECT
            id,
            username,
            password,
            role,
            password_change_required,
            created_at,
            updated_at
        FROM users
        WHERE username = ?
        `,
		username,
	)

	return scanUser(row)
}

// GetByID finds a user by ID.
func (r *UserRepository) GetByID(ctx context.Context, id string) (*model.User, error) {
	row := r.db.QueryRowContext(
		ctx,
		`
        SELECT
            id,
            username,
            password,
            role,
            password_change_required,
            created_at,
            updated_at
        FROM users
        WHERE id = ?
        `,
		id,
	)

	return scanUser(row)
}

// FindBootstrapUser returns the desktop bootstrap admin user, if one exists.
//
// A bootstrap user is intentionally narrow:
//   - password_change_required must be true
//   - role must be admin or owner
//
// This endpoint powers localhost-only desktop bootstrap. Once the user changes
// their password, password_change_required becomes false and this query returns
// nil naturally.
func (r *UserRepository) FindBootstrapUser(ctx context.Context) (*model.User, error) {
	row := r.db.QueryRowContext(
		ctx,
		`
        SELECT
            id,
            username,
            password,
            role,
            password_change_required,
            created_at,
            updated_at
        FROM users
        WHERE password_change_required = ?
          AND role IN ('admin', 'owner')
        ORDER BY created_at ASC, id ASC
        LIMIT 1
        `,
		true,
	)

	return scanUser(row)
}

// Create inserts a new user.
func (r *UserRepository) Create(ctx context.Context, u *model.User) error {
	if u == nil {
		return sql.ErrNoRows
	}

	if u.ID == "" {
		u.ID = uuid.New().String()
	}

	if u.Role == "" {
		u.Role = "user"
	}

	now := nowString()

	if u.CreatedAt == "" {
		u.CreatedAt = now
	}

	u.UpdatedAt = now

	_, err := r.db.ExecContext(
		ctx,
		`
        INSERT INTO users (
            id,
            username,
            password,
            role,
            password_change_required,
            created_at,
            updated_at
        )
        VALUES (?, ?, ?, ?, ?, ?, ?)
        `,
		u.ID,
		u.Username,
		u.Password,
		u.Role,
		u.PasswordChangeRequired,
		u.CreatedAt,
		u.UpdatedAt,
	)

	return err
}

// Count returns the total number of users.
func (r *UserRepository) Count(ctx context.Context) (int, error) {
	var count int

	err := r.db.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM users",
	).Scan(&count)

	return count, err
}

// UpdatePassword updates a user's password hash.
func (r *UserRepository) UpdatePassword(ctx context.Context, id string, hashedPassword string) error {
	_, err := r.db.ExecContext(
		ctx,
		`
        UPDATE users
        SET password = ?,
            updated_at = ?
        WHERE id = ?
        `,
		hashedPassword,
		nowString(),
		id,
	)

	return err
}

// UpdatePasswordAndFlag updates a user's password hash and password_change_required flag.
func (r *UserRepository) UpdatePasswordAndFlag(
	ctx context.Context,
	id string,
	hashedPassword string,
	passwordChangeRequired bool,
) error {
	_, err := r.db.ExecContext(
		ctx,
		`
        UPDATE users
        SET password = ?,
            password_change_required = ?,
            updated_at = ?
        WHERE id = ?
        `,
		hashedPassword,
		passwordChangeRequired,
		nowString(),
		id,
	)

	return err
}

type userScanner interface {
	Scan(dest ...any) error
}

func scanUser(row userScanner) (*model.User, error) {
	var u model.User

	err := row.Scan(
		&u.ID,
		&u.Username,
		&u.Password,
		&u.Role,
		&u.PasswordChangeRequired,
		&u.CreatedAt,
		&u.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	return &u, nil
}

func nowString() string {
	return time.Now().UTC().Format(time.RFC3339)
}
