package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/fyom/fyom/internal/model"
	"github.com/google/uuid"
)

const (
	defaultMediaStatus         = "available"
	defaultMediaProviderID     = "local"
	defaultMediaLibraryID      = "default"
	defaultMediaMetadataSource = "nfo"
)

// IntPtr returns a pointer to the given int value.
func IntPtr(v int) *int {
	return &v
}

// MediaRepository provides access to media_items.
type MediaRepository struct {
	db *DB
}

// NewMediaRepository creates a new MediaRepository.
func NewMediaRepository(db *DB) *MediaRepository {
	return &MediaRepository{db: db}
}

// MediaColumns is the canonical column list for media_items SELECT queries.
// Exported so handlers can build dynamic queries without hardcoding column names.
const MediaColumns = `id, type, title, sort_title, year, overview, rating, duration,
        file_path, root_path, primary_path, nfo_path, poster_path, backdrop_path, parent_id, season, episode,
        metadata_source, provider_id, library_id, status, created_at, updated_at,
        mpaa, genres, studios, actors, unique_ids, premiered, outline, tagline,
        countries, directors, credits, tags, set_name, set_overview, video_codec,
        video_width, video_height, video_duration_seconds, audio_codec, audio_channels,
        subtitle_languages, aired, logo_path, language, country_code, custom_rating,
        collection_number, end_date, release_date, display_order, original_title,
        user_rating, date_added, last_played, playcount`

const mediaColumns = MediaColumns

const mediaInsertColumns = `(id, type, title, sort_title, year, overview, rating, duration,
        file_path, root_path, primary_path, nfo_path, poster_path, backdrop_path, parent_id, season, episode,
        metadata_source, provider_id, library_id, status, created_at, updated_at,
        mpaa, genres, studios, actors, unique_ids, premiered, outline, tagline,
        countries, directors, credits, tags, set_name, set_overview, video_codec,
        video_width, video_height, video_duration_seconds, audio_codec, audio_channels,
        subtitle_languages, aired, logo_path, language, country_code, custom_rating,
        collection_number, end_date, release_date, display_order, original_title,
        user_rating, date_added, last_played, playcount)`

