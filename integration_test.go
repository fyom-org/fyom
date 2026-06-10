package fyom

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fyom/fyom/internal/config"
	"github.com/fyom/fyom/internal/handler"
	"github.com/fyom/fyom/internal/middleware"
	"github.com/fyom/fyom/internal/provider"
	"github.com/fyom/fyom/internal/repository"
	"github.com/fyom/fyom/pkg/presign"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupIntegrationRouter(t *testing.T) (http.Handler, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	db, err := repository.Open(tmpDir, 5, 2, 60)
	require.NoError(t, err)

	cfg := &config.Config{
		Auth: config.Auth{
			JWTSecret:   "integration-test-secret",
			TokenExpiry: 24,
		},
	}

	r := chi.NewRouter()
	r.Use(middleware.ErrorHandler())

	mediaRepo := repository.NewMediaRepository(db)
	userRepo := repository.NewUserRepository(db)
	jobRepo := repository.NewImportJobRepository(db)

	healthHandler := handler.NewHealthHandler("test", "abc", "now", "go1.26")

	// Provider registry with LocalProvider.
	reg := provider.NewRegistry()
	reg.Register(provider.NewLocalProvider(presign.NewSigner(cfg.Auth.JWTSecret, 3600)))
	mediaHandler := handler.NewMediaHandler(reg, db, mediaRepo, jobRepo, slog.Default())

	authHandler := handler.NewAuthHandler(userRepo, cfg.Auth.JWTSecret, cfg.Auth.TokenExpiry)

	// Public routes (no auth)
	r.Get("/api/v1/health", healthHandler.Health)
	r.Post("/api/v1/auth/register", authHandler.Register)
	r.Post("/api/v1/auth/login", authHandler.Login)

	// Protected routes (require auth)
	r.Group(func(r chi.Router) {
		r.Use(middleware.AuthMiddleware(cfg.Auth.JWTSecret))
		r.Get("/api/v1/auth/me", authHandler.Me)
		r.Post("/api/v1/library/import", mediaHandler.Import)
		r.Get("/api/v1/library/jobs/{id}", mediaHandler.GetJob)
		r.Get("/api/v1/library", mediaHandler.List)
		r.Get("/api/v1/library/{id}", mediaHandler.Get)
	})

	// Presigned media endpoints (no JWT, sig-based auth)
	r.Route("/api/v1/media", func(r chi.Router) {
		r.Use(middleware.RequireValidPresign(signer))
		r.Get("/{id}/stream", mediaHandler.Stream)
	})

	return r, func() { _ = db.Close() }
}

func apiCall(t *testing.T, router http.Handler, method, path, token string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	var req *http.Request
	if body != nil {
		req, _ = http.NewRequest(method, path, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req, _ = http.NewRequest(method, path, nil)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	router.ServeHTTP(w, req)
	return w
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, data, 0644))
}

func TestIntegration_NFOImportFlow(t *testing.T) {
	router, cleanup := setupIntegrationRouter(t)
	defer cleanup()

	// Step 1: Register + Login
	w := apiCall(t, router, "POST", "/api/v1/auth/register", "",
		[]byte(`{"username":"testuser","password":"testpass123"}`))
	assert.Equal(t, 201, w.Code)

	w = apiCall(t, router, "POST", "/api/v1/auth/login", "",
		[]byte(`{"username":"testuser","password":"testpass123"}`))
	assert.Equal(t, 200, w.Code)

	var loginResp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &loginResp))
	loginData := loginResp["data"].(map[string]interface{})
	token := loginData["access_token"].(string)
	assert.NotEmpty(t, token)

	// Step 2: Verify /auth/me
	w = apiCall(t, router, "GET", "/api/v1/auth/me", token, nil)
	assert.Equal(t, 200, w.Code)
	var meResp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &meResp))
	meData := meResp["data"].(map[string]interface{})
	assert.Equal(t, "testuser", meData["username"])
	assert.Equal(t, "user", meData["role"])

	// Step 3: Create test NFO files
	tmpMediaDir := t.TempDir()

	// Movie: Inception
	movieDir := filepath.Join(tmpMediaDir, "Inception (2010)")
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

	// TV Show: Breaking Bad with episode
	showDir := filepath.Join(tmpMediaDir, "Breaking Bad")
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

	// Step 4: Trigger async import
	w = apiCall(t, router, "POST", "/api/v1/library/import", token,
		[]byte(`{"source_path":"`+tmpMediaDir+`"}`))
	assert.Equal(t, 200, w.Code)
	var importResp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &importResp))
	importData := importResp["data"].(map[string]interface{})
	jobID := importData["job_id"].(string)
	assert.NotEmpty(t, jobID)

	// Step 5: Poll job status until done
	var jobStatus string
	for i := 0; i < 50; i++ {
		time.Sleep(100 * time.Millisecond)
		w = apiCall(t, router, "GET", "/api/v1/library/jobs/"+jobID, token, nil)
		var jobResp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &jobResp))
		jobDataRaw := jobResp["data"]
		if jobDataRaw == nil {
			continue
		}
		jobData := jobDataRaw.(map[string]interface{})
		jobStatus = jobData["status"].(string)
		if jobStatus == "done" || jobStatus == "error" {
			break
		}
	}
	assert.Equal(t, "done", jobStatus, "import job should complete")

	// Step 6: Query library — should return movie + show only (episodes excluded from grid)
	w = apiCall(t, router, "GET", "/api/v1/library?page_size=100", token, nil)
	assert.Equal(t, 200, w.Code)
	var listResp map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listResp))
	listData := listResp["data"].(map[string]interface{})
	items := listData["items"].([]interface{})
	assert.Equal(t, 2, len(items), "expect 1 movie + 1 show; episodes excluded from library grid")

	// Step 7: Stream request with Range header via presigned URL
	var streamURL string
	for _, raw := range items {
		item := raw.(map[string]interface{})
		if item["type"] == "movie" && item["title"] == "Inception" {
			streamURL = item["stream_url"].(string)
			break
		}
	}
	assert.NotEmpty(t, streamURL)

	w = httptest.NewRecorder()
	req, _ := http.NewRequest("GET", streamURL, nil)
	req.Header.Set("Range", "bytes=0-1023")
	router.ServeHTTP(w, req)
	assert.True(t, w.Code == 206 || w.Code == 200,
		"streaming should support Range requests, got %d", w.Code)
}

func TestIntegration_UnauthorizedAccess(t *testing.T) {
	router, cleanup := setupIntegrationRouter(t)
	defer cleanup()

	w := apiCall(t, router, "GET", "/api/v1/library", "", nil)
	assert.Equal(t, 401, w.Code)
}

func TestIntegration_RegisterDuplicate(t *testing.T) {
	router, cleanup := setupIntegrationRouter(t)
	defer cleanup()

	body := []byte(`{"username":"dupuser","password":"pass123"}`)
	w := apiCall(t, router, "POST", "/api/v1/auth/register", "", body)
	assert.Equal(t, 201, w.Code)

	w = apiCall(t, router, "POST", "/api/v1/auth/register", "", body)
	assert.Equal(t, 409, w.Code)
}
