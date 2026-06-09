package handler

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fyom/fyom/internal/model"
	"github.com/fyom/fyom/internal/repository"
	"github.com/fyom/fyom/internal/service"
	"github.com/fyom/fyom/pkg/errors"
	"github.com/fyom/fyom/pkg/response"
)

// MediaHandler handles media-related HTTP endpoints.
type MediaHandler struct {
	repo        *repository.MediaRepository
	jobRepo     *repository.ImportJobRepository
	importer    *service.Importer
	allowedRoots []string
}

// NewMediaHandler creates a new MediaHandler.
func NewMediaHandler(db *repository.DB, mediaRepo *repository.MediaRepository, jobRepo *repository.ImportJobRepository) *MediaHandler {
	return &MediaHandler{
		repo:     mediaRepo,
		jobRepo:  jobRepo,
		importer: service.NewImporter(db, mediaRepo, jobRepo),
	}
}

// SetAllowedRoots configures allowed root directories for file serving.
func (h *MediaHandler) SetAllowedRoots(roots []string) {
	h.allowedRoots = roots
	h.importer.SetAllowedRoots(roots)
}

// List returns a paginated list of media items.
func (h *MediaHandler) List(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	mediaType := r.URL.Query().Get("type")

	items, total, err := h.repo.ListPaged(r.Context(), mediaType, page, pageSize)
	if err != nil {
		response.Error(w, 500, "internal server error")
		return
	}

	if items == nil {
		items = []model.MediaItem{}
	}

	response.Success(w, map[string]interface{}{
		"items":       items,
		"total":       total,
		"page":        page,
		"page_size":   pageSize,
		"total_pages": (total + pageSize - 1) / pageSize,
	})
}

// Get returns a single media item.
func (h *MediaHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		response.Error(w, 400, "missing id")
		return
	}

	item, err := h.repo.Get(r.Context(), id)
	if err != nil {
		response.Error(w, 500, "internal server error")
		return
	}
	if item == nil {
		response.Error(w, 404, "resource not found")
		return
	}

	response.Success(w, item)
}

// Delete removes a media item from the catalog.
func (h *MediaHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.repo.Delete(r.Context(), id); err != nil {
		response.Error(w, 500, "internal server error")
		return
	}

	response.NoContent(w)
}

// ImportRequest triggers an async NFO-based import.
type ImportRequest struct {
	SourcePath string `json:"source_path"`
}

// ImportResponse returns the created job ID.
type ImportResponse struct {
	JobID  string `json:"job_id"`
	Status string `json:"status"`
}

// Import triggers an asynchronous media import from the given path.
func (h *MediaHandler) Import(w http.ResponseWriter, r *http.Request) {
	var req ImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SourcePath == "" {
		response.Error(w, 400, "validation error")
		return
	}

	job, err := h.importer.ImportRequest(r.Context(), req.SourcePath)
	if err != nil {
		if appErr, ok := errors.IsAppError(err); ok {
			response.Error(w, appErr.Code, appErr.Message)
			return
		}
		response.Error(w, 500, "internal server error")
		return
	}

	response.Success(w, ImportResponse{
		JobID:  job.ID,
		Status: job.Status,
	})
}

// GetJob returns the status and progress of an import job.
func (h *MediaHandler) GetJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job, err := h.jobRepo.Get(r.Context(), id)
	if err != nil {
		response.Error(w, 500, "internal server error")
		return
	}
	if job == nil {
		response.Error(w, 404, "resource not found")
		return
	}

	response.Success(w, job)
}

// ListEpisodes returns all episodes for a given show.
func (h *MediaHandler) ListEpisodes(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	items, err := h.repo.GetEpisodesByShowID(r.Context(), id)
	if err != nil {
		response.Error(w, 500, "internal server error")
		return
	}
	if items == nil {
		items = []model.MediaItem{}
	}
	response.Success(w, items)
}

// ServeBackdrop serves a backdrop image.
func (h *MediaHandler) ServeBackdrop(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	item, err := h.repo.Get(r.Context(), id)
	if err != nil {
		response.Error(w, 500, "internal server error")
		return
	}
	if item == nil || item.BackdropPath == "" {
		response.Error(w, 404, "resource not found")
		return
	}
	info, err := os.Stat(item.BackdropPath)
	if err != nil {
		if os.IsNotExist(err) {
			jsonError(w, 404, "backdrop file not found on disk")
			return
		}
		jsonError(w, 500, "internal server error")
		return
	}

	f, err := os.Open(item.BackdropPath)
	if err != nil {
		jsonError(w, 500, "cannot open backdrop file")
		return
	}
	defer func() { _ = f.Close() }()

	name := strings.TrimSuffix(filepath.Base(item.BackdropPath), filepath.Ext(item.BackdropPath))
	modTime := info.ModTime()
	http.ServeContent(w, r, name, modTime, f)
}

// ServeContent streams a media file with full HTTP Range Request support.
func (h *MediaHandler) ServeContent(w http.ResponseWriter, r *http.Request, item *model.MediaItem) {
	if !h.isPathAllowed(item.FilePath) {
		jsonError(w, 403, "access denied")
		return
	}

	info, err := os.Stat(item.FilePath)
	if err != nil {
		if os.IsNotExist(err) {
			jsonError(w, 404, "media file not found on disk")
			return
		}
		jsonError(w, 500, "internal server error")
		return
	}

	f, err := os.Open(item.FilePath)
	if err != nil {
		jsonError(w, 500, "cannot open media file")
		return
	}
	defer func() { _ = f.Close() }()

	name := strings.TrimSuffix(filepath.Base(item.FilePath), filepath.Ext(item.FilePath))
	modTime := info.ModTime()
	if t, err := time.Parse(time.RFC3339, item.UpdatedAt); err == nil {
		modTime = t
	}

	http.ServeContent(w, r, name, modTime, f)
}

// Stream serves a media file with Range request support.
func (h *MediaHandler) Stream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	item, err := h.repo.Get(r.Context(), id)
	if err != nil {
		response.Error(w, 500, "internal server error")
		return
	}
	if item == nil {
		response.Error(w, 404, "resource not found")
		return
	}

	h.ServeContent(w, r, item)
}

// Poster serves a poster/thumbnail image.
func (h *MediaHandler) Poster(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	item, err := h.repo.Get(r.Context(), id)
	if err != nil {
		response.Error(w, 500, "internal server error")
		return
	}
	if item == nil || item.PosterPath == "" {
		response.Error(w, 404, "resource not found")
		return
	}

	h.ServeContent(w, r, &model.MediaItem{
		FilePath:  item.PosterPath,
		UpdatedAt: item.UpdatedAt,
	})
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func (h *MediaHandler) isPathAllowed(path string) bool {
	if len(h.allowedRoots) == 0 {
		return true
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	for _, root := range h.allowedRoots {
		absRoot, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		if absPath == absRoot || strings.HasPrefix(absPath, absRoot+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// jsonError writes a JSON error response directly to http.ResponseWriter.
func jsonError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	resp := struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}{
		Code:    code,
		Message: message,
	}
	_ = json.NewEncoder(w).Encode(resp)
}
