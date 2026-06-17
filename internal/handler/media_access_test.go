package handler

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fyom/fyom/internal/middleware"
	"github.com/fyom/fyom/internal/model"
	"github.com/fyom/fyom/internal/provider"
	"github.com/fyom/fyom/internal/repository"
	"github.com/fyom/fyom/pkg/presign"
	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── Test fixtures ───────────────────────────────────────────────────────────
//
// media_access_test.go verifies the library-access security model end-to-end
// through the real chi route stack (registerUserRoutes + registerPresignedMediaRoutes).
//
// Fixture topology:
//
//   u-normal  (role=user)  -> can view lib-allowed ONLY
//   u-admin   (role=admin) -> unrestricted
//
//   lib-allowed  -> media-allowed (movie), show-allowed (show) -> ep-allowed (episode)
//   lib-denied   -> media-denied  (movie)
//
//   job-allowed  -> ImportJob(library_id=lib-allowed)
//   job-denied   -> ImportJob(library_id=lib-denied)
//
// media-allowed / media-denied / ep-allowed have real on-disk files so the
// presigned stream/poster endpoints can actually serve bytes.

type mediaAccessFixtures struct {
	router      http.Handler
	db          *repository.DB
	signer      *presign.Signer
	normalToken string
	adminToken  string

	libAllowed   string
	libDenied    string
	mediaAllowed string
	mediaDenied  string
	showAllowed  string
	episodeOK    string
	jobAllowed   string
	jobDenied    string
}

