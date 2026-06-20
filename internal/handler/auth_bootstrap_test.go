package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/fyom/fyom/internal/config"
	"github.com/fyom/fyom/internal/middleware"
	"github.com/fyom/fyom/internal/model"
	"github.com/fyom/fyom/internal/repository"
	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const bootstrapTestSecret = "bootstrap-test-secret"

type bootstrapTestEnv struct {
	db          *repository.DB
	userRepo    *repository.UserRepository
	authHandler *AuthHandler
	router      http.Handler
}

type bootstrapEnvelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Token                  string `json:"token"`
		AccessToken            string `json:"access_token"`
		TokenType              string `json:"token_type"`
		ExpiresIn              int    `json:"expires_in"`
		PasswordChangeRequired bool   `json:"password_change_required"`
		User                   struct {
			ID                     string `json:"id"`
			Username               string `json:"username"`
			Password               string `json:"password"`
			Role                   string `json:"role"`
			PasswordChangeRequired bool   `json:"password_change_required"`
		} `json:"user"`
	} `json:"data"`
}

func setupBootstrapSessionTestEnv(t *testing.T) *bootstrapTestEnv {
	t.Helper()

	tmpDir := t.TempDir()

	db, err := repository.Open(filepath.Join(tmpDir, "fyom.db"), 5, 2, 60)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	userRepo := repository.NewUserRepository(db)
	libPermRepo := repository.NewLibraryPermissionRepository(db)
	settingRepo := repository.NewSystemSettingRepository(db)

	cfg := config.Auth{
		JWTSecret:   bootstrapTestSecret,
		TokenExpiry: 24,
	}

	authHandler := NewAuthHandler(
		userRepo,
		libPermRepo,
		settingRepo,
		cfg.JWTSecret,
		cfg.TokenExpiry,
	)

	r := chi.NewRouter()
	r.Use(middleware.ErrorHandler())
	r.Group(func(r chi.Router) {
		r.Use(middleware.AllowLocalOnly)
		r.Get("/api/v1/internal/bootstrap-session", authHandler.InternalBootstrapSession)
	})

	return &bootstrapTestEnv{
		db:          db,
		userRepo:    userRepo,
		authHandler: authHandler,
		router:      r,
	}
}

func newLocalBootstrapRequest(t *testing.T) *http.Request {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, "/api/v1/internal/bootstrap-session", nil)
	require.NoError(t, err)

	req.RemoteAddr = "127.0.0.1:54321"

	return req
}

func newNonLocalBootstrapRequest(t *testing.T) *http.Request {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, "/api/v1/internal/bootstrap-session", nil)
	require.NoError(t, err)

	req.RemoteAddr = "203.0.113.10:54321"

	return req
}

func createBootstrapTestUser(
	t *testing.T,
	env *bootstrapTestEnv,
	id string,
	username string,
	role string,
	passwordChangeRequired bool,
) {
	t.Helper()

	err := env.userRepo.Create(context.Background(), &model.User{
		ID:                     id,
		Username:               username,
		Password:               "hashed-password",
		Role:                   role,
		PasswordChangeRequired: passwordChangeRequired,
	})
	require.NoError(t, err)
}

func performBootstrapSessionRequest(t *testing.T, env *bootstrapTestEnv, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()

	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)

	return w
}

func decodeBootstrapEnvelope(t *testing.T, body []byte) bootstrapEnvelope {
	t.Helper()

	var got bootstrapEnvelope
	require.NoError(t, json.Unmarshal(body, &got))

	return got
}

func parseBootstrapToken(t *testing.T, tokenString string) jwt.MapClaims {
	t.Helper()

	parsed, err := jwt.ParseWithClaims(
		tokenString,
		jwt.MapClaims{},
		func(_ *jwt.Token) (any, error) {
			return []byte(bootstrapTestSecret), nil
		},
	)
	require.NoError(t, err)
	require.True(t, parsed.Valid)

	claims, ok := parsed.Claims.(jwt.MapClaims)
	require.True(t, ok)

	return claims
}

func TestInternalBootstrapSession_ReturnsTokenForPasswordChangeRequiredAdmin(t *testing.T) {
	env := setupBootstrapSessionTestEnv(t)

	createBootstrapTestUser(
		t,
		env,
		"bootstrap-admin-id",
		"admin",
		"admin",
		true,
	)

	w := performBootstrapSessionRequest(t, env, newLocalBootstrapRequest(t))

	require.Equal(t, http.StatusOK, w.Code)

	got := decodeBootstrapEnvelope(t, w.Body.Bytes())

	assert.Equal(t, 0, got.Code)
	assert.Equal(t, "ok", got.Message)
	require.NotEmpty(t, got.Data.Token)
	require.NotEmpty(t, got.Data.AccessToken)
	assert.Equal(t, got.Data.Token, got.Data.AccessToken)
	assert.Equal(t, "Bearer", got.Data.TokenType)
	assert.Equal(t, 24*3600, got.Data.ExpiresIn)
	assert.True(t, got.Data.PasswordChangeRequired)

	assert.Equal(t, "bootstrap-admin-id", got.Data.User.ID)
	assert.Equal(t, "admin", got.Data.User.Username)
	assert.Equal(t, "admin", got.Data.User.Role)
	assert.True(t, got.Data.User.PasswordChangeRequired)
	assert.Empty(t, got.Data.User.Password)

	claims := parseBootstrapToken(t, got.Data.AccessToken)

	assert.Equal(t, "bootstrap-admin-id", claims["sub"])
	assert.Equal(t, "admin", claims["username"])
	assert.Equal(t, "admin", claims["role"])
	assert.NotEmpty(t, claims["iat"])
	assert.NotEmpty(t, claims["exp"])
}

