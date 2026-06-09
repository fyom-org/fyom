package service

import (
	"context"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fyom/fyom/internal/model"
	"github.com/fyom/fyom/internal/repository"
	"github.com/fyom/fyom/pkg/errors"
	"github.com/google/uuid"
)

// Importer handles asynchronous NFO-based media library imports.
type Importer struct {
	mediaRepo *repository.MediaRepository
	jobRepo   *repository.ImportJobRepository
	db        *repository.DB
}

// NewImporter creates a new Importer.
func NewImporter(db *repository.DB, mediaRepo *repository.MediaRepository, jobRepo *repository.ImportJobRepository) *Importer {
	return &Importer{
		mediaRepo: mediaRepo,
		jobRepo:   jobRepo,
		db:        db,
	}
}

// ImportRequest triggers an asynchronous import.
// It creates a pending job record and returns the job ID immediately.
// The actual import runs in a goroutine.
func (imp *Importer) ImportRequest(ctx context.Context, sourcePath string) (*model.ImportJob, error) {
	// Validate path exists and is a directory
	info, err := os.Stat(sourcePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &errors.AppError{Code: 404, Message: "directory not found"}
		}
		return nil, errors.Wrap(err, errors.ErrInternal)
	}
	if !info.IsDir() {
		return nil, &errors.AppError{Code: 400, Message: "path is not a directory"}
	}

	// Create pending job
	job, err := imp.jobRepo.Create(ctx, sourcePath)
	if err != nil {
		return nil, errors.Wrap(err, errors.ErrInternal)
	}

	// Launch async worker
	go imp.runImport(job.ID, sourcePath)

	return job, nil
}

// runImport does the actual NFO parsing and DB insertion in a goroutine.
func (imp *Importer) runImport(jobID, sourcePath string) {
	ctx := context.Background()

	// Mark running
	_ = imp.jobRepo.UpdateProgress(ctx, jobID, 0, 0, "running")

	// Collect all .nfo files recursively
	var nfoFiles []string
	if err := filepath.Walk(sourcePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.ToLower(filepath.Ext(path)) == ".nfo" {
			nfoFiles = append(nfoFiles, path)
		}
		return nil
	}); err != nil {
		_ = imp.jobRepo.UpdateError(ctx, jobID, err.Error())
		return
	}

	total := len(nfoFiles)
	if total == 0 {
		_ = imp.jobRepo.UpdateProgress(ctx, jobID, 0, 0, "done")
		return
	}
	_ = imp.jobRepo.UpdateProgress(ctx, jobID, total, 0, "running")

	// Build a lookup of existing file paths for dedup
	existing, _ := imp.mediaRepo.List(ctx, "")
	existingPaths := make(map[string]bool)
	for _, item := range existing {
		existingPaths[item.FilePath] = true
	}

	// Track show titles -> media_item IDs for episode parent linking
	showTitleToID := make(map[string]string)

	done := 0
	var importErr error

	for _, nfoPath := range nfoFiles {
		items, err := imp.parseNFOFile(nfoPath, existingPaths, showTitleToID)
		if err != nil {
			importErr = err
			continue
		}

		for _, item := range items {
			// Resolve parent_id for episodes
			if item.Type == "episode" && item.ParentID == "" {
				// Try to find the show by looking at sibling NFOs or directory name
				showTitle := imp.guessShowTitle(nfoPath, item.Title)
				if sid, ok := showTitleToID[showTitle]; ok {
					item.ParentID = sid
				}
			}

			if err := imp.mediaRepo.Create(ctx, item); err != nil {
				importErr = err
				continue
			}

			// Track show titles for episode linking
			if item.Type == "show" {
				showTitleToID[item.Title] = item.ID
			}
		}

		done++
		_ = imp.jobRepo.UpdateProgress(ctx, jobID, total, done, "running")
	}

	if importErr != nil {
		_ = imp.jobRepo.UpdateError(ctx, jobID, importErr.Error())
	} else {
		_ = imp.jobRepo.UpdateProgress(ctx, jobID, total, done, "done")
	}
}

