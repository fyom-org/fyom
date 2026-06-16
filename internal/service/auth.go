// Package service implements the business logic layer for fyom.
package service

import (
	"context"
	"time"

	"github.com/fyom/fyom/internal/model"
	"github.com/fyom/fyom/internal/repository"
	"github.com/fyom/fyom/pkg/errors"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// AuthService handles authentication business logic.
type AuthService struct {
	userRepo    *repository.UserRepository
	libPermRepo *repository.LibraryPermissionRepository
	jwtSecret   string
	tokenTTL    time.Duration
}

// NewAuthService creates a new AuthService.
func NewAuthService(userRepo *repository.UserRepository, libPermRepo *repository.LibraryPermissionRepository, jwtSecret string, tokenTTLHours int) *AuthService {
	return &AuthService{
		userRepo:    userRepo,
		libPermRepo: libPermRepo,
		jwtSecret:   jwtSecret,
		tokenTTL:    time.Duration(tokenTTLHours) * time.Hour,
	}
}

func bytesToStr(b []byte) string {
	return string(b)
}

// Register creates a new user with a bcrypt-hashed password.
// The first user (count == 0) is automatically assigned the "admin" role.
func (s *AuthService) Register(ctx context.Context, username, password string) (*model.User, error) {
	return s.RegisterWithFlag(ctx, username, password, false)
}

// RegisterWithFlag creates a new user with explicit password_change_required flag.
func (s *AuthService) RegisterWithFlag(ctx context.Context, username, password string, passwordChangeRequired bool) (*model.User, error) {
	if username == "" || password == "" {
		return nil, errors.Wrap(nil, errors.ErrValidation)
	}

	// Check if user already exists
	existing, err := s.userRepo.GetByUsername(ctx, username)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrInternal)
	}
	if existing != nil {
		return nil, errors.Wrap(nil, errors.ErrConflict)
	}

	// Determine role: first user becomes admin
	count, err := s.userRepo.Count(ctx)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrInternal)
	}
	role := "user"
	if count == 0 {
		role = "admin"
	}

	// Hash password with bcrypt
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrInternal)
	}

	return s.createUserWithRoleAndFlag(ctx, username, hashedBytes, role, passwordChangeRequired)
}

// createUserWithRoleAndFlag stores a new user with a pre-hashed password, explicit role, and password_change_required flag.
func (s *AuthService) createUserWithRoleAndFlag(ctx context.Context, username string, hashedBytes []byte, role string, passwordChangeRequired bool) (*model.User, error) {
	user := &model.User{
		Username:              username,
		Role:                  role,
		PasswordChangeRequired: passwordChangeRequired,
	}
	user.Password = bytesToStr(hashedBytes)

	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, errors.Wrap(err, errors.ErrInternal)
	}

	// Auto-grant access to all existing libraries.
	if s.libPermRepo != nil {
		_ = s.libPermRepo.GrantAllLibraries(ctx, user.ID)
	}

	user.Password = ""
	return user, nil
}

// Login validates credentials and returns a JWT token string and the user.
func (s *AuthService) Login(ctx context.Context, username, password string) (string, *model.User, error) {
	user, err := s.userRepo.GetByUsername(ctx, username)
	if err != nil {
		return "", nil, errors.Wrap(err, errors.ErrInternal)
	}
	if user == nil {
		return "", nil, &errors.AppError{Code: 401, Message: "invalid credentials"}
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return "", nil, &errors.AppError{Code: 401, Message: "invalid credentials"}
	}

	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":      user.ID,
		"username": user.Username,
		"role":     user.Role,
		"iat":      now.Unix(),
		"exp":      now.Add(s.tokenTTL).Unix(),
	})

	tokenString, err := token.SignedString([]byte(s.jwtSecret))
	if err != nil {
		return "", nil, errors.Wrap(err, errors.ErrInternal)
	}

	// Never return the password hash
	user.Password = ""
	return tokenString, user, nil
}

// GetUserByID returns a user by ID (includes password hash for internal use).
func (s *AuthService) GetUserByID(ctx context.Context, id string) (*model.User, error) {
	return s.userRepo.GetByID(ctx, id)
}

// GetUserByUsername returns a user by username.
func (s *AuthService) GetUserByUsername(ctx context.Context, username string) (*model.User, error) {
	return s.userRepo.GetByUsername(ctx, username)
}

// UpdatePassword updates a user's password hash and optionally clears password_change_required.
func (s *AuthService) UpdatePassword(ctx context.Context, id, hashedPassword string, clearPasswordChangeRequired bool) error {
	if clearPasswordChangeRequired {
		return s.userRepo.UpdatePasswordAndFlag(ctx, id, hashedPassword, false)
	}
	return s.userRepo.UpdatePassword(ctx, id, hashedPassword)
}
