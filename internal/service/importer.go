package service

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"path/filepath"
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
	fs           ImportFS
	providerID   string
	libraryID    string
	libraryType  string
	mediaRepo    *repository.MediaRepository
	jobRepo      *repository.ImportJobRepository
	db           *repository.DB
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

// ImportLibrary synchronously imports all media items from the library's
// source directory. Returns an ImportSummary with counts and warnings.
// This is the primary import entry point for admin-triggered scans.
func (imp *Importer) ImportLibrary(ctx context.Context, libraryID string) (*model.ImportSummary, error) {
	imp.SetLibraryID(libraryID)

	// Look up the library to get its source path and type.
	var sourcePath string
	var libType string
	err := imp.db.QueryRowContext(ctx,
		"SELECT source_path, type FROM libraries WHERE id = ?", libraryID,
	).Scan(&sourcePath, &libType)
	if err != nil {
		return nil, &errors.AppError{Code: 404, Message: "library not found"}
	}
	imp.libraryType = libType

	summary := &model.ImportSummary{}
	startTime := time.Now()

	// Stage 1: Build filesystem snapshot
	rootNode, err := imp.buildSnapshot(ctx, sourcePath)
	if err != nil {
		return nil, err
	}

	// Stage 2: Classify nodes into typed candidates
	walkCtx := &WalkContext{
		ScanID:        uuid.New().String(),
		LibraryID:     libraryID,
		SourceRoot:    sourcePath,
		ClaimedPaths:  make(map[string]PathClaim),
	}
	classResult := imp.classifyTree(ctx, rootNode, walkCtx)

	// Stage 3: Parse metadata by candidate type
	imp.parseCandidateMetadata(ctx, classResult.Candidates)

	// Stage 4: Reconcile candidates into DB writes
	reconResult, err := imp.reconcileCandidates(ctx, classResult.Candidates)
	if err != nil {
		summary.ParseWarnings = append(summary.ParseWarnings, fmt.Sprintf("reconcile error: %v", err))
	}

	// Build summary
	summary.ImportedItems = 0
	for _, r := range reconResult.Accepted {
		if r.Action == ReconcileCreate {
			summary.ImportedItems++
		}
	}
	summary.UpdatedItems = 0
	for _, r := range reconResult.Accepted {
		if r.Action == ReconcileUpdate {
			summary.UpdatedItems++
		}
	}
	summary.ScannedFiles = classResult.ScannedDirs
	summary.SkippedFiles = len(reconResult.Rejected)
	summary.ParseWarnings = append(summary.ParseWarnings, imp.buildRejectWarnings(reconResult.Rejected)...)
	summary.Duration = time.Since(startTime)

	return summary, nil
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

	// Stage 1: Build filesystem snapshot
	rootNode, err := imp.buildSnapshot(ctx, sourcePath)
	if err != nil {
		_ = imp.jobRepo.UpdateError(ctx, jobID, fmt.Sprintf("snapshot error: %v", err))
		return
	}

	// Stage 2: Classify nodes into typed candidates
	walkCtx := &WalkContext{
		ScanID:       uuid.New().String(),
		LibraryID:    imp.libraryID,
		SourceRoot:   sourcePath,
		ClaimedPaths: make(map[string]PathClaim),
	}
	classResult := imp.classifyTree(ctx, rootNode, walkCtx)

	// Stage 3: Parse metadata by candidate type
	imp.parseCandidateMetadata(ctx, classResult.Candidates)

	// Set total_items to final candidate count
	total := len(classResult.Candidates)
	_ = imp.jobRepo.UpdateProgress(ctx, jobID, total, 0, "running")

	// Stage 4: Reconcile candidates into DB writes
	reconResult, err := imp.reconcileCandidates(ctx, classResult.Candidates)
	if err != nil {
		_ = imp.jobRepo.UpdateError(ctx, jobID, fmt.Sprintf("reconcile error: %v", err))
		return
	}

	// done_items = number of candidates that reached a terminal state
	done := reconResult.DoneCandidates
	_ = imp.jobRepo.UpdateProgress(ctx, jobID, total, done, "done")
}

// ── Stage 1: Filesystem Snapshot ──────────────────────────────────────────

// buildSnapshot recursively reads the directory tree starting at rootPath
// and returns a tree of FSNode objects.
func (imp *Importer) buildSnapshot(ctx context.Context, rootPath string) (*FSNode, error) {
	return imp.buildSnapshotRecursive(ctx, rootPath)
}

