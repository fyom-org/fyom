package service

import "regexp"

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
