package handler

import (
	"encoding/json"
	"net/http"
	"runtime"
	"time"

	"github.com/fyom/fyom/internal/version"
)

// Pinger is the minimal interface needed by Readyz to check DB health.
type Pinger interface {
	Ping() error
}

// DiagHandler serves observability endpoints (/healthz, /readyz, /version).
type DiagHandler struct {
	db        Pinger
	assetHash string
	startedAt time.Time
}

// NewDiagHandler creates a new DiagHandler.
func NewDiagHandler(db Pinger, assetHash string) *DiagHandler {
	return &DiagHandler{db: db, assetHash: assetHash, startedAt: time.Now()}
}

// Healthz returns 200 OK if the process is alive.
// Does NOT check DB — a slow DB should not fail liveness.
func (h *DiagHandler) Healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// Readyz returns 200 if the process is up AND its dependencies (DB) are reachable.
// Returns 503 if the DB is unreachable.
func (h *DiagHandler) Readyz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")

	if err := h.db.Ping(); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "not_ready",
			"reason": "database unreachable",
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]any{
		"status":         "ready",
		"uptime_seconds": int(time.Since(h.startedAt).Seconds()),
	})
}

// Version returns build and runtime metadata.
func (h *DiagHandler) Version(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(map[string]string{
		"version":       version.Version,
		"commit":        version.Commit,
		"build_time":    version.BuildTime,
		"go_version":    runtime.Version(),
		"frontend_hash": h.assetHash,
	})
}
