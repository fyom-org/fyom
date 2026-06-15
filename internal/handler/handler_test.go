package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fyom/fyom/internal/config"
	"github.com/fyom/fyom/internal/middleware"
	"github.com/fyom/fyom/internal/model"
	"github.com/fyom/fyom/internal/provider"
	"github.com/fyom/fyom/internal/repository"
	"github.com/fyom/fyom/internal/service"
	"github.com/fyom/fyom/pkg/presign"
	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeTestToken(secret string) string {
	return makeTestTokenWithRole(secret, "user")
}

func makeTestTokenWithRole(secret, role string) string {
	claims := jwt.MapClaims{
		"sub":      "test-user-id",
		"username": "testuser",
		"role":     role,
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
	db, err := repository.Open(filepath.Join(tmpDir, "fyom.db"), 5, 2, 60)
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
	statusRepo := repository.NewUserMediaStatusRepository(db)

	require.NoError(t, userRepo.Create(context.Background(), &model.User{
		ID:       "test-user-id",
		Username: "testuser",
		Password: "hash",
		Role:     "user",
	}))

	healthHandler := NewHealthHandler("test", "abc123", "now", "go1.26")

	reg := provider.NewRegistry()
	reg.Register(provider.NewLocalProvider(presign.NewSigner("test-secret", 3600)))

	// Mock refresh coordinator for tests
	mockCoordinator := &mockRefreshCoordinator{}

	mediaHandler := NewMediaHandler(reg, db, mediaRepo, jobRepo, providerRepo, libRepo, statusRepo, slog.Default(), mockCoordinator)

	authHandler := NewAuthHandler(userRepo, libPermRepo, settingRepo, cfg.Auth.JWTSecret, cfg.Auth.TokenExpiry)

	r.Get("/api/v1/health", healthHandler.Health)
	r.Get("/api/v1/version", healthHandler.Version)
	r.Post("/api/v1/auth/login", authHandler.Login)
	r.Post("/api/v1/auth/register", authHandler.Register)

	r.Group(func(r chi.Router) {
		r.Use(middleware.AuthMiddlewareWithUserRepo(cfg.Auth.JWTSecret, userRepo))
		r.Get("/api/v1/library", mediaHandler.List)
		r.Get("/api/v1/library/{id}", mediaHandler.Get)
	})

	return r
}

func setupAdminMediaTestRouter(t *testing.T) (*repository.DB, *repository.MediaRepository, http.Handler) {
	t.Helper()

	tmpDir := t.TempDir()
	db, err := repository.Open(filepath.Join(tmpDir, "fyom.db"), 5, 2, 60)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, repository.NewUserRepository(db).Create(context.Background(), &model.User{
		ID:       "test-user-id",
		Username: "testuser",
		Password: "hash",
		Role:     "admin",
	}))

	mediaRepo := repository.NewMediaRepository(db)
	adminRepo := repository.NewAdminRepository(db)
	settingRepo := repository.NewSystemSettingRepository(db)
	libPermRepo := repository.NewLibraryPermissionRepository(db)
	adminHandler := NewAdminHandler(adminRepo, mediaRepo, settingRepo, libPermRepo, db)

	r := chi.NewRouter()
	r.Use(middleware.ErrorHandler())
	r.Route("/api/v1/admin", func(r chi.Router) {
		r.Use(middleware.AuthMiddleware("test-secret"))
		r.Use(middleware.RequireAdmin)
		r.Get("/media", adminHandler.ListMedia)
	})
	return db, mediaRepo, r
}

