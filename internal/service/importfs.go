package service

import (
	"context"
	"io"
)

// DirEntry represents a file or directory entry returned by ImportFS.ReadDir.
type DirEntry struct {
	Name  string // base name
	IsDir bool
}

// ImportFS abstracts filesystem operations needed by the media importer.
// Implementations exist for local filesystem and S3-compatible object storage.
// All methods accept context for cancellation — S3 operations are network-bound.
type ImportFS interface {
	// ReadDir lists entries in the given directory/prefix.
	// Returns entries sorted by name. Does NOT recurse into subdirectories.
	ReadDir(ctx context.Context, dir string) ([]DirEntry, error)

	// Open opens the named file for reading.
	Open(ctx context.Context, name string) (io.ReadCloser, error)

	// Exists reports whether the named file exists. Returns false on any error.
	Exists(ctx context.Context, name string) bool

	// Join joins path elements into a single path.
	// For local FS: uses filepath.Join.
	// For S3: uses "/" with prefix normalization.
	Join(elem ...string) string
}
