package handler

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockPinger implements Pinger for testing.
type mockPinger struct {
	err error
}

func (m *mockPinger) Ping() error { return m.err }

func TestHealthz(t *testing.T) {
	h := NewDiagHandler(&mockPinger{}, "abc123")
	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()
	h.Healthz(rec, req)

	assert.Equal(t, 200, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "ok", resp["status"])
}

func TestReadyz_DBHealthy(t *testing.T) {
	h := NewDiagHandler(&mockPinger{err: nil}, "abc123")
	req := httptest.NewRequest("GET", "/readyz", nil)
	rec := httptest.NewRecorder()
	h.Readyz(rec, req)

	assert.Equal(t, 200, rec.Code)
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "ready", resp["status"])
	assert.NotNil(t, resp["uptime_seconds"])
}

func TestReadyz_DBDown(t *testing.T) {
	h := NewDiagHandler(&mockPinger{err: errors.New("connection refused")}, "abc123")
	req := httptest.NewRequest("GET", "/readyz", nil)
	rec := httptest.NewRecorder()
	h.Readyz(rec, req)

	assert.Equal(t, 503, rec.Code)
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "not_ready", resp["status"])
	assert.Equal(t, "database unreachable", resp["reason"])
}

func TestVersion(t *testing.T) {
	h := NewDiagHandler(&mockPinger{}, "abc123def456")
	req := httptest.NewRequest("GET", "/version", nil)
	rec := httptest.NewRecorder()
	h.Version(rec, req)

	assert.Equal(t, 200, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp["version"])
	assert.NotEmpty(t, resp["commit"])
	assert.NotEmpty(t, resp["build_time"])
	assert.NotEmpty(t, resp["go_version"])
	assert.Equal(t, "abc123def456", resp["frontend_hash"])
}
