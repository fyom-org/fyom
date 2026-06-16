// Package handler implements the HTTP API endpoints for fyom.
package handler

import (
	"encoding/json"
	"net/http"

	"github.com/fyom/fyom/internal/model"
	"github.com/fyom/fyom/internal/repository"
	"github.com/fyom/fyom/internal/service"
	fyommiddleware "github.com/fyom/fyom/internal/middleware"
	"github.com/fyom/fyom/pkg/errors"
	"github.com/fyom/fyom/pkg/response"
	"golang.org/x/crypto/bcrypt"
)

// AuthHandler handles authentication endpoints.
type AuthHandler struct {
	authService  *service.AuthService
	settingRepo  *repository.SystemSettingRepository
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(userRepo *repository.UserRepository, libPermRepo *repository.LibraryPermissionRepository, settingRepo *repository.SystemSettingRepository, jwtSecret string, tokenTTLHours int) *AuthHandler {
	return &AuthHandler{
		authService: service.NewAuthService(userRepo, libPermRepo, jwtSecret, tokenTTLHours),
		settingRepo: settingRepo,
	}
}

// GetAuthService returns the internal auth service for use by other handlers.
func (h *AuthHandler) GetAuthService() *service.AuthService {
	return h.authService
}

// RegisterRequest represents a registration request body.
type RegisterRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// RegisterResponse represents a registration response.
type RegisterResponse struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

// LoginRequest represents a login request body.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse represents a login response.
type LoginResponse struct {
	AccessToken            string      `json:"access_token"`
	TokenType              string      `json:"token_type"`
	ExpiresIn              int         `json:"expires_in"`
	User                   *model.User `json:"user"`
	PasswordChangeRequired bool        `json:"password_change_required"`
}

// Register creates a new user account.
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	// Check if public registration is allowed
	allowReg, err := h.settingRepo.GetSetting(r.Context(), "allow_registration")
	if err != nil || allowReg != "true" {
		response.Error(w, 403, "registration is disabled")
		return
	}

	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Username == "" || req.Password == "" {
		response.Error(w, 400, "validation error")
		return
	}

	user, err := h.authService.Register(r.Context(), req.Username, req.Password)
	if err != nil {
		if appErr, ok := errors.IsAppError(err); ok {
			response.Error(w, appErr.Code, appErr.Message)
			return
		}
		response.Error(w, 500, "internal server error")
		return
	}

	response.Created(w, RegisterResponse{
		ID:       user.ID,
		Username: user.Username,
		Role:     user.Role,
	})
}

// Login authenticates a user and returns a JWT.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Username == "" || req.Password == "" {
		response.Error(w, 400, "validation error")
		return
	}

	token, user, err := h.authService.Login(r.Context(), req.Username, req.Password)
	if err != nil {
		if appErr, ok := errors.IsAppError(err); ok {
			response.Error(w, appErr.Code, appErr.Message)
			return
		}
		response.Error(w, 500, "internal server error")
		return
	}

	expiry := int((24 * 3600)) // seconds
	response.Success(w, LoginResponse{
		AccessToken:            token,
		TokenType:              "Bearer",
		ExpiresIn:              expiry,
		User:                   user,
		PasswordChangeRequired: user.PasswordChangeRequired,
	})
}

// Me returns the current authenticated user.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID, _ := fyommiddleware.GetUserID(r).(string)
	if userID == "" {
		response.Error(w, 401, "unauthorized")
		return
	}

	user, err := h.authService.GetUserByID(r.Context(), userID)
	if err != nil || user == nil {
		response.Error(w, 401, "unauthorized")
		return
	}

	// Never return the password hash
	user.Password = ""

	response.Success(w, user)
}

// DesktopBootstrap returns the desktop bootstrap token if one was created.
// This endpoint is used by the Tauri frontend on first run to auto-authenticate.
// It consumes the token (deletes it) so it can only be used once.
// Returns both the token and the user with password_change_required flag.
func (h *AuthHandler) DesktopBootstrap(w http.ResponseWriter, r *http.Request) {
	// Only allow in desktop/sidecar mode - check if setting exists
	token, err := h.settingRepo.GetSetting(r.Context(), "desktop_bootstrap_token")
	if err != nil || token == "" {
		response.Error(w, 404, "no bootstrap token available")
		return
	}

	// Consume the token - delete it so it can't be reused
	_ = h.settingRepo.SetSetting(r.Context(), "desktop_bootstrap_token", "")

	// Get the bootstrap admin user by username
	user, err := h.authService.GetUserByUsername(r.Context(), "admin")
	if err != nil || user == nil {
		// Fallback: just return the token
		response.Success(w, map[string]string{
			"token": token,
		})
		return
	}

	// Never return the password hash
	user.Password = ""

	response.Success(w, map[string]interface{}{
		"token": token,
		"user":  user,
	})
}

// InternalBootstrapSession returns the desktop bootstrap session if one exists
// and the user still has password_change_required=true.
// This is a localhost-only endpoint for the desktop bridge.
func (h *AuthHandler) InternalBootstrapSession(w http.ResponseWriter, r *http.Request) {
	// Only allow in desktop/sidecar mode
	token, err := h.settingRepo.GetSetting(r.Context(), "desktop_bootstrap_token")
	if err != nil || token == "" {
		response.Error(w, 404, "no bootstrap session available")
		return
	}

	// Get the bootstrap admin user
	user, err := h.authService.GetUserByUsername(r.Context(), "admin")
	if err != nil || user == nil {
		response.Error(w, 404, "no bootstrap session available")
		return
	}

	// Only return session if user still has password_change_required=true
	if !user.PasswordChangeRequired {
		// Clean up the token since it's no longer valid for bootstrap
		_ = h.settingRepo.SetSetting(r.Context(), "desktop_bootstrap_token", "")
		response.Error(w, 404, "bootstrap session no longer valid")
		return
	}

	// Never return the password hash
	user.Password = ""

	response.Success(w, map[string]interface{}{
		"token": token,
		"user":  user,
	})
}

// ChangePasswordRequest holds the password change form data.
type ChangePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

// ChangePassword allows the authenticated user to change their password.
func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	userID, _ := fyommiddleware.GetUserID(r).(string)
	if userID == "" {
		response.Error(w, 401, "unauthorized")
		return
	}

	var req ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.OldPassword == "" || req.NewPassword == "" {
		response.Error(w, 400, "validation error")
		return
	}

	// Fetch user to verify old password
	user, err := h.authService.GetUserByID(r.Context(), userID)
	if err != nil || user == nil {
		response.Error(w, 401, "unauthorized")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.OldPassword)); err != nil {
		response.Error(w, 401, "invalid credentials")
		return
	}

	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		response.Error(w, 500, "internal server error")
		return
	}

	// Update password and clear password_change_required
	if err := h.authService.UpdatePassword(r.Context(), userID, string(hashedBytes), true); err != nil {
		response.Error(w, 500, "internal server error")
		return
	}

	// Return updated user with cleared password_change_required
	user, err = h.authService.GetUserByID(r.Context(), userID)
	if err != nil || user == nil {
		response.Error(w, 500, "internal server error")
		return
	}
	user.Password = ""

	response.Success(w, map[string]interface{}{
		"user": user,
	})
}