func (imp *Importer) buildSnapshotRecursive(ctx context.Context, dirPath string) (*FSNode, error) {
	entries, err := imp.fs.ReadDir(ctx, dirPath)
	if err != nil {
		return nil, err
	}

	node := &FSNode{
		Path:     dirPath,
		Name:     baseName(dirPath),
		Kind:     ImportNodeDir,
		IsDir:    true,
		Children: make([]*FSNode, 0, len(entries)),
	}

	for _, entry := range entries {
		fullPath := imp.fs.Join(dirPath, entry.Name)

		if entry.IsDir {
			childNode, err := imp.buildSnapshotRecursive(ctx, fullPath)
			if err != nil {
				// Skip directories we can't read
				continue
			}
			node.Children = append(node.Children, childNode)
		} else {
			nodeKind := imp.classifyNodeFile(entry.Name)
			fileNode := &FSNode{
				Path:  fullPath,
				Name:  entry.Name,
				Kind:  nodeKind,
				IsDir: false,
			}
			node.Children = append(node.Children, fileNode)
		}
	}

	return node, nil
}

// classifyNodeFile determines the ImportNodeKind for a file based on its extension.
func (imp *Importer) classifyNodeFile(name string) ImportNodeKind {
	ext := ""
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		ext = strings.ToLower(name[idx:])
	}

	switch ext {
	case ".nfo":
		return ImportNodeNFO
	case ".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp":
		return ImportNodeImage
	case ".srt", ".sub", ".ass", ".ssa", ".vtt":
		return ImportNodeSubtitle
	default:
		if videoExtensions[ext] {
			return ImportNodeVideo
		}
		return ImportNodeOther
	}
}

// ── Stage 2: Classification ───────────────────────────────────────────────

// classifyTree walks the FSNode tree and produces ImportCandidate objects.
// It uses WalkContext to propagate parent semantics (show root, season number)
// and to track claimed paths so no file is claimed by multiple candidates.
func (imp *Importer) classifyTree(ctx context.Context, node *FSNode, walkCtx *WalkContext) *ClassificationResult {
	result := &ClassificationResult{
		Candidates:  make([]ImportCandidate, 0),
		Rejected:    make([]RejectedItem, 0),
		Unknown:     make([]ImportCandidate, 0),
		ScannedDirs: 0,
	}

	walkCtx.CurrentPath = node.Path
	walkCtx.Depth++

	if !node.IsDir {
		// Leaf file nodes are not classified directly;
		// they are collected by their parent directory classifier.
		return result
	}

	// Count this directory as scanned
	result.ScannedDirs++

	// Check if this directory is a show root (contains tvshow.nfo)
	tvshowNFOPath := imp.fs.Join(node.Path, "tvshow.nfo")
	if imp.fs.Exists(ctx, tvshowNFOPath) {
		imp.classifyShowDir(ctx, node, walkCtx, result)
		return result
	}

	// Check if this directory contains a movie NFO
	movieNFOPath := imp.findMovieNFO(ctx, node.Path)
	if movieNFOPath != "" {
		imp.classifyMovieDir(ctx, node, movieNFOPath, walkCtx, result)
		return result
	}

	// If we're inside a show context, look for season directories and episode files
	if walkCtx.ParentKind == ImportEntityShow || walkCtx.ParentKind == ImportEntitySeason {
		// Check if this is a season directory
		if seasonDirPattern.MatchString(node.Name) {
			imp.classifySeasonDir(ctx, node, walkCtx, result)
			return result
		}
		// If we're inside a season, look for episode files
		if walkCtx.ParentKind == ImportEntitySeason {
			imp.classifyEpisodeDir(ctx, node, walkCtx, result)
			return result
		}
	}

	// Fallback: if this directory contains video files and is not inside a show,
	// classify it as a movie with filename-derived metadata.
	// This handles the case where a directory has a malformed NFO or no NFO.
	if walkCtx.ParentKind == ImportEntityUnknown || walkCtx.ParentKind == "" {
		hasVideo := false
		for _, child := range node.Children {
			if !child.IsDir && child.Kind == ImportNodeVideo {
				hasVideo = true
				break
			}
		}
		if hasVideo {
			imp.classifyMovieDirNoNFO(ctx, node, walkCtx, result)
			return result
		}
	}

	// For each child directory, recurse
	for _, child := range node.Children {
		if !child.IsDir {
			continue
		}
		childResult := imp.classifyTree(ctx, child, walkCtx)
		result.Candidates = append(result.Candidates, childResult.Candidates...)
		result.Rejected = append(result.Rejected, childResult.Rejected...)
		result.Unknown = append(result.Unknown, childResult.Unknown...)
		result.ScannedDirs += childResult.ScannedDirs
	}

	return result
}