// parseNFOFile parses a single .nfo file and returns the media items it represents.
// An NFO can represent a movie, a TV show, or an episode.
func (imp *Importer) parseNFOFile(nfoPath string, existingPaths map[string]bool, showTitleToID map[string]string) ([]*model.MediaItem, error) {
	data, err := os.ReadFile(nfoPath)
	if err != nil {
		return nil, err
	}

	dir := filepath.Dir(nfoPath)
	baseName := strings.TrimSuffix(filepath.Base(nfoPath), filepath.Ext(nfoPath))

	// Try parsing as movie first
	var movie model.NFOMovie
	if err := xml.Unmarshal(data, &movie); err == nil && movie.Title != "" {
		return imp.buildMovieItems(nfoPath, dir, baseName, movie, existingPaths), nil
	}

	// Try parsing as episode
	var episode model.NFOEpisode
	if err := xml.Unmarshal(data, &episode); err == nil && episode.Title != "" {
		return imp.buildEpisodeItems(nfoPath, dir, baseName, episode, existingPaths, showTitleToID), nil
	}

	// Try parsing as TV show
	var tvshow model.NFOTVShow
	if err := xml.Unmarshal(data, &tvshow); err == nil && tvshow.Title != "" {
		return imp.buildTVShowItems(nfoPath, dir, baseName, tvshow, existingPaths), nil
	}

	return nil, nil // unrecognised NFO format — skip silently
}

// buildMovieItems creates MediaItem(s) from a parsed NFOMovie.
func (imp *Importer) buildMovieItems(_ string, dir, baseName string, movie model.NFOMovie, existingPaths map[string]bool) []*model.MediaItem {
	// Find the actual video file next to the NFO
	videoPath := imp.findVideoFile(dir, baseName)
	if videoPath == "" {
		return nil
	}
	if existingPaths[videoPath] {
		return nil
	}

	// Find poster
	posterPath := imp.findPoster(dir, baseName)

	title := movie.Title
	if title == "" {
		title = baseName
	}

	overview := movie.Overview
	if overview == "" {
		overview = movie.Plot
	}

	return []*model.MediaItem{{
		ID:             uuid.New().String(),
		Type:           "movie",
		Title:          title,
		SortTitle:      movie.SortTitle,
		Year:           movie.Year,
		Overview:       overview,
		Rating:         movie.Rating,
		Duration:       movie.Runtime * 60, // NFO runtime is in minutes
		FilePath:       videoPath,
		PosterPath:     posterPath,
		MetadataSource: "nfo",
		CreatedAt:      time.Now().UTC().Format(time.RFC3339),
		UpdatedAt:      time.Now().UTC().Format(time.RFC3339),
	}}
}

// buildEpisodeItems creates a MediaItem from a parsed NFOEpisode.
func (imp *Importer) buildEpisodeItems(_ string, dir, baseName string, ep model.NFOEpisode, existingPaths map[string]bool, showTitleToID map[string]string) []*model.MediaItem {
	videoPath := imp.findVideoFile(dir, baseName)
	if videoPath == "" {
		return nil
	}
	if existingPaths[videoPath] {
		return nil
	}

	posterPath := imp.findPoster(dir, baseName)

	title := ep.Title
	if title == "" {
		title = baseName
	}

	overview := ep.Overview
	if overview == "" {
		overview = ep.Plot
	}

	item := &model.MediaItem{
		ID:             uuid.New().String(),
		Type:           "episode",
		Title:          title,
		Year:           imp.parseYear(ep.FirstAired),
		Overview:       overview,
		Rating:         ep.Rating,
		Duration:       ep.Runtime * 60,
		FilePath:       videoPath,
		PosterPath:     posterPath,
		Season:         ep.Season,
		Episode:        ep.Episode,
		MetadataSource: "nfo",
		CreatedAt:      time.Now().UTC().Format(time.RFC3339),
		UpdatedAt:      time.Now().UTC().Format(time.RFC3339),
	}

	// Link to parent show
	if ep.ShowTitle != "" {
		if sid, ok := showTitleToID[ep.ShowTitle]; ok {
			item.ParentID = sid
		}
	}

	return []*model.MediaItem{item}
}

