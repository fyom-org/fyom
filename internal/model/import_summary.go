package model

import "time"

// ImportSummary captures the results of an import scan — counts, warnings,
// and timing. Returned by ImportLibrary and stored in the import_jobs row
// for admin inspection.
type ImportSummary struct {
	ScannedFiles  int           `json:"scanned_files"`
	ImportedItems int           `json:"imported_items"`
	UpdatedItems  int           `json:"updated_items"`
	SkippedFiles  int           `json:"skipped_files"`
	ParseWarnings []string      `json:"parse_warnings"`
	Duration     time.Duration `json:"duration_ms"`
}
