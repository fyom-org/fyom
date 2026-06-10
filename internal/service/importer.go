package service

import (
	"context"
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
	mediaRepo  *repository.MediaRepository
	jobRepo    *repository.ImportJobRepository
	db         *repository.DB
}

// NewImporter creates a new Importer.
func NewImporter(fs ImportFS, providerID string, db *repository.DB, mediaRepo *repository.MediaRepository, jobRepo *repository.ImportJobRepository) *Importer {
	return &Importer{
		fs:         fs,
		providerID: providerID,
		mediaRepo:  mediaRepo,
		jobRepo:    jobRepo,
		db:         db,
	}
}

// ImportRequest triggers an asynchronous import.
func (imp *Importer) ImportRequest(ctx context.Context, sourcePath string) (*model.ImportJob, error) {
	// For S3 imports, sourcePath is a prefix — it may not exist as a single
	// object. Check if we can list any entries under the prefix instead.
	if _, err := imp.fs.ReadDir(ctx, sourcePath); err != nil {
		return nil, &errors.AppError{Code: 404, Message: "directory not found"}
	}
	// Note: for local FS, ReadDir on a non-existent dir returns an error above.
	// For S3, an empty prefix listing is valid (import the whole bucket).

	job, err := imp.jobRepo.Create(ctx, sourcePath)
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

	// Phase 1: Count total video files for progress tracking.
	total := imp.countVideoFiles(ctx, sourcePath)
	if total == 0 {
		_ = imp.jobRepo.UpdateProgress(ctx, jobID, 0, 0, "done")
		return
	}
	_ = imp.jobRepo.UpdateProgress(ctx, jobID, total, 0, "running")

	// Phase 2: Build existing file path set for dedup.
	existing, _ := imp.mediaRepo.List(ctx, "")
	existingPaths := make(map[string]bool)
	for _, item := range existing {
		existingPaths[item.FilePath] = true
	}

	// Phase 3: Process each top-level subdirectory.
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

		// Check for tvshow.nfo — this is a TV show directory.
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

		// Check for any .nfo with <movie> root — this is a movie directory.
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

		// No NFO found — check for video files to treat as movie.
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
		ext := strings.ToLower(imp.fs.Join(path)) // get ext via string manipulation
		// We need the extension of the file name, not the joined path.
		// Use a simple approach: get the last dot in the name.
		name := entry.Name
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
// The callback receives the full path and the DirEntry.
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

// processShowDir handles a TV show directory containing tvshow.nfo.
// Returns the number of items created.
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
		// Use the last path component as title.
		title = baseName(showDirPath)
	}

	overview := showNFO.Overview
	if overview == "" {
		overview = showNFO.Plot
	}

	// Extract poster and backdrop from NFO.
	nfoPoster := extractNFOPosterThumb(showNFO.Thumbs)
	nfoBackdrop := extractNFOFanartPath(showNFO.Fanart)

	base := baseName(showDirPath)

	var posterPath, backdropPath string
	if _, ok := imp.fs.(*LocalImportFS); ok {
		posterPath = FindPosterPath(showDirPath, base, nfoPoster)
		backdropPath = FindBackdropPath(showDirPath, base, nfoBackdrop)
	} else {
		// For S3: use NFO thumb path directly (no disk-based file existence checks).
		posterPath = nfoPoster
		backdropPath = nfoBackdrop
	}

	// Create the show item.
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
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := imp.mediaRepo.Create(ctx, showItem); err != nil {
		return 0, err
	}

	// Scan season subdirectories and the show dir itself for episode files.
	created := 1 // the show itself

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

	// Collect video files.
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

		// Extract season/episode from filename.
		season, episode := extractEpisodeInfo(vf)

		// Look for matching episode NFO.
		episodeNFOPath := imp.fs.Join(dirPath, baseNameNoExt+".nfo")
		var epTitle, epOverview string
		var epRating float64
		var epRuntime int
		var nfoThumb string

		if imp.fs.Exists(ctx, episodeNFOPath) {
			if nf, err := imp.fs.Open(ctx, episodeNFOPath); err == nil {
				epNFO, err := ParseEpisodeNFO(nf)
				nf.Close()
				if err == nil {
					epTitle = epNFO.Title
					if epNFO.Overview != "" {
						epOverview = epNFO.Overview
					} else {
						epOverview = epNFO.Plot
					}
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

		// Find episode thumbnail.
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
			Duration:       epRuntime * 60, // minutes -> seconds
			FilePath:       videoPath,
			PosterPath:     thumbPath,
			ParentID:       showID,
			Season:         repository.IntPtr(season),
			Episode:        repository.IntPtr(episode),
			MetadataSource: "nfo",
			ProviderID:     imp.providerID,
			CreatedAt:      now,
			UpdatedAt:      now,
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

	overview := movieNFO.Overview
	if overview == "" {
		overview = movieNFO.Plot
	}

	// Find the video file.
	videoPath := imp.findVideoFileInDir(ctx, dirPath, title)
	if videoPath == "" {
		return 0, nil
	}
	if existingPaths[videoPath] {
		return 0, nil
	}

	// Poster and backdrop.
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
		Duration:       movieNFO.Runtime * 60, // minutes -> seconds
		FilePath:       videoPath,
		PosterPath:     posterPath,
		BackdropPath:   backdropPath,
		MetadataSource: "nfo",
		ProviderID:     imp.providerID,
		CreatedAt:      now,
		UpdatedAt:      now,
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

	// Use the first video file as the movie.
	videoPath := imp.fs.Join(dirPath, videoFiles[0])
	if existingPaths[videoPath] {
		return 0, nil
	}

	// Extract title from filename.
	title := trimExt(videoFiles[0])
	title = cleanTitle(title)

	// Look for poster.
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
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := imp.mediaRepo.Create(ctx, movieItem); err != nil {
		return 0, err
	}
	return 1, nil
}

// findMovieNFO looks for a .nfo file in dir that contains a <movie> root element.
// Returns the path to the NFO file, or "" if none found.
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
		// Prefer a file whose base name matches the title.
		baseNameNoExt := trimExt(entry.Name)
		if strings.EqualFold(baseNameNoExt, title) {
			return imp.fs.Join(dir, entry.Name)
		}
	}
	return firstVideo
}

// extractEpisodeInfo extracts season and episode numbers from a filename.
func extractEpisodeInfo(filename string) (season, episode int) {
	// Try S01E01 pattern first.
	if m := episodePattern.FindStringSubmatch(filename); m != nil {
		season, _ = strconv.Atoi(m[1])
		episode, _ = strconv.Atoi(m[2])
		return
	}
	// Try 1x01 pattern.
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
		if strings.ToLower(t.Aspect) == "poster" && t.Path != "" {
			return t.Path
		}
	}
	for _, t := range thumbs {
		if t.Path != "" {
			return t.Path
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
		if t.Path != "" {
			return t.Path
		}
	}
	return ""
}

// extractNFOEpisodeThumb extracts the thumb path from episode NFO thumb elements.
func extractNFOEpisodeThumb(thumbs []model.NFOThumb) string {
	for _, t := range thumbs {
		if t.Path != "" {
			return t.Path
		}
	}
	return ""
}

// baseName returns the last element of a path (the file or directory name).
// This replaces filepath.Base which is OS-dependent.
func baseName(path string) string {
	if path == "" {
		return ""
	}
	// Normalize to forward slashes for splitting.
	normalized := strings.ReplaceAll(path, "\\", "/")
	// Trim trailing slash.
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
// This replaces filepath.Ext + TrimSuffix patterns.
func trimExt(name string) string {
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		return name[:idx]
	}
	return name
}
