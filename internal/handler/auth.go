// Package handler implements the HTTP API endpoints for fyom.
package handler

import (
	"encoding/json"
	"net/http"

	"github.com/fyom/fyom/internal/repository"
	"github.com/fyom/fyom/internal/service"
	fyommiddleware "github.com/fyom/fyom/internal/middleware"
	"github.com/fyom/fyom/pkg/errors"
	"github.com/fyom/fyom/pkg/response"
)

// AuthHandler handles authentication endpoints.
type AuthHandler struct {
	authService *service.AuthService
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(userRepo *repository.UserRepository, jwtSecret string, tokenTTLHours int) *AuthHandler {
	return &AuthHandler{
		authService: service.NewAuthService(userRepo, jwtSecret, tokenTTLHours),
	}
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
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// Register creates a new user account.
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
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

	token, _, err := h.authService.Login(r.Context(), req.Username, req.Password)
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
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   expiry,
	})
}

// Me returns the current authenticated user.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID := fyommiddleware.GetUserID(r)
	username := fyommiddleware.GetUsername(r)
	role := fyommiddleware.GetRole(r)

	response.Success(w, map[string]interface{}{
		"user_id":  userID,
		"username": username,
		"role":     role,
	})
}
