package service

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/fyom/fyom/internal/model"
	"github.com/fyom/fyom/internal/repository"
	"github.com/fyom/fyom/pkg/errors"
	"github.com/google/uuid"
)

// videoExtensions lists all recognized video file extensions.
var videoExtensions = map[string]bool{
	".mkv": true, ".mp4": true, ".avi": true, ".mov": true,
	".wmv": true, ".flv": true, ".webm": true, ".m4v": true,
	".ts": true, ".m2ts": true, ".vob": true, ".iso": true,
}

// episodePattern matches S01E01, s01e01, S0E1, S1E10, etc.
var episodePattern = regexp.MustCompile(`(?i)[Ss](\d{1,2})[Ee](\d{1,3})`)

// altEpisodePattern matches 1x01, 1X01, etc.
var altEpisodePattern = regexp.MustCompile(`(?i)(\d{1,2})[xX](\d{1,3})`)

// seasonDirPattern matches "Season 01", "season 1", "Season01", etc.
var seasonDirPattern = regexp.MustCompile(`(?i)^season\s*\d+$`)

// Importer handles asynchronous NFO-based media library imports.
type Importer struct {
	fs         ImportFS
	providerID string
	libraryID  string
	mediaRepo  *repository.MediaRepository
	jobRepo    *repository.ImportJobRepository
	db         *repository.DB
}

// NewImporter creates a new Importer.
func NewImporter(fs ImportFS, providerID string, db *repository.DB, mediaRepo *repository.MediaRepository, jobRepo *repository.ImportJobRepository) *Importer {
	return &Importer{
		fs:         fs,
		providerID: providerID,
		libraryID:  "default",
		mediaRepo:  mediaRepo,
		jobRepo:    jobRepo,
		db:         db,
	}
}

// SetLibraryID sets the library ID for imported items.
func (imp *Importer) SetLibraryID(id string) {
	if id != "" {
		imp.libraryID = id
	}
}

// ImportRequest triggers an asynchronous import.
func (imp *Importer) ImportRequest(ctx context.Context, sourcePath string) (*model.ImportJob, error) {
	if _, err := imp.fs.ReadDir(ctx, sourcePath); err != nil {
		return nil, &errors.AppError{Code: 404, Message: "directory not found"}
	}

	job, err := imp.jobRepo.Create(ctx, sourcePath, imp.libraryID)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrInternal)
	}

	go imp.runImport(job.ID, sourcePath)

	return job, nil
}

// runImport does the actual directory scanning and DB insertion in a goroutine.
func (imp *Importer) runImport(jobID, sourcePath string) {
	ctx := context.Background()

	_ = imp.jobRepo.UpdateProgress(ctx, jobID, 0, 0, "running")

	total := imp.countVideoFiles(ctx, sourcePath)
	if total == 0 {
		_ = imp.jobRepo.UpdateProgress(ctx, jobID, 0, 0, "done")
		return
	}
	_ = imp.jobRepo.UpdateProgress(ctx, jobID, total, 0, "running")

	existing, _ := imp.mediaRepo.List(ctx, "")
	existingPaths := make(map[string]bool)
	for _, item := range existing {
		existingPaths[item.FilePath] = true
	}

	entries, err := imp.fs.ReadDir(ctx, sourcePath)
	if err != nil {
		_ = imp.jobRepo.UpdateError(ctx, jobID, err.Error())
		return
	}

	done := 0
	var importErr error

	for _, entry := range entries {
		if !entry.IsDir {
			continue
		}
		dirPath := imp.fs.Join(sourcePath, entry.Name)

		tvshowNFOPath := imp.fs.Join(dirPath, "tvshow.nfo")
		if imp.fs.Exists(ctx, tvshowNFOPath) {
			n, err := imp.processShowDir(ctx, dirPath, existingPaths)
			if err != nil {
				importErr = err
			}
			done += n
			_ = imp.jobRepo.UpdateProgress(ctx, jobID, total, done, "running")
			continue
		}

		movieNFO := imp.findMovieNFO(ctx, dirPath)
		if movieNFO != "" {
			n, err := imp.processMovieDir(ctx, dirPath, movieNFO, existingPaths)
			if err != nil {
				importErr = err
			}
			done += n
			_ = imp.jobRepo.UpdateProgress(ctx, jobID, total, done, "running")
			continue
		}

		n, err := imp.processDirAsMovie(ctx, dirPath, existingPaths)
		if err != nil {
			importErr = err
		}
		done += n
		_ = imp.jobRepo.UpdateProgress(ctx, jobID, total, done, "running")
	}

	if importErr != nil {
		_ = imp.jobRepo.UpdateError(ctx, jobID, importErr.Error())
	} else {
		_ = imp.jobRepo.UpdateProgress(ctx, jobID, total, done, "done")
	}
}

