package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"testing"

	"github.com/fyom/fyom/internal/model"
	"github.com/fyom/fyom/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── Test request builders ───────────────────────────────────────────────────
//
// These helpers simulate the request context state that AuthMiddleware and
// ResolvePermissions would normally populate, so the pure permission helpers
// can be unit-tested in isolation without spinning up a database.

func newPermTestRequest() *http.Request {
	return httptest.NewRequest(http.MethodGet, "/api/v1/library", nil)
}

func permReqWithRole(r *http.Request, role string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), keyRole, role))
}

func permReqWithUser(r *http.Request, userID string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), keyUserID, userID))
}

// permReqResolved simulates a request that has already passed through
// ResolvePermissions, with the given allowed library IDs stored in context.
// A nil slice means unrestricted (admin/owner); a non-nil empty slice means
// the user has explicit access to zero libraries.
func permReqResolved(r *http.Request, ids []string) *http.Request {
	return r.WithContext(withResolvedLibraryPermissions(r.Context(), ids))
}

// permReqPresigned simulates a request that has passed RequireValidPresign.
func permReqPresigned(r *http.Request) *http.Request {
	ctx := context.WithValue(r.Context(), ctxKeyPresignedAccess, true)
	ctx = context.WithValue(ctx, ctxKeyPresignedPath, r.URL.Path)
	return r.WithContext(ctx)
}

// applyMiddleware runs the given middleware chain against r with a terminal
// handler that captures the (possibly context-modified) request and writes a
// 200 OK. It returns the captured request (nil when the middleware
// short-circuited with a response) and the response recorder.
//
// Shared by permissions_test.go and presign_test.go (same package).
func applyMiddleware(t *testing.T, mw func(http.Handler) http.Handler, r *http.Request) (*http.Request, *httptest.ResponseRecorder) {
	t.Helper()
	var captured *http.Request
	rec := httptest.NewRecorder()
	next := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		captured = req
		w.WriteHeader(http.StatusOK)
	})
	mw(next).ServeHTTP(rec, r)
	return captured, rec
}

// ─── GetAllowedLibraryIDs ────────────────────────────────────────────────────

// Scenario 1: ResolvePermissions not executed + regular user => fail closed.
// The single most important security property: a route that forgot to mount
// ResolvePermissions must NOT silently grant unrestricted access.
func TestGetAllowedLibraryIDs_FailClosedWhenUnresolvedForRegularUser(t *testing.T) {
	r := permReqWithRole(newPermTestRequest(), "user")

	got := GetAllowedLibraryIDs(r)

	// Must be a non-nil empty slice. nil would mean unrestricted — a critical
	// regression. An empty slice means "access to nothing" (fail closed).
	require.NotNil(t, got, "unresolved regular user must get a non-nil slice (fail closed), not nil (unrestricted)")
	require.Empty(t, got, "unresolved regular user must get an empty slice")
}

// Scenario 2: ResolvePermissions not executed + admin/owner => unrestricted.
// Privileged roles are trusted to bypass library checks even when the
// middleware did not run (e.g. on admin-only routes that omit ResolvePermissions).
func TestGetAllowedLibraryIDs_UnrestrictedForAdminWhenUnresolved(t *testing.T) {
	r := permReqWithRole(newPermTestRequest(), "admin")
	require.Nil(t, GetAllowedLibraryIDs(r), "admin must be unrestricted even when ResolvePermissions did not run")
}

func TestGetAllowedLibraryIDs_UnrestrictedForOwnerWhenUnresolved(t *testing.T) {
	r := permReqWithRole(newPermTestRequest(), "owner")
	require.Nil(t, GetAllowedLibraryIDs(r), "owner must be unrestricted even when ResolvePermissions did not run")
}

// Scenario 3: ResolvePermissions executed + privileged role => nil (unrestricted).
func TestGetAllowedLibraryIDs_AdminGetsNilAfterResolve(t *testing.T) {
	r := permReqResolved(newPermTestRequest(), nil)
	require.Nil(t, GetAllowedLibraryIDs(r), "admin allowedIDs must be nil (unrestricted) after ResolvePermissions")
}

