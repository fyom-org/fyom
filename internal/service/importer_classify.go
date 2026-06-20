package service

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fyom/fyom/internal/repository"
	"github.com/google/uuid"
)

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
		return result
	}

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
		if seasonDirPattern.MatchString(node.Name) {
			imp.classifySeasonDir(ctx, node, walkCtx, result)
			return result
		}
		if walkCtx.ParentKind == ImportEntitySeason {
			imp.classifyEpisodeDir(ctx, node, walkCtx, result)
			return result
		}
	}

	// Check if this directory is a transparent container (grouping/wrapper directory).
	// A container has subdirectories but is not itself a media root.
	// We must recurse into it to find nested media entities.
	if imp.isTransparentContainer(node, walkCtx) {
		imp.classifyContainerDir(ctx, node, walkCtx, result)
		return result
	}

	// Fallback: if this directory contains video files and is not inside a show,
	// classify it as a movie with filename-derived metadata.
	if walkCtx.ParentKind == ImportEntityUnknown || walkCtx.ParentKind == "" || walkCtx.ParentKind == ImportEntityContainer {
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

// isTransparentContainer returns true if the directory is a grouping/wrapper
// container that should be traversed transparently rather than classified as a
// media root. A container has subdirectories but no direct indication of being
// a movie/show root (no NFO, no direct video files at this level).
func (imp *Importer) isTransparentContainer(node *FSNode, _ *WalkContext) bool {
	// Must have subdirectories to be a container
	hasSubdirs := false
	hasDirectVideo := false
	for _, child := range node.Children {
		if child.IsDir {
			hasSubdirs = true
		} else if child.Kind == ImportNodeVideo {
			hasDirectVideo = true
		}
	}
	// A container has subdirectories and no direct video files
	// (direct video files would make it a movie-fallback candidate)
	return hasSubdirs && !hasDirectVideo
}

// classifyContainerDir handles a transparent grouping/container directory.
// It recurses into children without creating any media candidate for the container itself.
func (imp *Importer) classifyContainerDir(ctx context.Context, node *FSNode, walkCtx *WalkContext, result *ClassificationResult) {
	// Propagate container context to children
	childWalkCtx := &WalkContext{
		ScanID:       walkCtx.ScanID,
		LibraryID:    walkCtx.LibraryID,
		SourceRoot:   walkCtx.SourceRoot,
		ParentKind:   ImportEntityContainer,
		ShowRootPath: walkCtx.ShowRootPath,
		ShowID:       walkCtx.ShowID,
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

// classifyShowDir creates a show candidate and processes child directories for seasons/episodes.
func (imp *Importer) classifyShowDir(ctx context.Context, node *FSNode, walkCtx *WalkContext, result *ClassificationResult) {
	// Enforce library type policy: movie-only libraries don't import shows
	if imp.libraryType == "movie" {
		result.Rejected = append(result.Rejected, RejectedItem{
			Path:   node.Path,
			Type:   ImportEntityShow,
			Reason: "show rejected in movie-only library",
		})
		return
	}

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
	// Enforce library type policy: show-only libraries don't import movies
	if imp.libraryType == "show" {
		result.Rejected = append(result.Rejected, RejectedItem{
			Path:   node.Path,
			Type:   ImportEntityMovie,
			Reason: "movie rejected in show-only library",
		})
		return
	}

	videoPath := imp.findVideoFileInDir(ctx, node.Path, "")
	if videoPath == "" {
		result.Rejected = append(result.Rejected, RejectedItem{
			Path:   node.Path,
			Type:   ImportEntityMovie,
			Reason: "movie NFO found but no video file in directory",
		})
		return
	}

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

	walkCtx.ClaimedPaths[videoPath] = PathClaim{
		CandidateID: candidate.ID,
		Kind:        ImportEntityMovie,
		Path:        videoPath,
	}
}

// classifyMovieDirNoNFO creates a movie candidate from a directory with video files
// but no valid NFO. The title is derived from the video filename.
func (imp *Importer) classifyMovieDirNoNFO(_ context.Context, node *FSNode, walkCtx *WalkContext, result *ClassificationResult) {
	// Enforce library type policy: show-only libraries don't import movies
	if imp.libraryType == "show" {
		result.Rejected = append(result.Rejected, RejectedItem{
			Path:   node.Path,
			Type:   ImportEntityMovie,
			Reason: "movie rejected in show-only library",
		})
		return
	}

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

	walkCtx.ClaimedPaths[videoPath] = PathClaim{
		CandidateID: candidate.ID,
		Kind:        ImportEntityMovie,
		Path:        videoPath,
	}
}

// classifySeasonDir creates season-level processing: scans child directories and files for episodes.
func (imp *Importer) classifySeasonDir(ctx context.Context, node *FSNode, walkCtx *WalkContext, result *ClassificationResult) {
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

	for _, child := range node.Children {
		if child.IsDir {
			childResult := imp.classifyTree(ctx, child, childWalkCtx)
			result.Candidates = append(result.Candidates, childResult.Candidates...)
			result.Rejected = append(result.Rejected, childResult.Rejected...)
			result.Unknown = append(result.Unknown, childResult.Unknown...)
			result.ScannedDirs += childResult.ScannedDirs
		} else if child.Kind == ImportNodeVideo {
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
func (imp *Importer) classifyEpisodeFile(ctx context.Context, node *FSNode, walkCtx *WalkContext, _ *ClassificationResult) *ImportCandidate {
	season, episode := extractEpisodeInfo(node.Name)

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
		nfoPath = ""
	}

	candidate := ImportCandidate{
		ID:            uuid.New().String(),
		LibraryID:     imp.libraryID,
		Kind:          ImportEntityEpisode,
		PrimaryPath:   node.Path,
		NFOPath:       nfoPath,
		ShowID:        walkCtx.ShowID,
		SeasonNumber:  repository.IntPtr(season),
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
	digits := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(name, "season"), "Season"))
	digits = strings.TrimSpace(digits)
	if n, err := strconv.Atoi(digits); err == nil {
		return &n
	}
	return nil
}
