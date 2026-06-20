package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fyom/fyom/internal/middleware"
	"github.com/fyom/fyom/internal/model"
	"github.com/fyom/fyom/internal/provider"
	"github.com/fyom/fyom/internal/repository"
	"github.com/fyom/fyom/internal/service"
	"github.com/fyom/fyom/pkg/errors"
	"github.com/fyom/fyom/pkg/response"
)

// ActorResponse represents a single actor/crew member in API responses.
type ActorResponse struct {
	Name      string `json:"name"`
	Role      string `json:"role"`
	Type      string `json:"type"`
	SortOrder int    `json:"sort_order"`
	Thumb     string `json:"thumb,omitempty"`
}

// MediaItemResponse is the JSON DTO returned by library API endpoints.
// Filesystem paths are never exposed; resource URLs are generated dynamically
// via the Provider registry.
type MediaItemResponse struct {
	ID                string            `json:"id"`
	Type              string            `json:"type"`
	Title             string            `json:"title"`
	SortTitle         string            `json:"sort_title,omitempty"`
	Year              *int              `json:"year,omitempty"`
	Overview          string            `json:"overview,omitempty"`
	Rating            *float64          `json:"rating,omitempty"`
	Duration          *int              `json:"duration,omitempty"`
	PosterURL         *string           `json:"poster_url,omitempty"`
	BackdropURL       *string           `json:"backdrop_url,omitempty"`
	StreamURL         *string           `json:"stream_url,omitempty"`
	Season            *int              `json:"season,omitempty"`
	Episode           *int              `json:"episode,omitempty"`
	ParentID          string            `json:"parent_id,omitempty"`
	LibraryID         string            `json:"library_id,omitempty"`
	MetadataSource    string            `json:"metadata_source,omitempty"`
	Status            string            `json:"status"`
	UserStatus        string            `json:"user_status,omitempty"`
	MPAA              string            `json:"mpaa,omitempty"`
	Genres            []string          `json:"genres,omitempty"`
	Studios           []string          `json:"studios,omitempty"`
	Actors            []ActorResponse   `json:"actors,omitempty"`
	GuestStars        []ActorResponse   `json:"guest_stars,omitempty"`
	UniqueIDs         map[string]string `json:"unique_ids,omitempty"`
	ShowID            string            `json:"show_id,omitempty"`
	Premiered         string            `json:"premiered,omitempty"`
	Outline           string            `json:"outline,omitempty"`
	Tagline           string            `json:"tagline,omitempty"`
	Countries         []string          `json:"countries,omitempty"`
	Directors         []string          `json:"directors,omitempty"`
	Credits           []string          `json:"credits,omitempty"`
	Tags              []string          `json:"tags,omitempty"`
	SetName           string            `json:"set_name,omitempty"`
	VideoCodec        string            `json:"video_codec,omitempty"`
	VideoWidth        int               `json:"video_width,omitempty"`
	VideoHeight       int               `json:"video_height,omitempty"`
	AudioCodec        string            `json:"audio_codec,omitempty"`
	AudioChannels     int               `json:"audio_channels,omitempty"`
	SubtitleLanguages []string          `json:"subtitle_languages,omitempty"`
	LogoURL           *string           `json:"logo_url,omitempty"`
	Aired             string            `json:"aired,omitempty"`
	Language          string            `json:"language,omitempty"`
	CountryCode       string            `json:"country_code,omitempty"`
	CustomRating      string            `json:"custom_rating,omitempty"`
	CollectionNumber  string            `json:"collection_number,omitempty"`
	EndDate           string            `json:"end_date,omitempty"`
	ReleaseDate       string            `json:"release_date,omitempty"`
	DisplayOrder      string            `json:"display_order,omitempty"`
	OriginalTitle     string            `json:"original_title,omitempty"`
	UserRating        *float64          `json:"user_rating,omitempty"`
	DateAdded         string            `json:"date_added,omitempty"`
	LastPlayed        string            `json:"last_played,omitempty"`
	Playcount         int               `json:"playcount,omitempty"`
	SetOverview       string            `json:"set_overview,omitempty"`
}

// MediaHandler handles media-related HTTP endpoints.
type MediaHandler struct {
	registry           *provider.Registry
	repo               *repository.MediaRepository
	jobRepo            *repository.ImportJobRepository
	providerRepo       *repository.ProviderRepository
	libRepo            *repository.LibraryRepository
	statusRepo         *repository.UserMediaStatusRepository
	db                 *repository.DB
	logger             *slog.Logger
	refreshCoordinator RefreshCoordinator
}

