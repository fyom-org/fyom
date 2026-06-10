package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fyom/fyom/internal/config"
	"github.com/fyom/fyom/internal/middleware"
	"github.com/fyom/fyom/internal/provider"
	"github.com/fyom/fyom/internal/repository"
	"github.com/fyom/fyom/pkg/presign"
	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeTestToken(secret string) string {
	claims := jwt.MapClaims{
		"sub":      "test-user-id",
		"username": "testuser",
		"role":     "user",
		"iat":      time.Now().Unix(),
		"exp":      time.Now().Add(24 * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, _ := token.SignedString([]byte(secret))
	return s
}

func setupTestRouter(t *testing.T) http.Handler {
	t.Helper()

	tmpDir := t.TempDir()
	db, err := repository.Open(tmpDir, 5, 2, 60)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	cfg := &config.Config{
		Auth: config.Auth{
			JWTSecret:   "test-secret",
			TokenExpiry: 24,
		},
	}

	r := chi.NewRouter()
	r.Use(middleware.ErrorHandler())

	mediaRepo := repository.NewMediaRepository(db)
	userRepo := repository.NewUserRepository(db)
	jobRepo := repository.NewImportJobRepository(db)
	providerRepo := repository.NewProviderRepository(db)
	settingRepo := repository.NewSystemSettingRepository(db)
	libRepo := repository.NewLibraryRepository(db)
	libPermRepo := repository.NewLibraryPermissionRepository(db)

	healthHandler := NewHealthHandler("test", "abc123", "now", "go1.26")

	reg := provider.NewRegistry()
	reg.Register(provider.NewLocalProvider(presign.NewSigner("test-secret", 3600)))
	mediaHandler := NewMediaHandler(reg, db, mediaRepo, jobRepo, providerRepo, libRepo, slog.Default())

	authHandler := NewAuthHandler(userRepo, libPermRepo, settingRepo, cfg.Auth.JWTSecret, cfg.Auth.TokenExpiry)

	r.Get("/api/v1/health", healthHandler.Health)
	r.Get("/api/v1/version", healthHandler.Version)
	r.Post("/api/v1/auth/login", authHandler.Login)
	r.Post("/api/v1/auth/register", authHandler.Register)

	r.Group(func(r chi.Router) {
		r.Use(middleware.AuthMiddleware(cfg.Auth.JWTSecret))
		r.Get("/api/v1/library", mediaHandler.List)
		r.Get("/api/v1/library/{id}", mediaHandler.Get)
	})

	return r
}

func TestHealthHandler_Health(t *testing.T) {
	router := setupTestRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/health", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, float64(0), resp["code"])
	assert.Equal(t, "ok", resp["message"])
}

func TestHealthHandler_Version(t *testing.T) {
	router := setupTestRouter(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/version", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].(map[string]interface{})
	assert.Equal(t, "test", data["version"])
	assert.Equal(t, "abc123", data["git_commit"])
}

func TestMediaHandler_ListEmpty(t *testing.T) {
	router := setupTestRouter(t)
	token := makeTestToken("test-secret")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/library", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	data := resp["data"].(map[string]interface{})
	items := data["items"].([]interface{})
	assert.Equal(t, 0, len(items))
}

func TestMediaHandler_GetNotFound(t *testing.T) {
	router := setupTestRouter(t)
	token := makeTestToken("test-secret")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/library/non-existent", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(w, req)

	assert.Equal(t, 404, w.Code)
}
