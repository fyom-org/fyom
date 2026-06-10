package repository

import (
	"context"
	"database/sql"
)

// LibraryPermissionRepository manages per-user library access control.
type LibraryPermissionRepository struct {
	db *DB
}

// NewLibraryPermissionRepository creates a new LibraryPermissionRepository.
func NewLibraryPermissionRepository(db *DB) *LibraryPermissionRepository {
	return &LibraryPermissionRepository{db: db}
}

// UserLibraryPermission represents a single user-library permission with joined names.
type UserLibraryPermission struct {
	UserID      string `json:"user_id" db:"user_id"`
	Username    string `json:"username" db:"username"`
	LibraryID   string `json:"library_id" db:"library_id"`
	LibraryName string `json:"library_name" db:"library_name"`
	CanView     bool   `json:"can_view" db:"can_view"`
}

// GetUserLibraries returns the list of library IDs the user can view.
func (r *LibraryPermissionRepository) GetUserLibraries(ctx context.Context, userID string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT library_id FROM library_permissions WHERE user_id = ? AND can_view = 1", userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// CanViewLibrary checks whether a user can view a specific library.
func (r *LibraryPermissionRepository) CanViewLibrary(ctx context.Context, userID, libraryID string) (bool, error) {
	var canView int
	err := r.db.QueryRowContext(ctx, "SELECT can_view FROM library_permissions WHERE user_id = ? AND library_id = ?", userID, libraryID).Scan(&canView)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return canView == 1, nil
}

// SetPermission grants or revokes a user's access to a library.
func (r *LibraryPermissionRepository) SetPermission(ctx context.Context, userID, libraryID string, canView bool) error {
	cv := 0
	if canView {
		cv = 1
	}
	_, err := r.db.ExecContext(ctx,
		"INSERT INTO library_permissions (user_id, library_id, can_view) VALUES (?, ?, ?) ON CONFLICT(user_id, library_id) DO UPDATE SET can_view = excluded.can_view",
		userID, libraryID, cv)
	return err
}

// RemovePermission removes a user-library permission entry.
func (r *LibraryPermissionRepository) RemovePermission(ctx context.Context, userID, libraryID string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM library_permissions WHERE user_id = ? AND library_id = ?", userID, libraryID)
	return err
}

// GetUserPermissions returns a map of library_id -> can_view for a user.
func (r *LibraryPermissionRepository) GetUserPermissions(ctx context.Context, userID string) (map[string]bool, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT library_id, can_view FROM library_permissions WHERE user_id = ?", userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	perms := make(map[string]bool)
	for rows.Next() {
		var libID string
		var canView int
		if err := rows.Scan(&libID, &canView); err != nil {
			return nil, err
		}
		perms[libID] = canView == 1
	}
	return perms, rows.Err()
}

// GetAllPermissions returns all permissions with user and library names.
func (r *LibraryPermissionRepository) GetAllPermissions(ctx context.Context) ([]UserLibraryPermission, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT lp.user_id, u.username, lp.library_id, l.name as library_name, lp.can_view
		FROM library_permissions lp
		JOIN users u ON u.id = lp.user_id
		JOIN libraries l ON l.id = lp.library_id
		ORDER BY u.username, l.name`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var perms []UserLibraryPermission
	for rows.Next() {
		var p UserLibraryPermission
		var canView int
		if err := rows.Scan(&p.UserID, &p.Username, &p.LibraryID, &p.LibraryName, &canView); err != nil {
			return nil, err
		}
		p.CanView = canView == 1
		perms = append(perms, p)
	}
	return perms, rows.Err()
}

// GrantAllLibraries grants a user access to all existing libraries.
func (r *LibraryPermissionRepository) GrantAllLibraries(ctx context.Context, userID string) error {
	_, err := r.db.ExecContext(ctx,
		"INSERT OR IGNORE INTO library_permissions (user_id, library_id, can_view) SELECT ?, id, 1 FROM libraries",
		userID)
	return err
}

// GrantNewLibrary grants all users access to a new library.
func (r *LibraryPermissionRepository) GrantNewLibrary(ctx context.Context, libraryID string) error {
	_, err := r.db.ExecContext(ctx,
		"INSERT OR IGNORE INTO library_permissions (user_id, library_id, can_view) SELECT id, ?, 1 FROM users",
		libraryID)
	return err
}
