// Package handler implements the HTTP API endpoints for fyom.
package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	fyommiddleware "github.com/fyom/fyom/internal/middleware"
	"github.com/fyom/fyom/internal/model"
	"github.com/fyom/fyom/internal/repository"
	"github.com/fyom/fyom/internal/service"
	"github.com/fyom/fyom/pkg/errors"
	"github.com/fyom/fyom/pkg/response"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// AuthHandler handles authentication endpoints.
type AuthHandler struct {
	authService   *service.AuthService
	userRepo      *repository.UserRepository
	settingRepo   *repository.SystemSettingRepository
	jwtSecret     string
	tokenTTLHours int
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(
	userRepo *repository.UserRepository,
	libPermRepo *repository.LibraryPermissionRepository,
	settingRepo *repository.SystemSettingRepository,
	jwtSecret string,
	tokenTTLHours int,
) *AuthHandler {
	return &AuthHandler{
		authService:   service.NewAuthService(userRepo, libPermRepo, jwtSecret, tokenTTLHours),
		userRepo:      userRepo,
		settingRepo:   settingRepo,
		jwtSecret:     jwtSecret,
		tokenTTLHours: tokenTTLHours,
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
	Token                  string      `json:"token,omitempty"`
	TokenType              string      `json:"token_type"`
	ExpiresIn              int         `json:"expires_in"`
	User                   *model.User `json:"user"`
	PasswordChangeRequired bool        `json:"password_change_required"`
}

// BootstrapSessionResponse represents a localhost-only desktop bootstrap session.
type BootstrapSessionResponse struct {
	Token                  string      `json:"token"`
	AccessToken            string      `json:"access_token"`
	TokenType              string      `json:"token_type"`
	ExpiresIn              int         `json:"expires_in"`
	User                   *model.User `json:"user"`
	PasswordChangeRequired bool        `json:"password_change_required"`
}

// Register creates a new user account.
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	allowReg, err := h.settingRepo.GetSetting(r.Context(), "allow_registration")
	if err != nil || allowReg != "true" {
		response.Error(w, 403, "registration is disabled")
		return
	}

	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, 400, "validation error")
		return
	}

	req.Username = strings.TrimSpace(req.Username)

	if req.Username == "" || req.Password == "" {
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, 400, "validation error")
		return
	}

	req.Username = strings.TrimSpace(req.Username)

	if req.Username == "" || req.Password == "" {
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

	user = sanitizeUser(user)

	response.Success(w, LoginResponse{
		AccessToken:            token,
		Token:                  token,
		TokenType:              "Bearer",
		ExpiresIn:              h.tokenTTLSeconds(),
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

	response.Success(w, sanitizeUser(user))
}

// DesktopBootstrap returns a legacy desktop bootstrap token if one was created.
//
// This endpoint is kept for backward compatibility with older desktop bootstrap
// flows that stored a one-time token in system settings. New desktop clients
// should use /api/v1/internal/bootstrap-session instead.
func (h *AuthHandler) DesktopBootstrap(w http.ResponseWriter, r *http.Request) {
	token, err := h.settingRepo.GetSetting(r.Context(), "desktop_bootstrap_token")
	if err != nil || token == "" {
		response.Error(w, 404, "no bootstrap token available")
		return
	}

	_ = h.settingRepo.SetSetting(r.Context(), "desktop_bootstrap_token", "")

	user, err := h.authService.GetUserByUsername(r.Context(), "admin")
	if err != nil || user == nil {
		response.Success(w, map[string]interface{}{
			"token":        token,
			"access_token": token,
			"token_type":   "Bearer",
			"expires_in":   h.tokenTTLSeconds(),
		})
		return
	}

	user = sanitizeUser(user)

	response.Success(w, BootstrapSessionResponse{
		Token:                  token,
		AccessToken:            token,
		TokenType:              "Bearer",
		ExpiresIn:              h.tokenTTLSeconds(),
		User:                   user,
		PasswordChangeRequired: user.PasswordChangeRequired,
	})
}

// InternalBootstrapSession returns a localhost-only desktop bootstrap session.
//
// This endpoint is intentionally unauthenticated because it is used before the
// desktop frontend has any token. The route must remain protected by
// middleware.AllowLocalOnly in server.go.
//
// Security model:
//   - Only a bootstrap admin with password_change_required=true is eligible.
//   - Once the user changes password, password_change_required becomes false.
//   - The endpoint then naturally returns 404.
//   - No persistent bootstrap token setting is required.
func (h *AuthHandler) InternalBootstrapSession(w http.ResponseWriter, r *http.Request) {
	user, err := h.userRepo.FindBootstrapUser(r.Context())
	if err != nil {
		response.Error(w, 500, "internal server error")
		return
	}

	if user == nil {
		response.Error(w, 404, "no bootstrap session available")
		return
	}

	if !isBootstrapSessionUser(user) {
		response.Error(w, 404, "no bootstrap session available")
		return
	}

	token, err := h.issueToken(user)
	if err != nil {
		response.Error(w, 500, "failed to issue token")
		return
	}

	user = sanitizeUser(user)

	response.Success(w, BootstrapSessionResponse{
		Token:                  token,
		AccessToken:            token,
		TokenType:              "Bearer",
		ExpiresIn:              h.tokenTTLSeconds(),
		User:                   user,
		PasswordChangeRequired: user.PasswordChangeRequired,
	})
}

// ChangePasswordRequest holds the password change form data.
type ChangePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

// ChangePassword allows the authenticated user to change their password.
//
// For users with password_change_required=true, old_password is not required.
// For regular users, old_password is mandatory.
func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	userID, _ := fyommiddleware.GetUserID(r).(string)
	if userID == "" {
		response.Error(w, 401, "unauthorized")
		return
	}

	var req ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, 400, "validation error")
		return
	}

	if req.NewPassword == "" {
		response.Error(w, 400, "new_password is required")
		return
	}

	user, err := h.authService.GetUserByID(r.Context(), userID)
	if err != nil || user == nil {
		response.Error(w, 401, "unauthorized")
		return
	}

	if !user.PasswordChangeRequired {
		if req.OldPassword == "" {
			response.Error(w, 400, "old_password is required")
			return
		}

		if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.OldPassword)); err != nil {
			response.Error(w, 401, "invalid credentials")
			return
		}
	}

	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		response.Error(w, 500, "internal server error")
		return
	}

	if err := h.authService.UpdatePassword(r.Context(), userID, string(hashedBytes), true); err != nil {
		response.Error(w, 500, "internal server error")
		return
	}

	user, err = h.authService.GetUserByID(r.Context(), userID)
	if err != nil || user == nil {
		response.Error(w, 500, "internal server error")
		return
	}

	response.Success(w, map[string]interface{}{
		"user": sanitizeUser(user),
	})
}

func (h *AuthHandler) issueToken(user *model.User) (string, error) {
	if user == nil {
		return "", fmt.Errorf("cannot issue token for nil user")
	}

	now := time.Now()
	ttl := time.Duration(h.tokenTTLHours) * time.Hour
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}

	claims := jwt.MapClaims{
		"sub":      user.ID,
		"username": user.Username,
		"role":     user.Role,
		"iat":      now.Unix(),
		"exp":      now.Add(ttl).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	return token.SignedString([]byte(h.jwtSecret))
}

func (h *AuthHandler) tokenTTLSeconds() int {
	if h.tokenTTLHours <= 0 {
		return 24 * 3600
	}

	return h.tokenTTLHours * 3600
}

func sanitizeUser(user *model.User) *model.User {
	if user == nil {
		return nil
	}

	copy := *user
	copy.Password = ""

	return &copy
}

func isBootstrapSessionUser(user *model.User) bool {
	if user == nil {
		return false
	}

	if !user.PasswordChangeRequired {
		return false
	}

	role := strings.ToLower(strings.TrimSpace(user.Role))

	return role == "admin" || role == "owner"
}
