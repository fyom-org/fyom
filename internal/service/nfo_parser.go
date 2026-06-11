package service

import (
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/fyom/fyom/internal/model"
)

// ParseShowNFO parses a tvshow.nfo file.
func ParseShowNFO(r io.Reader) (*model.NFOTVShow, error) {
	var show model.NFOTVShow
	if err := xml.NewDecoder(r).Decode(&show); err != nil {
		return nil, err
	}
	return &show, nil
}

// ParseEpisodeNFO parses an episode NFO file.
func ParseEpisodeNFO(r io.Reader) (*model.NFOEpisode, error) {
	var ep model.NFOEpisode
	if err := xml.NewDecoder(r).Decode(&ep); err != nil {
		return nil, err
	}
	return &ep, nil
}

// ParseEpisodeNFOs parses a multi-episode NFO file containing multiple
// <episodedetails> elements. Falls back to single-episode parsing if
// the multi-episode approach finds nothing.
func ParseEpisodeNFOs(r io.Reader) ([]model.NFOEpisode, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	var episodes []model.NFOEpisode
	content := string(data)
	parts := strings.Split(content, "<episodedetails")

	for i, part := range parts {
		if i == 0 {
			continue
		}
		block := "<episodedetails" + part
		var ep model.NFOEpisode
		if err := xml.Unmarshal([]byte(block), &ep); err == nil {
			if ep.Title != "" {
				episodes = append(episodes, ep)
			}
		}
	}

	if len(episodes) == 0 {
		var ep model.NFOEpisode
		if err := xml.Unmarshal(data, &ep); err != nil {
			return nil, err
		}
		if ep.Title != "" {
			episodes = append(episodes, ep)
		}
	}

	return episodes, nil
}

// ParseMovieNFO parses a movie NFO file.
func ParseMovieNFO(r io.Reader) (*model.NFOMovie, error) {
	var movie model.NFOMovie
	if err := xml.NewDecoder(r).Decode(&movie); err != nil {
		return nil, err
	}
	return &movie, nil
}

// fileExists checks if a path exists on disk.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// FindPosterPath resolves the poster path for a media item, checking both the NFO
// metadata and common filename conventions on disk.
func FindPosterPath(dir, baseName, nfoPoster string) string {
	// 1. NFO <thumb aspect="poster"> path (may be relative)
	if nfoPoster != "" {
		if filepath.IsAbs(nfoPoster) && fileExists(nfoPoster) {
			return nfoPoster
		}
		candidate := filepath.Join(dir, nfoPoster)
		if fileExists(candidate) {
			return candidate
		}
	}

	// 2. Common poster filenames in priority order
	posterNames := []string{
		"folder.jpg", "folder.png", "folder.jpeg",
		baseName + "-poster.jpg", baseName + "-poster.png", baseName + "-poster.jpeg",
		"poster.jpg", "poster.png", "poster.jpeg",
		"cover.jpg", "cover.png",
		"show.jpg", "show.png",
		baseName + ".jpg", baseName + ".png", baseName + ".jpeg",
	}
	for _, name := range posterNames {
		candidate := filepath.Join(dir, name)
		if fileExists(candidate) {
			return candidate
		}
	}
	return ""
}

// FindBackdropPath resolves the backdrop path, checking both NFO and disk.
func FindBackdropPath(dir, baseName, nfoBackdrop string) string {
	if nfoBackdrop != "" {
		if filepath.IsAbs(nfoBackdrop) && fileExists(nfoBackdrop) {
			return nfoBackdrop
		}
		candidate := filepath.Join(dir, nfoBackdrop)
		if fileExists(candidate) {
			return candidate
		}
	}

	backdropNames := []string{
		baseName + "-backdrop.jpg", baseName + "-backdrop.png",
		baseName + "-fanart.jpg", baseName + "-fanart.png",
		"backdrop.jpg", "backdrop.png", "backdrop.jpeg",
		"fanart.jpg", "fanart.png", "fanart.jpeg",
		"background.jpg", "background.png",
	}
	for _, name := range backdropNames {
		candidate := filepath.Join(dir, name)
		if fileExists(candidate) {
			return candidate
		}
	}
	return ""
}

// FindEpisodeThumbnailPath looks for an episode thumbnail like S01E01-thumb.jpg.
func FindEpisodeThumbnailPath(dir, baseName, nfoThumb string) string {
	if nfoThumb != "" {
		if filepath.IsAbs(nfoThumb) && fileExists(nfoThumb) {
			return nfoThumb
		}
		candidate := filepath.Join(dir, nfoThumb)
		if fileExists(candidate) {
			return candidate
		}
	}

	thumbNames := []string{
		baseName + "-thumb.jpg", baseName + "-thumb.png",
		baseName + "-landscape.jpg", baseName + "-landscape.png",
	}
	for _, name := range thumbNames {
		candidate := filepath.Join(dir, name)
		if fileExists(candidate) {
			return candidate
		}
	}
	return ""
}