// classifyShowDir creates a show candidate and processes child directories for seasons/episodes.
func (imp *Importer) classifyShowDir(ctx context.Context, node *FSNode, walkCtx *WalkContext, result *ClassificationResult) {
	showID := uuid.New().String()

	candidate := ImportCandidate{
		ID:        showID,
		LibraryID: imp.libraryID,
		Kind:      ImportEntityShow,
		RootPath:  node.Path,
		NFOPath:   imp.fs.Join(node.Path, "tvshow.nfo"),
		Confidence: 100,
		Evidence: []ImportEvidence{
			{Rule: "tvshow.nfo", Weight: 100, Message: "tvshow.nfo found"},
		},
	}
	result.Candidates = append(result.Candidates, candidate)

	// Claim the show directory
	walkCtx.ClaimedPaths[node.Path] = PathClaim{
		CandidateID: showID,
		Kind:        ImportEntityShow,
		Path:        node.Path,
	}

	// Process child directories as seasons
	childWalkCtx := &WalkContext{
		ScanID:       walkCtx.ScanID,
		LibraryID:    walkCtx.LibraryID,
		SourceRoot:   walkCtx.SourceRoot,
		ParentKind:   ImportEntityShow,
		ShowRootPath: node.Path,
		ShowID:       showID,
		ClaimedPaths: walkCtx.ClaimedPaths,
	}

	for _, child := range node.Children {
		if !child.IsDir {
			continue
		}
		childResult := imp.classifyTree(ctx, child, childWalkCtx)
		result.Candidates = append(result.Candidates, childResult.Candidates...)
		result.Rejected = append(result.Rejected, childResult.Rejected...)
		result.Unknown = append(result.Unknown, childResult.Unknown...)
		result.ScannedDirs += childResult.ScannedDirs
	}
}

// classifyMovieDir creates a movie candidate from a directory with a movie NFO.
func (imp *Importer) classifyMovieDir(ctx context.Context, node *FSNode, nfoPath string, walkCtx *WalkContext, result *ClassificationResult) {
	// Find the video file in this directory
	videoPath := imp.findVideoFileInDir(ctx, node.Path, "")
	if videoPath == "" {
		result.Rejected = append(result.Rejected, RejectedItem{
			Path:   node.Path,
			Type:   ImportEntityMovie,
			Reason: "movie NFO found but no video file in directory",
		})
		return
	}

	// Check if the video path is already claimed
	if _, claimed := walkCtx.ClaimedPaths[videoPath]; claimed {
		return
	}

	candidate := ImportCandidate{
		ID:          uuid.New().String(),
		LibraryID:   imp.libraryID,
		Kind:        ImportEntityMovie,
		RootPath:    node.Path,
		PrimaryPath: videoPath,
		NFOPath:     nfoPath,
		Confidence:  100,
		Evidence: []ImportEvidence{
			{Rule: "movie_nfo", Weight: 100, Message: "movie NFO found: " + nfoPath},
		},
	}
	result.Candidates = append(result.Candidates, candidate)

	// Claim the video file
	walkCtx.ClaimedPaths[videoPath] = PathClaim{
		CandidateID: candidate.ID,
		Kind:        ImportEntityMovie,
		Path:        videoPath,
	}
}

// classifyMovieDirNoNFO creates a movie candidate from a directory with video files
// but no valid NFO. The title is derived from the video filename.
func (imp *Importer) classifyMovieDirNoNFO(ctx context.Context, node *FSNode, walkCtx *WalkContext, result *ClassificationResult) {
	// Find the first video file in this directory
	var videoPath string
	for _, child := range node.Children {
		if !child.IsDir && child.Kind == ImportNodeVideo {
			videoPath = child.Path
			break
		}
	}
	if videoPath == "" {
		return
	}

	// Check if the video path is already claimed
	if _, claimed := walkCtx.ClaimedPaths[videoPath]; claimed {
		return
	}

	candidate := ImportCandidate{
		ID:          uuid.New().String(),
		LibraryID:   imp.libraryID,
		Kind:        ImportEntityMovie,
		RootPath:    node.Path,
		PrimaryPath: videoPath,
		Confidence:  50,
		Evidence: []ImportEvidence{
			{Rule: "filename_fallback", Weight: 50, Message: "no valid NFO, using filename"},
		},
	}
	result.Candidates = append(result.Candidates, candidate)

	// Claim the video file
	walkCtx.ClaimedPaths[videoPath] = PathClaim{
		CandidateID: candidate.ID,
		Kind:        ImportEntityMovie,
		Path:        videoPath,
	}
}