// buildTVShowItems creates a MediaItem from a parsed NFOTVShow.
func (imp *Importer) buildTVShowItems(_ string, dir, baseName string, show model.NFOTVShow, existingPaths map[string]bool) []*model.MediaItem {
	// TV show NFOs don't have a video file — they're metadata-only
	// We create a placeholder item with the directory as the "file_path"
	showPath := filepath.Join(dir, "tvshow.nfo")
	if existingPaths[showPath] {
		// Already imported — skip
		return nil
	}

	title := show.Title
	if title == "" {
		title = baseName
	}

	overview := show.Overview
	if overview == "" {
		overview = show.Plot
	}

	posterPath := imp.findPoster(dir, baseName)

	return []*model.MediaItem{{
		ID:             uuid.New().String(),
		Type:           "show",
		Title:          title,
		Overview:       overview,
		Rating:         show.Rating,
		FilePath:       showPath,
		PosterPath:     posterPath,
		MetadataSource: "nfo",
		CreatedAt:      time.Now().UTC().Format(time.RFC3339),
		UpdatedAt:      time.Now().UTC().Format(time.RFC3339),
	}}
}

// findVideoFile looks for a video file matching the NFO's base name in the same directory.
func (imp *Importer) findVideoFile(dir, baseName string) string {
	videoExts := []string{".mkv", ".mp4", ".avi", ".mov", ".wmv", ".flv", ".webm", ".m4v", ".ts", ".m2ts", ".vob", ".iso"}
	// Try exact match first
	for _, ext := range videoExts {
		candidate := filepath.Join(dir, baseName+ext)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	// Try case-insensitive match
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		stem := strings.TrimSuffix(name, ext)
		for _, vext := range videoExts {
			if ext == vext && strings.EqualFold(stem, baseName) {
				return filepath.Join(dir, name)
			}
		}
	}
	return ""
}

// findPoster looks for a poster image next to the NFO file.
func (imp *Importer) findPoster(dir, baseName string) string {
	posterNames := []string{
		"poster.jpg", "poster.png", "poster.jpeg",
		"folder.jpg", "folder.png",
		baseName + "-poster.jpg", baseName + "-poster.png",
		baseName + ".jpg", baseName + ".png",
		"cover.jpg", "cover.png",
	}
	for _, name := range posterNames {
		candidate := filepath.Join(dir, name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

// guessShowTitle attempts to determine the show title for an episode NFO.
func (imp *Importer) guessShowTitle(nfoPath, _ string) string {
	// The parent directory name is often the show title
	dir := filepath.Base(filepath.Dir(nfoPath))
	// Clean it up
	dir = strings.ReplaceAll(dir, ".", " ")
	dir = strings.ReplaceAll(dir, "_", " ")
	return strings.TrimSpace(dir)
}

// parseYear extracts a 4-digit year from a date string like "2024-03-15".
func (imp *Importer) parseYear(dateStr string) int {
	if len(dateStr) >= 4 {
		var y int
		_, err := strings.NewReader(dateStr[:4]).Read(make([]byte, 0))
		_ = err
		// Simple parse
		for _, c := range dateStr[:4] {
			if c < '0' || c > '9' {
				return 0
			}
		}
		y = int(dateStr[0]-'0')*1000 + int(dateStr[1]-'0')*100 + int(dateStr[2]-'0')*10 + int(dateStr[3]-'0')
		if y > 1900 && y < 2100 {
			return y
		}
	}
	return 0
}

// TODO: re-add path restriction when multi-user mode is implemented
