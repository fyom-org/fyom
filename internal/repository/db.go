// Package repository provides database access for fyom.
package repository

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	// Register the modernc sqlite driver for database/sql.
	_ "modernc.org/sqlite"
)

// DB wraps sql.DB with migration support.
type DB struct {
	*sql.DB
}

// Open initializes a SQLite database, runs migrations, and configures the pool.
// dbPath must be the absolute or relative path to the SQLite database file.
// Parent directories are created automatically if they do not exist.
func Open(dbPath string, maxOpen, maxIdle, maxLifetimeSec int) (*DB, error) {
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL", dbPath)

	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetMaxIdleConns(maxIdle)
	sqlDB.SetConnMaxLifetime(time.Duration(maxLifetimeSec) * time.Second)

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	db := &DB{sqlDB}

	if err := db.migrate(); err != nil {
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return db, nil
}

// migrate reads SQL files from the migrations directory and applies pending ones.
func (db *DB) migrate() error {
	// Create migration tracking table
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`)
	if err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}

	// Locate migrations directory
	migrationDir := findMigrationsDir()
	if migrationDir == "" {
		// No migrations directory found — skip
		return nil
	}

	entries, err := os.ReadDir(migrationDir)
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	// Collect and sort .up.sql files
	type migration struct {
		version int
		path    string
	}
	var migrations []migration
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		version, err := strconv.Atoi(strings.SplitN(name, "_", 2)[0])
		if err != nil || version == 0 {
			continue
		}
		migrations = append(migrations, migration{version, filepath.Join(migrationDir, name)})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].version < migrations[j].version
	})

	// Apply pending migrations in a transaction
	for _, m := range migrations {
		var dummy int
		err := db.QueryRow("SELECT 1 FROM schema_migrations WHERE version = ?", m.version).Scan(&dummy)
		if err == nil {
			continue // already applied
		}

		content, err := os.ReadFile(m.path)
		if err != nil {
			return fmt.Errorf("read migration %d: %w", m.version, err)
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin transaction: %w", err)
		}

		if _, err := tx.Exec(string(content)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %d: %w", m.version, err)
		}

		if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", m.version); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %d: %w", m.version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", m.version, err)
		}
	}

	return nil
}

// findMigrationsDir locates the migrations directory relative to the working directory.
func findMigrationsDir() string {
	candidates := []string{
		"migrations",
		"../migrations",
		"../../migrations",
	}
	for _, dir := range candidates {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}
	return ""
}