// RefreshCoordinator defines the interface for coordinating refresh jobs.
// Implemented by server.RefreshCoordinator to avoid import cycles.
type RefreshCoordinator interface {
	TryStart(libraryID string) bool
	Finish(libraryID string)
}

// NewMediaHandler creates a new MediaHandler.
func NewMediaHandler(registry *provider.Registry, db *repository.DB, mediaRepo *repository.MediaRepository, jobRepo *repository.ImportJobRepository, providerRepo *repository.ProviderRepository, libRepo *repository.LibraryRepository, statusRepo *repository.UserMediaStatusRepository, logger *slog.Logger, refreshCoordinator RefreshCoordinator) *MediaHandler {
	return &MediaHandler{
		registry:           registry,
		repo:               mediaRepo,
		jobRepo:            jobRepo,
		providerRepo:       providerRepo,
		libRepo:            libRepo,
		statusRepo:         statusRepo,
		db:                 db,
		logger:             logger,
		refreshCoordinator: refreshCoordinator,
	}
}

// mediaItemToResponse copies all non-URL fields from model.MediaItem to MediaItemResponse.
func mediaItemToResponse(item *model.MediaItem) MediaItemResponse {
	resp := MediaItemResponse{
		ID:               item.ID,
		Type:             item.Type,
		Title:            item.Title,
		SortTitle:        item.SortTitle,
		Overview:         item.Overview,
		MetadataSource:   item.MetadataSource,
		Status:           item.Status,
		MPAA:             item.MPAA,
		Premiered:        item.Premiered,
		Outline:          item.Outline,
		Tagline:          item.Tagline,
		SetName:          item.SetName,
		VideoCodec:       item.VideoCodec,
		VideoWidth:       item.VideoWidth,
		VideoHeight:      item.VideoHeight,
		AudioCodec:       item.AudioCodec,
		AudioChannels:    item.AudioChannels,
		ShowID:           item.ParentID, // for episodes, parent_id is the show
		Aired:            item.Aired,
		Language:         item.Language,
		CountryCode:      item.CountryCode,
		CustomRating:     item.CustomRating,
		CollectionNumber: item.CollectionNumber,
		EndDate:          item.EndDate,
		ReleaseDate:      item.ReleaseDate,
		DisplayOrder:     item.DisplayOrder,
		OriginalTitle:    item.OriginalTitle,
		DateAdded:        item.DateAdded,
		LastPlayed:       item.LastPlayed,
		Playcount:        item.Playcount,
		SetOverview:      item.SetOverview,
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
	if item.UserRating != 0 {
		resp.UserRating = &item.UserRating
	}
	if item.Season != nil {
		resp.Season = item.Season
	}
	if item.Episode != nil {
		resp.Episode = item.Episode
	}

	resp.Genres = decodeStrings(item.Genres)
	resp.Studios = decodeStrings(item.Studios)
	resp.Actors = decodeActors(item.Actors)
	resp.GuestStars = decodeGuestStars(item.Actors)
	resp.UniqueIDs = decodeUniqueIDs(item.UniqueIDs)
	resp.Countries = decodeStrings(item.Countries)
	resp.Directors = decodeStrings(item.Directors)
	resp.Credits = decodeStrings(item.Credits)
	resp.Tags = decodeStrings(item.Tags)
	resp.SubtitleLanguages = decodeStrings(item.SubtitleLanguages)

	return resp
}

func decodeStrings(s string) []string {
	if s == "" {
		return nil
	}
	var r []string
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		return nil
	}
	return r
}

func decodeActors(s string) []ActorResponse {
	if s == "" {
		return nil
	}
	var all []ActorResponse
	if err := json.Unmarshal([]byte(s), &all); err != nil {
		return nil
	}
	// Sort by SortOrder ascending
	sort.Slice(all, func(i, j int) bool {
		return all[i].SortOrder < all[j].SortOrder
	})
	// Filter: only Actor or GuestStar types
	var mainCast []ActorResponse
	for _, a := range all {
		switch a.Type {
		case "Actor", "":
			mainCast = append(mainCast, a)
		}
	}
	// Limit to top 6
	if len(mainCast) > 6 {
		mainCast = mainCast[:6]
	}
	return mainCast
}

