package service

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// LocalImportFS implements ImportFS for the local filesystem.
type LocalImportFS struct{}

// NewLocalImportFS creates a new LocalImportFS.
func NewLocalImportFS() *LocalImportFS { return &LocalImportFS{} }

// ReadDir returns a sorted list of directory entries for the given path.
func (fs *LocalImportFS) ReadDir(_ context.Context, dir string) ([]DirEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	result := make([]DirEntry, 0, len(entries))
	for _, e := range entries {
		result = append(result, DirEntry{Name: e.Name(), IsDir: e.IsDir()})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

// Open opens the named file for reading and returns its contents as a ReadCloser.
func (fs *LocalImportFS) Open(_ context.Context, name string) (io.ReadCloser, error) {
	return os.Open(name)
}

// Exists reports whether the named file or directory exists on the local filesystem.
func (fs *LocalImportFS) Exists(_ context.Context, name string) bool {
	_, err := os.Stat(name)
	return err == nil
}

// Join joins any number of path elements into a single path, separating them with the OS-specific separator.
func (fs *LocalImportFS) Join(elem ...string) string {
	return filepath.Join(elem...)
}
