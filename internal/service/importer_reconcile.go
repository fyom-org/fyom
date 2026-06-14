package service

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/fyom/fyom/internal/model"
	"github.com/fyom/fyom/internal/repository"
)

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

	existingID, _ := imp.mediaRepo.FindByRootPath(ctx, imp.libraryID, c.RootPath, "show")

	now := time.Now().UTC().Format(time.RFC3339)
	showID := c.ID
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
		FilePath:       "",
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
		showItem.FilePath = ""
		showItem.RootPath = c.RootPath
		showItem.NFOPath = c.NFOPath
		showItem.MetadataSource = "nfo"
	}

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

	if title == "" {
		title = trimExt(baseName(videoPath))
		title = cleanTitle(title)
	}

	base := trimExt(baseName(videoPath))
	var posterPath, backdropPath string
	if _, ok := imp.fs.(*LocalImportFS); ok {
		posterPath = FindPosterPath(c.RootPath, base, nfoPoster)
		backdropPath = FindBackdropPath(c.RootPath, base, nfoBackdrop)
	} else {
		posterPath = nfoPoster
		backdropPath = nfoBackdrop
	}

	existingID, _ := imp.mediaRepo.FindExistingItem(ctx, imp.libraryID, videoPath, "movie")

	now := time.Now().UTC().Format(time.RFC3339)
	movieID := c.ID
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
		FilePath:       videoPath,
		PrimaryPath:    videoPath,
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
		movieItem.Title = title
		movieItem.FilePath = videoPath
		movieItem.PrimaryPath = videoPath
		movieItem.RootPath = c.RootPath
		movieItem.NFOPath = c.NFOPath
		movieItem.MetadataSource = metadataSource
	}

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

	season, episode := extractEpisodeInfo(baseName(videoPath))

	// Resolve the actual show DB ID from the show root path
	showID := c.ShowID
	if c.ShowID != "" && c.SeasonNumber != nil {
		existingShowID, _ := imp.mediaRepo.FindByRootPath(ctx, imp.libraryID, imp.getShowRootPath(c), "show")
		if existingShowID != "" {
			showID = existingShowID
		}
	}

	epTitle := baseNameNoExt
	var epOverview string
	var epRating float64
	var epRuntime int
	var nfoThumb string
	var nfoSeason, nfoEpisode int

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

	existingID, _ := imp.mediaRepo.FindExistingItem(ctx, imp.libraryID, videoPath, "episode")

	now := time.Now().UTC().Format(time.RFC3339)
	epID := c.ID
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
		FilePath:       videoPath,
		PrimaryPath:    videoPath,
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
