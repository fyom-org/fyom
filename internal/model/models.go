// Package model defines the core domain types used across fyom.
package model

// User represents a fyom user.
type User struct {
	ID        string `json:"id" db:"id"`
	Username  string `json:"username" db:"username"`
	Password  string `json:"-" db:"password"`
	Role      string `json:"role" db:"role"`
	CreatedAt string `json:"created_at" db:"created_at"`
	UpdatedAt string `json:"updated_at" db:"updated_at"`
}

// MediaItem represents a single media entry (movie, episode, or show).
type MediaItem struct {
	ID                  string  `json:"id" db:"id"`
	Type                string  `json:"type" db:"type"`
	Title               string  `json:"title" db:"title"`
	SortTitle           string  `json:"sort_title,omitempty" db:"sort_title"`
	Year                int     `json:"year,omitempty" db:"year"`
	Overview            string  `json:"overview,omitempty" db:"overview"`
	Rating              float64 `json:"rating,omitempty" db:"rating"`
	Duration            int     `json:"duration,omitempty" db:"duration"`
	FilePath            string  `json:"file_path" db:"file_path"`
	PosterPath          string  `json:"poster_path,omitempty" db:"poster_path"`
	BackdropPath        string  `json:"backdrop_path,omitempty" db:"backdrop_path"`
	ParentID            string  `json:"parent_id,omitempty" db:"parent_id"`
	Season              *int    `json:"season,omitempty" db:"season"`
	Episode             *int    `json:"episode,omitempty" db:"episode"`
	MetadataSource      string  `json:"metadata_source,omitempty" db:"metadata_source"`
	ProviderID          string  `json:"provider_id" db:"provider_id"`
	LibraryID           string  `json:"library_id" db:"library_id"`
	Status              string  `json:"status" db:"status"`
	CreatedAt           string  `json:"created_at" db:"created_at"`
	UpdatedAt           string  `json:"updated_at" db:"updated_at"`
	MPAA                string  `json:"mpaa,omitempty" db:"mpaa"`
	Genres              string  `json:"genres,omitempty" db:"genres"`
	Studios             string  `json:"studios,omitempty" db:"studios"`
	Actors              string  `json:"actors,omitempty" db:"actors"`
	UniqueIDs           string  `json:"unique_ids,omitempty" db:"unique_ids"`
	Premiered           string  `json:"premiered,omitempty" db:"premiered"`
	Outline             string  `json:"outline,omitempty" db:"outline"`
	Tagline             string  `json:"tagline,omitempty" db:"tagline"`
	Countries           string  `json:"countries,omitempty" db:"countries"`
	Directors           string  `json:"directors,omitempty" db:"directors"`
	Credits             string  `json:"credits,omitempty" db:"credits"`
	Tags                string  `json:"tags,omitempty" db:"tags"`
	SetName             string  `json:"set_name,omitempty" db:"set_name"`
	VideoCodec          string  `json:"video_codec,omitempty" db:"video_codec"`
	VideoWidth          int     `json:"video_width,omitempty" db:"video_width"`
	VideoHeight         int     `json:"video_height,omitempty" db:"video_height"`
	VideoDurationSeconds int    `json:"video_duration_seconds,omitempty" db:"video_duration_seconds"`
	AudioCodec          string  `json:"audio_codec,omitempty" db:"audio_codec"`
	AudioChannels       int     `json:"audio_channels,omitempty" db:"audio_channels"`
	SubtitleLanguages   string  `json:"subtitle_languages,omitempty" db:"subtitle_languages"`
	Aired               string  `json:"aired,omitempty" db:"aired"`
}

// ImportJob tracks an import operation.
type ImportJob struct {
	ID         string `json:"id" db:"id"`
	SourcePath string `json:"source_path" db:"source_path"`
	Status     string `json:"status" db:"status"`
	TotalItems int    `json:"total_items" db:"total_items"`
	DoneItems  int    `json:"done_items" db:"done_items"`
	LibraryID  string `json:"library_id" db:"library_id"`
	ErrorMsg   string `json:"error_msg,omitempty" db:"error_msg"`
	CreatedAt  string `json:"created_at" db:"created_at"`
	UpdatedAt  string `json:"updated_at" db:"updated_at"`
}

// VersionInfo holds build-time version data.
type VersionInfo struct {
	Version   string `json:"version"`
	GitCommit string `json:"git_commit"`
	BuildTime string `json:"build_time"`
	GoVersion string `json:"go_version"`
}
