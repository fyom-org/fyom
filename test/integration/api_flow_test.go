package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/fyom/fyom/internal/config"
	"github.com/fyom/fyom/internal/handler"
	"github.com/fyom/fyom/internal/middleware"
	"github.com/fyom/fyom/internal/provider"
	"github.com/fyom/fyom/internal/repository"
	"github.com/fyom/fyom/pkg/presign"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockRefreshCoordinator implements handler.RefreshCoordinator for integration tests.
type mockRefreshCoordinator struct {
	mu      sync.Mutex
	running map[string]bool
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

type apiResponse[T any] struct {
	Data    T      `json:"data"`
	Error   any    `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
	Success bool   `json:"success,omitempty"`
}

type loginData struct {
	AccessToken string `json:"access_token"`
}

type meData struct {
	Username string `json:"username"`
}

type importJobData struct {
	JobID string `json:"job_id"`
}

type jobData struct {
	Status string `json:"status"`
}

type libraryListData struct {
	Items []libraryItem `json:"items"`
}

type libraryItem struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Title     string `json:"title"`
	StreamURL string `json:"stream_url"`
}

type importRequest struct {
	SourcePath string `json:"source_path"`
	LibraryID  string `json:"library_id"`
}

type testApp struct {
	router http.Handler
	db     *repository.DB
}

func setupIntegrationApp(t *testing.T) testApp {
	t.Helper()

	tmpDir := t.TempDir()

	db, err := repository.Open(filepath.Join(tmpDir, "fyom.db"), 5, 2, 60)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	cfg := &config.Config{
		Auth: config.Auth{
			JWTSecret:   "integration-test-secret",
			TokenExpiry: 24,
		},
	}

	router := chi.NewRouter()
	router.Use(middleware.ErrorHandler())

	mediaRepo := repository.NewMediaRepository(db)
	userRepo := repository.NewUserRepository(db)
	jobRepo := repository.NewImportJobRepository(db)
	settingRepo := repository.NewSystemSettingRepository(db)
	providerRepo := repository.NewProviderRepository(db)
	libraryRepo := repository.NewLibraryRepository(db)
	libraryPermissionRepo := repository.NewLibraryPermissionRepository(db)
	statusRepo := repository.NewUserMediaStatusRepository(db)

	require.NoError(t, settingRepo.SetSetting(context.Background(), "allow_registration", "true"))

	signer := presign.NewSigner(cfg.Auth.JWTSecret, 3600)

	healthHandler := handler.NewHealthHandler("test", "abc", "now", "go1.26")

	providerRegistry := provider.NewRegistry()
	providerRegistry.Register(provider.NewLocalProvider(signer))

	mediaHandler := handler.NewMediaHandler(
		providerRegistry,
		db,
		mediaRepo,
		jobRepo,
		providerRepo,
		libraryRepo,
		statusRepo,
		slog.Default(),
		&mockRefreshCoordinator{},
	)

	authHandler := handler.NewAuthHandler(
		userRepo,
		libraryPermissionRepo,
		settingRepo,
		cfg.Auth.JWTSecret,
		cfg.Auth.TokenExpiry,
	)

	registerRoutes(router, cfg, signer, healthHandler, authHandler, mediaHandler)

	return testApp{
		router: router,
		db:     db,
	}
}

func registerRoutes(
	router chi.Router,
	cfg *config.Config,
	signer *presign.Signer,
	healthHandler *handler.HealthHandler,
	authHandler *handler.AuthHandler,
	mediaHandler *handler.MediaHandler,
) {
	router.Get("/api/v1/health", healthHandler.Health)
	router.Post("/api/v1/auth/register", authHandler.Register)
	router.Post("/api/v1/auth/login", authHandler.Login)

	router.Group(func(router chi.Router) {
		router.Use(middleware.AuthMiddleware(cfg.Auth.JWTSecret))

		router.Get("/api/v1/auth/me", authHandler.Me)
		router.Post("/api/v1/library/import", mediaHandler.Import)
		router.Get("/api/v1/library/jobs/{id}", mediaHandler.GetJob)
		router.Get("/api/v1/library", mediaHandler.List)
		router.Get("/api/v1/library/{id}", mediaHandler.Get)
	})

	router.Route("/api/v1/media", func(router chi.Router) {
		router.Use(middleware.RequireValidPresign(signer))
		router.Get("/{id}/stream", mediaHandler.Stream)
	})
}

func apiCall(
	t *testing.T,
	router http.Handler,
	method string,
	path string,
	token string,
	body any,
) *httptest.ResponseRecorder {
	t.Helper()

	var bodyReader io.Reader

	if body != nil {
		payload, err := encodeRequestBody(body)
		require.NoError(t, err)

		bodyReader = bytes.NewReader(payload)
	}

	req, err := http.NewRequest(method, path, bodyReader)
	require.NoError(t, err)

	req.Header.Set("Accept", "application/json")

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	return recorder
}

func encodeRequestBody(body any) ([]byte, error) {
	switch value := body.(type) {
	case []byte:
		return value, nil
	case string:
		return []byte(value), nil
	default:
		return json.Marshal(value)
	}
}

func decodeAPIResponse[T any](t *testing.T, recorder *httptest.ResponseRecorder) apiResponse[T] {
	t.Helper()

	var response apiResponse[T]
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))

	return response
}

func registerAndLogin(t *testing.T, router http.Handler, username string, password string) string {
	t.Helper()

	registerResponse := apiCall(t, router, http.MethodPost, "/api/v1/auth/register", "", map[string]string{
		"username": username,
		"password": password,
	})
	require.Equal(t, http.StatusCreated, registerResponse.Code)

	loginResponse := apiCall(t, router, http.MethodPost, "/api/v1/auth/login", "", map[string]string{
		"username": username,
		"password": password,
	})
	require.Equal(t, http.StatusOK, loginResponse.Code)

	response := decodeAPIResponse[loginData](t, loginResponse)
	require.NotEmpty(t, response.Data.AccessToken)

	return response.Data.AccessToken
}

func createNFOFixture(t *testing.T) string {
	t.Helper()

	mediaDir := t.TempDir()

	createMovieFixture(t, mediaDir)
	createTVShowFixture(t, mediaDir)

	return mediaDir
}

func createMovieFixture(t *testing.T, mediaDir string) {
	t.Helper()

	movieDir := filepath.Join(mediaDir, "Inception (2010)")
	require.NoError(t, os.MkdirAll(movieDir, 0755))

	writeFile(t, filepath.Join(movieDir, "Inception.mkv"), make([]byte, 4096))
	writeFile(t, filepath.Join(movieDir, "Inception.nfo"), []byte(`<?xml version="1.0" encoding="UTF-8"?>
<movie>
  <title>Inception</title>
  <sorttitle>Inception</sorttitle>
  <year>2010</year>
  <plot>A thief who steals corporate secrets through dream-sharing technology.</plot>
  <rating>8.8</rating>
  <runtime>148</runtime>
</movie>`))
}

func createTVShowFixture(t *testing.T, mediaDir string) {
	t.Helper()

	showDir := filepath.Join(mediaDir, "Breaking Bad")
	seasonDir := filepath.Join(showDir, "Season 01")

	require.NoError(t, os.MkdirAll(seasonDir, 0755))

	writeFile(t, filepath.Join(showDir, "tvshow.nfo"), []byte(`<?xml version="1.0" encoding="UTF-8"?>
<tvshow>
  <title>Breaking Bad</title>
  <plot>A high school chemistry teacher turned methamphetamine manufacturer.</plot>
  <rating>9.5</rating>
</tvshow>`))

	writeFile(t, filepath.Join(seasonDir, "S01E01.mkv"), make([]byte, 2048))
	writeFile(t, filepath.Join(seasonDir, "S01E01.nfo"), []byte(`<?xml version="1.0" encoding="UTF-8"?>
<episodedetails>
  <title>Pilot</title>
  <showtitle>Breaking Bad</showtitle>
  <season>1</season>
  <episode>1</episode>
  <plot>Walter White is diagnosed with lung cancer.</plot>
  <rating>8.7</rating>
</episodedetails>`))
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()

	require.NoError(t, os.WriteFile(path, data, 0644))
}

func createLibrary(t *testing.T, db *repository.DB, name string, sourcePath string) string {
	t.Helper()

	libraryID := uuid.NewString()

	_, err := db.ExecContext(
		context.Background(),
		`INSERT INTO libraries (id, name, source_path, provider_id) VALUES (?, ?, ?, ?)`,
		libraryID,
		name,
		sourcePath,
		"local",
	)
	require.NoError(t, err)

	return libraryID
}

func startImportJob(t *testing.T, router http.Handler, token string, sourcePath string, libraryID string) string {
	t.Helper()

	response := apiCall(t, router, http.MethodPost, "/api/v1/library/import", token, importRequest{
		SourcePath: sourcePath,
		LibraryID:  libraryID,
	})
	require.Equal(t, http.StatusOK, response.Code)

	payload := decodeAPIResponse[importJobData](t, response)
	require.NotEmpty(t, payload.Data.JobID)

	return payload.Data.JobID
}

func waitForImportJob(t *testing.T, router http.Handler, token string, jobID string) string {
	t.Helper()

	var lastStatus string

	for attempt := 0; attempt < 50; attempt++ {
		time.Sleep(100 * time.Millisecond)

		response := apiCall(t, router, http.MethodGet, "/api/v1/library/jobs/"+jobID, token, nil)
		if response.Code != http.StatusOK {
			continue
		}

		payload := decodeAPIResponse[jobData](t, response)
		lastStatus = payload.Data.Status

		if lastStatus == "done" || lastStatus == "error" {
			return lastStatus
		}
	}

	return lastStatus
}

func listLibraryItems(t *testing.T, router http.Handler, token string) []libraryItem {
	t.Helper()

	response := apiCall(t, router, http.MethodGet, "/api/v1/library?page_size=100", token, nil)
	require.Equal(t, http.StatusOK, response.Code)

	payload := decodeAPIResponse[libraryListData](t, response)

	return payload.Data.Items
}

func findMovieStreamURL(items []libraryItem, title string) string {
	for _, item := range items {
		if item.Type == "movie" && item.Title == title {
			return item.StreamURL
		}
	}

	return ""
}

func assertStreamSupportsRange(t *testing.T, router http.Handler, streamURL string) {
	t.Helper()

	recorder := httptest.NewRecorder()

	req, err := http.NewRequest(http.MethodGet, streamURL, nil)
	require.NoError(t, err)

	req.Header.Set("Range", "bytes=0-1023")

	router.ServeHTTP(recorder, req)

	assert.Contains(t, []int{http.StatusOK, http.StatusPartialContent}, recorder.Code)
}

func TestIntegration_NFOImportFlow(t *testing.T) {
	app := setupIntegrationApp(t)

	token := registerAndLogin(t, app.router, "testuser", "testpass123")

	meResponse := apiCall(t, app.router, http.MethodGet, "/api/v1/auth/me", token, nil)
	require.Equal(t, http.StatusOK, meResponse.Code)

	mePayload := decodeAPIResponse[meData](t, meResponse)
	assert.Equal(t, "testuser", mePayload.Data.Username)

	mediaDir := createNFOFixture(t)
	libraryID := createLibrary(t, app.db, "Test Library", mediaDir)

	jobID := startImportJob(t, app.router, token, mediaDir, libraryID)
	jobStatus := waitForImportJob(t, app.router, token, jobID)

	require.Equal(t, "done", jobStatus, "import job should complete successfully")

	items := listLibraryItems(t, app.router, token)
	require.Len(t, items, 2, "expect 1 movie + 1 show; episodes should be excluded from library grid")

	streamURL := findMovieStreamURL(items, "Inception")
	require.NotEmpty(t, streamURL)

	assertStreamSupportsRange(t, app.router, streamURL)
}

func TestIntegration_UnauthorizedAccess(t *testing.T) {
	app := setupIntegrationApp(t)

	response := apiCall(t, app.router, http.MethodGet, "/api/v1/library", "", nil)

	assert.Equal(t, http.StatusUnauthorized, response.Code)
}

func TestIntegration_RegisterDuplicate(t *testing.T) {
	app := setupIntegrationApp(t)

	body := map[string]string{
		"username": "dupuser",
		"password": "pass123",
	}

	firstResponse := apiCall(t, app.router, http.MethodPost, "/api/v1/auth/register", "", body)
	require.Equal(t, http.StatusCreated, firstResponse.Code)

	secondResponse := apiCall(t, app.router, http.MethodPost, "/api/v1/auth/register", "", body)
	assert.Equal(t, http.StatusConflict, secondResponse.Code)
}