// Scenario 4: ResolvePermissions executed + regular user with no perms => empty.
// nil (unrestricted) and []string{} (no access) must stay distinguishable.
func TestGetAllowedLibraryIDs_RegularUserNoPermsGetsEmptyAfterResolve(t *testing.T) {
	r := permReqResolved(newPermTestRequest(), []string{})

	got := GetAllowedLibraryIDs(r)

	require.NotNil(t, got, "must be non-nil empty (not nil, which would mean unrestricted)")
	require.Empty(t, got)
}

// Scenario 4b: ResolvePermissions executed + regular user with explicit IDs.
func TestGetAllowedLibraryIDs_RegularUserGetsExplicitIDsAfterResolve(t *testing.T) {
	r := permReqResolved(newPermTestRequest(), []string{"lib-A", "lib-B"})

	got := GetAllowedLibraryIDs(r)

	require.Equal(t, []string{"lib-A", "lib-B"}, got)
}

// Context value of the wrong type must degrade to fail-closed, never panic.
func TestGetAllowedLibraryIDs_WrongContextTypeFailsClosed(t *testing.T) {
	// Start from a resolved request, then corrupt the allowed_libraries value
	// with a non-slice type to exercise the typed-assertion failure branch.
	r := permReqResolved(newPermTestRequest(), nil)
	r = r.WithContext(context.WithValue(r.Context(), ctxKeyAllowedLibraries, 12345))

	got := GetAllowedLibraryIDs(r)

	require.NotNil(t, got, "bad type must degrade to fail-closed empty slice, not nil")
	require.Empty(t, got)
}

// ─── IsLibraryAllowed ────────────────────────────────────────────────────────

// Scenario 5: allowed library => true.
func TestIsLibraryAllowed_AllowsPermittedLibrary(t *testing.T) {
	r := permReqResolved(newPermTestRequest(), []string{"lib-A", "lib-B"})

	assert.True(t, IsLibraryAllowed(r, "lib-A"))
	assert.True(t, IsLibraryAllowed(r, "lib-B"))
}

// Scenario 6: forbidden library => false. Empty library id => false.
func TestIsLibraryAllowed_DeniesForbiddenLibrary(t *testing.T) {
	r := permReqResolved(newPermTestRequest(), []string{"lib-A"})

	assert.False(t, IsLibraryAllowed(r, "lib-X"), "library not in allowed list must be denied")
	assert.False(t, IsLibraryAllowed(r, ""), "empty library id must be denied")
}

// Scenario 6b: unresolved regular user (fail-closed) cannot access any library.
func TestIsLibraryAllowed_UnresolvedRegularUserDeniedForAll(t *testing.T) {
	r := permReqWithRole(newPermTestRequest(), "user")

	assert.False(t, IsLibraryAllowed(r, "lib-A"))
	assert.False(t, IsLibraryAllowed(r, "anything"))
}

// Scenario 7: a valid presigned request is authorized for the signed path,
// regardless of the user's library permissions.
func TestIsLibraryAllowed_PresignedAccessBypassesLibraryCheck(t *testing.T) {
	// Even a fail-closed user with zero libraries is allowed when the request
	// carries a valid presign marker.
	r := permReqPresigned(permReqResolved(newPermTestRequest(), []string{}))

	assert.True(t, IsLibraryAllowed(r, "any-lib-id"), "presigned request must bypass library check")
}

// Scenario 7b: presigned marker alone (without ResolvePermissions) still authorizes.
// This mirrors the real presigned media route group, which mounts only
// RequireValidPresign and never ResolvePermissions.
func TestIsLibraryAllowed_PresignedWithoutResolveStillAuthorizes(t *testing.T) {
	r := permReqPresigned(newPermTestRequest())

	assert.True(t, IsLibraryAllowed(r, "any-lib-id"))
}

// ─── IsUnrestrictedLibraryAccess ─────────────────────────────────────────────

