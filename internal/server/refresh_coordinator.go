package server

import (
	"sync"
)

// RefreshCoordinator prevents duplicate refresh jobs for the same library.
// It tracks which libraries currently have a running refresh/import job.
type RefreshCoordinator struct {
	mu      sync.Mutex
	running map[string]bool
}

// NewRefreshCoordinator creates a new RefreshCoordinator.
func NewRefreshCoordinator() *RefreshCoordinator {
	return &RefreshCoordinator{
		running: make(map[string]bool),
	}
}

// TryStart attempts to mark a library as running a refresh job.
// Returns true if the library was not already running and is now marked as running.
// Returns false if a refresh job for this library is already running.
func (c *RefreshCoordinator) TryStart(libraryID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running[libraryID] {
		return false
	}
	c.running[libraryID] = true
	return true
}

// Finish marks a library's refresh job as completed, allowing new jobs to start.
// Safe to call multiple times; idempotent.
func (c *RefreshCoordinator) Finish(libraryID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.running, libraryID)
}

// IsRunning returns true if a refresh job for the given library is currently running.
func (c *RefreshCoordinator) IsRunning(libraryID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running[libraryID]
}
