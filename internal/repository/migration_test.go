package repository

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// setupMigrationTestDB opens a fresh DB and runs all migrations.
func setupMigrationTestDB(t *testing.T) *DB {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "fyom.db")
	db, err := Open(dbPath, 5, 2, 60)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = db.Close()
		_ = os.RemoveAll(tmpDir)
	})
	return db
}

// TestMigration_EmptyToLatest verifies all migrations apply cleanly from
// an empty database and the final schema includes Phase 8 columns.
func TestMigration_EmptyToLatest(t *testing.T) {
	db := setupMigrationTestDB(t)
	ctx := context.Background()

	// Verify media_items table exists with Phase 8 columns
	rows, err := db.QueryContext(ctx, "PRAGMA table_info(media_items)")
	require.NoError(t, err)
	defer rows.Close()

	columns := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull, pk int
		var dflt sql.NullString
		require.NoError(t, rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk))
		columns[name] = true
	}

	// Spot-check Phase 8 / 0015 columns
	assert.True(t, columns["set_overview"], "set_overview column should exist")
	assert.True(t, columns["language"], "language column should exist")
	assert.True(t, columns["country_code"], "country_code column should exist")
	assert.True(t, columns["custom_rating"], "custom_rating column should exist")
	assert.True(t, columns["collection_number"], "collection_number column should exist")
	assert.True(t, columns["end_date"], "end_date column should exist")
	assert.True(t, columns["release_date"], "release_date column should exist")
	assert.True(t, columns["display_order"], "display_order column should exist")
	assert.True(t, columns["original_title"], "original_title column should exist")
	assert.True(t, columns["user_rating"], "user_rating column should exist")
	assert.True(t, columns["date_added"], "date_added column should exist")
	assert.True(t, columns["last_played"], "last_played column should exist")
	assert.True(t, columns["playcount"], "playcount column should exist")

	// Verify earlier columns still exist
	assert.True(t, columns["id"], "id column should exist")
	assert.True(t, columns["title"], "title column should exist")
	assert.True(t, columns["type"], "type column should exist")
	assert.True(t, columns["year"], "year column should exist")
	assert.True(t, columns["file_path"], "file_path column should exist")
	assert.True(t, columns["mpaa"], "mpaa column should exist")
	assert.True(t, columns["genres"], "genres column should exist")
	assert.True(t, columns["actors"], "actors column should exist")
	assert.True(t, columns["video_codec"], "video_codec column should exist")
	assert.True(t, columns["logo_path"], "logo_path column should exist")

	// Verify schema_migrations table has all 18 migrations applied
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 18, count, "all 18 migrations should be applied")
}

// TestMigration_PrePhase8ToLatest simulates a database at schema version 0011
// (just before Phase 8's first migration 0012) and runs remaining migrations.
func TestMigration_PrePhase8ToLatest(t *testing.T) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/fyom.db"
	dsn := "file:" + dbPath + "?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL"

	sqlDB, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = sqlDB.Close()
		_ = os.RemoveAll(tmpDir)
	})

	// Create schema_migrations table manually
	_, err = sqlDB.Exec(`CREATE TABLE schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`)
	require.NoError(t, err)

	// Run migrations 0001 through 0011 manually
	migrationDir := findMigrationsDir()
	require.NotEmpty(t, migrationDir, "migrations directory must be found")

	for v := 1; v <= 11; v++ {
		name := ""
		entries, _ := os.ReadDir(migrationDir)
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), padInt(v)+"_") && strings.HasSuffix(e.Name(), ".up.sql") {
				name = e.Name()
				break
			}
		}
		require.NotEmpty(t, name, "migration %d not found", v)

		content, err := os.ReadFile(migrationDir + "/" + name)
		require.NoError(t, err)

		_, err = sqlDB.Exec(string(content))
		require.NoError(t, err, "migration %d should apply cleanly", v)

		_, err = sqlDB.Exec("INSERT INTO schema_migrations (version) VALUES (?)", v)
		require.NoError(t, err)
	}

	// Insert a sample media_items row using only pre-Phase-8 columns
	_, err = sqlDB.Exec(`INSERT INTO media_items (id, type, title, year, file_path, metadata_source)
		VALUES (?, ?, ?, ?, ?, ?)`,
		"test-1", "movie", "Test Movie", 2020, "/test.mkv", "filename")
	require.NoError(t, err)

	// Now open via the normal path — this should run remaining migrations (0012-0016)
	sqlDB.Close()
	db, err := Open(dbPath, 5, 2, 60)
	require.NoError(t, err)
	defer db.Close()

	ctx := context.Background()

	// Verify the pre-existing row survived
	var title string
	err = db.QueryRowContext(ctx, "SELECT title FROM media_items WHERE id = ?", "test-1").Scan(&title)
	require.NoError(t, err)
	assert.Equal(t, "Test Movie", title)

	// Verify new columns exist and have defaults
	var setOverview, language, countryCode string
	err = db.QueryRowContext(ctx,
		"SELECT set_overview, language, country_code FROM media_items WHERE id = ?",
		"test-1").Scan(&setOverview, &language, &countryCode)
	require.NoError(t, err)
	assert.Equal(t, "", setOverview, "set_overview should default to empty string")
	assert.Equal(t, "", language, "language should default to empty string")
	assert.Equal(t, "", countryCode, "country_code should default to empty string")

	// Verify schema_migrations has all 18 entries
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 18, count, "all 18 migrations should be applied")
}

// padInt zero-pads an integer to 4 digits for migration filename matching.
func padInt(n int) string {
	return fmt.Sprintf("%04d", n)
}