func TestInternalBootstrapSession_ReturnsTokenForPasswordChangeRequiredOwner(t *testing.T) {
	env := setupBootstrapSessionTestEnv(t)

	createBootstrapTestUser(
		t,
		env,
		"bootstrap-owner-id",
		"owner",
		"owner",
		true,
	)

	w := performBootstrapSessionRequest(t, env, newLocalBootstrapRequest(t))

	require.Equal(t, http.StatusOK, w.Code)

	got := decodeBootstrapEnvelope(t, w.Body.Bytes())

	require.NotEmpty(t, got.Data.AccessToken)
	assert.Equal(t, "bootstrap-owner-id", got.Data.User.ID)
	assert.Equal(t, "owner", got.Data.User.Role)

	claims := parseBootstrapToken(t, got.Data.AccessToken)

	assert.Equal(t, "bootstrap-owner-id", claims["sub"])
	assert.Equal(t, "owner", claims["role"])
}

func TestInternalBootstrapSession_Returns404WhenNoBootstrapUserExists(t *testing.T) {
	env := setupBootstrapSessionTestEnv(t)

	w := performBootstrapSessionRequest(t, env, newLocalBootstrapRequest(t))

	assert.Equal(t, http.StatusNotFound, w.Code)

	var got map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, float64(404), got["code"])
	assert.Equal(t, "no bootstrap session available", got["message"])
}

func TestInternalBootstrapSession_Returns404WhenAdminDoesNotRequirePasswordChange(t *testing.T) {
	env := setupBootstrapSessionTestEnv(t)

	createBootstrapTestUser(
		t,
		env,
		"regular-admin-id",
		"admin",
		"admin",
		false,
	)

	w := performBootstrapSessionRequest(t, env, newLocalBootstrapRequest(t))

	assert.Equal(t, http.StatusNotFound, w.Code)

	var got map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, float64(404), got["code"])
	assert.Equal(t, "no bootstrap session available", got["message"])
}

func TestInternalBootstrapSession_DoesNotReturnRegularUser(t *testing.T) {
	env := setupBootstrapSessionTestEnv(t)

	createBootstrapTestUser(
		t,
		env,
		"regular-user-id",
		"regular-user",
		"user",
		true,
	)

	w := performBootstrapSessionRequest(t, env, newLocalBootstrapRequest(t))

	assert.Equal(t, http.StatusNotFound, w.Code)

	var got map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, float64(404), got["code"])
	assert.Equal(t, "no bootstrap session available", got["message"])
}

func TestInternalBootstrapSession_Returns404AfterPasswordChangeFlagCleared(t *testing.T) {
	env := setupBootstrapSessionTestEnv(t)

	createBootstrapTestUser(
		t,
		env,
		"bootstrap-admin-id",
		"admin",
		"admin",
		true,
	)

	first := performBootstrapSessionRequest(t, env, newLocalBootstrapRequest(t))
	require.Equal(t, http.StatusOK, first.Code)

	err := env.userRepo.UpdatePasswordAndFlag(
		context.Background(),
		"bootstrap-admin-id",
		"new-hashed-password",
		false,
	)
	require.NoError(t, err)

	second := performBootstrapSessionRequest(t, env, newLocalBootstrapRequest(t))

	assert.Equal(t, http.StatusNotFound, second.Code)

	var got map[string]any
	require.NoError(t, json.Unmarshal(second.Body.Bytes(), &got))
	assert.Equal(t, float64(404), got["code"])
	assert.Equal(t, "no bootstrap session available", got["message"])
}

func TestInternalBootstrapSession_RejectsNonLocalRequest(t *testing.T) {
	env := setupBootstrapSessionTestEnv(t)

	createBootstrapTestUser(
		t,
		env,
		"bootstrap-admin-id",
		"admin",
		"admin",
		true,
	)

	w := performBootstrapSessionRequest(t, env, newNonLocalBootstrapRequest(t))

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestInternalBootstrapSession_TokenExpiresInConfiguredTTL(t *testing.T) {
	env := setupBootstrapSessionTestEnv(t)

	createBootstrapTestUser(
		t,
		env,
		"bootstrap-admin-id",
		"admin",
		"admin",
		true,
	)

	w := performBootstrapSessionRequest(t, env, newLocalBootstrapRequest(t))
	require.Equal(t, http.StatusOK, w.Code)

	got := decodeBootstrapEnvelope(t, w.Body.Bytes())
	claims := parseBootstrapToken(t, got.Data.AccessToken)

	iatFloat, ok := claims["iat"].(float64)
	require.True(t, ok)

	expFloat, ok := claims["exp"].(float64)
	require.True(t, ok)

	iat := time.Unix(int64(iatFloat), 0)
	exp := time.Unix(int64(expFloat), 0)

	assert.Equal(t, 24*time.Hour, exp.Sub(iat))
}
