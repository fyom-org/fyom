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

func (fs *LocalImportFS) Open(_ context.Context, name string) (io.ReadCloser, error) {
	return os.Open(name)
}

func (fs *LocalImportFS) Exists(_ context.Context, name string) bool {
	_, err := os.Stat(name)
	return err == nil
}

func (fs *LocalImportFS) Join(elem ...string) string {
	return filepath.Join(elem...)
}