const mediaInsertPlaceholders = `(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

func nullableParentID(parentID string) any {
	if parentID == "" {
		return nil
	}

	return parentID
}

func stringFromNullString(v sql.NullString) string {
	if !v.Valid {
		return ""
	}

	return v.String
}

func intPtrFromNullInt64(v sql.NullInt64) *int {
	if !v.Valid {
		return nil
	}

	return IntPtr(int(v.Int64))
}

func normalizeMediaItemForWrite(m *model.MediaItem) {
	if m.ProviderID == "" {
		m.ProviderID = defaultMediaProviderID
	}

	if m.LibraryID == "" {
		m.LibraryID = defaultMediaLibraryID
	}

	if m.MetadataSource == "" {
		m.MetadataSource = defaultMediaMetadataSource
	}

	if m.Status == "" {
		m.Status = defaultMediaStatus
	}

	if m.PrimaryPath == "" && m.FilePath != "" {
		m.PrimaryPath = m.FilePath
	}

	if m.RootPath == "" {
		m.RootPath = "/"
	}
}

func scanMediaItem(scanner interface{ Scan(dest ...any) error }, m *model.MediaItem) error {
	var parentID sql.NullString
	var season sql.NullInt64
	var episode sql.NullInt64

	if err := scanner.Scan(
		&m.ID,
		&m.Type,
		&m.Title,
		&m.SortTitle,
		&m.Year,
		&m.Overview,
		&m.Rating,
		&m.Duration,
		&m.FilePath,
		&m.RootPath,
		&m.PrimaryPath,
		&m.NFOPath,
		&m.PosterPath,
		&m.BackdropPath,
		&parentID,
		&season,
		&episode,
		&m.MetadataSource,
		&m.ProviderID,
		&m.LibraryID,
		&m.Status,
		&m.CreatedAt,
		&m.UpdatedAt,
		&m.MPAA,
		&m.Genres,
		&m.Studios,
		&m.Actors,
		&m.UniqueIDs,
		&m.Premiered,
		&m.Outline,
		&m.Tagline,
		&m.Countries,
		&m.Directors,
		&m.Credits,
		&m.Tags,
		&m.SetName,
		&m.SetOverview,
		&m.VideoCodec,
		&m.VideoWidth,
		&m.VideoHeight,
		&m.VideoDurationSeconds,
		&m.AudioCodec,
		&m.AudioChannels,
		&m.SubtitleLanguages,
		&m.Aired,
		&m.LogoPath,
		&m.Language,
		&m.CountryCode,
		&m.CustomRating,
		&m.CollectionNumber,
		&m.EndDate,
		&m.ReleaseDate,
		&m.DisplayOrder,
		&m.OriginalTitle,
		&m.UserRating,
		&m.DateAdded,
		&m.LastPlayed,
		&m.Playcount,
	); err != nil {
		return err
	}

	m.ParentID = stringFromNullString(parentID)
	m.Season = intPtrFromNullInt64(season)
	m.Episode = intPtrFromNullInt64(episode)

	return nil
}

// ScanMediaItemRows hydrates media rows using the same order as MediaColumns.
func ScanMediaItemRows(rows *sql.Rows) ([]model.MediaItem, error) {
	var items []model.MediaItem

	for rows.Next() {
		var m model.MediaItem
		if err := scanMediaItem(rows, &m); err != nil {
			return nil, err
		}

		items = append(items, m)
	}

	return items, rows.Err()
}

func scanMediaRow(row *sql.Row, m *model.MediaItem) error {
	return scanMediaItem(row, m)
}

// List returns all media items, optionally filtered by type.
func (r *MediaRepository) List(ctx context.Context, mediaType string) ([]model.MediaItem, error) {
	query := `SELECT ` + mediaColumns + ` FROM media_items`
	args := []interface{}{}

	if mediaType != "" {
		query += " WHERE type = ?"
		args = append(args, mediaType)
	}

	query += " ORDER BY sort_title ASC, title ASC"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var items []model.MediaItem

	for rows.Next() {
		var m model.MediaItem
		if err := scanMediaItem(rows, &m); err != nil {
			return nil, err
		}

		items = append(items, m)
	}

	return items, rows.Err()
}

// Get returns a single media item by ID.
func (r *MediaRepository) Get(ctx context.Context, id string) (*model.MediaItem, error) {
	var m model.MediaItem

	err := scanMediaRow(r.db.QueryRowContext(ctx, `SELECT `+mediaColumns+` FROM media_items WHERE id = ?`, id), &m)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	return &m, nil
}

// Create inserts a new media item.
func (r *MediaRepository) Create(ctx context.Context, m *model.MediaItem) error {
	if m.ID == "" {
		m.ID = uuid.New().String()
	}

	now := time.Now().UTC().Format(time.RFC3339)
	m.CreatedAt = now
	m.UpdatedAt = now

	normalizeMediaItemForWrite(m)

	season := 0
	if m.Season != nil {
		season = *m.Season
	}

	episode := 0
	if m.Episode != nil {
		episode = *m.Episode
	}

	return WithSQLiteBusyRetry(ctx, func() error {
		_, err := r.db.ExecContext(ctx, `INSERT INTO media_items
            `+mediaInsertColumns+`
            VALUES `+mediaInsertPlaceholders,
			m.ID,
			m.Type,
			m.Title,
			m.SortTitle,
			m.Year,
			m.Overview,
			m.Rating,
			m.Duration,
			m.FilePath,
			m.RootPath,
			m.PrimaryPath,
			m.NFOPath,
			m.PosterPath,
			m.BackdropPath,
			nullableParentID(m.ParentID),
			season,
			episode,
			m.MetadataSource,
			m.ProviderID,
			m.LibraryID,
			m.Status,
			m.CreatedAt,
			m.UpdatedAt,
			m.MPAA,
			m.Genres,
			m.Studios,
			m.Actors,
			m.UniqueIDs,
			m.Premiered,
			m.Outline,
			m.Tagline,
			m.Countries,
			m.Directors,
			m.Credits,
			m.Tags,
			m.SetName,
			m.SetOverview,
			m.VideoCodec,
			m.VideoWidth,
			m.VideoHeight,
			m.VideoDurationSeconds,
			m.AudioCodec,
			m.AudioChannels,
			m.SubtitleLanguages,
			m.Aired,
			m.LogoPath,
			m.Language,
			m.CountryCode,
			m.CustomRating,
			m.CollectionNumber,
			m.EndDate,
			m.ReleaseDate,
			m.DisplayOrder,
			m.OriginalTitle,
			m.UserRating,
			m.DateAdded,
			m.LastPlayed,
			m.Playcount,
		)

		return err
	})
}

// Update modifies an existing media item by ID.
// Only updates user-editable/metadata fields; does not touch status.
func (r *MediaRepository) Update(ctx context.Context, m *model.MediaItem) error {
	now := time.Now().UTC().Format(time.RFC3339)
	m.UpdatedAt = now

	normalizeMediaItemForWrite(m)

	season := 0
	if m.Season != nil {
		season = *m.Season
	}

	episode := 0
	if m.Episode != nil {
		episode = *m.Episode
	}

	return WithSQLiteBusyRetry(ctx, func() error {
		_, err := r.db.ExecContext(ctx, `UPDATE media_items SET
            title = ?, sort_title = ?, year = ?, overview = ?, rating = ?,
            duration = ?, file_path = ?, root_path = ?, primary_path = ?, nfo_path = ?, poster_path = ?, backdrop_path = ?,
            parent_id = ?, season = ?, episode = ?, metadata_source = ?,
            provider_id = ?, library_id = ?, updated_at = ?,
            mpaa = ?, genres = ?, studios = ?, actors = ?, unique_ids = ?,
            premiered = ?, outline = ?, tagline = ?, countries = ?, directors = ?,
            credits = ?, tags = ?, set_name = ?, set_overview = ?, video_codec = ?,
            video_width = ?, video_height = ?, video_duration_seconds = ?,
            audio_codec = ?, audio_channels = ?, subtitle_languages = ?,
            aired = ?, logo_path = ?, language = ?, country_code = ?,
            custom_rating = ?, collection_number = ?, end_date = ?, release_date = ?,
            display_order = ?, original_title = ?, user_rating = ?, date_added = ?,
            last_played = ?, playcount = ?
            WHERE id = ?`,
			m.Title,
			m.SortTitle,
			m.Year,
			m.Overview,
			m.Rating,
			m.Duration,
			m.FilePath,
			m.RootPath,
			m.PrimaryPath,
			m.NFOPath,
			m.PosterPath,
			m.BackdropPath,
			nullableParentID(m.ParentID),
			season,
			episode,
			m.MetadataSource,
			m.ProviderID,
			m.LibraryID,
			m.UpdatedAt,
			m.MPAA,
			m.Genres,
			m.Studios,
			m.Actors,
			m.UniqueIDs,
			m.Premiered,
			m.Outline,
			m.Tagline,
			m.Countries,
			m.Directors,
			m.Credits,
			m.Tags,
			m.SetName,
			m.SetOverview,
			m.VideoCodec,
			m.VideoWidth,
			m.VideoHeight,
			m.VideoDurationSeconds,
			m.AudioCodec,
			m.AudioChannels,
			m.SubtitleLanguages,
			m.Aired,
			m.LogoPath,
			m.Language,
			m.CountryCode,
			m.CustomRating,
			m.CollectionNumber,
			m.EndDate,
			m.ReleaseDate,
			m.DisplayOrder,
			m.OriginalTitle,
			m.UserRating,
			m.DateAdded,
			m.LastPlayed,
			m.Playcount,
			m.ID,
		)

		return err
	})
}

// FindExistingItem returns the ID of a media item matching the given
// library_id, file_path, and type. Returns empty string if not found.
func (r *MediaRepository) FindExistingItem(ctx context.Context, libraryID, filePath, mediaType string) (string, error) {
	var id string

	err := r.db.QueryRowContext(ctx,
		"SELECT id FROM media_items WHERE library_id = ? AND file_path = ? AND type = ?",
		libraryID,
		filePath,
		mediaType,
	).Scan(&id)

	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}

	if err != nil {
		return "", err
	}

	return id, nil
}

// FindByRootPath returns the ID of a media item matching the given
// library_id, root_path, and type. Returns empty string if not found.
// Used for show lookups where file_path is empty and root_path is the show directory.
func (r *MediaRepository) FindByRootPath(ctx context.Context, libraryID, rootPath, mediaType string) (string, error) {
	var id string

	err := r.db.QueryRowContext(ctx,
		"SELECT id FROM media_items WHERE library_id = ? AND root_path = ? AND type = ?",
		libraryID,
		rootPath,
		mediaType,
	).Scan(&id)

	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}

	if err != nil {
		return "", err
	}

	return id, nil
}

// Delete removes a media item by ID.
func (r *MediaRepository) Delete(ctx context.Context, id string) error {
	return WithSQLiteBusyRetry(ctx, func() error {
		_, err := r.db.ExecContext(ctx, "DELETE FROM media_items WHERE id = ?", id)
		return err
	})
}

// DeleteShowWithEpisodes deletes a show and all its episodes plus watch progress in a transaction.
func (r *MediaRepository) DeleteShowWithEpisodes(ctx context.Context, showID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, "DELETE FROM watch_progress WHERE media_item_id IN (SELECT id FROM media_items WHERE parent_id = ?)", showID); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, "DELETE FROM watch_progress WHERE media_item_id = ?", showID); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, "DELETE FROM media_items WHERE parent_id = ?", showID); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, "DELETE FROM media_items WHERE id = ?", showID); err != nil {
		return err
	}

	return tx.Commit()
}

// Count returns the total number of media items.
func (r *MediaRepository) Count(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM media_items").Scan(&count)
	return count, err
}

// GetEpisodesByShowID returns all episodes for a given show, sorted by season and episode.
func (r *MediaRepository) GetEpisodesByShowID(ctx context.Context, showID string) ([]model.MediaItem, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, type, title, season, episode, duration, overview, poster_path, backdrop_path, provider_id, library_id, status
        FROM media_items WHERE parent_id = ? AND type = 'episode'
        ORDER BY season ASC, episode ASC`, showID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var items []model.MediaItem

	for rows.Next() {
		var m model.MediaItem
		var season sql.NullInt64
		var episode sql.NullInt64

		if err := rows.Scan(
			&m.ID,
			&m.Type,
			&m.Title,
			&season,
			&episode,
			&m.Duration,
			&m.Overview,
			&m.PosterPath,
			&m.BackdropPath,
			&m.ProviderID,
			&m.LibraryID,
			&m.Status,
		); err != nil {
			return nil, err
		}

		m.Season = intPtrFromNullInt64(season)
		m.Episode = intPtrFromNullInt64(episode)

		items = append(items, m)
	}

	return items, rows.Err()
}
