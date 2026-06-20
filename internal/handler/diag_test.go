package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockPinger implements Pinger for testing.
type mockPinger struct {
	err error
}

func (m *mockPinger) Ping() error {
	return m.err
}

func TestHealthz(t *testing.T) {
	h := NewDiagHandler(&mockPinger{}, "abc123")
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	h.Healthz(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, diagContentType, rec.Header().Get("Content-Type"))
	assert.Equal(t, diagCacheControl, rec.Header().Get("Cache-Control"))
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))

	var resp diagHealthResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "ok", resp.Status)
	assert.GreaterOrEqual(t, resp.UptimeSeconds, int64(0))
}

func TestHealthz_Head(t *testing.T) {
	h := NewDiagHandler(&mockPinger{}, "abc123")
	req := httptest.NewRequest(http.MethodHead, "/healthz", nil)
	rec := httptest.NewRecorder()

	h.Healthz(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, diagContentType, rec.Header().Get("Content-Type"))
	assert.Equal(t, diagCacheControl, rec.Header().Get("Cache-Control"))
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	assert.Empty(t, rec.Body.String())
}

func TestHealthz_MethodNotAllowed(t *testing.T) {
	h := NewDiagHandler(&mockPinger{}, "abc123")
	req := httptest.NewRequest(http.MethodPost, "/healthz", nil)
	rec := httptest.NewRecorder()

	h.Healthz(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	assert.Equal(t, "GET, HEAD", rec.Header().Get("Allow"))
	assert.Equal(t, diagContentType, rec.Header().Get("Content-Type"))
	assert.Equal(t, diagCacheControl, rec.Header().Get("Cache-Control"))
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))

	var resp diagErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "error", resp.Status)
	assert.Equal(t, "method not allowed", resp.Error)
}

func TestReadyz_DBHealthy(t *testing.T) {
	h := NewDiagHandler(&mockPinger{}, "abc123")
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	h.Readyz(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, diagContentType, rec.Header().Get("Content-Type"))
	assert.Equal(t, diagCacheControl, rec.Header().Get("Cache-Control"))
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))

	var resp diagReadyResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "ready", resp.Status)
	assert.Empty(t, resp.Reason)
	assert.GreaterOrEqual(t, resp.UptimeSeconds, int64(0))

	require.Contains(t, resp.Checks, "database")
	assert.Equal(t, "ok", resp.Checks["database"].Status)
	assert.Empty(t, resp.Checks["database"].Error)
}

func TestReadyz_DBDown(t *testing.T) {
	h := NewDiagHandler(&mockPinger{err: errors.New("connection refused")}, "abc123")
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	h.Readyz(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Equal(t, diagContentType, rec.Header().Get("Content-Type"))
	assert.Equal(t, diagCacheControl, rec.Header().Get("Cache-Control"))
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))

	var resp diagReadyResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "not_ready", resp.Status)
	assert.Equal(t, "database unreachable", resp.Reason)
	assert.GreaterOrEqual(t, resp.UptimeSeconds, int64(0))

	require.Contains(t, resp.Checks, "database")
	assert.Equal(t, "unreachable", resp.Checks["database"].Status)
	assert.Equal(t, "database unreachable", resp.Checks["database"].Error)
}

func TestReadyz_DBNotConfigured(t *testing.T) {
	h := NewDiagHandler(nil, "abc123")
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	h.Readyz(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Equal(t, diagContentType, rec.Header().Get("Content-Type"))
	assert.Equal(t, diagCacheControl, rec.Header().Get("Cache-Control"))
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))

	var resp diagReadyResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "not_ready", resp.Status)
	assert.Equal(t, "database dependency not configured", resp.Reason)
	assert.GreaterOrEqual(t, resp.UptimeSeconds, int64(0))

	require.Contains(t, resp.Checks, "database")
	assert.Equal(t, "not_configured", resp.Checks["database"].Status)
	assert.Equal(t, "database dependency is not configured", resp.Checks["database"].Error)
}

func TestReadyz_NilHandler(t *testing.T) {
	var h *DiagHandler

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	h.Readyz(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Equal(t, diagContentType, rec.Header().Get("Content-Type"))
	assert.Equal(t, diagCacheControl, rec.Header().Get("Cache-Control"))
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))

	var resp diagReadyResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "not_ready", resp.Status)
	assert.Equal(t, "database dependency not configured", resp.Reason)
	assert.Equal(t, int64(0), resp.UptimeSeconds)

	require.Contains(t, resp.Checks, "database")
	assert.Equal(t, "not_configured", resp.Checks["database"].Status)
	assert.Equal(t, "database dependency is not configured", resp.Checks["database"].Error)
}

func TestReadyz_MethodNotAllowed(t *testing.T) {
	h := NewDiagHandler(&mockPinger{}, "abc123")
	req := httptest.NewRequest(http.MethodPost, "/readyz", nil)
	rec := httptest.NewRecorder()

	h.Readyz(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	assert.Equal(t, "GET, HEAD", rec.Header().Get("Allow"))

	var resp diagErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "error", resp.Status)
	assert.Equal(t, "method not allowed", resp.Error)
}

func TestVersion(t *testing.T) {
	h := NewDiagHandler(&mockPinger{}, "abc123def456")
	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	rec := httptest.NewRecorder()

	h.Version(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, diagContentType, rec.Header().Get("Content-Type"))
	assert.Equal(t, diagCacheControl, rec.Header().Get("Cache-Control"))
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))

	var resp diagVersionResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, runtime.Version(), resp.GoVersion)
	assert.Equal(t, "abc123def456", resp.FrontendHash)
	assert.GreaterOrEqual(t, resp.UptimeSeconds, int64(0))
}

func TestVersion_Head(t *testing.T) {
	h := NewDiagHandler(&mockPinger{}, "abc123def456")
	req := httptest.NewRequest(http.MethodHead, "/version", nil)
	rec := httptest.NewRecorder()

	h.Version(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, diagContentType, rec.Header().Get("Content-Type"))
	assert.Equal(t, diagCacheControl, rec.Header().Get("Cache-Control"))
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	assert.Empty(t, rec.Body.String())
}

func TestVersion_MethodNotAllowed(t *testing.T) {
	h := NewDiagHandler(&mockPinger{}, "abc123def456")
	req := httptest.NewRequest(http.MethodPost, "/version", nil)
	rec := httptest.NewRecorder()

	h.Version(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	assert.Equal(t, "GET, HEAD", rec.Header().Get("Allow"))

	var resp diagErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "error", resp.Status)
	assert.Equal(t, "method not allowed", resp.Error)
}