// countVideoFiles counts all video files under sourcePath for progress tracking.
func (imp *Importer) countVideoFiles(ctx context.Context, sourcePath string) int {
	count := 0
	imp.walkDir(ctx, sourcePath, func(path string, entry DirEntry) {
		if entry.IsDir {
			return
		}
		name := entry.Name
		var ext string
		if idx := strings.LastIndex(name, "."); idx >= 0 {
			ext = strings.ToLower(name[idx:])
		} else {
			ext = ""
		}
		if videoExtensions[ext] {
			count++
		}
	})
	return count
}

// walkDir recursively walks the directory tree using imp.fs.ReadDir.
func (imp *Importer) walkDir(ctx context.Context, dir string, cb func(path string, entry DirEntry)) {
	entries, err := imp.fs.ReadDir(ctx, dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		fullPath := imp.fs.Join(dir, entry.Name)
		cb(fullPath, entry)
		if entry.IsDir {
			imp.walkDir(ctx, fullPath, cb)
		}
	}
}

// stringsToJSON serializes a string slice to JSON.
func stringsToJSON(ss []string) string {
	if len(ss) == 0 {
		return ""
	}
	b, _ := json.Marshal(ss)
	return string(b)
}

// actorsToJSON serializes NFOActor slice to a JSON array with type, sortorder, and thumb.
func actorsToJSON(actors []model.NFOActor) string {
	if len(actors) == 0 {
		return ""
	}
	type actorJSON struct {
		Name      string `json:"name"`
		Role      string `json:"role"`
		Type      string `json:"type"`
		SortOrder int    `json:"sort_order"`
		Thumb     string `json:"thumb,omitempty"`
	}
	out := make([]actorJSON, 0, len(actors))
	for _, a := range actors {
		if a.Name != "" {
			out = append(out, actorJSON{Name: a.Name, Role: a.Role, Type: a.Type, SortOrder: a.SortOrder, Thumb: a.Thumb})
		}
	}
	b, _ := json.Marshal(out)
	return string(b)
}

// hasUniqueID checks if a NFOUniqueID slice already contains the given type.
func hasUniqueID(ids []model.NFOUniqueID, idType string) bool {
	for _, id := range ids {
		if id.Type == idType {
			return true
		}
	}
	return false
}

