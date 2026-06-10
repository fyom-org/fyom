package handler

import (
	"encoding/json"
	"net/http"

	"github.com/fyom/fyom/internal/model"
	"github.com/fyom/fyom/internal/repository"
	"github.com/fyom/fyom/pkg/response"
)

// AdminLibraryHandler handles library CRUD for admins.
type AdminLibraryHandler struct {
	repo         *repository.LibraryRepository
	providerRepo *repository.ProviderRepository
}

// NewAdminLibraryHandler creates a new AdminLibraryHandler.
func NewAdminLibraryHandler(repo *repository.LibraryRepository, providerRepo *repository.ProviderRepository) *AdminLibraryHandler {
	return &AdminLibraryHandler{repo: repo, providerRepo: providerRepo}
}

// libraryWithCounts extends Library with item count fields for the API response.
type libraryWithCounts struct {
	model.Library
	ItemCount    int `json:"item_count"`
	MovieCount   int `json:"movie_count"`
	ShowCount    int `json:"show_count"`
	EpisodeCount int `json:"episode_count"`
}

// Create creates a new library.
func (h *AdminLibraryHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		Type           string `json:"type"`
		ProviderID     string `json:"provider_id"`
		SourcePath     string `json:"source_path"`
		MetadataSource string `json:"metadata_source"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, 400, "invalid JSON")
		return
	}

	if req.Name == "" {
		response.Error(w, 400, "name is required")
		return
	}
	lib := &model.Library{
		ID:             req.ID,
		Name:           req.Name,
		Type:           req.Type,
		ProviderID:     req.ProviderID,
		SourcePath:     req.SourcePath,
		MetadataSource: req.MetadataSource,
	}

	if err := h.repo.Create(r.Context(), lib); err != nil {
		response.Error(w, 400, err.Error())
		return
	}

	movies, shows, episodes, _ := h.repo.ItemCountsByType(r.Context(), lib.ID)
	response.Success(w, libraryWithCounts{
		Library: *lib, ItemCount: movies + shows + episodes,
		MovieCount: movies, ShowCount: shows, EpisodeCount: episodes,
	})
}

// List returns all libraries with item counts.
func (h *AdminLibraryHandler) List(w http.ResponseWriter, r *http.Request) {
	libs, err := h.repo.List(r.Context())
	if err != nil {
		response.Error(w, 500, "internal server error")
		return
	}

	result := make([]libraryWithCounts, len(libs))
	for i, lib := range libs {
		movies, shows, episodes, _ := h.repo.ItemCountsByType(r.Context(), lib.ID)
		result[i] = libraryWithCounts{
			Library: lib, ItemCount: movies + shows + episodes,
			MovieCount: movies, ShowCount: shows, EpisodeCount: episodes,
		}
	}
	response.Success(w, result)
}

// Get returns a single library with item counts.
func (h *AdminLibraryHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	lib, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		response.Error(w, 500, "internal server error")
		return
	}
	if lib == nil {
		response.Error(w, 404, "not found")
		return
	}
	movies, shows, episodes, _ := h.repo.ItemCountsByType(r.Context(), id)
	response.Success(w, libraryWithCounts{
		Library: *lib, ItemCount: movies + shows + episodes,
		MovieCount: movies, ShowCount: shows, EpisodeCount: episodes,
	})
}

// Update modifies a library.
func (h *AdminLibraryHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	lib, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		response.Error(w, 500, "internal server error")
		return
	}
	if lib == nil {
		response.Error(w, 404, "not found")
		return
	}

	var req struct {
		Name           *string `json:"name"`
		Type           *string `json:"type"`
		ProviderID     *string `json:"provider_id"`
		SourcePath     *string `json:"source_path"`
		MetadataSource *string `json:"metadata_source"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, 400, "invalid JSON")
		return
	}

	if req.Name != nil {
		lib.Name = *req.Name
	}
	if req.Type != nil {
		lib.Type = *req.Type
	}
	if req.ProviderID != nil {
		lib.ProviderID = *req.ProviderID
	}
	if req.SourcePath != nil {
		lib.SourcePath = *req.SourcePath
	}
	if req.MetadataSource != nil {
		lib.MetadataSource = *req.MetadataSource
	}

	if err := h.repo.Update(r.Context(), lib); err != nil {
		response.Error(w, 400, err.Error())
		return
	}
	movies, shows, episodes, _ := h.repo.ItemCountsByType(r.Context(), id)
	response.Success(w, libraryWithCounts{
		Library: *lib, ItemCount: movies + shows + episodes,
		MovieCount: movies, ShowCount: shows, EpisodeCount: episodes,
	})
}

// Delete removes a library.
func (h *AdminLibraryHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "default" {
		response.Error(w, 403, "cannot delete the default library")
		return
	}
	if err := h.repo.Delete(r.Context(), id); err != nil {
		response.Error(w, 409, err.Error())
		return
	}
	response.NoContent(w)
}
