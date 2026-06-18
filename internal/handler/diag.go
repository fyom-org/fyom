package handler

import (
	"encoding/json"
	"net/http"
	"runtime"
	"time"

	"github.com/fyom/fyom/internal/version"
)

const (
	diagContentType  = "application/json; charset=utf-8"
	diagCacheControl = "no-store"
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
	return &DiagHandler{
		db:        db,
		assetHash: assetHash,
		startedAt: time.Now(),
	}
}

type diagHealthResponse struct {
	Status        string `json:"status"`
	UptimeSeconds int64  `json:"uptime_seconds"`
}

type diagReadyResponse struct {
	Status        string                    `json:"status"`
	Reason        string                    `json:"reason,omitempty"`
	UptimeSeconds int64                     `json:"uptime_seconds"`
	Checks        map[string]diagCheckState `json:"checks,omitempty"`
}

type diagCheckState struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type diagVersionResponse struct {
	Version       string `json:"version"`
	Commit        string `json:"commit"`
	BuildTime     string `json:"build_time"`
	GoVersion     string `json:"go_version"`
	FrontendHash  string `json:"frontend_hash"`
	UptimeSeconds int64  `json:"uptime_seconds"`
}

type diagErrorResponse struct {
	Status string `json:"status"`
	Error  string `json:"error"`
}

// Healthz returns 200 OK if the process is alive.
// It intentionally does not check DB health. A slow or unavailable dependency
// must not cause the liveness endpoint to fail.
func (h *DiagHandler) Healthz(w http.ResponseWriter, r *http.Request) {
	if !allowDiagReadMethod(w, r) {
		return
	}

	writeDiagJSON(w, r, http.StatusOK, diagHealthResponse{
		Status:        "ok",
		UptimeSeconds: h.uptimeSeconds(),
	})
}

// Readyz returns 200 if the process is up and required dependencies are reachable.
// It returns 503 if the database dependency is missing or unreachable.
func (h *DiagHandler) Readyz(w http.ResponseWriter, r *http.Request) {
	if !allowDiagReadMethod(w, r) {
		return
	}

	checks := map[string]diagCheckState{
		"database": {
			Status: "unknown",
		},
	}

	if h == nil || h.db == nil {
		checks["database"] = diagCheckState{
			Status: "not_configured",
			Error:  "database dependency is not configured",
		}

		writeDiagJSON(w, r, http.StatusServiceUnavailable, diagReadyResponse{
			Status:        "not_ready",
			Reason:        "database dependency not configured",
			UptimeSeconds: h.uptimeSeconds(),
			Checks:        checks,
		})
		return
	}

	if err := h.db.Ping(); err != nil {
		checks["database"] = diagCheckState{
			Status: "unreachable",
			Error:  "database unreachable",
		}

		writeDiagJSON(w, r, http.StatusServiceUnavailable, diagReadyResponse{
			Status:        "not_ready",
			Reason:        "database unreachable",
			UptimeSeconds: h.uptimeSeconds(),
			Checks:        checks,
		})
		return
	}

	checks["database"] = diagCheckState{
		Status: "ok",
	}

	writeDiagJSON(w, r, http.StatusOK, diagReadyResponse{
		Status:        "ready",
		UptimeSeconds: h.uptimeSeconds(),
		Checks:        checks,
	})
}

// Version returns build and runtime metadata.
func (h *DiagHandler) Version(w http.ResponseWriter, r *http.Request) {
	if !allowDiagReadMethod(w, r) {
		return
	}

	writeDiagJSON(w, r, http.StatusOK, diagVersionResponse{
		Version:       version.Version,
		Commit:        version.Commit,
		BuildTime:     version.BuildTime,
		GoVersion:     runtime.Version(),
		FrontendHash:  h.frontendHash(),
		UptimeSeconds: h.uptimeSeconds(),
	})
}

func allowDiagReadMethod(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return true
	}

	w.Header().Set("Allow", "GET, HEAD")

	writeDiagJSON(w, r, http.StatusMethodNotAllowed, diagErrorResponse{
		Status: "error",
		Error:  "method not allowed",
	})

	return false
}

func writeDiagJSON(w http.ResponseWriter, r *http.Request, statusCode int, payload any) {
	w.Header().Set("Content-Type", diagContentType)
	w.Header().Set("Cache-Control", diagCacheControl)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(statusCode)

	if r.Method == http.MethodHead {
		return
	}

	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)

	if err := encoder.Encode(payload); err != nil {
		// At this point headers have already been written. There is no safe way
		// to change the HTTP status code, so the handler intentionally drops the
		// encode error.
		return
	}
}

func (h *DiagHandler) uptimeSeconds() int64 {
	if h == nil || h.startedAt.IsZero() {
		return 0
	}

	uptime := int64(time.Since(h.startedAt).Seconds())
	if uptime < 0 {
		return 0
	}

	return uptime
}

func (h *DiagHandler) frontendHash() string {
	if h == nil {
		return ""
	}

	return h.assetHash
}
