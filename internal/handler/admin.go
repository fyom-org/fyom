package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/fyom/fyom/internal/model"
	"github.com/fyom/fyom/internal/repository"
	"github.com/fyom/fyom/pkg/response"
)

// AdminHandler handles admin-only system endpoints.
type AdminHandler struct {
	repo        *repository.AdminRepository
	mediaRepo   *repository.MediaRepository
	settingRepo *repository.SystemSettingRepository
	libPermRepo *repository.LibraryPermissionRepository
	db          *repository.DB
}

// NewAdminHandler creates a new AdminHandler.
func NewAdminHandler(repo *repository.AdminRepository, mediaRepo *repository.MediaRepository, settingRepo *repository.SystemSettingRepository, libPermRepo *repository.LibraryPermissionRepository, db *repository.DB) *AdminHandler {
	return &AdminHandler{repo: repo, mediaRepo: mediaRepo, settingRepo: settingRepo, libPermRepo: libPermRepo, db: db}
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

// GetSettings returns all system settings as a flat object.
func (h *AdminHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	rows, err := h.settingRepo.List(r.Context())
	if err != nil {
		response.Error(w, 500, "internal server error")
		return
	}
	result := make(map[string]string)
	for _, row := range rows {
		result[row.Key] = row.Value
	}
	response.Success(w, result)
}

// UpdateSettings updates system settings. Only existing keys are allowed.
func (h *AdminHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	var body map[string]string
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		response.Error(w, 400, "invalid JSON")
		return
	}

	existing, err := h.settingRepo.List(r.Context())
	if err != nil {
		response.Error(w, 500, "internal server error")
		return
	}
	allowed := make(map[string]bool)
	for _, row := range existing {
		allowed[row.Key] = true
	}

	for key, value := range body {
		if !allowed[key] {
			response.Error(w, 400, "unknown setting: "+key)
			return
		}
		if err := h.settingRepo.SetSetting(r.Context(), key, value); err != nil {
			response.Error(w, 500, "internal server error")
			return
		}
	}
	response.NoContent(w)
}