// makeTokenForUser mints a JWT for an arbitrary user id/role. The existing
// makeTestTokenWithRole hardcodes sub="test-user-id", which is insufficient for
// tests that need two distinct users (normal + admin) in the same DB.
func makeTokenForUser(secret, userID, username, role string) string {
	claims := jwt.MapClaims{
		"sub":      userID,
		"username": username,
		"role":     role,
		"iat":      time.Now().Unix(),
		"exp":      time.Now().Add(24 * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, _ := token.SignedString([]byte(secret))
	return s
}

// doReq is a thin wrapper around httptest.NewRecorder + http.NewRequest that
// sets the Authorization bearer token and JSON content type when a body is
// supplied. Returns the recorded response.
func doReq(t *testing.T, router http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, path, reader)
	require.NoError(t, err)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// requireDenied404 asserts that a forbidden-library response is a 404 (not 403),
// which is the anti-enumeration contract: forbidden access must be
// indistinguishable from "resource not found".
func requireDenied404(t *testing.T, rec *httptest.ResponseRecorder, label string) {
	t.Helper()
	require.Equalf(t, http.StatusNotFound, rec.Code, "%s: expected 404 (anti-enumeration), got %d body=%s", label, rec.Code, rec.Body.String())
}

func setupMediaAccessRouter(t *testing.T) *mediaAccessFixtures {
	t.Helper()
	tmpDir := t.TempDir()
	db, err := repository.Open(filepath.Join(tmpDir, "fyom.db"), 5, 2, 60)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	ctx := context.Background()
	userRepo := repository.NewUserRepository(db)
	libRepo := repository.NewLibraryRepository(db)
	libPermRepo := repository.NewLibraryPermissionRepository(db)
	mediaRepo := repository.NewMediaRepository(db)
	jobRepo := repository.NewImportJobRepository(db)
	providerRepo := repository.NewProviderRepository(db)
	statusRepo := repository.NewUserMediaStatusRepository(db)

	// Users: one regular (restricted), one admin (unrestricted).
	require.NoError(t, userRepo.Create(ctx, &model.User{ID: "u-normal", Username: "normal", Password: "x", Role: "user"}))
	require.NoError(t, userRepo.Create(ctx, &model.User{ID: "u-admin", Username: "admin", Password: "x", Role: "admin"}))

	// Libraries.
	require.NoError(t, libRepo.Create(ctx, &model.Library{ID: "lib-allowed", Name: "Allowed", Type: "movie", ProviderID: "local", SourcePath: "/tmp/allowed", MetadataSource: "nfo"}))
	require.NoError(t, libRepo.Create(ctx, &model.Library{ID: "lib-denied", Name: "Denied", Type: "movie", ProviderID: "local", SourcePath: "/tmp/denied", MetadataSource: "nfo"}))

	// Grant u-normal access ONLY to lib-allowed.
	require.NoError(t, libPermRepo.SetPermission(ctx, "u-normal", "lib-allowed", true))

	// Real on-disk media files (file_path is NOT NULL UNIQUE, so each must differ).
	allowedFile := filepath.Join(tmpDir, "allowed.mkv")
	allowedPoster := filepath.Join(tmpDir, "allowed.jpg")
	deniedFile := filepath.Join(tmpDir, "denied.mkv")
	deniedPoster := filepath.Join(tmpDir, "denied.jpg")
	epFile := filepath.Join(tmpDir, "ep.mkv")
	for _, p := range []string{allowedFile, allowedPoster, deniedFile, deniedPoster, epFile} {
		require.NoError(t, os.WriteFile(p, []byte("fake-media-bytes"), 0o644))
	}

	// Media items.
	require.NoError(t, mediaRepo.Create(ctx, &model.MediaItem{
		ID: "media-allowed", Type: "movie", Title: "Allowed Movie", SortTitle: "allowed movie",
		FilePath: allowedFile, RootPath: tmpDir, PrimaryPath: allowedFile, NFOPath: filepath.Join(tmpDir, "allowed.nfo"),
		PosterPath: allowedPoster, MetadataSource: "nfo", ProviderID: "local", LibraryID: "lib-allowed", Status: "available",
	}))
	require.NoError(t, mediaRepo.Create(ctx, &model.MediaItem{
		ID: "media-denied", Type: "movie", Title: "Denied Movie", SortTitle: "denied movie",
		FilePath: deniedFile, RootPath: tmpDir, PrimaryPath: deniedFile, NFOPath: filepath.Join(tmpDir, "denied.nfo"),
		PosterPath: deniedPoster, MetadataSource: "nfo", ProviderID: "local", LibraryID: "lib-denied", Status: "available",
	}))
	require.NoError(t, mediaRepo.Create(ctx, &model.MediaItem{
		ID: "show-allowed", Type: "show", Title: "Allowed Show", SortTitle: "allowed show",
		FilePath: filepath.Join(tmpDir, "show.nfo"), RootPath: tmpDir, PrimaryPath: filepath.Join(tmpDir, "show.nfo"),
		NFOPath: filepath.Join(tmpDir, "show.nfo"), MetadataSource: "nfo", ProviderID: "local", LibraryID: "lib-allowed", Status: "available",
	}))
	season1, episode1 := 1, 1
	require.NoError(t, mediaRepo.Create(ctx, &model.MediaItem{
		ID: "ep-allowed", Type: "episode", Title: "Episode 1", SortTitle: "episode 1",
		ParentID: "show-allowed", Season: &season1, Episode: &episode1,
		FilePath: epFile, RootPath: tmpDir, PrimaryPath: epFile, NFOPath: filepath.Join(tmpDir, "ep.nfo"),
		MetadataSource: "nfo", ProviderID: "local", LibraryID: "lib-allowed", Status: "available",
	}))

	// Import jobs in both libraries.
	jobAllowed, err := jobRepo.Create(ctx, "/tmp/source-allowed", "lib-allowed")
	require.NoError(t, err)
	jobDenied, err := jobRepo.Create(ctx, "/tmp/source-denied", "lib-denied")
	require.NoError(t, err)

	// Provider registry + signer (shared secret with the router below).
	signer := presign.NewSigner("test-secret", 3600)
	reg := provider.NewRegistry()
	reg.Register(provider.NewLocalProvider(signer))

	mediaHandler := NewMediaHandler(reg, db, mediaRepo, jobRepo, providerRepo, libRepo, statusRepo, slog.Default(), &mockRefreshCoordinator{})

	// Router mirrors internal/server/server.go registerUserRoutes +
	// registerPresignedMediaRoutes exactly (including route ordering, since chi
	// matches literals before params only when registered first).
	r := chi.NewRouter()
	r.Use(middleware.ErrorHandler())
	r.Group(func(r chi.Router) {
		r.Use(middleware.AuthMiddlewareWithUserRepo("test-secret", userRepo))
		r.Use(middleware.ResolvePermissions(libPermRepo))

		r.Get("/api/v1/libraries", mediaHandler.GetLibraries)
		r.Get("/api/v1/library", mediaHandler.List)
		r.Get("/api/v1/library/continue", mediaHandler.GetContinueWatching)
		r.Get("/api/v1/library/by-status", mediaHandler.GetByStatus)
		r.Get("/api/v1/library/jobs/{id}", mediaHandler.GetJob)
		r.Get("/api/v1/library/{id}/episodes", mediaHandler.ListEpisodes)
		r.Get("/api/v1/library/{id}", mediaHandler.Get)

		r.Put("/api/v1/media/{id}/progress", mediaHandler.UpdateProgress)
		r.Put("/api/v1/media/{id}/status", mediaHandler.SetStatus)
		r.Get("/api/v1/media/{id}/status", mediaHandler.GetStatus)

		r.With(middleware.RequireAdmin).Post("/api/v1/library/import", mediaHandler.Import)
		r.With(middleware.RequireAdmin).Delete("/api/v1/library/{id}", mediaHandler.Delete)
	})
	r.Route("/api/v1/media", func(r chi.Router) {
		r.Use(middleware.RequireValidPresign(signer))
		r.Get("/{id}/poster", mediaHandler.Poster)
		r.Get("/{id}/backdrop", mediaHandler.ServeBackdrop)
		r.Get("/{id}/stream", mediaHandler.Stream)
		r.Get("/{id}/logo", mediaHandler.ServeLogo)
	})

	return &mediaAccessFixtures{
		router:       r,
		db:           db,
		signer:       signer,
		normalToken:  makeTokenForUser("test-secret", "u-normal", "normal", "user"),
		adminToken:   makeTokenForUser("test-secret", "u-admin", "admin", "admin"),
		libAllowed:   "lib-allowed",
		libDenied:    "lib-denied",
		mediaAllowed: "media-allowed",
		mediaDenied:  "media-denied",
		showAllowed:  "show-allowed",
		episodeOK:    "ep-allowed",
		jobAllowed:   jobAllowed.ID,
		jobDenied:    jobDenied.ID,
	}
}

// presignURL builds a valid presigned URL for the given media resource path.
func (f *mediaAccessFixtures) presignURL(path string) string {
	return f.signer.Generate(path)
}

// ─── Read endpoints: anti-enumeration (404 for forbidden) ───────────────────

// Scenario 1: denied-library media detail returns 404 for a normal user.
func TestMediaAccess_DeniedLibraryGetReturns404(t *testing.T) {
	f := setupMediaAccessRouter(t)

	// Allowed media -> 200.
	rec := doReq(t, f.router, http.MethodGet, "/api/v1/library/"+f.mediaAllowed, f.normalToken, "")
	require.Equal(t, http.StatusOK, rec.Code, "allowed media should be accessible")

	// Denied media -> 404 (NOT 403, to avoid leaking existence).
	rec = doReq(t, f.router, http.MethodGet, "/api/v1/library/"+f.mediaDenied, f.normalToken, "")
	requireDenied404(t, rec, "denied media detail")
}

// Anti-enumeration: a forbidden media id and a truly-nonexistent media id must
// both return 404 with the same response shape.
func TestMediaAccess_ForbiddenAndNotFoundAreIndistinguishable(t *testing.T) {
	f := setupMediaAccessRouter(t)

	recDenied := doReq(t, f.router, http.MethodGet, "/api/v1/library/"+f.mediaDenied, f.normalToken, "")
	recMissing := doReq(t, f.router, http.MethodGet, "/api/v1/library/does-not-exist-at-all", f.normalToken, "")

	require.Equal(t, http.StatusNotFound, recDenied.Code)
	require.Equal(t, http.StatusNotFound, recMissing.Code)

	var deniedBody, missingBody map[string]interface{}
	require.NoError(t, json.Unmarshal(recDenied.Body.Bytes(), &deniedBody))
	require.NoError(t, json.Unmarshal(recMissing.Body.Bytes(), &missingBody))
	assert.Equal(t, deniedBody["message"], missingBody["message"], "forbidden and not-found must share the same error message")
}

// ─── Write endpoints: progress / status ──────────────────────────────────────

// Scenario 2: denied-library update progress returns 404.
func TestMediaAccess_DeniedLibraryUpdateProgressReturns404(t *testing.T) {
	f := setupMediaAccessRouter(t)
	body := `{"position":50,"duration":200,"finished":false}`

	// Allowed -> 204 (NoContent).
	rec := doReq(t, f.router, http.MethodPut, "/api/v1/media/"+f.mediaAllowed+"/progress", f.normalToken, body)
	require.Equal(t, http.StatusNoContent, rec.Code, "allowed media progress should be accepted")

	// Denied -> 404.
	rec = doReq(t, f.router, http.MethodPut, "/api/v1/media/"+f.mediaDenied+"/progress", f.normalToken, body)
	requireDenied404(t, rec, "denied media update progress")
}

// Scenario 3: denied-library set status returns 404.
func TestMediaAccess_DeniedLibrarySetStatusReturns404(t *testing.T) {
	f := setupMediaAccessRouter(t)
	body := `{"status":"watching"}`

	rec := doReq(t, f.router, http.MethodPut, "/api/v1/media/"+f.mediaAllowed+"/status", f.normalToken, body)
	require.Equal(t, http.StatusOK, rec.Code, "allowed media set status should succeed")

	rec = doReq(t, f.router, http.MethodPut, "/api/v1/media/"+f.mediaDenied+"/status", f.normalToken, body)
	requireDenied404(t, rec, "denied media set status")
}

// Scenario 4: denied-library get status returns 404.
func TestMediaAccess_DeniedLibraryGetStatusReturns404(t *testing.T) {
	f := setupMediaAccessRouter(t)

	rec := doReq(t, f.router, http.MethodGet, "/api/v1/media/"+f.mediaAllowed+"/status", f.normalToken, "")
	require.Equal(t, http.StatusOK, rec.Code, "allowed media get status should succeed")

	rec = doReq(t, f.router, http.MethodGet, "/api/v1/media/"+f.mediaDenied+"/status", f.normalToken, "")
	requireDenied404(t, rec, "denied media get status")
}

// ─── Serve endpoints (presigned route group) ────────────────────────────────
//
// The presigned media group is guarded ONLY by RequireValidPresign (no JWT, no
// ResolvePermissions). A valid presigned URL is a path-bound, time-limited
// capability token that authorizes direct access to the signed resource —
// including media in libraries the user has otherwise lost access to. This is
// the intended design: <img>/<video> tags cannot send JWTs, so the presigned
// URL IS the authorization.
//
// Therefore:
//   - no presign params          -> 403 (RequireValidPresign rejects)
//   - valid presign              -> 200 (capability honored, library check bypassed)
//   - valid presign, other path  -> 403 (path-bound)
//   - valid presign, missing id  -> 404 (genuinely not found)

// Scenario 5: denied-library stream without a presign is blocked at the middleware.
func TestMediaAccess_StreamWithoutPresignReturns403(t *testing.T) {
	f := setupMediaAccessRouter(t)

	rec := doReq(t, f.router, http.MethodGet, "/api/v1/media/"+f.mediaDenied+"/stream", "", "")
	require.Equal(t, http.StatusForbidden, rec.Code, "stream without presign must be rejected by RequireValidPresign")
}

// Scenario 5b: a valid presign authorizes streaming a denied-library media
// (presigned URLs are capability tokens that bypass library revocation until exp).
func TestMediaAccess_StreamWithValidPresignBypassesLibraryCheck(t *testing.T) {
	f := setupMediaAccessRouter(t)

	url := f.presignURL("/api/v1/media/" + f.mediaDenied + "/stream")
	rec := doReq(t, f.router, http.MethodGet, url, "", "")
	require.Equal(t, http.StatusOK, rec.Code, "valid presign must bypass library check for direct media fetch")
	assert.NotEmpty(t, rec.Body.Bytes(), "stream should serve file bytes")
}

// Scenario 5c: a presign bound to media A must not authorize media B.
func TestMediaAccess_StreamPresignIsPathBound(t *testing.T) {
	f := setupMediaAccessRouter(t)

	// Sign for media-allowed, request media-denied with the same exp+sig.
	allowedURL := f.presignURL("/api/v1/media/" + f.mediaAllowed + "/stream")
	// Swap the path segment while keeping the query string.
	deniedWithWrongSig := strings.Replace(allowedURL, f.mediaAllowed, f.mediaDenied, 1)
	rec := doReq(t, f.router, http.MethodGet, deniedWithWrongSig, "", "")
	require.Equal(t, http.StatusForbidden, rec.Code, "presign for media A must not authorize media B")
}

// Scenario 5d: a valid presign for a non-existent media id still returns 404
// (the handler's not-found path is reachable even through the presign group).
func TestMediaAccess_StreamValidPresignNotFoundReturns404(t *testing.T) {
	f := setupMediaAccessRouter(t)

	url := f.presignURL("/api/v1/media/no-such-media/stream")
	rec := doReq(t, f.router, http.MethodGet, url, "", "")
	require.Equal(t, http.StatusNotFound, rec.Code, "not-found media must 404 even with a valid presign")
}

// Scenario 6: poster behaves identically to stream — presign-gated.
func TestMediaAccess_PosterWithoutPresignReturns403_WithPresignBypasses(t *testing.T) {
	f := setupMediaAccessRouter(t)

	// No presign -> 403.
	rec := doReq(t, f.router, http.MethodGet, "/api/v1/media/"+f.mediaDenied+"/poster", "", "")
	require.Equal(t, http.StatusForbidden, rec.Code)

	// Valid presign on denied-library poster -> 200.
	url := f.presignURL("/api/v1/media/" + f.mediaDenied + "/poster")
	rec = doReq(t, f.router, http.MethodGet, url, "", "")
	require.Equal(t, http.StatusOK, rec.Code, "valid presign must bypass library check for poster")
	assert.NotEmpty(t, rec.Body.Bytes())
}

// Scenario 6b: backdrop + logo presign gating.
func TestMediaAccess_BackdropAndLogoPresignGated(t *testing.T) {
	f := setupMediaAccessRouter(t)

	for _, res := range []string{"backdrop", "logo"} {
		path := "/api/v1/media/" + f.mediaDenied + "/" + res
		// No presign -> 403.
		rec := doReq(t, f.router, http.MethodGet, path, "", "")
		require.Equalf(t, http.StatusForbidden, rec.Code, "%s without presign must be 403", res)
	}
}

// ─── List endpoints: library filtering ───────────────────────────────────────

// Scenario 7: GetByStatus filters out items from forbidden libraries.
// Simulates "user had access, set a status, then lost library access": we seed
// a user_media_status row for a denied-library item directly in the DB, then
// verify GetByStatus does NOT return it.
func TestMediaAccess_GetByStatusFiltersForbiddenLibraries(t *testing.T) {
	f := setupMediaAccessRouter(t)
	ctx := context.Background()
	statusRepo := repository.NewUserMediaStatusRepository(f.db)

	// Seed status on BOTH an allowed and a denied item for u-normal. Only the
	// allowed one should surface through the API.
	require.NoError(t, statusRepo.SetStatus(ctx, "u-normal", f.mediaAllowed, "watching"))
	require.NoError(t, statusRepo.SetStatus(ctx, "u-normal", f.mediaDenied, "watching"))

	rec := doReq(t, f.router, http.MethodGet, "/api/v1/library/by-status?status=watching", f.normalToken, "")
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Data struct {
			Items []struct {
				ID        string `json:"id"`
				LibraryID string `json:"library_id"`
			} `json:"items"`
			Total int `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	// Only media-allowed must appear; media-denied must be filtered out.
	ids := make(map[string]bool, len(resp.Data.Items))
	for _, it := range resp.Data.Items {
		ids[it.ID] = true
		assert.NotEqual(t, f.libDenied, it.LibraryID, "denied-library item must not appear in by-status")
	}
	assert.True(t, ids[f.mediaAllowed], "allowed item must appear in by-status")
	assert.False(t, ids[f.mediaDenied], "denied-library item must be filtered out of by-status")
}

// Scenario 8: GetContinueWatching filters out items from forbidden libraries.
// Watch-progress filtering happens at the SQL layer (allowedLibraryIDs IN ...),
// but the contract is the same: revoked-library progress must not surface.
func TestMediaAccess_ContinueWatchingFiltersForbiddenLibraries(t *testing.T) {
	f := setupMediaAccessRouter(t)
	ctx := context.Background()
	mediaRepo := repository.NewMediaRepository(f.db)

	// Seed unfinished progress on both an allowed and a denied item.
	require.NoError(t, mediaRepo.UpsertProgress(ctx, "u-normal", f.mediaAllowed, 50, 200, false))
	require.NoError(t, mediaRepo.UpsertProgress(ctx, "u-normal", f.mediaDenied, 50, 200, false))

	rec := doReq(t, f.router, http.MethodGet, "/api/v1/library/continue", f.normalToken, "")
	require.Equal(t, http.StatusOK, rec.Code)

	// The response is a JSON array (wrapped in data). Parse generically.
	var resp struct {
		Data []map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))

	ids := make(map[string]bool, len(resp.Data))
	for _, it := range resp.Data {
		if id, ok := it["id"].(string); ok {
			ids[id] = true
		}
	}
	assert.True(t, ids[f.mediaAllowed], "allowed item with progress must appear in continue-watching")
	assert.False(t, ids[f.mediaDenied], "denied-library item must be filtered out of continue-watching")
}

// ─── Job endpoint ────────────────────────────────────────────────────────────

// Scenario 9: GetJob returns 404 for a job belonging to a forbidden library.
func TestMediaAccess_GetJobForbiddenLibraryReturns404(t *testing.T) {
	f := setupMediaAccessRouter(t)

	// Allowed-library job -> 200.
	rec := doReq(t, f.router, http.MethodGet, "/api/v1/library/jobs/"+f.jobAllowed, f.normalToken, "")
	require.Equal(t, http.StatusOK, rec.Code, "allowed-library job should be visible")

	// Denied-library job -> 404.
	rec = doReq(t, f.router, http.MethodGet, "/api/v1/library/jobs/"+f.jobDenied, f.normalToken, "")
	requireDenied404(t, rec, "denied-library job")
}

// ─── Admin-only write endpoints ──────────────────────────────────────────────

// Scenario 10: Import returns 403 for a non-admin (RequireAdmin route gate).
func TestMediaAccess_ImportRejectsNonAdmin(t *testing.T) {
	f := setupMediaAccessRouter(t)
	body := `{"source_path":"/tmp/x","provider_id":"local","library_id":"` + f.libAllowed + `"}`

	rec := doReq(t, f.router, http.MethodPost, "/api/v1/library/import", f.normalToken, body)
	require.Equal(t, http.StatusForbidden, rec.Code, "non-admin import must be rejected by RequireAdmin")
}

// Scenario 11: Delete returns 403 for a non-admin.
func TestMediaAccess_DeleteRejectsNonAdmin(t *testing.T) {
	f := setupMediaAccessRouter(t)

	rec := doReq(t, f.router, http.MethodDelete, "/api/v1/library/"+f.mediaAllowed, f.normalToken, "")
	require.Equal(t, http.StatusForbidden, rec.Code, "non-admin delete must be rejected by RequireAdmin")
}

// Scenario 12: admin can access every library — denied media, denied job, and
// delete across libraries all succeed for an admin token.
func TestMediaAccess_AdminCanAccessAllLibraries(t *testing.T) {
	f := setupMediaAccessRouter(t)

	// Admin reads denied-library media -> 200.
	rec := doReq(t, f.router, http.MethodGet, "/api/v1/library/"+f.mediaDenied, f.adminToken, "")
	require.Equal(t, http.StatusOK, rec.Code, "admin must read denied-library media")

	// Admin reads denied-library job -> 200.
	rec = doReq(t, f.router, http.MethodGet, "/api/v1/library/jobs/"+f.jobDenied, f.adminToken, "")
	require.Equal(t, http.StatusOK, rec.Code, "admin must read denied-library job")

	// Admin sets status on denied-library media -> 200.
	rec = doReq(t, f.router, http.MethodPut, "/api/v1/media/"+f.mediaDenied+"/status", f.adminToken, `{"status":"watched"}`)
	require.Equal(t, http.StatusOK, rec.Code, "admin must set status on denied-library media")
}

// Admin can delete across libraries.
func TestMediaAccess_AdminCanDeleteAcrossLibraries(t *testing.T) {
	f := setupMediaAccessRouter(t)

	rec := doReq(t, f.router, http.MethodDelete, "/api/v1/library/"+f.mediaDenied, f.adminToken, "")
	require.Equal(t, http.StatusNoContent, rec.Code, "admin must delete denied-library media")

	// Subsequent get -> 404 (truly gone).
	rec = doReq(t, f.router, http.MethodGet, "/api/v1/library/"+f.mediaDenied, f.adminToken, "")
	require.Equal(t, http.StatusNotFound, rec.Code)
}

// ─── List with library_id filter + GetLibraries ─────────────────────────────

// Normal user listing with library_id=lib-denied gets 404 (IsLibraryAllowed).
func TestMediaAccess_ListDeniedLibraryIDReturns404(t *testing.T) {
	f := setupMediaAccessRouter(t)

	rec := doReq(t, f.router, http.MethodGet, "/api/v1/library?library_id="+f.libAllowed, f.normalToken, "")
	require.Equal(t, http.StatusOK, rec.Code, "listing an allowed library must succeed")

	rec = doReq(t, f.router, http.MethodGet, "/api/v1/library?library_id="+f.libDenied, f.normalToken, "")
	requireDenied404(t, rec, "listing a denied library")
}

// Admin can list any library_id.
func TestMediaAccess_AdminCanListAnyLibraryID(t *testing.T) {
	f := setupMediaAccessRouter(t)

	rec := doReq(t, f.router, http.MethodGet, "/api/v1/library?library_id="+f.libDenied, f.adminToken, "")
	require.Equal(t, http.StatusOK, rec.Code, "admin must list denied-library items")
}

// GetLibraries returns only allowed libraries for a normal user, all for admin.
func TestMediaAccess_GetLibrariesFiltersByPermission(t *testing.T) {
	f := setupMediaAccessRouter(t)

	// Normal user -> only lib-allowed.
	rec := doReq(t, f.router, http.MethodGet, "/api/v1/libraries", f.normalToken, "")
	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Data []map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	ids := make(map[string]bool, len(resp.Data))
	for _, lib := range resp.Data {
		if id, ok := lib["id"].(string); ok {
			ids[id] = true
		}
	}
	assert.True(t, ids[f.libAllowed], "normal user must see allowed library")
	assert.False(t, ids[f.libDenied], "normal user must NOT see denied library")

	// Admin -> both libraries.
	rec = doReq(t, f.router, http.MethodGet, "/api/v1/libraries", f.adminToken, "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	ids = make(map[string]bool, len(resp.Data))
	for _, lib := range resp.Data {
		if id, ok := lib["id"].(string); ok {
			ids[id] = true
		}
	}
	assert.True(t, ids[f.libAllowed])
	assert.True(t, ids[f.libDenied], "admin must see denied library")
}

// ─── ListEpisodes ────────────────────────────────────────────────────────────

// Normal user can list episodes for an allowed show, but not for a denied-library parent.
func TestMediaAccess_ListEpisodesEnforcesLibraryAccess(t *testing.T) {
	f := setupMediaAccessRouter(t)

	// Allowed show -> 200, includes ep-allowed.
	rec := doReq(t, f.router, http.MethodGet, "/api/v1/library/"+f.showAllowed+"/episodes", f.normalToken, "")
	require.Equal(t, http.StatusOK, rec.Code)
	// The response is a JSON array (wrapped in data).
	var resp struct {
		Data []map[string]interface{} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NotEmpty(t, resp.Data, "allowed show should have at least one episode")

	// Denied-library parent (media-denied is a movie in lib-denied) -> 404
	// (getAccessibleMediaItem blocks before the show-type check).
	rec = doReq(t, f.router, http.MethodGet, "/api/v1/library/"+f.mediaDenied+"/episodes", f.normalToken, "")
	requireDenied404(t, rec, "episodes of denied-library parent")
}

// ListEpisodes rejects a movie parent with 400 (not a show) when the user has access.
func TestMediaAccess_ListEpisodesRejectsNonShow(t *testing.T) {
	f := setupMediaAccessRouter(t)

	rec := doReq(t, f.router, http.MethodGet, "/api/v1/library/"+f.mediaAllowed+"/episodes", f.normalToken, "")
	require.Equal(t, http.StatusBadRequest, rec.Code, "movie parent must be rejected as not-a-show")
}

// ─── Fail-closed: missing ResolvePermissions ─────────────────────────────────
//
// If a route forgot to mount ResolvePermissions, a regular user must NOT get
// unrestricted access. This is the most dangerous regression class and is
// locked in by middleware-level tests too; here we verify it end-to-end by
// mounting a route WITHOUT ResolvePermissions and confirming a normal user
// sees an empty result set (fail closed), not the full library.

func TestMediaAccess_FailClosedWhenResolvePermissionsAbsent(t *testing.T) {
	f := setupMediaAccessRouter(t)

	// Build a side router that mounts AuthMiddleware but OMITS ResolvePermissions,
	// exposing only the List endpoint.
	userRepo := repository.NewUserRepository(f.db)
	mediaRepo := repository.NewMediaRepository(f.db)
	libRepo := repository.NewLibraryRepository(f.db)
	jobRepo := repository.NewImportJobRepository(f.db)
	providerRepo := repository.NewProviderRepository(f.db)
	statusRepo := repository.NewUserMediaStatusRepository(f.db)
	signer := presign.NewSigner("test-secret", 3600)
	reg := provider.NewRegistry()
	reg.Register(provider.NewLocalProvider(signer))
	mediaHandler := NewMediaHandler(reg, f.db, mediaRepo, jobRepo, providerRepo, libRepo, statusRepo, slog.Default(), &mockRefreshCoordinator{})

	r := chi.NewRouter()
	r.Use(middleware.ErrorHandler())
	r.Group(func(r chi.Router) {
		r.Use(middleware.AuthMiddlewareWithUserRepo("test-secret", userRepo))
		// NOTE: ResolvePermissions intentionally OMITTED.
		r.Get("/api/v1/library", mediaHandler.List)
	})

	// Normal user listing without ResolvePermissions -> empty result (fail closed),
	// NOT the full library.
	rec := doReq(t, r, http.MethodGet, "/api/v1/library", f.normalToken, "")
	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Data struct {
			Items []map[string]interface{} `json:"items"`
			Total int                      `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Empty(t, resp.Data.Items, "unresolved regular user must see NO items (fail closed), not the full library")
	assert.Equal(t, 0, resp.Data.Total)

	// Admin without ResolvePermissions -> still unrestricted (sees items).
	rec = doReq(t, r, http.MethodGet, "/api/v1/library", f.adminToken, "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.Data.Items, "admin must remain unrestricted even without ResolvePermissions")
}

// ─── Unauthenticated requests ────────────────────────────────────────────────

// A missing/invalid token on the authenticated group is rejected with 401.
func TestMediaAccess_MissingTokenReturns401(t *testing.T) {
	f := setupMediaAccessRouter(t)

	rec := doReq(t, f.router, http.MethodGet, "/api/v1/library/"+f.mediaAllowed, "", "")
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	rec = doReq(t, f.router, http.MethodGet, "/api/v1/library", "not-a-real-token", "")
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}
