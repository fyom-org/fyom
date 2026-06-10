package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/fyom/fyom/internal/model"
	"github.com/fyom/fyom/internal/provider"
	"github.com/fyom/fyom/internal/repository"
	"github.com/fyom/fyom/internal/service"
	"github.com/fyom/fyom/pkg/errors"
	"github.com/fyom/fyom/pkg/response"
)

// MediaItemResponse is the JSON DTO returned by library API endpoints.
// Filesystem paths are never exposed; resource URLs are generated dynamically
// via the Provider registry.
type MediaItemResponse struct {
	ID             string  `json:"id"`
	Type           string  `json:"type"`
	Title          string  `json:"title"`
	SortTitle      string  `json:"sort_title,omitempty"`
	Year           *int    `json:"year,omitempty"`
	Overview       string  `json:"overview,omitempty"`
	Rating         *float64 `json:"rating,omitempty"`
	Duration       *int    `json:"duration,omitempty"`
	PosterURL      *string `json:"poster_url,omitempty"`
	BackdropURL    *string `json:"backdrop_url,omitempty"`
	StreamURL      *string `json:"stream_url,omitempty"`
	Season         *int    `json:"season,omitempty"`
	Episode        *int    `json:"episode,omitempty"`
	ParentID       string  `json:"parent_id,omitempty"`
	MetadataSource string  `json:"metadata_source,omitempty"`
}

// MediaHandler handles media-related HTTP endpoints.
type MediaHandler struct {
	registry     *provider.Registry
	repo         *repository.MediaRepository
	jobRepo      *repository.ImportJobRepository
	providerRepo *repository.ProviderRepository
	db           *repository.DB
	logger       *slog.Logger
}

// NewMediaHandler creates a new MediaHandler.
func NewMediaHandler(registry *provider.Registry, db *repository.DB, mediaRepo *repository.MediaRepository, jobRepo *repository.ImportJobRepository, providerRepo *repository.ProviderRepository, logger *slog.Logger) *MediaHandler {
	return &MediaHandler{
		registry:     registry,
		repo:         mediaRepo,
		jobRepo:      jobRepo,
		providerRepo: providerRepo,
		db:           db,
		logger:       logger,
	}
}

// mediaItemToResponse copies all non-URL fields from model.MediaItem to MediaItemResponse.
func mediaItemToResponse(item *model.MediaItem) MediaItemResponse {
	resp := MediaItemResponse{
		ID:             item.ID,
		Type:           item.Type,
		Title:          item.Title,
		SortTitle:      item.SortTitle,
		Overview:       item.Overview,
		MetadataSource: item.MetadataSource,
		ParentID:       item.ParentID,
	}

	if item.Year != 0 {
		resp.Year = &item.Year
	}
	if item.Rating != 0 {
		resp.Rating = &item.Rating
	}
	if item.Duration != 0 {
		resp.Duration = &item.Duration
	}
	if item.Season != 0 {
		resp.Season = &item.Season
	}
	if item.Episode != 0 {
		resp.Episode = &item.Episode
	}

	return resp
}

// attachPresignedURLs resolves resource URLs for a media item via its provider.
//
// If the item's provider is not registered (stale provider_id, removed config),
// the response is returned without URLs and a warning is logged. This is a
// graceful degradation — the client will see nil URL fields rather than a 500.
func attachPresignedURLs(ctx context.Context, item *model.MediaItem, registry *provider.Registry, logger *slog.Logger) MediaItemResponse {
	resp := mediaItemToResponse(item)

	p, ok := registry.Get(item.ProviderID)
	if !ok {
		logger.Warn("provider not found for media item",
			"item_id",     item.ID,
			"provider_id", item.ProviderID,
		)
		return resp
	}

	if u, err := p.PosterURL(ctx, item); err == nil && u != "" {
		resp.PosterURL = &u
	}
	if u, err := p.BackdropURL(ctx, item); err == nil && u != "" {
		resp.BackdropURL = &u
	}
	if u, err := p.StreamURL(ctx, item); err == nil && u != "" {
		resp.StreamURL = &u
	}
	return resp
}