// uniqueIDsToJSON serializes NFOUniqueID slice to a map.
func uniqueIDsToJSON(ids []model.NFOUniqueID) string {
	if len(ids) == 0 {
		return ""
	}
	m := make(map[string]string)
	for _, id := range ids {
		if id.Type != "" && id.Value != "" {
			m[id.Type] = id.Value
		}
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// subtitlesToJSON serializes NFOSubtitle slice to a language array.
func subtitlesToJSON(subs []model.NFOSubtitle) string {
	if len(subs) == 0 {
		return ""
	}
	var langs []string
	for _, s := range subs {
		if s.Language != "" {
			langs = append(langs, s.Language)
		}
	}
	return stringsToJSON(langs)
}

// applyMovieNFOFields populates enhanced fields from parsed NFO onto a MediaItem.
// Aligned with Jellyfin NFO spec for full movie metadata support.
func applyMovieNFOFields(item *model.MediaItem, nfo *model.NFOMovie) {
	if nfo.Title != "" {
		item.Title = nfo.Title
	}
	item.MPAA = nfo.MPAA
	item.Genres = stringsToJSON(nfo.Genres)
	item.Studios = stringsToJSON(nfo.Studios)
	item.Actors = actorsToJSON(nfo.Actors)

	// Build uniqueIDs from new-format <uniqueid> elements
	ids := make([]model.NFOUniqueID, len(nfo.UniqueIDs))
	copy(ids, nfo.UniqueIDs)

	// Merge old-format ID fields if not already present
	if nfo.ImdbID != "" && !hasUniqueID(ids, "imdb") {
		ids = append(ids, model.NFOUniqueID{Type: "imdb", Value: nfo.ImdbID})
	}
	if nfo.TmdbID != "" && !hasUniqueID(ids, "tmdb") {
		ids = append(ids, model.NFOUniqueID{Type: "tmdb", Value: nfo.TmdbID})
	}
	if nfo.TvdbID != "" && !hasUniqueID(ids, "tvdb") {
		ids = append(ids, model.NFOUniqueID{Type: "tvdb", Value: nfo.TvdbID})
	}
	if nfo.LegacyID != "" && !hasUniqueID(ids, "tvdb") {
		ids = append(ids, model.NFOUniqueID{Type: "tvdb", Value: nfo.LegacyID})
	}
	item.UniqueIDs = uniqueIDsToJSON(ids)
	// Title metadata
	if nfo.OriginalTitle != "" {
		item.OriginalTitle = nfo.OriginalTitle
	}
	if nfo.SortTitle != "" {
		item.SortTitle = nfo.SortTitle
	} else if nfo.SortName != "" {
		item.SortTitle = nfo.SortName
	}

	// Release dates
	item.Premiered = nfo.Premiered
	item.ReleaseDate = nfo.ReleaseDate
	item.EndDate = nfo.EndDate

	// Plot and tagline
	item.Outline = nfo.Outline
	item.Tagline = nfo.Tagline

	// Country and language
	item.Countries = stringsToJSON(nfo.Countries)
	item.CountryCode = nfo.CountryCode
	item.Language = nfo.Language

	// Crew
	item.Directors = stringsToJSON(nfo.Directors)
	item.Credits = stringsToJSON(nfo.Credits)
	item.Tags = stringsToJSON(nfo.Tags)

	// Collection / set
	if nfo.Set != nil {
		item.SetName = nfo.Set.Name
		item.SetOverview = nfo.Set.Overview
	}
	item.CollectionNumber = nfo.CollectionNumber

	// Ratings
	item.CustomRating = nfo.CustomRating
	item.UserRating = nfo.UserRating

	// Playback state
	item.LastPlayed = nfo.LastPlayed
	item.Playcount = nfo.Playcount

	// Date added
	item.DateAdded = nfo.DateAdded

	// Display order
	item.DisplayOrder = nfo.DisplayOrder

	// Stream details
	item.VideoCodec = nfo.FileInfo.StreamDetails.Video.Codec
	item.VideoWidth = nfo.FileInfo.StreamDetails.Video.Width
	item.VideoHeight = nfo.FileInfo.StreamDetails.Video.Height
	item.VideoDurationSeconds = nfo.FileInfo.StreamDetails.Video.DurationInSeconds
	if len(nfo.FileInfo.StreamDetails.Audios) > 0 {
		item.AudioCodec = nfo.FileInfo.StreamDetails.Audios[0].Codec
		item.AudioChannels = nfo.FileInfo.StreamDetails.Audios[0].Channels
	}
	item.SubtitleLanguages = subtitlesToJSON(nfo.FileInfo.StreamDetails.Subtitles)

	// Use premiered for year if year is 0
	if item.Year == 0 && nfo.Premiered != "" && len(nfo.Premiered) >= 4 {
		if y, err := strconv.Atoi(nfo.Premiered[:4]); err == nil {
			item.Year = y
		}
	}

	// Use FileInfo duration if runtime is 0
	if item.Duration == 0 && nfo.FileInfo.StreamDetails.Video.DurationInSeconds > 0 {
		item.Duration = nfo.FileInfo.StreamDetails.Video.DurationInSeconds
	}

	// Use best rating from ratings block if legacy rating is 0
	if item.Rating == 0 && len(nfo.Ratings.Rating) > 0 {
		for _, r := range nfo.Ratings.Rating {
			if r.Value > 0 {
				item.Rating = r.Value
				break
			}
		}
	}
}

// applyShowNFOFields populates enhanced fields from parsed NFO onto a MediaItem (show).
func applyShowNFOFields(item *model.MediaItem, nfo *model.NFOTVShow) {
	if nfo.Title != "" {
		item.Title = nfo.Title
	}
	item.Genres = stringsToJSON(nfo.Genres)
	item.Studios = stringsToJSON(nfo.Studios)
	item.Actors = actorsToJSON(nfo.Actors)

	// Build uniqueIDs from new-format <uniqueid> elements
	ids := make([]model.NFOUniqueID, len(nfo.UniqueIDs))
	copy(ids, nfo.UniqueIDs)

	// Merge old-format ID fields if not already present
	if nfo.ImdbID != "" && !hasUniqueID(ids, "imdb") {
		ids = append(ids, model.NFOUniqueID{Type: "imdb", Value: nfo.ImdbID})
	}
	if nfo.TmdbID != "" && !hasUniqueID(ids, "tmdb") {
		ids = append(ids, model.NFOUniqueID{Type: "tmdb", Value: nfo.TmdbID})
	}
	if nfo.TvdbID != "" && !hasUniqueID(ids, "tvdb") {
		ids = append(ids, model.NFOUniqueID{Type: "tvdb", Value: nfo.TvdbID})
	}
	if nfo.LegacyID != "" && !hasUniqueID(ids, "tvdb") {
		ids = append(ids, model.NFOUniqueID{Type: "tvdb", Value: nfo.LegacyID})
	}
	item.UniqueIDs = uniqueIDsToJSON(ids)

	item.Premiered = nfo.Premiered
	item.Outline = nfo.Outline
	item.Tags = stringsToJSON(nfo.Tags)
	item.MPAA = nfo.MPAA

	if nfo.Status != "" {
		// passthrough — stored in a dedicated column if present,
		// but for now it's informational only.
	}

	if item.Year == 0 && nfo.Premiered != "" && len(nfo.Premiered) >= 4 {
		if y, err := strconv.Atoi(nfo.Premiered[:4]); err == nil {
			item.Year = y
		}
	}

	if item.Rating == 0 && len(nfo.Ratings.Rating) > 0 {
		for _, r := range nfo.Ratings.Rating {
			if r.Value > 0 {
				item.Rating = r.Value
				break
			}
		}
	}
}

// applyEpisodeNFOFields populates enhanced fields from parsed NFO onto a MediaItem (episode).
func applyEpisodeNFOFields(item *model.MediaItem, nfoEp *model.NFOEpisode) {
	if nfoEp.Title != "" {
		item.Title = nfoEp.Title
	}
	item.MPAA = nfoEp.MPAA
	item.Genres = stringsToJSON(nfoEp.Genres)
	item.Studios = stringsToJSON(nfoEp.Studios)
	item.Actors = actorsToJSON(nfoEp.Actors)
	item.UniqueIDs = uniqueIDsToJSON(nfoEp.UniqueIDs)
	item.Outline = nfoEp.Outline
	item.Premiered = nfoEp.Premiered
	item.Directors = stringsToJSON(nfoEp.Directors)
	item.Credits = stringsToJSON(nfoEp.Credits)
	item.VideoCodec = nfoEp.FileInfo.StreamDetails.Video.Codec
	item.VideoWidth = nfoEp.FileInfo.StreamDetails.Video.Width
	item.VideoHeight = nfoEp.FileInfo.StreamDetails.Video.Height
	if len(nfoEp.FileInfo.StreamDetails.Audios) > 0 {
		item.AudioCodec = nfoEp.FileInfo.StreamDetails.Audios[0].Codec
		item.AudioChannels = nfoEp.FileInfo.StreamDetails.Audios[0].Channels
	}
	item.SubtitleLanguages = subtitlesToJSON(nfoEp.FileInfo.StreamDetails.Subtitles)
	item.Aired = nfoEp.Aired
	if nfoEp.Plot != "" {
		item.Overview = nfoEp.Plot
	}

	if item.Rating == 0 && len(nfoEp.Ratings.Rating) > 0 {
		for _, r := range nfoEp.Ratings.Rating {
			if r.Value > 0 {
				item.Rating = r.Value
				break
			}
		}
	}
}

// processShowDir handles a TV show directory containing tvshow.nfo.
func (imp *Importer) processShowDir(ctx context.Context, showDirPath string, existingPaths map[string]bool) (int, error) {
	showNFOPath := imp.fs.Join(showDirPath, "tvshow.nfo")

	f, err := imp.fs.Open(ctx, showNFOPath)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	showNFO, err := ParseShowNFO(f)
	if err != nil {
		return 0, err
	}

	title := showNFO.Title
	if title == "" {
		title = baseName(showDirPath)
	}

	overview := showNFO.Plot

	nfoPoster := extractNFOPosterThumb(showNFO.Thumbs)
	nfoBackdrop := extractNFOFanartPath(showNFO.Fanart)

	base := baseName(showDirPath)

	var posterPath, backdropPath string
	if _, ok := imp.fs.(*LocalImportFS); ok {
		posterPath = FindPosterPath(showDirPath, base, nfoPoster)
		backdropPath = FindBackdropPath(showDirPath, base, nfoBackdrop)
	} else {
		posterPath = nfoPoster
		backdropPath = nfoBackdrop
	}

	showID := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	showItem := &model.MediaItem{
		ID:             showID,
		Type:           "show",
		Title:          title,
		Year:           showNFO.Year,
		Overview:       overview,
		Rating:         showNFO.Rating,
		Duration:       showNFO.Runtime * 60,
		FilePath:       showDirPath,
		PosterPath:     posterPath,
		BackdropPath:   backdropPath,
		MetadataSource: "nfo",
		ProviderID:     imp.providerID,
		LibraryID:      imp.libraryID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	applyShowNFOFields(showItem, showNFO)

	// Find logo.png
	if logoPath := FindLogoPath(showDirPath); logoPath != "" {
		showItem.LogoPath = logoPath
	}

	if err := imp.mediaRepo.Create(ctx, showItem); err != nil {
		return 0, err
	}

	created := 1

	entries, err := imp.fs.ReadDir(ctx, showDirPath)
	if err != nil {
		return created, nil
	}

	for _, entry := range entries {
		if !entry.IsDir {
			continue
		}
		seasonDirPath := imp.fs.Join(showDirPath, entry.Name)

		n, err := imp.processEpisodeFilesInDir(ctx, seasonDirPath, showID, existingPaths)
		if err != nil {
			return created, err
		}
		created += n
	}

	return created, nil
}

// processEpisodeFilesInDir scans a directory for video files and creates episode items.
func (imp *Importer) processEpisodeFilesInDir(ctx context.Context, dirPath, showID string, existingPaths map[string]bool) (int, error) {
	entries, err := imp.fs.ReadDir(ctx, dirPath)
	if err != nil {
		return 0, nil
	}

	var videoFiles []string
	for _, entry := range entries {
		if entry.IsDir {
			continue
		}
		name := entry.Name
		var ext string
		if idx := strings.LastIndex(name, "."); idx >= 0 {
			ext = strings.ToLower(name[idx:])
		}
		if videoExtensions[ext] {
			videoFiles = append(videoFiles, entry.Name)
		}
	}

	created := 0
	for _, vf := range videoFiles {
		videoPath := imp.fs.Join(dirPath, vf)
		if existingPaths[videoPath] {
			continue
		}

		baseNameNoExt := trimExt(vf)
		season, episode := extractEpisodeInfo(vf)

		episodeNFOPath := imp.fs.Join(dirPath, baseNameNoExt+".nfo")
		var epTitle, epOverview string
		var epRating float64
		var epRuntime int
		var nfoThumb string

		if imp.fs.Exists(ctx, episodeNFOPath) {
			if nf, err := imp.fs.Open(ctx, episodeNFOPath); err == nil {
				episodes, err := ParseEpisodeNFOs(nf)
				nf.Close()
				if err == nil && len(episodes) > 0 {
					epNFO := episodes[0]
					epTitle = epNFO.Title
					epOverview = epNFO.Plot
					epRating = epNFO.Rating
					epRuntime = epNFO.Runtime
					nfoThumb = extractNFOEpisodeThumb(epNFO.Thumbs)
					if epNFO.Season > 0 {
						season = epNFO.Season
					}
					if epNFO.Episode > 0 {
						episode = epNFO.Episode
					}
				}
			}
		}

		if epTitle == "" {
			epTitle = baseNameNoExt
		}

		var thumbPath string
		if _, ok := imp.fs.(*LocalImportFS); ok {
			thumbPath = FindEpisodeThumbnailPath(dirPath, baseNameNoExt, nfoThumb)
		} else {
			thumbPath = nfoThumb
		}

		now := time.Now().UTC().Format(time.RFC3339)
		epItem := &model.MediaItem{
			ID:             uuid.New().String(),
			Type:           "episode",
			Title:          epTitle,
			Overview:       epOverview,
			Rating:         epRating,
			Duration:       epRuntime * 60,
			FilePath:       videoPath,
			PosterPath:     thumbPath,
			BackdropPath:   thumbPath,
			ParentID:       showID,
			Season:         repository.IntPtr(season),
			Episode:        repository.IntPtr(episode),
			MetadataSource: "nfo",
			ProviderID:     imp.providerID,
			LibraryID:      imp.libraryID,
			CreatedAt:      now,
			UpdatedAt:      now,
		}

		// Apply enhanced fields from NFO if available
		if imp.fs.Exists(ctx, episodeNFOPath) {
			if nf, err := imp.fs.Open(ctx, episodeNFOPath); err == nil {
				episodes, err := ParseEpisodeNFOs(nf)
				nf.Close()
				if err == nil && len(episodes) > 0 {
					applyEpisodeNFOFields(epItem, &episodes[0])
				}
			}
		}

		if err := imp.mediaRepo.Create(ctx, epItem); err != nil {
			return created, err
		}
		created++
	}

	return created, nil
}

// processMovieDir handles a directory with a movie NFO file.
func (imp *Importer) processMovieDir(ctx context.Context, dirPath, nfoPath string, existingPaths map[string]bool) (int, error) {
	f, err := imp.fs.Open(ctx, nfoPath)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	movieNFO, err := ParseMovieNFO(f)
	if err != nil {
		return 0, err
	}

	title := movieNFO.Title
	if title == "" {
		title = trimExt(baseName(nfoPath))
	}

	overview := movieNFO.Plot

	videoPath := imp.findVideoFileInDir(ctx, dirPath, title)
	if videoPath == "" {
		return 0, nil
	}
	if existingPaths[videoPath] {
		return 0, nil
	}

	nfoPoster := extractNFOPosterThumb(movieNFO.Thumbs)
	nfoBackdrop := extractNFOFanartPath(movieNFO.Fanart)
	base := trimExt(baseName(nfoPath))

	var posterPath, backdropPath string
	if _, ok := imp.fs.(*LocalImportFS); ok {
		posterPath = FindPosterPath(dirPath, base, nfoPoster)
		backdropPath = FindBackdropPath(dirPath, base, nfoBackdrop)
	} else {
		posterPath = nfoPoster
		backdropPath = nfoBackdrop
	}

	now := time.Now().UTC().Format(time.RFC3339)
	movieItem := &model.MediaItem{
		ID:             uuid.New().String(),
		Type:           "movie",
		Title:          title,
		SortTitle:      movieNFO.SortTitle,
		Year:           movieNFO.Year,
		Overview:       overview,
		Rating:         movieNFO.Rating,
		Duration:       movieNFO.Runtime * 60,
		FilePath:       videoPath,
		PosterPath:     posterPath,
		BackdropPath:   backdropPath,
		MetadataSource: "nfo",
		ProviderID:     imp.providerID,
		LibraryID:      imp.libraryID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	applyMovieNFOFields(movieItem, movieNFO)
	// Find logo.png
	if logoPath := FindLogoPath(dirPath); logoPath != "" {
		movieItem.LogoPath = logoPath
	}

	if err := imp.mediaRepo.Create(ctx, movieItem); err != nil {
		return 0, err
	}
	return 1, nil
}

// processDirAsMovie handles a directory with video files but no NFO.
func (imp *Importer) processDirAsMovie(ctx context.Context, dirPath string, existingPaths map[string]bool) (int, error) {
	entries, err := imp.fs.ReadDir(ctx, dirPath)
	if err != nil {
		return 0, nil
	}

	var videoFiles []string
	for _, entry := range entries {
		if entry.IsDir {
			continue
		}
		name := entry.Name
		var ext string
		if idx := strings.LastIndex(name, "."); idx >= 0 {
			ext = strings.ToLower(name[idx:])
		}
		if videoExtensions[ext] {
			videoFiles = append(videoFiles, entry.Name)
		}
	}

	if len(videoFiles) == 0 {
		return 0, nil
	}

	videoPath := imp.fs.Join(dirPath, videoFiles[0])
	if existingPaths[videoPath] {
		return 0, nil
	}

	title := trimExt(videoFiles[0])
	title = cleanTitle(title)

	base := trimExt(videoFiles[0])
	var posterPath string
	if _, ok := imp.fs.(*LocalImportFS); ok {
		posterPath = FindPosterPath(dirPath, base, "")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	movieItem := &model.MediaItem{
		ID:             uuid.New().String(),
		Type:           "movie",
		Title:          title,
		FilePath:       videoPath,
		PosterPath:     posterPath,
		MetadataSource: "filename",
		ProviderID:     imp.providerID,
		LibraryID:      imp.libraryID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := imp.mediaRepo.Create(ctx, movieItem); err != nil {
		return 0, err
	}
	return 1, nil
}

// findMovieNFO looks for a .nfo file in dir that contains a <movie> root element.
func (imp *Importer) findMovieNFO(ctx context.Context, dir string) string {
	entries, err := imp.fs.ReadDir(ctx, dir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir {
			continue
		}
		name := entry.Name
		var ext string
		if idx := strings.LastIndex(name, "."); idx >= 0 {
			ext = strings.ToLower(name[idx:])
		}
		if ext != ".nfo" {
			continue
		}
		path := imp.fs.Join(dir, entry.Name)
		f, err := imp.fs.Open(ctx, path)
		if err != nil {
			continue
		}
		var movie model.NFOMovie
		err = xml.NewDecoder(f).Decode(&movie)
		f.Close()
		if err == nil && movie.Title != "" {
			return path
		}
	}
	return ""
}

// findVideoFileInDir finds a video file in the directory, preferring one matching the title.
func (imp *Importer) findVideoFileInDir(ctx context.Context, dir, title string) string {
	entries, err := imp.fs.ReadDir(ctx, dir)
	if err != nil {
		return ""
	}

	var firstVideo string
	for _, entry := range entries {
		if entry.IsDir {
			continue
		}
		name := entry.Name
		var ext string
		if idx := strings.LastIndex(name, "."); idx >= 0 {
			ext = strings.ToLower(name[idx:])
		}
		if !videoExtensions[ext] {
			continue
		}
		if firstVideo == "" {
			firstVideo = imp.fs.Join(dir, entry.Name)
		}
		baseNameNoExt := trimExt(entry.Name)
		if strings.EqualFold(baseNameNoExt, title) {
			return imp.fs.Join(dir, entry.Name)
		}
	}
	return firstVideo
}

// extractEpisodeInfo extracts season and episode numbers from a filename.
func extractEpisodeInfo(filename string) (season, episode int) {
	if m := episodePattern.FindStringSubmatch(filename); m != nil {
		season, _ = strconv.Atoi(m[1])
		episode, _ = strconv.Atoi(m[2])
		return
	}
	if m := altEpisodePattern.FindStringSubmatch(filename); m != nil {
		season, _ = strconv.Atoi(m[1])
		episode, _ = strconv.Atoi(m[2])
		return
	}
	return 0, 0
}

// cleanTitle cleans up a filename-derived title.
func cleanTitle(s string) string {
	s = strings.ReplaceAll(s, ".", " ")
	s = strings.ReplaceAll(s, "_", " ")
	return strings.TrimSpace(s)
}

// extractNFOPosterThumb extracts the poster path from NFO thumb elements.
// aspect="poster" is preferred; otherwise the first thumb is used.
func extractNFOPosterThumb(thumbs []model.NFOThumb) string {
	for _, t := range thumbs {
		if strings.ToLower(t.Aspect) == "poster" && t.URL != "" {
			return t.URL
		}
	}
	for _, t := range thumbs {
		if t.URL != "" {
			return t.URL
		}
	}
	return ""
}

// extractNFOFanartPath extracts the backdrop path from NFO fanart elements.
func extractNFOFanartPath(fanart *model.NFOFanart) string {
	if fanart == nil {
		return ""
	}
	for _, t := range fanart.Thumbs {
		if t.URL != "" {
			return t.URL
		}
	}
	return ""
}

// extractNFOEpisodeThumb extracts the thumb path from episode NFO thumb elements.
func extractNFOEpisodeThumb(thumbs []model.NFOThumb) string {
	for _, t := range thumbs {
		if t.URL != "" {
			return t.URL
		}
	}
	return ""
}

// baseName returns the last element of a path (the file or directory name).
func baseName(path string) string {
	if path == "" {
		return ""
	}
	normalized := strings.ReplaceAll(path, "\\", "/")
	normalized = strings.TrimRight(normalized, "/")
	if normalized == "" {
		return ""
	}
	idx := strings.LastIndex(normalized, "/")
	if idx < 0 {
		return normalized
	}
	return normalized[idx+1:]
}

// trimExt removes the extension from a filename.
func trimExt(name string) string {
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		return name[:idx]
	}
	return name
}
