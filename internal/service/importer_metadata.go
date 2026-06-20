package service

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/fyom/fyom/internal/model"
)

// ensure io is used
var _ = io.Discard
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
func (imp *Importer) parseShowMetadata(ctx context.Context, c *ImportCandidate) {
	if c.NFOPath == "" {
		return
	}
	f, err := imp.fs.Open(ctx, c.NFOPath)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()

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
	defer func() { _ = f.Close() }()

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
	defer func() { _ = f.Close() }()

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

// ── Metadata enrichment helpers ──────────────────────────────────────────

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

	ids := make([]model.NFOUniqueID, len(nfo.UniqueIDs))
	copy(ids, nfo.UniqueIDs)

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
	if nfo.OriginalTitle != "" {
		item.OriginalTitle = nfo.OriginalTitle
	}
	if nfo.SortTitle != "" {
		item.SortTitle = nfo.SortTitle
	} else if nfo.SortName != "" {
		item.SortTitle = nfo.SortName
	}

	item.Premiered = nfo.Premiered
	item.ReleaseDate = nfo.ReleaseDate
	item.EndDate = nfo.EndDate
	item.Outline = nfo.Outline
	item.Tagline = nfo.Tagline
	item.Countries = stringsToJSON(nfo.Countries)
	item.CountryCode = nfo.CountryCode
	item.Language = nfo.Language
	item.Directors = stringsToJSON(nfo.Directors)
	item.Credits = stringsToJSON(nfo.Credits)
	item.Tags = stringsToJSON(nfo.Tags)

	if nfo.Set != nil {
		item.SetName = nfo.Set.Name
		item.SetOverview = nfo.Set.Overview
	}
	item.CollectionNumber = nfo.CollectionNumber
	item.CustomRating = nfo.CustomRating
	item.UserRating = nfo.UserRating
	item.LastPlayed = nfo.LastPlayed
	item.Playcount = nfo.Playcount
	item.DateAdded = nfo.DateAdded
	item.DisplayOrder = nfo.DisplayOrder
	item.VideoCodec = nfo.FileInfo.StreamDetails.Video.Codec
	item.VideoWidth = nfo.FileInfo.StreamDetails.Video.Width
	item.VideoHeight = nfo.FileInfo.StreamDetails.Video.Height
	item.VideoDurationSeconds = nfo.FileInfo.StreamDetails.Video.DurationInSeconds
	if len(nfo.FileInfo.StreamDetails.Audios) > 0 {
		item.AudioCodec = nfo.FileInfo.StreamDetails.Audios[0].Codec
		item.AudioChannels = nfo.FileInfo.StreamDetails.Audios[0].Channels
	}
	item.SubtitleLanguages = subtitlesToJSON(nfo.FileInfo.StreamDetails.Subtitles)

	if item.Year == 0 && nfo.Premiered != "" && len(nfo.Premiered) >= 4 {
		if y, err := strconv.Atoi(nfo.Premiered[:4]); err == nil {
			item.Year = y
		}
	}
	if item.Duration == 0 && nfo.FileInfo.StreamDetails.Video.DurationInSeconds > 0 {
		item.Duration = nfo.FileInfo.StreamDetails.Video.DurationInSeconds
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

// applyShowNFOFields populates enhanced fields from parsed NFO onto a MediaItem (show).
func applyShowNFOFields(item *model.MediaItem, nfo *model.NFOTVShow) {
	if nfo.Title != "" {
		item.Title = nfo.Title
	}
	item.Genres = stringsToJSON(nfo.Genres)
	item.Studios = stringsToJSON(nfo.Studios)
	item.Actors = actorsToJSON(nfo.Actors)

	ids := make([]model.NFOUniqueID, len(nfo.UniqueIDs))
	copy(ids, nfo.UniqueIDs)

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

	// nfo.Status is a passthrough — stored in a dedicated column if present.

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

// parseShowNFO is a helper to parse a show NFO from a reader.
// (Already exists in nfo_parser.go; this is a local alias for convenience.)
var _ = io.Discard // ensure io import is used
