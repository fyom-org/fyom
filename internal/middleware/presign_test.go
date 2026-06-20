package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fyom/fyom/pkg/presign"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── Test fixtures ───────────────────────────────────────────────────────────

const (
	presignTestSecret = "presign-test-secret"
	presignTestPath   = "/api/v1/media/abc-123/poster"
)

func newPresignTestSigner() *presign.Signer {
	return presign.NewSigner(presignTestSecret, 3600)
}

// buildSignedURL produces a request URL for presignTestPath carrying a valid
// signature for the given method and expiry. sig is hex (URL-safe), so no
// escaping is required.
func buildSignedURL(_ string, path string, exp int64, sig string) string {
	return fmt.Sprintf("%s?exp=%d&sig=%s", path, exp, sig)
}

// ─── RequireValidPresign: happy paths ────────────────────────────────────────

// Scenario 1: valid GET presign passes.
func TestRequireValidPresign_ValidGETPasses(t *testing.T) {
	signer := newPresignTestSigner()
	signed := signer.Generate(presignTestPath)

	r := httptest.NewRequest(http.MethodGet, signed, nil)
	captured, rec := applyMiddleware(t, RequireValidPresign(signer), r)

	require.NotNil(t, captured, "valid presign must call next handler")
	require.Equal(t, http.StatusOK, rec.Code)
}

// Scenario 2: valid HEAD presign passes (GET and HEAD share a signature).
func TestRequireValidPresign_ValidHEADPasses(t *testing.T) {
	signer := newPresignTestSigner()
	signed := signer.Generate(presignTestPath) // Generate signs for GET.

	r := httptest.NewRequest(http.MethodHead, signed, nil)
	captured, rec := applyMiddleware(t, RequireValidPresign(signer), r)

	require.NotNil(t, captured, "HEAD must pass with a GET-signed URL (normalized)")
	require.Equal(t, http.StatusOK, rec.Code)
}

// ─── RequireValidPresign: method enforcement ────────────────────────────────

// Scenario 3: POST (even with a valid GET signature) is rejected with 405 and
// an Allow header. The method check runs before signature validation, so a
// non-GET/HEAD verb can never reach the signer.
func TestRequireValidPresign_POSTRejectedWith405(t *testing.T) {
	signer := newPresignTestSigner()
	signed := signer.Generate(presignTestPath)

	r := httptest.NewRequest(http.MethodPost, signed, nil)
	captured, rec := applyMiddleware(t, RequireValidPresign(signer), r)

	require.Nil(t, captured, "POST must not reach the next handler")
	require.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	require.Equal(t, "GET, HEAD", rec.Header().Get("Allow"))
}

// DELETE is likewise rejected.
func TestRequireValidPresign_DELETERejectedWith405(t *testing.T) {
	signer := newPresignTestSigner()
	signed := signer.Generate(presignTestPath)

	r := httptest.NewRequest(http.MethodDelete, signed, nil)
	_, rec := applyMiddleware(t, RequireValidPresign(signer), r)

	require.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	require.Equal(t, "GET, HEAD", rec.Header().Get("Allow"))
}

// ─── RequireValidPresign: signature failures (403) ──────────────────────────

// Scenario 4: missing exp => 403.
func TestRequireValidPresign_MissingExpReturns403(t *testing.T) {
	signer := newPresignTestSigner()

	// A request carrying only sig (no exp) must be rejected.
	r := httptest.NewRequest(http.MethodGet, presignTestPath+"?sig=deadbeef", nil)
	_, rec := applyMiddleware(t, RequireValidPresign(signer), r)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
}

// Scenario 5: missing sig => 403.
func TestRequireValidPresign_MissingSigReturns403(t *testing.T) {
	signer := newPresignTestSigner()
	exp := time.Now().Unix() + 600

	r := httptest.NewRequest(http.MethodGet, fmt.Sprintf("%s?exp=%d", presignTestPath, exp), nil)
	_, rec := applyMiddleware(t, RequireValidPresign(signer), r)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
}

// Scenario 6: tampered path => 403. The signature is valid for path A but the
// request targets path B, so the recomputed MAC will not match.
func TestRequireValidPresign_TamperedPathReturns403(t *testing.T) {
	signer := newPresignTestSigner()
	exp := time.Now().Unix() + 600
	sig := signer.Sign(http.MethodGet, presignTestPath, exp)

	tamperedPath := "/api/v1/media/xyz-999/poster"
	r := httptest.NewRequest(http.MethodGet, buildSignedURL(http.MethodGet, tamperedPath, exp, sig), nil)
	_, rec := applyMiddleware(t, RequireValidPresign(signer), r)

	require.Equal(t, http.StatusForbidden, rec.Code)
	require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
}

