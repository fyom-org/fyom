package server

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRefreshCoordinator_TryStart_Success(t *testing.T) {
	c := NewRefreshCoordinator()
	assert.True(t, c.TryStart("lib-1"), "first start should succeed")
	assert.True(t, c.IsRunning("lib-1"), "library should be marked as running")
}

func TestRefreshCoordinator_TryStart_DuplicateSkipped(t *testing.T) {
	c := NewRefreshCoordinator()
	assert.True(t, c.TryStart("lib-1"), "first start should succeed")
	assert.False(t, c.TryStart("lib-1"), "second start for same library should be skipped")
}

func TestRefreshCoordinator_DifferentLibrariesIndependent(t *testing.T) {
	c := NewRefreshCoordinator()
	assert.True(t, c.TryStart("lib-1"), "lib-1 should start")
	assert.True(t, c.TryStart("lib-2"), "lib-2 should start independently")
	assert.True(t, c.IsRunning("lib-1"))
	assert.True(t, c.IsRunning("lib-2"))
}

func TestRefreshCoordinator_Finish_ClearsRunning(t *testing.T) {
	c := NewRefreshCoordinator()
	assert.True(t, c.TryStart("lib-1"))
	c.Finish("lib-1")
	assert.False(t, c.IsRunning("lib-1"), "Finish should clear running mark")
	assert.True(t, c.TryStart("lib-1"), "should be able to start again after Finish")
}

func TestRefreshCoordinator_Finish_Idempotent(t *testing.T) {
	c := NewRefreshCoordinator()
	assert.True(t, c.TryStart("lib-1"))
	c.Finish("lib-1")
	c.Finish("lib-1") // double finish should not panic
	assert.False(t, c.IsRunning("lib-1"))
}

func TestRefreshCoordinator_ConcurrentAccess(t *testing.T) {
	c := NewRefreshCoordinator()
	var wg sync.WaitGroup
	const numGoroutines = 100

	// Concurrent TryStart for same library - only one should succeed
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.TryStart("lib-concurrent")
		}()
	}
	wg.Wait()

	// Exactly one should have succeeded
	count := 0
	if c.IsRunning("lib-concurrent") {
		count = 1
	}
	assert.Equal(t, 1, count, "only one goroutine should have acquired the lock")
}

func TestRefreshCoordinator_FinishOnErrorPath(t *testing.T) {
	c := NewRefreshCoordinator()
	assert.True(t, c.TryStart("lib-error2"))
	func() {
		defer c.Finish("lib-error2")
		// normal completion
	}()
	assert.False(t, c.IsRunning("lib-error2"), "Finish via defer should clear running mark")
}