// classifySeasonDir creates season-level processing: scans child directories and files for episodes.
func (imp *Importer) classifySeasonDir(ctx context.Context, node *FSNode, walkCtx *WalkContext, result *ClassificationResult) {
	// Extract season number from directory name
	seasonNum := imp.extractSeasonNumber(node.Name)

	childWalkCtx := &WalkContext{
		ScanID:        walkCtx.ScanID,
		LibraryID:     walkCtx.LibraryID,
		SourceRoot:    walkCtx.SourceRoot,
		ParentKind:    ImportEntitySeason,
		ShowRootPath:  walkCtx.ShowRootPath,
		ShowID:        walkCtx.ShowID,
		SeasonRootPath: node.Path,
		SeasonNumber:  seasonNum,
		ClaimedPaths:  walkCtx.ClaimedPaths,
	}

	// Process children: could be episode files directly or subdirectories with episodes
	for _, child := range node.Children {
		if child.IsDir {
			// Recurse into subdirectories
			childResult := imp.classifyTree(ctx, child, childWalkCtx)
			result.Candidates = append(result.Candidates, childResult.Candidates...)
			result.Rejected = append(result.Rejected, childResult.Rejected...)
			result.Unknown = append(result.Unknown, childResult.Unknown...)
			result.ScannedDirs += childResult.ScannedDirs
		} else if child.Kind == ImportNodeVideo {
			// Direct video file in season directory
			if _, claimed := walkCtx.ClaimedPaths[child.Path]; claimed {
				continue
			}
			epCandidate := imp.classifyEpisodeFile(ctx, child, childWalkCtx, result)
			if epCandidate != nil {
				result.Candidates = append(result.Candidates, *epCandidate)
			}
		}
	}
}

// classifyEpisodeDir handles a directory inside a season that contains video files.
func (imp *Importer) classifyEpisodeDir(ctx context.Context, node *FSNode, walkCtx *WalkContext, result *ClassificationResult) {
	for _, child := range node.Children {
		if child.IsDir {
			childResult := imp.classifyTree(ctx, child, walkCtx)
			result.Candidates = append(result.Candidates, childResult.Candidates...)
			result.Rejected = append(result.Rejected, childResult.Rejected...)
			result.Unknown = append(result.Unknown, childResult.Unknown...)
			result.ScannedDirs += childResult.ScannedDirs
		} else if child.Kind == ImportNodeVideo {
			if _, claimed := walkCtx.ClaimedPaths[child.Path]; claimed {
				continue
			}
			epCandidate := imp.classifyEpisodeFile(ctx, child, walkCtx, result)
			if epCandidate != nil {
				result.Candidates = append(result.Candidates, *epCandidate)
			}
		}
	}
}

// classifyEpisodeFile creates an episode candidate from a video file node.
func (imp *Importer) classifyEpisodeFile(ctx context.Context, node *FSNode, walkCtx *WalkContext, result *ClassificationResult) *ImportCandidate {
	// Extract episode info from filename
	season, episode := extractEpisodeInfo(node.Name)

	// If we have a season context but the filename doesn't have season info, use context
	if season == 0 && walkCtx.SeasonNumber != nil {
		season = *walkCtx.SeasonNumber
	}

	// Look for a matching episode NFO in the same directory
	epDir := filepath.Dir(node.Name)
	if epDir == "." || epDir == "/" {
		epDir = walkCtx.CurrentPath
	}
	nfoPath := imp.fs.Join(epDir, trimExt(node.Name)+".nfo")
	if !imp.fs.Exists(ctx, nfoPath) {
		// Also check the parent directory of the video file
		nfoPath = ""
	}

	candidate := ImportCandidate{
		ID:            uuid.New().String(),
		LibraryID:     imp.libraryID,
		Kind:          ImportEntityEpisode,
		PrimaryPath:   node.Path,
		NFOPath:       nfoPath,
		ShowID:        walkCtx.ShowID,
		SeasonNumber:  walkCtx.SeasonNumber,
		EpisodeNumber: repository.IntPtr(episode),
		Confidence:    80,
		Evidence: []ImportEvidence{
			{Rule: "episode_file", Weight: 80, Message: "video file with episode pattern"},
		},
	}

	if nfoPath != "" {
		candidate.Confidence = 100
		candidate.Evidence = append(candidate.Evidence, ImportEvidence{
			Rule:    "episode_nfo",
			Weight:  100,
			Message: "episode NFO found",
		})
	}

	// Claim the video file
	walkCtx.ClaimedPaths[node.Path] = PathClaim{
		CandidateID: candidate.ID,
		Kind:        ImportEntityEpisode,
		Path:        node.Path,
	}

	return &candidate
}

// extractSeasonNumber parses the season number from a directory name like "Season 01".
func (imp *Importer) extractSeasonNumber(name string) *int {
	m := seasonDirPattern.FindStringSubmatch(name)
	if m == nil {
		return nil
	}
	// Extract digits
	digits := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(name, "season"), "Season"))
	digits = strings.TrimSpace(digits)
	if n, err := strconv.Atoi(digits); err == nil {
		return &n
	}
	return nil
}

