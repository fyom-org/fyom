package service

import (
	"context"
	stderrors "errors"
	"testing"

	"github.com/fyom/fyom/internal/repository"
	"github.com/fyom/fyom/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupAuthTest(t *testing.T) (*AuthService, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	db, err := repository.Open(tmpDir, 5, 2, 60)
	require.NoError(t, err)

	userRepo := repository.NewUserRepository(db)
	libPermRepo := repository.NewLibraryPermissionRepository(db)
	svc := NewAuthService(userRepo, libPermRepo, "test-secret-key", 24)

	cleanup := func() { _ = db.Close() }
	return svc, cleanup
}

func TestAuthService_Register(t *testing.T) {
	svc, cleanup := setupAuthTest(t)
	defer cleanup()
	ctx := context.Background()

	// Successful registration
	user, err := svc.Register(ctx, "alice", "password123")
	require.NoError(t, err)
	assert.NotEmpty(t, user.ID)
	assert.Equal(t, "alice", user.Username)
	// First user is auto-promoted to admin
	assert.Equal(t, "admin", user.Role)
	assert.Empty(t, user.Password, "password should not be returned")

	// Duplicate username — should return a conflict error
	_, err = svc.Register(ctx, "alice", "differentpass")
	assert.Error(t, err)
	var appErr *errors.AppError
	if stderrors.As(err, &appErr) {
		assert.Equal(t, 409, appErr.Code)
	}
	// Should be a conflict error
}

func TestAuthService_Register_Validation(t *testing.T) {
	svc, cleanup := setupAuthTest(t)
	defer cleanup()
	ctx := context.Background()

	// Empty username
	_, err := svc.Register(ctx, "", "password123")
	assert.Error(t, err)

	// Empty password
	_, err = svc.Register(ctx, "bob", "")
	assert.Error(t, err)
}

func TestAuthService_Login(t *testing.T) {
	svc, cleanup := setupAuthTest(t)
	defer cleanup()
	ctx := context.Background()

	// Register first
	_, err := svc.Register(ctx, "charlie", "mypassword")
	require.NoError(t, err)

	// Successful login
	token, user, err := svc.Login(ctx, "charlie", "mypassword")
	require.NoError(t, err)
	assert.NotEmpty(t, token)
	assert.Equal(t, "charlie", user.Username)
	assert.Empty(t, user.Password)

	// Wrong password
	_, _, err = svc.Login(ctx, "charlie", "wrongpassword")
	assert.Error(t, err)

	// Non-existent user
	_, _, err = svc.Login(ctx, "nobody", "password")
	assert.Error(t, err)
}

func TestAuthService_Login_WrongPassword(t *testing.T) {
	svc, cleanup := setupAuthTest(t)
	defer cleanup()
	ctx := context.Background()

	_, err := svc.Register(ctx, "dave", "correcthorse")
	require.NoError(t, err)

	_, _, err = svc.Login(ctx, "dave", "wrong")
	assert.Error(t, err)
}