func TestIsUnrestrictedLibraryAccess(t *testing.T) {
	// Unresolved admin/owner => unrestricted.
	assert.True(t, IsUnrestrictedLibraryAccess(permReqWithRole(newPermTestRequest(), "admin")))
	assert.True(t, IsUnrestrictedLibraryAccess(permReqWithRole(newPermTestRequest(), "owner")))
	// Resolved nil => unrestricted.
	assert.True(t, IsUnrestrictedLibraryAccess(permReqResolved(newPermTestRequest(), nil)))
	// Resolved empty or non-empty => restricted.
	assert.False(t, IsUnrestrictedLibraryAccess(permReqResolved(newPermTestRequest(), []string{})))
	assert.False(t, IsUnrestrictedLibraryAccess(permReqResolved(newPermTestRequest(), []string{"lib-A"})))
	// Unresolved regular user => fail-closed (empty, not nil) => restricted.
	assert.False(t, IsUnrestrictedLibraryAccess(permReqWithRole(newPermTestRequest(), "user")))
}

// ─── ArePermissionsResolved ──────────────────────────────────────────────────

// Scenario 8: ArePermissionsResolved reflects whether ResolvePermissions ran.
func TestArePermissionsResolved_ReflectsMiddlewareExecution(t *testing.T) {
	assert.False(t, ArePermissionsResolved(newPermTestRequest()), "fresh request must report unresolved")
	assert.False(t, ArePermissionsResolved(permReqWithRole(newPermTestRequest(), "admin")), "role alone does not resolve permissions")
	assert.True(t, ArePermissionsResolved(permReqResolved(newPermTestRequest(), nil)), "resolved admin => true")
	assert.True(t, ArePermissionsResolved(permReqResolved(newPermTestRequest(), []string{})), "resolved empty => true")
	assert.True(t, ArePermissionsResolved(permReqResolved(newPermTestRequest(), []string{"lib-A"})), "resolved non-empty => true")
}

// ─── Nil-request safety ──────────────────────────────────────────────────────

func TestPermissionHelpers_NilRequestIsSafe(t *testing.T) {
	require.Empty(t, GetAllowedLibraryIDs(nil))
	assert.False(t, ArePermissionsResolved(nil))
	assert.False(t, IsLibraryAllowed(nil, "lib-A"))
	assert.False(t, IsUnrestrictedLibraryAccess(nil))
}

// ─── isPrivilegedRole ────────────────────────────────────────────────────────
//
// "owner" is accepted as privileged even though no code path currently mints
// an owner role. This is deliberate future-proofing for multi-admin ownership
// semantics; keep it locked in by a test so the behavior is not accidentally
// narrowed.

func TestIsPrivilegedRole(t *testing.T) {
	assert.True(t, isPrivilegedRole("admin"))
	assert.True(t, isPrivilegedRole("owner"))
	assert.False(t, isPrivilegedRole("user"))
	assert.False(t, isPrivilegedRole(""))
	assert.False(t, isPrivilegedRole("ADMIN")) // case-sensitive
}

// ─── ResolvePermissions (end-to-end middleware) ──────────────────────────────
//
// These tests exercise ResolvePermissions against a real SQLite database so
// that the context-writing behavior (nil for admin, explicit slice for users,
// empty for users with no permissions, 401 when user id is missing) is locked
// in. The pure helpers above cover the read side; these cover the write side.