// ── Stage 3: Metadata Parsing ─────────────────────────────────────────────

// parseCandidateMetadata parses NFO files and enriches candidates with metadata.
func (imp *Importer) parseCandidateMetadata(ctx context.Context, candidates []ImportCandidate) {
	for i := range candidates {
		c := &candidates[i]

		switch c.Kind {
		case ImportEntityShow:
			imp.parseShowMetadata(ctx, c)
		case ImportEntityMovie:
			imp.parseMovieMetadata(ctx, c)
		case ImportEntityEpisode:
			imp.parseEpisodeMetadata(ctx, c)
		}
	}
}

// parseShowMetadata reads and parses the tvshow.nFO for a show candidate.
// The parsed NFO is stored via the candidate's Evidence for later use in reconciliation.
func (imp *Importer) parseShowMetadata(ctx context.Context, c *ImportCandidate) {
	if c.NFOPath == "" {
		return
	}
	f, err := imp.fs.Open(ctx, c.NFOPath)
	if err != nil {
		return
	}
	defer f.Close()

	showNFO, err := ParseShowNFO(f)
	if err != nil {
		return
	}

	c.Evidence = append(c.Evidence, ImportEvidence{
		Rule:    "show_nfo_parsed",
		Weight:  100,
		Message: fmt.Sprintf("show title: %s", showNFO.Title),
	})
	c.Confidence = 100
	_ = showNFO
}

// parseMovieMetadata reads and parses the movie NFO for a movie candidate.
func (imp *Importer) parseMovieMetadata(ctx context.Context, c *ImportCandidate) {
	if c.NFOPath == "" {
		return
	}
	f, err := imp.fs.Open(ctx, c.NFOPath)
	if err != nil {
		return
	}
	defer f.Close()

	movieNFO, err := ParseMovieNFO(f)
	if err != nil {
		return
	}

	c.Evidence = append(c.Evidence, ImportEvidence{
		Rule:    "movie_nfo_parsed",
		Weight:  100,
		Message: fmt.Sprintf("movie title: %s", movieNFO.Title),
	})
	c.Confidence = 100
	_ = movieNFO
}

// parseEpisodeMetadata reads and parses the episode NFO for an episode candidate.
func (imp *Importer) parseEpisodeMetadata(ctx context.Context, c *ImportCandidate) {
	if c.NFOPath == "" {
		return
	}
	f, err := imp.fs.Open(ctx, c.NFOPath)
	if err != nil {
		return
	}
	defer f.Close()

	episodes, err := ParseEpisodeNFOs(f)
	if err != nil || len(episodes) == 0 {
		return
	}

	c.Evidence = append(c.Evidence, ImportEvidence{
		Rule:    "episode_nfo_parsed",
		Weight:  100,
		Message: fmt.Sprintf("episode title: %s", episodes[0].Title),
	})
}

// ── Stage 4: Reconciliation ───────────────────────────────────────────────

// reconcileCandidates takes classified, metadata-parsed candidates and
// creates or updates database records. It returns a ReconcileResult with
// counts of accepted/rejected/unknown items.
func (imp *Importer) reconcileCandidates(ctx context.Context, candidates []ImportCandidate) (*ReconcileResult, error) {
	result := &ReconcileResult{
		Accepted:        make([]ResolvedItem, 0),
		Rejected:        make([]RejectedItem, 0),
		Unknown:         make([]ImportCandidate, 0),
		TotalCandidates: len(candidates),
		DoneCandidates:  0,
	}

	for i := range candidates {
		c := &candidates[i]

		switch c.Kind {
		case ImportEntityShow:
			resolved, err := imp.reconcileShow(ctx, c)
			if err != nil {
				result.Rejected = append(result.Rejected, RejectedItem{
					Path:   c.RootPath,
					Type:   c.Kind,
					Reason: err.Error(),
				})
			} else {
				result.Accepted = append(result.Accepted, *resolved)
			}

		case ImportEntityMovie:
			resolved, err := imp.reconcileMovie(ctx, c)
			if err != nil {
				result.Rejected = append(result.Rejected, RejectedItem{
					Path:   c.PrimaryPath,
					Type:   c.Kind,
					Reason: err.Error(),
				})
			} else {
				result.Accepted = append(result.Accepted, *resolved)
			}

		case ImportEntityEpisode:
			resolved, err := imp.reconcileEpisode(ctx, c)
			if err != nil {
				result.Rejected = append(result.Rejected, RejectedItem{
					Path:   c.PrimaryPath,
					Type:   c.Kind,
					Reason: err.Error(),
				})
			} else {
				result.Accepted = append(result.Accepted, *resolved)
			}

		default:
			result.Unknown = append(result.Unknown, *c)
		}

		result.DoneCandidates++
	}

	return result, nil
}

