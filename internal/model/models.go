// Package model defines the core domain types used across fyom.
package model

// User represents a fyom user.
type User struct {
	ID                     string `json:"id" db:"id"`
	Username               string `json:"username" db:"username"`
	Password               string `json:"-" db:"password"`
	Role                   string `json:"role" db:"role"`
	PasswordChangeRequired bool   `json:"password_change_required" db:"password_change_required"`
	PreferredLanguage      string `json:"preferred_language" db:"preferred_language"`
	CreatedAt              string `json:"created_at" db:"created_at"`
	UpdatedAt              string `json:"updated_at" db:"updated_at"`
}

// MediaItem represents a single media entry (movie, episode, or show).
type MediaItem struct {
	ID                   string  `json:"id" db:"id"`
	Type                 string  `json:"type" db:"type"`
	Title                string  `json:"title" db:"title"`
	SortTitle            string  `json:"sort_title,omitempty" db:"sort_title"`
	Year                 int     `json:"year,omitempty" db:"year"`
	Overview             string  `json:"overview,omitempty" db:"overview"`
	Rating               float64 `json:"rating,omitempty" db:"rating"`
	Duration             int     `json:"duration,omitempty" db:"duration"`
	FilePath             string  `json:"file_path" db:"file_path"`
	RootPath             string  `json:"root_path" db:"root_path"`
	PrimaryPath          string  `json:"primary_path" db:"primary_path"`
	NFOPath              string  `json:"nfo_path" db:"nfo_path"`
	PosterPath           string  `json:"poster_path,omitempty" db:"poster_path"`
	BackdropPath         string  `json:"backdrop_path,omitempty" db:"backdrop_path"`
	ParentID             string  `json:"parent_id,omitempty" db:"parent_id"`
	Season               *int    `json:"season,omitempty" db:"season"`
	Episode              *int    `json:"episode,omitempty" db:"episode"`
	MetadataSource       string  `json:"metadata_source,omitempty" db:"metadata_source"`
	ProviderID           string  `json:"provider_id" db:"provider_id"`
	LibraryID            string  `json:"library_id" db:"library_id"`
	Status               string  `json:"status" db:"status"`
	CreatedAt            string  `json:"created_at" db:"created_at"`
	UpdatedAt            string  `json:"updated_at" db:"updated_at"`
	MPAA                 string  `json:"mpaa,omitempty" db:"mpaa"`
	Genres               string  `json:"genres,omitempty" db:"genres"`
	Studios              string  `json:"studios,omitempty" db:"studios"`
	Actors               string  `json:"actors,omitempty" db:"actors"`
	UniqueIDs            string  `json:"unique_ids,omitempty" db:"unique_ids"`
	Premiered            string  `json:"premiered,omitempty" db:"premiered"`
	Outline              string  `json:"outline,omitempty" db:"outline"`
	Tagline              string  `json:"tagline,omitempty" db:"tagline"`
	Countries            string  `json:"countries,omitempty" db:"countries"`
	Directors            string  `json:"directors,omitempty" db:"directors"`
	Credits              string  `json:"credits,omitempty" db:"credits"`
	Tags                 string  `json:"tags,omitempty" db:"tags"`
	SetName              string  `json:"set_name,omitempty" db:"set_name"`
	SetOverview          string  `json:"set_overview,omitempty" db:"set_overview"`
	VideoCodec           string  `json:"video_codec,omitempty" db:"video_codec"`
	VideoWidth           int     `json:"video_width,omitempty" db:"video_width"`
	VideoHeight          int     `json:"video_height,omitempty" db:"video_height"`
	VideoDurationSeconds int     `json:"video_duration_seconds,omitempty" db:"video_duration_seconds"`
	AudioCodec           string  `json:"audio_codec,omitempty" db:"audio_codec"`
	AudioChannels        int     `json:"audio_channels,omitempty" db:"audio_channels"`
	SubtitleLanguages    string  `json:"subtitle_languages,omitempty" db:"subtitle_languages"`
	Aired                string  `json:"aired,omitempty" db:"aired"`
	LogoPath             string  `json:"logo_path,omitempty" db:"logo_path"`
	Language             string  `json:"language,omitempty" db:"language"`
	CountryCode          string  `json:"country_code,omitempty" db:"country_code"`
	CustomRating         string  `json:"custom_rating,omitempty" db:"custom_rating"`
	CollectionNumber     string  `json:"collection_number,omitempty" db:"collection_number"`
	EndDate              string  `json:"end_date,omitempty" db:"end_date"`
	ReleaseDate          string  `json:"release_date,omitempty" db:"release_date"`
	DisplayOrder         string  `json:"display_order,omitempty" db:"display_order"`
	OriginalTitle        string  `json:"original_title,omitempty" db:"original_title"`
	UserRating           float64 `json:"user_rating,omitempty" db:"user_rating"`
	DateAdded            string  `json:"date_added,omitempty" db:"date_added"`
	LastPlayed           string  `json:"last_played,omitempty" db:"last_played"`
	Playcount            int     `json:"playcount,omitempty" db:"playcount"`
}

// ImportJob tracks an import operation.
type ImportJob struct {
	ID            string   `json:"id" db:"id"`
	SourcePath    string   `json:"source_path" db:"source_path"`
	Status        string   `json:"status" db:"status"`
	TotalItems    int      `json:"total_items" db:"total_items"`
	DoneItems     int      `json:"done_items" db:"done_items"`
	LibraryID     string   `json:"library_id" db:"library_id"`
	ErrorMsg      string   `json:"error_msg,omitempty" db:"error_msg"`
	ScannedFiles  int      `json:"scanned_files" db:"scanned_files"`
	ImportedItems int      `json:"imported_items" db:"imported_items"`
	UpdatedItems  int      `json:"updated_items" db:"updated_items"`
	SkippedFiles  int      `json:"skipped_files" db:"skipped_files"`
	ParseWarnings []string `json:"parse_warnings" db:"-"`
	DurationMS    int64    `json:"duration_ms" db:"duration_ms"`
	CreatedAt     string   `json:"created_at" db:"created_at"`
	UpdatedAt     string   `json:"updated_at" db:"updated_at"`
}

// VersionInfo holds build-time version data.
type VersionInfo struct {
	Version   string `json:"version"`
	GitCommit string `json:"git_commit"`
	BuildTime string `json:"build_time"`
	GoVersion string `json:"go_version"`
}