// attachPresignedURLsList maps a slice of MediaItem through attachPresignedURLs.
func attachPresignedURLsList(ctx context.Context, items []model.MediaItem, registry *provider.Registry, logger *slog.Logger) []MediaItemResponse {
	result := make([]MediaItemResponse, len(items))
	for i := range items {
		result[i] = attachPresignedURLs(ctx, &items[i], registry, logger)
	}
	return result
}

// TODO: re-add path restriction when multi-user mode is implemented

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
	// Default to movies and shows only — episodes are fetched via /library/:id/episodes.
	if mediaType == "" {
		mediaType = "movie,show"
	}

	// Search query.
	q := r.URL.Query().Get("q")

	// Sort — validate against allowed set, degrade silently.
	sort := r.URL.Query().Get("sort")
	allowedSorts := map[string]bool{
		"title_asc": true, "title_desc": true,
		"year_asc": true, "year_desc": true,
		"rating_desc": true, "created_desc": true,
	}
	if !allowedSorts[sort] {
		sort = "title_asc"
	}

	items, total, err := h.repo.ListPaged(r.Context(), page, pageSize, mediaType, q, sort)
	if err != nil {
		response.Error(w, 500, "internal server error")
		return
	}

	if items == nil {
		items = []model.MediaItem{}
	}

	result := attachPresignedURLsList(r.Context(), items, h.registry, h.logger)

	response.Success(w, map[string]interface{}{
		"items":       result,
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

	result := attachPresignedURLs(r.Context(), item, h.registry, h.logger)
	response.Success(w, result)
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

	result := attachPresignedURLsList(r.Context(), items, h.registry, h.logger)
	response.Success(w, result)
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
	ProviderID string `json:"provider_id"`
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

	if req.ProviderID == "" {
		req.ProviderID = "local"
	}

	var fs service.ImportFS
	var providerID string

	if req.ProviderID == "local" {
		fs = service.NewLocalImportFS()
		providerID = "local"
	} else {
		p, ok := h.registry.Get(req.ProviderID)
		if !ok {
			response.Error(w, 400, "provider not found: "+req.ProviderID)
			return
		}
		if p.Type() != "s3" {
			response.Error(w, 400, "import from provider type '"+p.Type()+"' is not supported yet")
			return
		}
		// Get provider record from DB for S3 config
		rec, err := h.providerRepo.GetByID(r.Context(), req.ProviderID)
		if err != nil || rec == nil {
			response.Error(w, 500, "failed to load provider config")
			return
		}
		s3fs, err := service.NewS3ImportFS(r.Context(), rec, req.SourcePath)
		if err != nil {
			response.Error(w, 500, "failed to create S3 client: "+err.Error())
			return
		}
		fs = s3fs
		providerID = req.ProviderID
	}

	// Create a fresh importer per request — each import gets its own FS
	imp := service.NewImporter(fs, providerID, h.db, h.repo, h.jobRepo)
	job, err := imp.ImportRequest(r.Context(), req.SourcePath)
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

// NOTE: These serve handlers (ServeStream, ServePoster, ServeBackdrop above)
// are LocalProvider implementation details. When S3Provider is added, its URLs
// point directly to S3 — these handlers are never called for S3 items.
//
// TODO(phase5): When RemoteFyomProvider is added, the handler that resolves
// a media item's stream URL must check SupportsRedirect() and issue an
// HTTP 302 rather than embedding the URL in JSON. This applies to the
// GetByID and stream-initiation paths, not to these serve handlers.

// ServeContent streams a media file with full HTTP Range Request support.
func (h *MediaHandler) ServeContent(w http.ResponseWriter, r *http.Request, item *model.MediaItem) {
	// TODO: re-add path restriction when multi-user mode is implemented

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
