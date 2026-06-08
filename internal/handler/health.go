package handler

import (
	"net/http"

	"github.com/fyom/fyom/internal/model"
	"github.com/fyom/fyom/pkg/response"
)

// HealthHandler handles health check and version endpoints.
type HealthHandler struct {
	version   string
	gitCommit string
	buildTime string
	goVersion string
}

// NewHealthHandler creates a new HealthHandler.
func NewHealthHandler(version, gitCommit, buildTime, goVersion string) *HealthHandler {
	return &HealthHandler{
		version:   version,
		gitCommit: gitCommit,
		buildTime: buildTime,
		goVersion: goVersion,
	}
}

// Health returns service health status.
func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	response.Success(w, map[string]string{"status": "healthy"})
}

// Version returns build version info.
func (h *HealthHandler) Version(w http.ResponseWriter, r *http.Request) {
	response.Success(w, model.VersionInfo{
		Version:   h.version,
		GitCommit: h.gitCommit,
		BuildTime: h.buildTime,
		GoVersion: h.goVersion,
	})
}