// Scenario 7: expired URL => 403.
func TestRequireValidPresign_ExpiredURLReturns403(t *testing.T) {
	signer := newPresignTestSigner()
	expiredExp := time.Now().Unix() - 100
	sig := signer.Sign(http.MethodGet, presignTestPath, expiredExp)

	r := httptest.NewRequest(http.MethodGet, buildSignedURL(http.MethodGet, presignTestPath, expiredExp, sig), nil)
	_, rec := applyMiddleware(t, RequireValidPresign(signer), r)

	require.Equal(t, http.StatusForbidden, rec.Code)
}

// Scenario 8: far-future exp => 403. A signature whose exp exceeds the
// configured expiry window (expirySeconds + clockSkew) must be rejected even
// if the HMAC itself is correct. This prevents accepting arbitrarily
// long-lived tokens.
func TestRequireValidPresign_FarFutureExpReturns403(t *testing.T) {
	signer := newPresignTestSigner()          // expirySeconds=3600, clockSkew=30
	farFutureExp := time.Now().Unix() + 86400 // 1 day, far beyond 3630s window
	sig := signer.Sign(http.MethodGet, presignTestPath, farFutureExp)

	r := httptest.NewRequest(http.MethodGet, buildSignedURL(http.MethodGet, presignTestPath, farFutureExp, sig), nil)
	_, rec := applyMiddleware(t, RequireValidPresign(signer), r)

	require.Equal(t, http.StatusForbidden, rec.Code, "far-future exp must be rejected")
}

// Tampered signature (wrong hex / not a valid MAC) => 403.
func TestRequireValidPresign_TamperedSignatureReturns403(t *testing.T) {
	signer := newPresignTestSigner()
	exp := time.Now().Unix() + 600

	r := httptest.NewRequest(http.MethodGet, buildSignedURL(http.MethodGet, presignTestPath, exp, "deadbeef"), nil)
	_, rec := applyMiddleware(t, RequireValidPresign(signer), r)

	require.Equal(t, http.StatusForbidden, rec.Code)
}

// Wrong secret => 403 (defense against a signer/key mismatch).
func TestRequireValidPresign_WrongSecretReturns403(t *testing.T) {
	signer := newPresignTestSigner()
	// A URL signed with a different secret must not validate.
	otherSigner := presign.NewSigner("a-completely-different-secret", 3600)
	signed := otherSigner.Generate(presignTestPath)

	r := httptest.NewRequest(http.MethodGet, signed, nil)
	_, rec := applyMiddleware(t, RequireValidPresign(signer), r)

	require.Equal(t, http.StatusForbidden, rec.Code)
}

// Nil signer => 500 (server misconfiguration, never a client-auth issue).
func TestRequireValidPresign_NilSignerReturns500(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, presignTestPath+"?exp=1&sig=x", nil)
	_, rec := applyMiddleware(t, RequireValidPresign(nil), r)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
}

// ─── Context marking ─────────────────────────────────────────────────────────

// Scenario 9: a valid presign marks the request context with
// IsPresignedAccess == true and records the validated path.
func TestRequireValidPresign_ValidMarksContextPresigned(t *testing.T) {
	signer := newPresignTestSigner()
	signed := signer.Generate(presignTestPath)

	r := httptest.NewRequest(http.MethodGet, signed, nil)
	captured, rec := applyMiddleware(t, RequireValidPresign(signer), r)

	require.NotNil(t, captured)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, IsPresignedAccess(captured), "valid presign must mark context")
	assert.Equal(t, presignTestPath, GetPresignedPath(captured))
	// Success responses advertise a long immutable cache window.
	assert.Equal(t, "public, max-age=3600, immutable", rec.Header().Get("Cache-Control"))
}

// Scenario 10: an invalid presign must NOT mark the context as presigned.
// Because the middleware short-circuits with 403, the next handler (which
// would observe the context) is never reached — so IsPresignedAccess is
// effectively always false downstream of a failure. We assert the property
// directly on a request that bypassed the middleware's success branch.
func TestRequireValidPresign_InvalidDoesNotMarkContext(t *testing.T) {
	signer := newPresignTestSigner()
	exp := time.Now().Unix() + 600

	r := httptest.NewRequest(http.MethodGet, buildSignedURL(http.MethodGet, presignTestPath, exp, "deadbeef"), nil)
	captured, rec := applyMiddleware(t, RequireValidPresign(signer), r)

	require.Nil(t, captured, "invalid presign must not reach next handler")
	require.Equal(t, http.StatusForbidden, rec.Code)
	// captured is nil; verify the marker reads false on a plain (un-marked)
	// request to lock the IsPresignedAccess default.
	assert.False(t, IsPresignedAccess(httptest.NewRequest(http.MethodGet, presignTestPath, nil)))
}

// ─── IsPresignedAccess / GetPresignedPath: nil safety ───────────────────────

func TestPresignedHelpers_NilRequestIsSafe(t *testing.T) {
	assert.False(t, IsPresignedAccess(nil))
	assert.Equal(t, "", GetPresignedPath(nil))
}
