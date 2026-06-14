package service

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fyom/fyom/internal/model"
)

// ── JSON helpers ──────────────────────────────────────────────────────────

func stringsToJSON(ss []string) string {
	if len(ss) == 0 {
		return ""
	}
	b, _ := json.Marshal(ss)
	return string(b)
}

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

func hasUniqueID(ids []model.NFOUniqueID, idType string) bool {
	for _, id := range ids {
		if id.Type == idType {
			return true
		}
	}
	return false
}

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

// ── NFO discovery helpers ────────────────────────────────────────────────

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
			continue
		}
		ext := ""
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
		ext := ""
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

// ── Path/name helpers ────────────────────────────────────────────────────

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

func cleanTitle(s string) string {
	s = strings.ReplaceAll(s, ".", " ")
	s = strings.ReplaceAll(s, "_", " ")
	return strings.TrimSpace(s)
}

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

func extractNFOEpisodeThumb(thumbs []model.NFOThumb) string {
	for _, t := range thumbs {
		if t.URL != "" {
			return t.URL
		}
	}
	return ""
}

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

func trimExt(name string) string {
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		return name[:idx]
	}
	return name
}

// getShowRootPath returns the show root path for an episode candidate.
// It walks up from the episode's directory to find the show root.
func (imp *Importer) getShowRootPath(c *ImportCandidate) string {
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

// ── Misc helpers ──────────────────────────────────────────────────────────

func (imp *Importer) buildRejectWarnings(rejected []RejectedItem) []string {
	warnings := make([]string, 0, len(rejected))
	for _, r := range rejected {
		warnings = append(warnings, fmt.Sprintf("%s: %s", r.Path, r.Reason))
	}
	return warnings
}
