package handler

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/fyom/fyom/internal/model"
	"github.com/fyom/fyom/internal/repository"
	"github.com/fyom/fyom/pkg/response"
)

// AdminLibraryHandler handles library CRUD for admins.
type AdminLibraryHandler struct {
	repo         *repository.LibraryRepository
	providerRepo *repository.ProviderRepository
	libPermRepo  *repository.LibraryPermissionRepository
}

// NewAdminLibraryHandler creates a new AdminLibraryHandler.
func NewAdminLibraryHandler(repo *repository.LibraryRepository, providerRepo *repository.ProviderRepository, libPermRepo *repository.LibraryPermissionRepository) *AdminLibraryHandler {
	return &AdminLibraryHandler{repo: repo, providerRepo: providerRepo, libPermRepo: libPermRepo}
}

// libraryWithCounts extends Library with item count fields for the API response.
type libraryWithCounts struct {
	model.Library
	ItemCount    int `json:"item_count"`
	MovieCount   int `json:"movie_count"`
	ShowCount    int `json:"show_count"`
	EpisodeCount int `json:"episode_count"`
	MissingCount int `json:"missing_count"`
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

	// Grant all existing users access to the new library.
	_ = h.libPermRepo.GrantNewLibrary(r.Context(), lib.ID)

	movies, shows, episodes, _ := h.repo.ItemCountsByType(r.Context(), lib.ID)
	response.Success(w, libraryWithCounts{
		Library: *lib, ItemCount: movies + shows + episodes,
		MovieCount: movies, ShowCount: shows, EpisodeCount: episodes,
		MissingCount: 0,
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
		missing, _ := h.repo.MissingCount(r.Context(), lib.ID)
		result[i] = libraryWithCounts{
			Library: lib, ItemCount: movies + shows + episodes,
			MovieCount: movies, ShowCount: shows, EpisodeCount: episodes,
			MissingCount: missing,
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
	missing, _ := h.repo.MissingCount(r.Context(), id)
	response.Success(w, libraryWithCounts{
		Library: *lib, ItemCount: movies + shows + episodes,
		MovieCount: movies, ShowCount: shows, EpisodeCount: episodes,
		MissingCount: missing,
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

// Delete removes a library (empty libraries only).
func (h *AdminLibraryHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.repo.Delete(r.Context(), id); err != nil {
		response.Error(w, 409, err.Error())
		return
	}
	response.NoContent(w)
}

// DeleteLibraryWithItems deletes a library and all its items (cascade) or
// orphans items to the default library.
func (h *AdminLibraryHandler) DeleteLibraryWithItems(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = "cascade"
	}

	if mode == "cascade" {
		if err := h.repo.DeleteWithItems(r.Context(), id); err != nil {
			response.Error(w, 500, "internal server error")
			return
		}
	} else if mode == "orphan" {
		// Move items to an empty placeholder — for now just error since
		// there's no "default" library concept anymore.
		// Admins should delete items first, then delete the empty library.
		response.Error(w, 400, "orphan mode: delete items first, then delete the empty library")
		return
	} else {
		response.Error(w, 400, "invalid mode: use 'cascade'")
		return
	}

	response.NoContent(w)
}

// Refresh triggers a re-import of the library using its stored configuration.
func (h *AdminLibraryHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	lib, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		response.Error(w, 500, "internal server error")
		return
	}
	if lib == nil {
		response.Error(w, 404, "library not found")
		return
	}

	// Return the library's stored import configuration.
	// The frontend triggers import via POST /api/v1/library/import.
	response.Success(w, map[string]interface{}{
		"id":              lib.ID,
		"source_path":     lib.SourcePath,
		"provider_id":     lib.ProviderID,
		"metadata_source": lib.MetadataSource,
		"message":         "Trigger import using POST /api/v1/library/import with the above configuration",
	})
}

// CheckMissing checks for items whose files no longer exist on disk.
func (h *AdminLibraryHandler) CheckMissing(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	lib, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		response.Error(w, 500, "internal server error")
		return
	}
	if lib == nil {
		response.Error(w, 404, "library not found")
		return
	}

	paths, err := h.repo.GetLocalItemPaths(r.Context(), id)
	if err != nil {
		response.Error(w, 500, "internal server error")
		return
	}

	var missingIDs []string
	checked := 0
	for _, p := range paths {
		checked++
		if _, err := os.Stat(p.FilePath); os.IsNotExist(err) {
			missingIDs = append(missingIDs, p.ID)
		}
	}

	if len(missingIDs) > 0 {
		if err := h.repo.MarkMissing(r.Context(), missingIDs); err != nil {
			response.Error(w, 500, "internal server error")
			return
		}
	}

	if err := h.repo.MarkAvailableByLibrary(r.Context(), id, missingIDs); err != nil {
		response.Error(w, 500, "internal server error")
		return
	}

	response.Success(w, map[string]interface{}{
		"checked": checked,
		"missing": len(missingIDs),
	})
}
