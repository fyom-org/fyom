package model

// ProviderRecord is the persisted representation of a configurable provider.
// LocalProvider is NOT stored in this table — it is always registered at startup.
type ProviderRecord struct {
	ID          string `json:"id"           db:"id"`
	Type        string `json:"type"         db:"type"`
	DisplayName string `json:"display_name" db:"display_name"`
	Config      string `json:"config"       db:"config"` // raw JSON
	Enabled     bool   `json:"enabled"      db:"enabled"`
	CreatedAt   string `json:"created_at"   db:"created_at"`
	UpdatedAt   string `json:"updated_at"   db:"updated_at"`
}
