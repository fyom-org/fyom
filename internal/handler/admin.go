package handler

import (
	"net/http"
	"strconv"

	"github.com/fyom/fyom/internal/model"
	"github.com/fyom/fyom/internal/repository"
	"github.com/fyom/fyom/pkg/response"
)

// AdminHandler handles admin-only system endpoints.
type AdminHandler struct {
	repo *repository.AdminRepository
}

// NewAdminHandler creates a new AdminHandler.
func NewAdminHandler(repo *repository.AdminRepository) *AdminHandler {
	return &AdminHandler{repo: repo}
}

// GetStats returns aggregate system statistics.
func (h *AdminHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.repo.GetStats(r.Context())
	if err != nil {
		response.Error(w, 500, "internal server error")
		return
	}
	response.Success(w, stats)
}

// ListImportJobs returns a paginated list of import jobs.
func (h *AdminHandler) ListImportJobs(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}

	jobs, total, err := h.repo.ListJobs(r.Context(), page, limit)
	if err != nil {
		response.Error(w, 500, "internal server error")
		return
	}
	if jobs == nil {
		jobs = []model.ImportJob{}
	}

	response.Success(w, map[string]interface{}{
		"items": jobs,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}