func openPermMiddlewareTestDB(t *testing.T) *repository.DB {
	t.Helper()
	tmpDir := t.TempDir()
	db, err := repository.Open(filepath.Join(tmpDir, "fyom.db"), 5, 2, 60)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// seedPermFixtures creates three users (admin, regular with 2 libs, noperm)
// and two libraries, granting the regular user access to both. Returns the
// configured LibraryPermissionRepository for use with ResolvePermissions.
func seedPermFixtures(t *testing.T, db *repository.DB) *repository.LibraryPermissionRepository {
	t.Helper()
	ctx := context.Background()

	userRepo := repository.NewUserRepository(db)
	libRepo := repository.NewLibraryRepository(db)
	libPermRepo := repository.NewLibraryPermissionRepository(db)

	for _, u := range []*model.User{
		{ID: "u-admin", Username: "admin", Password: "x", Role: "admin"},
		{ID: "u-regular", Username: "regular", Password: "x", Role: "user"},
		{ID: "u-noperm", Username: "noperm", Password: "x", Role: "user"},
	} {
		require.NoError(t, userRepo.Create(ctx, u))
	}

	for _, lib := range []*model.Library{
		{ID: "lib-A", Name: "Lib A", Type: "movie", ProviderID: "local", SourcePath: "/tmp/a", MetadataSource: "nfo"},
		{ID: "lib-B", Name: "Lib B", Type: "movie", ProviderID: "local", SourcePath: "/tmp/b", MetadataSource: "nfo"},
	} {
		require.NoError(t, libRepo.Create(ctx, lib))
	}

	require.NoError(t, libPermRepo.SetPermission(ctx, "u-regular", "lib-A", true))
	require.NoError(t, libPermRepo.SetPermission(ctx, "u-regular", "lib-B", true))

	return libPermRepo
}

func TestResolvePermissions_AdminStoresNilAndMarksResolved(t *testing.T) {
	libPermRepo := seedPermFixtures(t, openPermMiddlewareTestDB(t))

	r := permReqWithUser(permReqWithRole(newPermTestRequest(), "admin"), "u-admin")
	captured, rec := applyMiddleware(t, ResolvePermissions(libPermRepo), r)

	require.NotNil(t, captured, "admin should pass through to next handler")
	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, ArePermissionsResolved(captured), "permissions must be marked resolved")
	require.Nil(t, GetAllowedLibraryIDs(captured), "admin allowedIDs must be nil (unrestricted)")
}

func TestResolvePermissions_RegularUserStoresExplicitLibraryIDs(t *testing.T) {
	libPermRepo := seedPermFixtures(t, openPermMiddlewareTestDB(t))

	r := permReqWithUser(permReqWithRole(newPermTestRequest(), "user"), "u-regular")
	captured, rec := applyMiddleware(t, ResolvePermissions(libPermRepo), r)

	require.NotNil(t, captured)
	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, ArePermissionsResolved(captured))

	got := GetAllowedLibraryIDs(captured)
	require.NotNil(t, got, "regular user must get a non-nil slice")
	sort.Strings(got)
	assert.Equal(t, []string{"lib-A", "lib-B"}, got)
}

func TestResolvePermissions_RegularUserWithNoPermissionsStoresEmpty(t *testing.T) {
	libPermRepo := seedPermFixtures(t, openPermMiddlewareTestDB(t))

	r := permReqWithUser(permReqWithRole(newPermTestRequest(), "user"), "u-noperm")
	captured, rec := applyMiddleware(t, ResolvePermissions(libPermRepo), r)

	require.NotNil(t, captured)
	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, ArePermissionsResolved(captured))

	got := GetAllowedLibraryIDs(captured)
	require.NotNil(t, got, "user with no permissions must get non-nil empty slice (not nil/unrestricted)")
	require.Empty(t, got)
}

// A regular user whose user_id is missing from context (e.g. AuthMiddleware
// was misconfigured) must be rejected with 401, not silently granted access.
func TestResolvePermissions_RegularUserMissingUserIDReturns401(t *testing.T) {
	libPermRepo := seedPermFixtures(t, openPermMiddlewareTestDB(t))

	// role=user but no user_id in context.
	r := permReqWithRole(newPermTestRequest(), "user")
	captured, rec := applyMiddleware(t, ResolvePermissions(libPermRepo), r)

	require.Nil(t, captured, "middleware must short-circuit when user id is missing")
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

// A regular user whose user_id is present but not a string must be rejected.
func TestResolvePermissions_RegularUserNonStringUserIDReturns401(t *testing.T) {
	libPermRepo := seedPermFixtures(t, openPermMiddlewareTestDB(t))

	r := permReqWithRole(newPermTestRequest(), "user")
	// Stash a non-string user id (simulating a malformed context).
	r = r.WithContext(context.WithValue(r.Context(), keyUserID, 42))
	captured, rec := applyMiddleware(t, ResolvePermissions(libPermRepo), r)

	require.Nil(t, captured)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}