// ListMedia returns a paginated, searchable list of all media items (admin).
func (h *AdminHandler) ListMedia(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	q := r.URL.Query().Get("q")
	mediaType := r.URL.Query().Get("type")
	libraryID := r.URL.Query().Get("library_id")
	sort := r.URL.Query().Get("sort")
	if sort == "" {
		sort = "created_desc"
	}

	var whereClauses []string
	var whereArgs []interface{}

	if q != "" {
		pattern := "%" + q + "%"
		whereClauses = append(whereClauses, "(title LIKE ? OR sort_title LIKE ?)")
		whereArgs = append(whereArgs, pattern, pattern)
	}
	if mediaType != "" {
		whereClauses = append(whereClauses, "type = ?")
		whereArgs = append(whereArgs, mediaType)
	}
	if libraryID != "" {
		whereClauses = append(whereClauses, "library_id = ?")
		whereArgs = append(whereArgs, libraryID)
	}

	var where string
	if len(whereClauses) > 0 {
		where = " WHERE " + strings.Join(whereClauses, " AND ")
	}

	orderBy := "created_at DESC"
	switch sort {
	case "title_asc":
		orderBy = "title ASC"
	case "title_desc":
		orderBy = "title DESC"
	case "created_asc":
		orderBy = "created_at ASC"
	}

	var total int
	countQuery := "SELECT COUNT(*) FROM media_items" + where
	if err := h.db.QueryRowContext(r.Context(), countQuery, whereArgs...).Scan(&total); err != nil {
		response.Error(w, 500, "internal server error")
		return
	}

	offset := (page - 1) * limit
	dataQuery := "SELECT id, type, title, sort_title, year, overview, rating, duration, file_path, poster_path, backdrop_path, parent_id, season, episode, metadata_source, provider_id, library_id, status, created_at, updated_at, mpaa, genres, studios, actors, unique_ids, premiered, outline, tagline, countries, directors, credits, tags, set_name, video_codec, video_width, video_height, video_duration_seconds, audio_codec, audio_channels, subtitle_languages FROM media_items" + where + " ORDER BY " + orderBy + " LIMIT ? OFFSET ?"
	dataArgs := append(whereArgs, limit, offset)

	rows, err := h.db.QueryContext(r.Context(), dataQuery, dataArgs...)
	if err != nil {
		response.Error(w, 500, "internal server error")
		return
	}
	defer func() { _ = rows.Close() }()

	var items []model.MediaItem
	for rows.Next() {
		var m model.MediaItem
		var season, episode int
		if err := rows.Scan(&m.ID, &m.Type, &m.Title, &m.SortTitle, &m.Year,
			&m.Overview, &m.Rating, &m.Duration, &m.FilePath, &m.PosterPath,
			&m.BackdropPath, &m.ParentID, &season, &episode,
			&m.MetadataSource, &m.ProviderID, &m.LibraryID, &m.Status, &m.CreatedAt, &m.UpdatedAt,
			&m.MPAA, &m.Genres, &m.Studios, &m.Actors, &m.UniqueIDs, &m.Premiered,
			&m.Outline, &m.Tagline, &m.Countries, &m.Directors, &m.Credits, &m.Tags,
			&m.SetName, &m.VideoCodec, &m.VideoWidth, &m.VideoHeight, &m.VideoDurationSeconds,
			&m.AudioCodec, &m.AudioChannels, &m.SubtitleLanguages); err != nil {
			response.Error(w, 500, "internal server error")
			return
		}
		m.Season = repository.IntPtr(season)
		m.Episode = repository.IntPtr(episode)
		items = append(items, m)
	}

	response.Success(w, map[string]interface{}{
		"items": items,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// DeleteMedia deletes a single media item. Shows cascade to episodes.
func (h *AdminHandler) DeleteMedia(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		response.Error(w, 400, "missing id")
		return
	}

	item, err := h.mediaRepo.Get(r.Context(), id)
	if err != nil {
		response.Error(w, 500, "internal server error")
		return
	}
	if item == nil {
		response.Error(w, 404, "not found")
		return
	}

	if item.Type == "show" {
		if err := h.mediaRepo.DeleteShowWithEpisodes(r.Context(), id); err != nil {
			response.Error(w, 500, "internal server error")
			return
		}
	} else {
		if err := h.mediaRepo.Delete(r.Context(), id); err != nil {
			response.Error(w, 500, "internal server error")
			return
		}
	}
	response.NoContent(w)
}

// ListPermissions returns all library permissions (admin only).
func (h *AdminHandler) ListPermissions(w http.ResponseWriter, r *http.Request) {
	perms, err := h.libPermRepo.GetAllPermissions(r.Context())
	if err != nil {
		response.Error(w, 500, "internal server error")
		return
	}
	if perms == nil {
		perms = []repository.UserLibraryPermission{}
	}
	response.Success(w, perms)
}

// UpdatePermission sets a single library permission (admin only).
func (h *AdminHandler) UpdatePermission(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UserID    string `json:"user_id"`
		LibraryID string `json:"library_id"`
		CanView   bool   `json:"can_view"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, 400, "invalid JSON")
		return
	}
	if req.UserID == "" || req.LibraryID == "" {
		response.Error(w, 400, "user_id and library_id are required")
		return
	}

	if err := h.libPermRepo.SetPermission(r.Context(), req.UserID, req.LibraryID, req.CanView); err != nil {
		response.Error(w, 500, "internal server error")
		return
	}
	response.NoContent(w)
}

// ListMissing returns paginated missing items (admin only).
func (h *AdminHandler) ListMissing(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	libraryID := r.URL.Query().Get("library_id")

	query := "SELECT id, type, title, sort_title, year, overview, rating, duration, file_path, poster_path, backdrop_path, parent_id, season, episode, metadata_source, provider_id, library_id, status, created_at, updated_at, mpaa, genres, studios, actors, unique_ids, premiered, outline, tagline, countries, directors, credits, tags, set_name, video_codec, video_width, video_height, video_duration_seconds, audio_codec, audio_channels, subtitle_languages FROM media_items WHERE status = 'missing'"
	var args []interface{}
	if libraryID != "" {
		query += " AND library_id = ?"
		args = append(args, libraryID)
	}
	query += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, (page-1)*limit)

	rows, err := h.db.QueryContext(r.Context(), query, args...)
	if err != nil {
		response.Error(w, 500, "internal server error")
		return
	}
	defer func() { _ = rows.Close() }()

	var items []model.MediaItem
	for rows.Next() {
		var m model.MediaItem
		var season, episode int
		if err := rows.Scan(&m.ID, &m.Type, &m.Title, &m.SortTitle, &m.Year,
			&m.Overview, &m.Rating, &m.Duration, &m.FilePath, &m.PosterPath,
			&m.BackdropPath, &m.ParentID, &season, &episode,
			&m.MetadataSource, &m.ProviderID, &m.LibraryID, &m.Status, &m.CreatedAt, &m.UpdatedAt,
			&m.MPAA, &m.Genres, &m.Studios, &m.Actors, &m.UniqueIDs, &m.Premiered,
			&m.Outline, &m.Tagline, &m.Countries, &m.Directors, &m.Credits, &m.Tags,
			&m.SetName, &m.VideoCodec, &m.VideoWidth, &m.VideoHeight, &m.VideoDurationSeconds,
			&m.AudioCodec, &m.AudioChannels, &m.SubtitleLanguages); err != nil {
			response.Error(w, 500, "internal server error")
			return
		}
		m.Season = repository.IntPtr(season)
		m.Episode = repository.IntPtr(episode)
		items = append(items, m)
	}

	var total int
	countQuery := "SELECT COUNT(*) FROM media_items WHERE status = 'missing'"
	var countArgs []interface{}
	if libraryID != "" {
		countQuery += " AND library_id = ?"
		countArgs = append(countArgs, libraryID)
	}
	if err := h.db.QueryRowContext(r.Context(), countQuery, countArgs...).Scan(&total); err != nil {
		response.Error(w, 500, "internal server error")
		return
	}

	response.Success(w, map[string]interface{}{
		"items": items,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// DeleteMissing deletes all missing items (admin only).
func (h *AdminHandler) DeleteMissing(w http.ResponseWriter, r *http.Request) {
	var req struct {
		LibraryID string `json:"library_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	tx, err := h.db.BeginTx(r.Context(), nil)
	if err != nil {
		response.Error(w, 500, "internal server error")
		return
	}
	defer func() { _ = tx.Rollback() }()

	// Get IDs of missing items to delete.
	query := "SELECT id FROM media_items WHERE status = 'missing'"
	var args []interface{}
	if req.LibraryID != "" {
		query += " AND library_id = ?"
		args = append(args, req.LibraryID)
	}
	rows, err := tx.QueryContext(r.Context(), query, args...)
	if err != nil {
		response.Error(w, 500, "internal server error")
		return
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			response.Error(w, 500, "internal server error")
			return
		}
		ids = append(ids, id)
	}
	rows.Close()

	if len(ids) == 0 {
		response.Success(w, map[string]interface{}{"deleted_count": 0})
		return
	}

	// Delete watch progress for these items.
	placeholders := make([]string, len(ids))
	idArgs := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		idArgs[i] = id
	}
	if _, err := tx.ExecContext(r.Context(), "DELETE FROM watch_progress WHERE media_item_id IN ("+strings.Join(placeholders, ",")+")", idArgs...); err != nil {
		response.Error(w, 500, "internal server error")
		return
	}
	// Delete episodes whose parent is in the list.
	if _, err := tx.ExecContext(r.Context(), "DELETE FROM media_items WHERE parent_id IN ("+strings.Join(placeholders, ",")+")", idArgs...); err != nil {
		response.Error(w, 500, "internal server error")
		return
	}
	// Delete the items themselves.
	if _, err := tx.ExecContext(r.Context(), "DELETE FROM media_items WHERE id IN ("+strings.Join(placeholders, ",")+")", idArgs...); err != nil {
		response.Error(w, 500, "internal server error")
		return
	}

	if err := tx.Commit(); err != nil {
		response.Error(w, 500, "internal server error")
		return
	}

	response.Success(w, map[string]interface{}{"deleted_count": len(ids)})
}