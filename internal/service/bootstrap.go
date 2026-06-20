package service

import (
	"crypto/rand"
	"encoding/hex"
)

import (
	"context"

	"github.com/fyom/fyom/internal/repository"
	"github.com/fyom/fyom/pkg/errors"
)

// BootstrapMode represents the runtime mode for bootstrap.
type BootstrapMode string

const (
	// BootstrapModeServer creates an admin with a generated password for server mode.
	BootstrapModeServer BootstrapMode = "server"
	// BootstrapModeDesktop creates an admin with auto-auth for desktop mode.
	BootstrapModeDesktop BootstrapMode = "desktop"
)

// BootstrapResult holds the outcome of an initial bootstrap.
type BootstrapResult struct {
	Created           bool
	Mode              BootstrapMode
	Username          string
	GeneratedPassword string // only set in server mode
	SessionToken      string // only set in desktop mode
	UserID            string
}

// BootstrapService handles first-run admin creation.
type BootstrapService struct {
	authService *AuthService
	userRepo    *repository.UserRepository
	settingRepo *repository.SystemSettingRepository
}

// NewBootstrapService creates a new BootstrapService.
func NewBootstrapService(authService *AuthService, userRepo *repository.UserRepository, settingRepo *repository.SystemSettingRepository) *BootstrapService {
	return &BootstrapService{
		authService: authService,
		userRepo:    userRepo,
		settingRepo: settingRepo,
	}
}

// EnsureInitialBootstrap checks if the system has zero users and, if so,
// creates the initial admin. It is safe to call on every startup.
func (s *BootstrapService) EnsureInitialBootstrap(ctx context.Context, mode BootstrapMode) (*BootstrapResult, error) {
	count, err := s.userRepo.Count(ctx)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrInternal)
	}
	if count > 0 {
		return &BootstrapResult{Created: false}, nil
	}

	// Zero users — create initial admin.
	username := "admin"
	password := generateBootstrapPassword()

	user, err := s.authService.RegisterWithFlag(ctx, username, password, true)
	if err != nil {
		return nil, err
	}

	// Mark system as initialized.
	_ = s.settingRepo.SetSetting(ctx, "initialized", "true")
	// Disable public registration by default.
	_ = s.settingRepo.SetSetting(ctx, "allow_registration", "false")

	result := &BootstrapResult{
		Created:  true,
		Mode:     mode,
		Username: user.Username,
		UserID:   user.ID,
	}

	switch mode {
	case BootstrapModeServer:
		result.GeneratedPassword = password
	case BootstrapModeDesktop:
		// Issue a persistent session token for desktop auto-auth.
		// Store it in settings for the DesktopBootstrap endpoint to retrieve.
		token, _, err := s.authService.Login(ctx, username, password)
		if err != nil {
			return nil, err
		}
		_ = s.settingRepo.SetSetting(ctx, "desktop_bootstrap_token", token)
		result.SessionToken = token
	}

	return result, nil
}

// generateBootstrapPassword generates a strong random password (24 bytes = 48 hex chars).
func generateBootstrapPassword() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
