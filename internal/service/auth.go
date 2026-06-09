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
	userRepo  *repository.UserRepository
	jwtSecret string
	tokenTTL  time.Duration
}

// NewAuthService creates a new AuthService.
func NewAuthService(userRepo *repository.UserRepository, jwtSecret string, tokenTTLHours int) *AuthService {
	return &AuthService{
		userRepo:  userRepo,
		jwtSecret: jwtSecret,
		tokenTTL:  time.Duration(tokenTTLHours) * time.Hour,
	}
}

func bytesToStr(b []byte) string {
	return string(b)
}

// Register creates a new user with a bcrypt-hashed password.
func (s *AuthService) Register(ctx context.Context, username, password string) (*model.User, error) {
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

	// Hash password with bcrypt
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrInternal)
	}

	return s.createUserWithHashedPassword(ctx, username, hashedBytes)
}

// createUserWithHashedPassword stores a new user with a pre-hashed password.
func (s *AuthService) createUserWithHashedPassword(ctx context.Context, username string, hashedBytes []byte) (*model.User, error) {
	user := &model.User{Username: username, Role: "user"}
	user.Password = bytesToStr(hashedBytes)

	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, errors.Wrap(err, errors.ErrInternal)
	}

	user.Password = ""
	return user, nil
}

// Login validates credentials and returns a JWT token string.
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