func decodeGuestStars(s string) []ActorResponse {
	if s == "" {
		return nil
	}
	var all []ActorResponse
	if err := json.Unmarshal([]byte(s), &all); err != nil {
		return nil
	}
	// Sort by SortOrder ascending
	sort.Slice(all, func(i, j int) bool {
		return all[i].SortOrder < all[j].SortOrder
	})
	// Filter: only GuestStar type
	var guests []ActorResponse
	for _, a := range all {
		if a.Type == "GuestStar" {
			guests = append(guests, a)
		}
	}
	// Limit to top 12
	if len(guests) > 12 {
		guests = guests[:12]
	}
	return guests
}

func decodeUniqueIDs(s string) map[string]string {
	if s == "" {
		return nil
	}
	var r map[string]string
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		return nil
	}
	return r
}

// attachPresignedURLs resolves resource URLs for a media item via its provider.
//
// If the item's provider is not registered (stale provider_id, removed config),
// the response is returned without URLs and a warning is logged. This is a
// graceful degradation — the client will see nil URL fields rather than a 500.
func attachPresignedURLs(ctx context.Context, item *model.MediaItem, registry *provider.Registry, logger *slog.Logger) MediaItemResponse {
	resp := mediaItemToResponse(item)

	// Missing items don't have playable URLs.
	if item.Status == "missing" {
		return resp
	}

	p, ok := registry.Get(item.ProviderID)
	if !ok {
		logger.Warn("provider not found for media item",
			"item_id", item.ID,
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
	if u, err := p.LogoURL(ctx, item); err == nil && u != "" {
		resp.LogoURL = &u
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

// attachUserStatuses populates the UserStatus field for each response item.
func attachUserStatuses(ctx context.Context, statusRepo *repository.UserMediaStatusRepository, userID string, result []MediaItemResponse) {
	if userID == "" || len(result) == 0 {
		return
	}
	ids := make([]string, len(result))
	for i, r := range result {
		ids[i] = r.ID
	}
	statusMap, err := statusRepo.GetStatusesForItems(ctx, userID, ids)
	if err != nil {
		return
	}
	for i := range result {
		if s, ok := statusMap[result[i].ID]; ok {
			result[i].UserStatus = s
		}
	}
}

// filterMediaItemsByAllowedLibraries removes media items whose library the
// current user is not allowed to access.
//
// A nil allowedIDs slice means unrestricted access (admin/owner): the input
// slice is returned as-is. An empty (non-nil) slice means the user has access
// to no libraries, so an empty slice is returned.
func filterMediaItemsByAllowedLibraries(r *http.Request, items []model.MediaItem) []model.MediaItem {
	allowedIDs := middleware.GetAllowedLibraryIDs(r)

	// nil means unrestricted access, typically admin or owner.
	if allowedIDs == nil {
		return items
	}

	if len(items) == 0 || len(allowedIDs) == 0 {
		return []model.MediaItem{}
	}

	allowed := make(map[string]struct{}, len(allowedIDs))
	for _, id := range allowedIDs {
		allowed[id] = struct{}{}
	}

	filtered := make([]model.MediaItem, 0, len(items))
	for _, item := range items {
		if _, ok := allowed[item.LibraryID]; ok {
			filtered = append(filtered, item)
		}
	}

	return filtered
}

// getAccessibleMediaItem loads a single media item by id and enforces that the
// current user is allowed to access the library it belongs to.
//
// On any failure (missing id, load error, not found, forbidden library) it
// writes the appropriate HTTP response and returns (nil, false). The caller
// must abort the request when ok is false.
//
// Forbidden access is reported as 404 "resource not found" to avoid leaking
// the existence of a resource (resource enumeration protection).
func (h *MediaHandler) getAccessibleMediaItem(w http.ResponseWriter, r *http.Request, id string) (*model.MediaItem, bool) {
	if id == "" {
		response.ErrorCode(w, http.StatusBadRequest, errors.CodeMissingID, "")
		return nil, false
	}

	item, err := h.repo.Get(r.Context(), id)
	if err != nil {
		h.logger.Error("failed to load media item", "id", id, "err", err)
		response.ErrorCode(w, http.StatusInternalServerError, errors.CodeInternal, "")
		return nil, false
	}

	if item == nil {
		response.ErrorCode(w, http.StatusNotFound, errors.CodeResourceNotFound, "")
		return nil, false
	}

	if !middleware.IsLibraryAllowed(r, item.LibraryID) {
		response.ErrorCode(w, http.StatusNotFound, errors.CodeResourceNotFound, "")
		return nil, false
	}

	return item, true
}

// getAuthenticatedUserID extracts the authenticated user id from the request
// context. On failure it writes a 401 response and returns ("", false).
func getAuthenticatedUserID(w http.ResponseWriter, r *http.Request) (string, bool) {
	userID := middleware.GetUserID(r)
	userIDStr, ok := userID.(string)
	if !ok || userIDStr == "" {
		response.ErrorCode(w, http.StatusUnauthorized, errors.CodeUnauthorized, "")
		return "", false
	}

	return userIDStr, true
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
	if mediaType == "" {
		mediaType = "movie,show"
	}

	q := r.URL.Query().Get("q")

	sort := r.URL.Query().Get("sort")
	allowedSorts := map[string]bool{
		"title_asc":    true,
		"title_desc":   true,
		"year_asc":     true,
		"year_desc":    true,
		"rating_desc":  true,
		"created_desc": true,
	}
	if !allowedSorts[sort] {
		sort = "title_asc"
	}

	allowedIDs := middleware.GetAllowedLibraryIDs(r)

	if libID := r.URL.Query().Get("library_id"); libID != "" {
		if !middleware.IsLibraryAllowed(r, libID) {
			response.ErrorCode(w, http.StatusNotFound, errors.CodeResourceNotFound, "")
			return
		}

		allowedIDs = []string{libID}
	}

	items, total, err := h.repo.ListPaged(r.Context(), page, pageSize, mediaType, q, sort, allowedIDs, true)
	if err != nil {
		h.logger.Error("failed to list media", "err", err)
		response.ErrorCode(w, http.StatusInternalServerError, errors.CodeInternal, "")
		return
	}

	if items == nil {
		items = []model.MediaItem{}
	}

	result := attachPresignedURLsList(r.Context(), items, h.registry, h.logger)

	if userIDStr, ok := middleware.GetUserID(r).(string); ok && userIDStr != "" {
		attachUserStatuses(r.Context(), h.statusRepo, userIDStr, result)
	}

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

	item, ok := h.getAccessibleMediaItem(w, r, id)
	if !ok {
		return
	}

	result := attachPresignedURLs(r.Context(), item, h.registry, h.logger)

	if userIDStr, ok := middleware.GetUserID(r).(string); ok && userIDStr != "" {
		status, _ := h.statusRepo.GetStatus(r.Context(), userIDStr, id)
		result.UserStatus = status
	}

	response.Success(w, result)
}

// ListEpisodes returns all episodes for a given show.
func (h *MediaHandler) ListEpisodes(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	parent, ok := h.getAccessibleMediaItem(w, r, id)
	if !ok {
		return
	}

	if parent.Type != "show" {
		response.ErrorCode(w, http.StatusBadRequest, errors.CodeMediaItemNotShow, "")
		return
	}

	items, err := h.repo.GetEpisodesByShowID(r.Context(), id)
	if err != nil {
		h.logger.Error("failed to list episodes", "show_id", id, "err", err)
		response.ErrorCode(w, http.StatusInternalServerError, errors.CodeInternal, "")
		return
	}

	if items == nil {
		items = []model.MediaItem{}
	}

	items = filterMediaItemsByAllowedLibraries(r, items)

	result := attachPresignedURLsList(r.Context(), items, h.registry, h.logger)

	if userIDStr, ok := middleware.GetUserID(r).(string); ok && userIDStr != "" {
		attachUserStatuses(r.Context(), h.statusRepo, userIDStr, result)
	}

	response.Success(w, result)
}

// UpdateProgress records watch progress for the current user.
//
// Accepts two payload shapes:
//   - Legacy: { "position": int, "duration": int, "finished": bool }
//   - Launcher: { "played": bool } — marks the item as played (position=0, finished=false)
//   - Launcher: { "finished": bool } — when finished=true, marks the item as watched
func (h *MediaHandler) UpdateProgress(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	item, ok := h.getAccessibleMediaItem(w, r, id)
	if !ok {
		return
	}

	if item.Type == "show" {
		response.ErrorCode(w, http.StatusBadRequest, errors.CodeCannotUpdateProgressForShow, "")
		return
	}

	userIDStr, ok := getAuthenticatedUserID(w, r)
	if !ok {
		return
	}

	var req struct {
		Position int  `json:"position"`
		Duration int  `json:"duration"`
		Finished bool `json:"finished"`
		Played   bool `json:"played"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.ErrorCode(w, http.StatusBadRequest, errors.CodeInvalidJSON, "")
		return
	}

	// Launcher-mode: { "played": true } shorthand.
	if req.Played && req.Position == 0 && req.Duration == 0 && !req.Finished {
		if err := h.repo.UpsertProgress(r.Context(), userIDStr, id, 0, 0, false); err != nil {
			h.logger.Error("failed to update progress", "media_id", id, "user_id", userIDStr, "err", err)
			response.ErrorCode(w, http.StatusInternalServerError, errors.CodeInternal, "")
			return
		}

		currentStatus, _ := h.statusRepo.GetStatus(r.Context(), userIDStr, id)
		if currentStatus == "none" || currentStatus == "want_to_watch" {
			_ = h.statusRepo.SetStatus(r.Context(), userIDStr, id, "watching")
		}

		response.NoContent(w)
		return
	}

	if req.Position < 0 || req.Duration < 0 {
		response.ErrorCode(w, http.StatusBadRequest, errors.CodeInvalidProgress, "")
		return
	}

	if req.Duration > 0 && req.Position > req.Duration {
		req.Position = req.Duration
	}

	if err := h.repo.UpsertProgress(r.Context(), userIDStr, id, req.Position, req.Duration, req.Finished); err != nil {
		h.logger.Error("failed to update progress", "media_id", id, "user_id", userIDStr, "err", err)
		response.ErrorCode(w, http.StatusInternalServerError, errors.CodeInternal, "")
		return
	}

	if req.Position > 0 || req.Finished {
		currentStatus, _ := h.statusRepo.GetStatus(r.Context(), userIDStr, id)

		if req.Position > 0 && (currentStatus == "none" || currentStatus == "want_to_watch") {
			_ = h.statusRepo.SetStatus(r.Context(), userIDStr, id, "watching")
		}

		if req.Finished && req.Position > 0 && currentStatus != "dropped" {
			_ = h.statusRepo.SetStatus(r.Context(), userIDStr, id, "watched")
		}
	}

	response.NoContent(w)
}

// GetContinueWatching returns media items with unfinished progress for the current user.
func (h *MediaHandler) GetContinueWatching(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	if userID == nil {
		response.ErrorCode(w, http.StatusUnauthorized, errors.CodeUnauthorized, "")
		return
	}
	userIDStr, ok := userID.(string)
	if !ok {
		response.ErrorCode(w, http.StatusUnauthorized, errors.CodeUnauthorized, "")
		return
	}

	allowedIDs := middleware.GetAllowedLibraryIDs(r)

	items, err := h.repo.GetContinueWatching(r.Context(), userIDStr, 20, allowedIDs)
	if err != nil {
		h.logger.Error("failed to get continue watching", "err", err)
		response.ErrorCode(w, http.StatusInternalServerError, errors.CodeInternal, "")
		return
	}
	if items == nil {
		items = []repository.MediaItemWithProgress{}
	}

	// Return items with progress embedded
	type progressItem struct {
		MediaItemResponse
		Position int  `json:"position"`
		Duration int  `json:"duration"`
		Finished bool `json:"finished"`
	}
	progressItems := make([]progressItem, len(items))
	for i := range items {
		resp := attachPresignedURLs(r.Context(), &items[i].MediaItem, h.registry, h.logger)
		progressItems[i] = progressItem{
			MediaItemResponse: resp,
			Position:          items[i].Position,
			Duration:          items[i].Duration,
			Finished:          items[i].Finished,
		}
	}

	// Attach user statuses.
	if userIDStr != "" {
		ids := make([]string, len(progressItems))
		for i := range progressItems {
			ids[i] = progressItems[i].ID
		}
		statusMap, _ := h.statusRepo.GetStatusesForItems(r.Context(), userIDStr, ids)
		for i := range progressItems {
			if s, ok := statusMap[progressItems[i].ID]; ok {
				progressItems[i].UserStatus = s
			}
		}
	}

	response.Success(w, progressItems)
}

// Delete removes a media item from the catalog.
func (h *MediaHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if !middleware.IsUnrestrictedLibraryAccess(r) {
		response.ErrorCode(w, http.StatusForbidden, errors.CodeForbidden, "")
		return
	}

	id := r.PathValue("id")
	if id == "" {
		response.ErrorCode(w, http.StatusBadRequest, errors.CodeMissingID, "")
		return
	}

	if _, ok := h.getAccessibleMediaItem(w, r, id); !ok {
		return
	}

	if err := h.repo.Delete(r.Context(), id); err != nil {
		h.logger.Error("failed to delete media item", "media_id", id, "err", err)
		response.ErrorCode(w, http.StatusInternalServerError, errors.CodeInternal, "")
		return
	}

	response.NoContent(w)
}

// GetLibraries returns libraries the current user can view, with item counts.
func (h *MediaHandler) GetLibraries(w http.ResponseWriter, r *http.Request) {
	allowedIDs := middleware.GetAllowedLibraryIDs(r)

	var allLibs []model.Library
	var err error
	if allowedIDs == nil {
		// Admin: get all libraries.
		allLibs, err = h.libRepo.List(r.Context())
	} else if len(allowedIDs) == 0 {
		// User with no library access: return empty.
		allLibs = []model.Library{}
	} else {
		// Regular user: fetch only allowed libraries.
		allLibs, err = h.libRepo.List(r.Context())
		if err == nil {
			var filtered []model.Library
			for _, lib := range allLibs {
				for _, id := range allowedIDs {
					if lib.ID == id {
						filtered = append(filtered, lib)
						break
					}
				}
			}
			allLibs = filtered
		}
	}

	if err != nil {
		response.ErrorCode(w, http.StatusInternalServerError, errors.CodeInternal, "")
		return
	}

	result := make([]map[string]interface{}, len(allLibs))
	for i, lib := range allLibs {
		movies, shows, episodes, _ := h.libRepo.ItemCountsByType(r.Context(), lib.ID)
		result[i] = map[string]interface{}{
			"id":              lib.ID,
			"name":            lib.Name,
			"type":            lib.Type,
			"provider_id":     lib.ProviderID,
			"source_path":     lib.SourcePath,
			"metadata_source": lib.MetadataSource,
			"item_count":      movies + shows + episodes,
			"movie_count":     movies,
			"show_count":      shows,
			"episode_count":   episodes,
		}
	}

	response.Success(w, result)
}

// SetStatus sets the user's media status for an item.
func (h *MediaHandler) SetStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if _, ok := h.getAccessibleMediaItem(w, r, id); !ok {
		return
	}

	userIDStr, ok := getAuthenticatedUserID(w, r)
	if !ok {
		return
	}

	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.ErrorCode(w, http.StatusBadRequest, errors.CodeInvalidJSON, "")
		return
	}

	valid := map[string]bool{
		"watching":      true,
		"want_to_watch": true,
		"watched":       true,
		"dropped":       true,
		"none":          true,
	}
	if !valid[req.Status] {
		response.ErrorCode(w, http.StatusBadRequest, errors.CodeInvalidStatus, "invalid status: must be one of: watching, want_to_watch, watched, dropped, none")
		return
	}

	if err := h.statusRepo.SetStatus(r.Context(), userIDStr, id, req.Status); err != nil {
		h.logger.Error("failed to set status", "media_id", id, "user_id", userIDStr, "err", err)
		response.ErrorCode(w, http.StatusInternalServerError, errors.CodeInternal, "")
		return
	}

	response.Success(w, map[string]interface{}{
		"status": req.Status,
	})
}

// GetStatus returns the user's media status for an item.
func (h *MediaHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if _, ok := h.getAccessibleMediaItem(w, r, id); !ok {
		return
	}

	userIDStr, ok := getAuthenticatedUserID(w, r)
	if !ok {
		return
	}

	status, err := h.statusRepo.GetStatus(r.Context(), userIDStr, id)
	if err != nil {
		h.logger.Error("failed to get status", "media_id", id, "user_id", userIDStr, "err", err)
		response.ErrorCode(w, http.StatusInternalServerError, errors.CodeInternal, "")
		return
	}

	response.Success(w, map[string]interface{}{
		"status": status,
	})
}

// GetProgress returns the user's watch progress for an item.
//
// Phase 2.5: this backs the resume-from-position flow on the native player
// (PlayerView fetches progress before `play_media`, then seeks after
// `MPV_EVENT_FILE_LOADED`). Returns 200 with the progress object when a row
// exists, 200 with `null` data when no progress has been recorded yet (so the
// caller can distinguish "no progress" from "error" without a 404 round-trip).
func (h *MediaHandler) GetProgress(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if _, ok := h.getAccessibleMediaItem(w, r, id); !ok {
		return
	}

	userIDStr, ok := getAuthenticatedUserID(w, r)
	if !ok {
		return
	}

	progress, err := h.repo.GetProgress(r.Context(), userIDStr, id)
	if err != nil {
		h.logger.Error("failed to get progress", "media_id", id, "user_id", userIDStr, "err", err)
		response.ErrorCode(w, http.StatusInternalServerError, errors.CodeInternal, "")
		return
	}

	if progress == nil {
		response.Success(w, nil)
		return
	}

	response.Success(w, map[string]interface{}{
		"position":   progress.Position,
		"duration":   progress.Duration,
		"finished":   progress.Finished,
		"updated_at": progress.UpdatedAt,
	})
}

// GetByStatus returns media items filtered by user status.
func (h *MediaHandler) GetByStatus(w http.ResponseWriter, r *http.Request) {
	userIDStr, ok := getAuthenticatedUserID(w, r)
	if !ok {
		return
	}

	status := r.URL.Query().Get("status")
	valid := map[string]bool{
		"watching":      true,
		"want_to_watch": true,
		"watched":       true,
		"dropped":       true,
	}
	if !valid[status] {
		response.ErrorCode(w, http.StatusBadRequest, errors.CodeInvalidStatus, "invalid status: must be one of: watching, want_to_watch, watched, dropped")
		return
	}

	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	items, err := h.statusRepo.GetItemsByStatus(r.Context(), userIDStr, status, limit)
	if err != nil {
		h.logger.Error("failed to get items by status", "user_id", userIDStr, "status", status, "err", err)
		response.ErrorCode(w, http.StatusInternalServerError, errors.CodeInternal, "")
		return
	}

	if items == nil {
		items = []model.MediaItem{}
	}

	items = filterMediaItemsByAllowedLibraries(r, items)

	result := attachPresignedURLsList(r.Context(), items, h.registry, h.logger)

	for i := range result {
		result[i].UserStatus = status
	}

	response.Success(w, map[string]interface{}{
		"items": result,
		"total": len(items),
	})
}

// ImportRequest triggers an async NFO-based import.
type ImportRequest struct {
	SourcePath string `json:"source_path"`
	ProviderID string `json:"provider_id"`
	LibraryID  string `json:"library_id"`
}

// ImportResponse returns the created job ID.
type ImportResponse struct {
	JobID  string `json:"job_id"`
	Status string `json:"status"`
}

// Import triggers an asynchronous media import from the given path.
func (h *MediaHandler) Import(w http.ResponseWriter, r *http.Request) {
	if !middleware.IsUnrestrictedLibraryAccess(r) {
		response.ErrorCode(w, http.StatusForbidden, errors.CodeForbidden, "")
		return
	}

	var req ImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SourcePath == "" {
		response.ErrorCode(w, http.StatusBadRequest, errors.CodeValidation, "")
		return
	}

	if req.ProviderID == "" {
		req.ProviderID = "local"
	}
	if req.LibraryID == "" {
		response.ErrorCode(w, http.StatusBadRequest, errors.CodeLibraryIDRequired, "")
		return
	}

	if !middleware.IsLibraryAllowed(r, req.LibraryID) {
		response.ErrorCode(w, http.StatusNotFound, errors.CodeResourceNotFound, "")
		return
	}

	if !h.refreshCoordinator.TryStart(req.LibraryID) {
		response.ErrorCode(w, http.StatusConflict, errors.CodeRefreshAlreadyInProgress, "")
		return
	}

	lib, err := h.libRepo.GetByID(r.Context(), req.LibraryID)
	if err != nil || lib == nil {
		h.refreshCoordinator.Finish(req.LibraryID)
		response.ErrorCode(w, http.StatusBadRequest, errors.CodeLibraryNotFound, "library not found: "+req.LibraryID)
		return
	}

	var fs service.ImportFS
	var providerID string

	if req.ProviderID == "local" {
		fs = service.NewLocalImportFS()
		providerID = "local"
	} else {
		p, ok := h.registry.Get(req.ProviderID)
		if !ok {
			h.refreshCoordinator.Finish(req.LibraryID)
			response.ErrorCode(w, http.StatusBadRequest, errors.CodeProviderNotFound, "provider not found: "+req.ProviderID)
			return
		}
		if p.Type() != "s3" {
			h.refreshCoordinator.Finish(req.LibraryID)
			response.ErrorCode(w, http.StatusBadRequest, errors.CodeImportFromProviderTypeUnsupported, "import from provider type '"+p.Type()+"' is not supported yet")
			return
		}

		rec, err := h.providerRepo.GetByID(r.Context(), req.ProviderID)
		if err != nil || rec == nil {
			h.refreshCoordinator.Finish(req.LibraryID)
			response.ErrorCode(w, http.StatusInternalServerError, errors.CodeFailedToLoadProviderConfig, "")
			return
		}

		s3fs, err := service.NewS3ImportFS(r.Context(), rec, req.SourcePath)
		if err != nil {
			h.refreshCoordinator.Finish(req.LibraryID)
			response.ErrorCode(w, http.StatusInternalServerError, errors.CodeFailedToCreateS3Client, "failed to create S3 client: "+err.Error())
			return
		}

		fs = s3fs
		providerID = req.ProviderID
	}

	imp := service.NewImporter(fs, providerID, h.db, h.repo, h.jobRepo)
	imp.SetLibraryID(req.LibraryID)

	job, err := imp.ImportRequest(r.Context(), req.SourcePath)
	if err != nil {
		h.refreshCoordinator.Finish(req.LibraryID)

		if appErr, ok := errors.IsAppError(err); ok {
			response.AppError(w, appErr)
			return
		}

		response.ErrorCode(w, http.StatusInternalServerError, errors.CodeInternal, "")
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
	if id == "" {
		response.ErrorCode(w, http.StatusBadRequest, errors.CodeMissingID, "")
		return
	}

	job, err := h.jobRepo.Get(r.Context(), id)
	if err != nil {
		response.ErrorCode(w, http.StatusInternalServerError, errors.CodeInternal, "")
		return
	}

	if job == nil {
		response.ErrorCode(w, http.StatusNotFound, errors.CodeResourceNotFound, "")
		return
	}

	if !middleware.IsLibraryAllowed(r, job.LibraryID) {
		response.ErrorCode(w, http.StatusNotFound, errors.CodeResourceNotFound, "")
		return
	}

	response.Success(w, job)
}

// ServeBackdrop serves a backdrop image.
func (h *MediaHandler) ServeBackdrop(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	item, ok := h.getAccessibleMediaItem(w, r, id)
	if !ok {
		return
	}

	if item.BackdropPath == "" {
		response.ErrorCode(w, http.StatusNotFound, errors.CodeResourceNotFound, "")
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
//
// ServeContent is a low-level primitive and performs NO permission checks.
// Every caller (Stream, Poster, ...) MUST verify library access via
// getAccessibleMediaItem (or an equivalent IsLibraryAllowed check) before
// invoking ServeContent.
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

// ServeLogo serves the logo image.
func (h *MediaHandler) ServeLogo(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	item, ok := h.getAccessibleMediaItem(w, r, id)
	if !ok {
		return
	}

	if item.LogoPath == "" {
		response.ErrorCode(w, http.StatusNotFound, errors.CodeResourceNotFound, "")
		return
	}

	_, err := os.Stat(item.LogoPath)
	if err != nil {
		if os.IsNotExist(err) {
			jsonError(w, 404, "logo file not found on disk")
			return
		}
		jsonError(w, 500, "internal server error")
		return
	}

	http.ServeFile(w, r, item.LogoPath)
}

// Stream serves a media file with Range request support.
func (h *MediaHandler) Stream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	item, ok := h.getAccessibleMediaItem(w, r, id)
	if !ok {
		return
	}

	if item.Status == "missing" || item.FilePath == "" {
		response.ErrorCode(w, http.StatusNotFound, errors.CodeResourceNotFound, "")
		return
	}

	h.ServeContent(w, r, item)
}

// Poster serves a poster/thumbnail image.
func (h *MediaHandler) Poster(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	item, ok := h.getAccessibleMediaItem(w, r, id)
	if !ok {
		return
	}

	if item.PosterPath == "" {
		response.ErrorCode(w, http.StatusNotFound, errors.CodeResourceNotFound, "")
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