func setupAdminSecurityRouter(t *testing.T) (*repository.DB, *repository.UserRepository, *repository.LibraryRepository, http.Handler) {
	t.Helper()

	tmpDir := t.TempDir()
	db, err := repository.Open(filepath.Join(tmpDir, "fyom.db"), 5, 2, 60)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	userRepo := repository.NewUserRepository(db)
	libRepo := repository.NewLibraryRepository(db)
	settingRepo := repository.NewSystemSettingRepository(db)
	libPermRepo := repository.NewLibraryPermissionRepository(db)
	adminRepo := repository.NewAdminRepository(db)
	providerRepo := repository.NewProviderRepository(db)
	mediaRepo := repository.NewMediaRepository(db)
	adminHandler := NewAdminHandler(adminRepo, mediaRepo, settingRepo, libPermRepo, db)
	adminLibHandler := NewAdminLibraryHandler(libRepo, providerRepo, libPermRepo)
	authService := service.NewAuthService(userRepo, libPermRepo, "test-secret", 24)
	systemHandler := NewSystemHandler(settingRepo, authService)

	r := chi.NewRouter()
	r.Use(middleware.ErrorHandler())
	r.Post("/api/v1/system/initialize", systemHandler.Initialize)
	r.Route("/api/v1/admin", func(r chi.Router) {
		r.Use(middleware.AuthMiddlewareWithUserRepo("test-secret", userRepo))
		r.Use(middleware.RequireAdmin)
		r.Post("/libraries", adminLibHandler.Create)
		r.Get("/stats", adminHandler.GetStats)
	})
	return db, userRepo, libRepo, r
}

// mockRefreshCoordinator implements RefreshCoordinator for testing.
type mockRefreshCoordinator struct {
	running map[string]bool
	mu      sync.Mutex
}

func (m *mockRefreshCoordinator) TryStart(libraryID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running == nil {
		m.running = make(map[string]bool)
	}
	if m.running[libraryID] {
		return false
	}
	m.running[libraryID] = true
	return true
}

func (m *mockRefreshCoordinator) Finish(libraryID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running != nil {
		delete(m.running, libraryID)
	}
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

func TestAdminListMedia_ReturnsItemsAfterImporterSchemaExpansion(t *testing.T) {
	_, mediaRepo, router := setupAdminMediaTestRouter(t)

	item := &model.MediaItem{
		ID:             "admin-media-expanded-schema",
		Type:           "movie",
		Title:          "Admin Media Expanded Schema",
		SortTitle:      "admin media expanded schema",
		FilePath:       "/media/Admin Media Expanded Schema/movie.mkv",
		RootPath:       "/media/Admin Media Expanded Schema",
		PrimaryPath:    "/media/Admin Media Expanded Schema/movie.mkv",
		NFOPath:        "/media/Admin Media Expanded Schema/movie.nfo",
		MetadataSource: "nfo",
		ProviderID:     "local",
		LibraryID:      "lib-admin-media",
		Status:         "available",
	}
	require.NoError(t, mediaRepo.Create(context.Background(), item))

	w := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "/api/v1/admin/media?limit=20", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+makeTestTokenWithRole("test-secret", "admin"))
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Items []model.MediaItem `json:"items"`
			Total int               `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Len(t, resp.Data.Items, 1)
	assert.Equal(t, 1, resp.Data.Total)
	assert.Equal(t, item.ID, resp.Data.Items[0].ID)
	assert.Equal(t, item.Title, resp.Data.Items[0].Title)
	assert.Equal(t, item.Type, resp.Data.Items[0].Type)
}

func TestAdminListMedia_ResponseIncludesNewPathFields(t *testing.T) {
	_, mediaRepo, router := setupAdminMediaTestRouter(t)

	item := &model.MediaItem{
		ID:             "admin-media-path-fields",
		Type:           "episode",
		Title:          "Path Fields Episode",
		FilePath:       "/media/Show/Season 01/episode.mkv",
		RootPath:       "/media/Show",
		PrimaryPath:    "/media/Show/Season 01/episode.mkv",
		NFOPath:        "/media/Show/Season 01/episode.nfo",
		MetadataSource: "nfo",
		ProviderID:     "local",
		LibraryID:      "lib-admin-media-paths",
		Status:         "available",
	}
	require.NoError(t, mediaRepo.Create(context.Background(), item))

	w := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "/api/v1/admin/media?limit=20", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+makeTestTokenWithRole("test-secret", "admin"))
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	var resp struct {
		Data struct {
			Items []model.MediaItem `json:"items"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Data.Items, 1)
	got := resp.Data.Items[0]
	assert.Equal(t, item.RootPath, got.RootPath)
	assert.Equal(t, item.PrimaryPath, got.PrimaryPath)
	assert.Equal(t, item.NFOPath, got.NFOPath)
	assert.Equal(t, item.FilePath, got.FilePath)
	assert.Equal(t, item.Title, got.Title)
	assert.Equal(t, item.Type, got.Type)
}

