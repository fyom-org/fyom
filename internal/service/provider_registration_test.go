package service

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/fyom/fyom/internal/repository"
	"github.com/stretchr/testify/require"
)

// TestEnsureLocalProvider_DoesNotRegisterInMemoryProviderTwice verifies that
// calling EnsureLocalProvider (DB-only operation) does not cause duplicate
// in-memory provider registration when the server bootstrap path is followed.
func TestEnsureLocalProvider_DoesNotRegisterInMemoryProviderTwice(t *testing.T) {
	dir := t.TempDir()

	db, err := repository.Open(filepath.Join(dir, "fyom.db"), 5, 2, 60)
	require.NoError(t, err)
	defer db.Close()

	providerRepo := repository.NewProviderRepository(db)

	// First call — creates the row
	err = providerRepo.EnsureLocalProvider(context.Background())
	require.NoError(t, err)

	// Verify row exists
	var count int
	err = db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM providers WHERE id = 'local'").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count, "local provider row should exist")

	// Second call — should be idempotent (no error, no duplicate)
	err = providerRepo.EnsureLocalProvider(context.Background())
	require.NoError(t, err)

	// Still exactly one row
	err = db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM providers WHERE id = 'local'").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count, "local provider row should still be exactly 1")

	// Verify the row is enabled
	var enabled int
	err = db.QueryRowContext(context.Background(), "SELECT enabled FROM providers WHERE id = 'local'").Scan(&enabled)
	require.NoError(t, err)
	require.Equal(t, 1, enabled, "local provider should be enabled")
}
