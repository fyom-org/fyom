package model

// Library represents a media library — an organizational unit that binds
// storage, metadata rules, and access permissions together.
type Library struct {
	ID             string `json:"id" db:"id"`
	Name           string `json:"name" db:"name"`
	Type           string `json:"type" db:"type"`                       // movie | show | mixed
	ProviderID     string `json:"provider_id" db:"provider_id"`
	SourcePath     string `json:"source_path" db:"source_path"`
	MetadataSource string `json:"metadata_source" db:"metadata_source"` // nfo | filename
	CreatedAt      string `json:"created_at" db:"created_at"`
	UpdatedAt      string `json:"updated_at" db:"updated_at"`
}