func TestAdminListMedia_WithRealImportedData_NoLongerReturnsEmpty(t *testing.T) {
	db, _, router := setupAdminMediaTestRouter(t)
	ctx := context.Background()

	libRepo := repository.NewLibraryRepository(db)
	lib := &model.Library{
		Name:           "Admin Media Real Import",
		Type:           "movie",
		SourcePath:     filepath.Join(t.TempDir(), "library"),
		ProviderID:     "local",
		MetadataSource: "nfo",
	}
	require.NoError(t, libRepo.Create(ctx, lib))
	require.NoError(t, os.MkdirAll(lib.SourcePath, 0755))

	movieDir := filepath.Join(lib.SourcePath, "Imported Movie (2024)")
	require.NoError(t, os.MkdirAll(movieDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(movieDir, "movie.nfo"), []byte(`<?xml version="1.0" encoding="UTF-8"?>
<movie>
  <title>Imported Movie</title>
  <year>2024</year>
  <plot>Real import regression fixture.</plot>
  <genre>Drama</genre>
  <uniqueid type="imdb">tt0000002</uniqueid>
</movie>`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(movieDir, "movie.mkv"), []byte(""), 0644))

	jobRepo := repository.NewImportJobRepository(db)
	importer := service.NewImporter(service.NewLocalImportFS(), "local", db, repository.NewMediaRepository(db), jobRepo)
	importer.SetLibraryID(lib.ID)
	job, err := importer.ImportRequest(ctx, lib.SourcePath)
	require.NoError(t, err)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		stored, err := jobRepo.Get(ctx, job.ID)
		require.NoError(t, err)
		if stored != nil && stored.Status == "done" {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	stored, err := jobRepo.Get(ctx, job.ID)
	require.NoError(t, err)
	require.NotNil(t, stored)
	require.Equal(t, "done", stored.Status)

	w := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "/api/v1/admin/media?limit=20", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+makeTestTokenWithRole("test-secret", "admin"))
	router.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	var resp struct {
		Data struct {
			Items []model.MediaItem `json:"items"`
			Total int               `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotEmpty(t, resp.Data.Items)
	assert.Equal(t, 1, resp.Data.Total)
	assert.Equal(t, "movie", resp.Data.Items[0].Type)
	assert.NotEmpty(t, resp.Data.Items[0].RootPath)
	assert.NotEmpty(t, resp.Data.Items[0].PrimaryPath)
	assert.NotEmpty(t, resp.Data.Items[0].NFOPath)
}

func TestAdminRequest_WithTokenForDeletedOrMissingUser_IsRejected(t *testing.T) {
	db, userRepo, libRepo, router := setupAdminSecurityRouter(t)
	ctx := context.Background()

	require.NoError(t, userRepo.Create(ctx, &model.User{ID: "test-user-id", Username: "admin", Password: "hash", Role: "admin"}))
	token := makeTestTokenWithRole("test-secret", "admin")
	_, err := db.ExecContext(ctx, "DELETE FROM users WHERE id = ?", "test-user-id")
	require.NoError(t, err)

	w := httptest.NewRecorder()
	req, err := http.NewRequest("POST", "/api/v1/admin/libraries", strings.NewReader(`{"name":"Missing User Library","type":"movie","provider_id":"local","source_path":"/tmp/media"}`))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, 401, w.Code)
	libs, err := libRepo.List(ctx)
	require.NoError(t, err)
	assert.Empty(t, libs)
}

func TestAdminRequest_WithStaleToken_AfterDBReset_IsRejected(t *testing.T) {
	db, userRepo, libRepo, router := setupAdminSecurityRouter(t)
	ctx := context.Background()

	require.NoError(t, userRepo.Create(ctx, &model.User{ID: "test-user-id", Username: "admin", Password: "hash", Role: "admin"}))
	token := makeTestTokenWithRole("test-secret", "admin")
	_, err := db.ExecContext(ctx, "DELETE FROM users")
	require.NoError(t, err)

	w := httptest.NewRecorder()
	req, err := http.NewRequest("POST", "/api/v1/admin/libraries", strings.NewReader(`{"name":"Reset DB Library","type":"movie","provider_id":"local","source_path":"/tmp/media"}`))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, 401, w.Code)
	libs, err := libRepo.List(ctx)
	require.NoError(t, err)
	assert.Empty(t, libs)
}

func TestAdminRequest_RequiresCurrentDBAdminRole_NotJustTokenClaim(t *testing.T) {
	db, userRepo, libRepo, router := setupAdminSecurityRouter(t)
	ctx := context.Background()

	require.NoError(t, userRepo.Create(ctx, &model.User{ID: "test-user-id", Username: "admin", Password: "hash", Role: "admin"}))
	token := makeTestTokenWithRole("test-secret", "admin")
	_, err := db.ExecContext(ctx, "UPDATE users SET role = 'user' WHERE id = ?", "test-user-id")
	require.NoError(t, err)

	w := httptest.NewRecorder()
	req, err := http.NewRequest("POST", "/api/v1/admin/libraries", strings.NewReader(`{"name":"Downgraded Library","type":"movie","provider_id":"local","source_path":"/tmp/media"}`))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	assert.Equal(t, 403, w.Code)
	libs, err := libRepo.List(ctx)
	require.NoError(t, err)
	assert.Empty(t, libs)
}

func TestSetupMode_DoesNotAcceptOrphanedAdminToken(t *testing.T) {
	_, _, _, router := setupAdminSecurityRouter(t)
	token := makeTestTokenWithRole("test-secret", "admin")

	w := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "/api/v1/admin/stats", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(w, req)
	assert.Equal(t, 401, w.Code)

	w = httptest.NewRecorder()
	req, err = http.NewRequest("POST", "/api/v1/system/initialize", strings.NewReader(`{"username":"setup-admin","password":"setup-password","allow_registration":false}`))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	assert.Equal(t, 200, w.Code)

	w = httptest.NewRecorder()
	req, err = http.NewRequest("GET", "/api/v1/admin/stats", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(w, req)
	assert.Equal(t, 401, w.Code)
}

func TestImportJobStatusResponse_IncludesImportSummary(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := repository.Open(filepath.Join(tmpDir, "fyom.db"), 5, 2, 60)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	jobRepo := repository.NewImportJobRepository(db)
	job, err := jobRepo.Create(context.Background(), "/tmp/source", "lib-1")
	require.NoError(t, err)
	require.NoError(t, jobRepo.UpdateProgress(context.Background(), job.ID, 4, 2, "running"))

	summary := &model.ImportSummary{
		ScannedFiles:  7,
		ImportedItems: 3,
		UpdatedItems:  1,
		SkippedFiles:  2,
		ParseWarnings: []string{"bad.nfo: malformed XML"},
		Duration:      1234 * time.Millisecond,
	}
	require.NoError(t, jobRepo.UpdateSummary(context.Background(), job.ID, summary))

	claims := jwt.MapClaims{
		"sub":      "test-user-id",
		"username": "testuser",
		"role":     "admin",
		"iat":      time.Now().Unix(),
		"exp":      time.Now().Add(24 * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte("test-secret"))
	require.NoError(t, err)

	r := chi.NewRouter()
	mediaHandler := NewMediaHandler(nil, db, nil, jobRepo, nil, nil, nil, slog.Default(), &mockRefreshCoordinator{})
	r.Group(func(r chi.Router) {
		r.Use(middleware.AuthMiddleware("test-secret"))
		r.Get("/api/v1/library/jobs/{id}", mediaHandler.GetJob)
	})

	w := httptest.NewRecorder()
	req, err := http.NewRequest("GET", "/api/v1/library/jobs/"+job.ID, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+signed)
	r.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	dataBytes, err := json.Marshal(resp["data"])
	require.NoError(t, err)
	var got model.ImportJob
	require.NoError(t, json.Unmarshal(dataBytes, &got))

	assert.Equal(t, job.ID, got.ID)
	assert.Equal(t, summary.ScannedFiles, got.ScannedFiles)
	assert.Equal(t, summary.ImportedItems, got.ImportedItems)
	assert.Equal(t, summary.UpdatedItems, got.UpdatedItems)
	assert.Equal(t, summary.SkippedFiles, got.SkippedFiles)
	assert.Equal(t, summary.ParseWarnings, got.ParseWarnings)
	assert.Equal(t, summary.Duration.Milliseconds(), got.DurationMS)
	assert.Equal(t, 4, got.TotalItems)
	assert.Equal(t, 2, got.DoneItems)
}