// reconcileShow creates or updates a show media item.
func (imp *Importer) reconcileShow(ctx context.Context, c *ImportCandidate) (*ResolvedItem, error) {
	nfoPath := imp.fs.Join(c.RootPath, "tvshow.nfo")
	var showNFO *model.NFOTVShow

	if imp.fs.Exists(ctx, nfoPath) {
		f, err := imp.fs.Open(ctx, nfoPath)
		if err == nil {
			showNFO, err = ParseShowNFO(f)
			f.Close()
			if err != nil {
				showNFO = nil
			}
		}
	}

	title := ""
	overview := ""
	var year int
	var rating float64
	var runtime int

	if showNFO != nil {
		title = showNFO.Title
		overview = showNFO.Plot
		year = showNFO.Year
		rating = showNFO.Rating
		runtime = showNFO.Runtime
	}

	if title == "" {
		title = baseName(c.RootPath)
	}

	// Find poster/backdrop
	nfoPoster := ""
	nfoBackdrop := ""
	if showNFO != nil {
		nfoPoster = extractNFOPosterThumb(showNFO.Thumbs)
		nfoBackdrop = extractNFOFanartPath(showNFO.Fanart)
	}

	base := baseName(c.RootPath)
	var posterPath, backdropPath string
	if _, ok := imp.fs.(*LocalImportFS); ok {
		posterPath = FindPosterPath(c.RootPath, base, nfoPoster)
		backdropPath = FindBackdropPath(c.RootPath, base, nfoBackdrop)
	} else {
		posterPath = nfoPoster
		backdropPath = nfoBackdrop
	}

	// Look for existing show by library_id + root_path + type
	existingID, _ := imp.mediaRepo.FindByRootPath(ctx, imp.libraryID, c.RootPath, "show")

	now := time.Now().UTC().Format(time.RFC3339)
	showID := c.ID // Use candidate ID as default
	action := ReconcileCreate
	if existingID != "" {
		showID = existingID
		action = ReconcileUpdate
	}

	showItem := &model.MediaItem{
		ID:             showID,
		Type:           "show",
		Title:          title,
		Year:           year,
		Overview:       overview,
		Rating:         rating,
		Duration:       runtime * 60,
		FilePath:       "", // Show file_path must be empty
		RootPath:       c.RootPath,
		NFOPath:        c.NFOPath,
		PosterPath:     posterPath,
		BackdropPath:   backdropPath,
		MetadataSource: "nfo",
		ProviderID:     imp.providerID,
		LibraryID:      imp.libraryID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if showNFO != nil {
		applyShowNFOFields(showItem, showNFO)
		// Re-apply our core fields to ensure correctness
		showItem.FilePath = ""
		showItem.RootPath = c.RootPath
		showItem.NFOPath = c.NFOPath
		showItem.MetadataSource = "nfo"
	}

	// Find logo
	if logoPath := FindLogoPath(c.RootPath); logoPath != "" {
		showItem.LogoPath = logoPath
	}

	if existingID != "" {
		if err := imp.mediaRepo.Update(ctx, showItem); err != nil {
			return nil, err
		}
	} else {
		if err := imp.mediaRepo.Create(ctx, showItem); err != nil {
			return nil, err
		}
	}

	return &ResolvedItem{
		CandidateID: c.ID,
		MediaID:     showID,
		Kind:        ImportEntityShow,
		Action:      action,
		RootPath:    c.RootPath,
		PrimaryPath: "",
	}, nil
}

// reconcileMovie creates or updates a movie media item.
func (imp *Importer) reconcileMovie(ctx context.Context, c *ImportCandidate) (*ResolvedItem, error) {
	if c.PrimaryPath == "" {
		return nil, fmt.Errorf("movie candidate has no video path")
	}

	videoPath := c.PrimaryPath

	// Parse NFO if available
	var title, overview string
	var year int
	var rating float64
	var runtime int
	var nfoPoster, nfoBackdrop string
	var sortTitle string
	var movieNFO *model.NFOMovie

	if c.NFOPath != "" {
		f, err := imp.fs.Open(ctx, c.NFOPath)
		if err == nil {
			movieNFO, err = ParseMovieNFO(f)
			f.Close()
			if err == nil && movieNFO != nil {
				title = movieNFO.Title
				overview = movieNFO.Plot
				year = movieNFO.Year
				rating = movieNFO.Rating
				runtime = movieNFO.Runtime
				sortTitle = movieNFO.SortTitle
				nfoPoster = extractNFOPosterThumb(movieNFO.Thumbs)
				nfoBackdrop = extractNFOFanartPath(movieNFO.Fanart)
			}
		}
	}

	// Fallback to filename-derived title if no NFO title
	if title == "" {
		title = trimExt(baseName(videoPath))
		title = cleanTitle(title)
	}

	// Find poster/backdrop
	base := trimExt(baseName(videoPath))
	var posterPath, backdropPath string
	if _, ok := imp.fs.(*LocalImportFS); ok {
		posterPath = FindPosterPath(c.RootPath, base, nfoPoster)
		backdropPath = FindBackdropPath(c.RootPath, base, nfoBackdrop)
	} else {
		posterPath = nfoPoster
		backdropPath = nfoBackdrop
	}

	// Look for existing movie by library_id + primary_path + type
	existingID, _ := imp.mediaRepo.FindExistingItem(ctx, imp.libraryID, videoPath, "movie")

	now := time.Now().UTC().Format(time.RFC3339)
	movieID := c.ID // Use candidate ID as default
	action := ReconcileCreate
	if existingID != "" {
		movieID = existingID
		action = ReconcileUpdate
	}

	metadataSource := "nfo"
	if c.NFOPath == "" {
		metadataSource = "filename"
	}

	movieItem := &model.MediaItem{
		ID:             movieID,
		Type:           "movie",
		Title:          title,
		SortTitle:      sortTitle,
		Year:           year,
		Overview:       overview,
		Rating:         rating,
		Duration:       runtime * 60,
		FilePath:       videoPath,    // Movie file_path = actual video file
		PrimaryPath:    videoPath,    // Movie primary_path = actual video file
		RootPath:       c.RootPath,
		NFOPath:        c.NFOPath,
		PosterPath:     posterPath,
		BackdropPath:   backdropPath,
		MetadataSource: metadataSource,
		ProviderID:     imp.providerID,
		LibraryID:      imp.libraryID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if movieNFO != nil {
		applyMovieNFOFields(movieItem, movieNFO)
		// Re-apply our core fields after NFO processing to ensure correctness
		movieItem.Title = title
		movieItem.FilePath = videoPath
		movieItem.PrimaryPath = videoPath
		movieItem.RootPath = c.RootPath
		movieItem.NFOPath = c.NFOPath
		movieItem.MetadataSource = metadataSource
	}

	// Find logo
	if logoPath := FindLogoPath(c.RootPath); logoPath != "" {
		movieItem.LogoPath = logoPath
	}

	if existingID != "" {
		if err := imp.mediaRepo.Update(ctx, movieItem); err != nil {
			return nil, err
		}
	} else {
		if err := imp.mediaRepo.Create(ctx, movieItem); err != nil {
			return nil, err
		}
	}

	return &ResolvedItem{
		CandidateID: c.ID,
		MediaID:     movieID,
		Kind:        ImportEntityMovie,
		Action:      action,
		RootPath:    c.RootPath,
		PrimaryPath: videoPath,
	}, nil
}

// reconcileEpisode creates or updates an episode media item.
func (imp *Importer) reconcileEpisode(ctx context.Context, c *ImportCandidate) (*ResolvedItem, error) {
	if c.PrimaryPath == "" {
		return nil, fmt.Errorf("episode candidate has no video path")
	}

	videoPath := c.PrimaryPath
	baseNameNoExt := trimExt(baseName(videoPath))

	// Extract season/episode from filename
	season, episode := extractEpisodeInfo(baseName(videoPath))

	// Resolve the actual show DB ID from the show root path
	showID := c.ShowID
	if c.ShowID != "" && c.SeasonNumber != nil {
		// Try to find the show by root_path to get the stable DB ID
		existingShowID, _ := imp.mediaRepo.FindByRootPath(ctx, imp.libraryID, imp.getShowRootPath(c), "show")
		if existingShowID != "" {
			showID = existingShowID
		}
	}

	// Look for episode NFO
	epTitle := baseNameNoExt
	var epOverview string
	var epRating float64
	var epRuntime int
	var nfoThumb string
	var nfoSeason, nfoEpisode int

	// Check for NFO: first try same-name NFO in same directory
	epDir := filepath.Dir(videoPath)
	epNFOPath := imp.fs.Join(epDir, baseNameNoExt+".nfo")

	if imp.fs.Exists(ctx, epNFOPath) {
		if nf, err := imp.fs.Open(ctx, epNFOPath); err == nil {
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
					nfoSeason = epNFO.Season
				}
				if epNFO.Episode > 0 {
					nfoEpisode = epNFO.Episode
				}
			}
		}
	}

	if epTitle == "" {
		epTitle = baseNameNoExt
	}

	// Use NFO season/episode if available
	if nfoSeason > 0 {
		season = nfoSeason
	}
	if nfoEpisode > 0 {
		episode = nfoEpisode
	}

	var thumbPath string
	if _, ok := imp.fs.(*LocalImportFS); ok {
		thumbPath = FindEpisodeThumbnailPath(epDir, baseNameNoExt, nfoThumb)
	} else {
		thumbPath = nfoThumb
	}

	// Look for existing episode by library_id + primary_path + type
	existingID, _ := imp.mediaRepo.FindExistingItem(ctx, imp.libraryID, videoPath, "episode")

	now := time.Now().UTC().Format(time.RFC3339)
	epID := c.ID // Use candidate ID as default
	action := ReconcileCreate
	if existingID != "" {
		epID = existingID
		action = ReconcileUpdate
	}

	epItem := &model.MediaItem{
		ID:             epID,
		Type:           "episode",
		Title:          epTitle,
		Overview:       epOverview,
		Rating:         epRating,
		Duration:       epRuntime * 60,
		FilePath:       videoPath, // Episode file_path = actual video file (no double-nesting)
		PrimaryPath:    videoPath, // Episode primary_path = actual video file
		PosterPath:     thumbPath,
		BackdropPath:   thumbPath,
		ParentID:       showID, // Use resolved show DB ID, not candidate ID
		Season:         repository.IntPtr(season),
		Episode:        repository.IntPtr(episode),
		MetadataSource: "nfo",
		ProviderID:     imp.providerID,
		LibraryID:      imp.libraryID,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	// Apply enhanced NFO fields if NFO exists
	if imp.fs.Exists(ctx, epNFOPath) {
		if nf, err := imp.fs.Open(ctx, epNFOPath); err == nil {
			episodes, err := ParseEpisodeNFOs(nf)
			nf.Close()
			if err == nil && len(episodes) > 0 {
				applyEpisodeNFOFields(epItem, &episodes[0])
			}
		}
	}

	if existingID != "" {
		if err := imp.mediaRepo.Update(ctx, epItem); err != nil {
			return nil, err
		}
	} else {
		if err := imp.mediaRepo.Create(ctx, epItem); err != nil {
			return nil, err
		}
	}

	return &ResolvedItem{
		CandidateID: c.ID,
		MediaID:     epID,
		Kind:        ImportEntityEpisode,
		Action:      action,
		RootPath:    "",
		PrimaryPath: videoPath,
	}, nil
}

// getShowRootPath returns the show root path for an episode candidate.
// It walks up from the episode's directory to find the show root.
func (imp *Importer) getShowRootPath(c *ImportCandidate) string {
	// The episode's video path is in a season directory inside the show root.
	// We need to find the show root by looking for tvshow.nfo in parent directories.
	videoPath := c.PrimaryPath
	if videoPath == "" {
		return ""
	}

	dir := filepath.Dir(videoPath)
	for dir != "" && dir != "/" && dir != "." {
		tvshowPath := imp.fs.Join(dir, "tvshow.nfo")
		if imp.fs.Exists(context.Background(), tvshowPath) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// buildRejectWarnings converts rejected items to warning strings.
func (imp *Importer) buildRejectWarnings(rejected []RejectedItem) []string {
	warnings := make([]string, 0, len(rejected))
	for _, r := range rejected {
		warnings = append(warnings, fmt.Sprintf("%s: %s", r.Path, r.Reason))
	}
	return warnings
}

// ── Utility functions (preserved from original) ───────────────────────────

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

// findMovieNFO looks for a .nfo file in dir that contains a <movie> root element.
// It prefers "movie.nfo" (the Kodi/Jellyfin standard name), and only falls back to
// other .nfo files if movie.nfo does not exist or fails to parse.
func (imp *Importer) findMovieNFO(ctx context.Context, dir string) string {
	entries, err := imp.fs.ReadDir(ctx, dir)
	if err != nil {
		return ""
	}

	// First pass: look for the standard "movie.nfo"
	for _, entry := range entries {
		if entry.IsDir {
			continue
		}
		name := entry.Name
		if strings.EqualFold(name, "movie.nfo") {
			path := imp.fs.Join(dir, name)
			if nf, err := imp.fs.Open(ctx, path); err == nil {
				var movie model.NFOMovie
				err = xml.NewDecoder(nf).Decode(&movie)
				nf.Close()
				if err == nil {
					// Accept movie.nfo even with empty title;
					// reconciler will fall back to filename.
					return path
				}
			}
		}
	}

	// Second pass: fall back to any other .nfo with a valid <movie> root
	for _, entry := range entries {
		if entry.IsDir {
			continue
		}
		name := entry.Name
		if strings.EqualFold(name, "movie.nfo") {
			continue // already tried
		}
		var ext string
		if idx := strings.LastIndex(name, "."); idx >= 0 {
			ext = strings.ToLower(name[idx:])
		}
		if ext != ".nfo" {
			continue
		}
		path := imp.fs.Join(dir, name)
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
