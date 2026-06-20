// Package repository provides database access for fyom.
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	// Register the modernc sqlite driver for database/sql.
	_ "modernc.org/sqlite"
)

const (
	sqliteDriverName = "sqlite"

	defaultSQLiteBusyTimeoutMS     = 5000
	defaultSQLiteWALAutocheckpoint = 1000
	defaultSQLiteMaxOpenConns      = 1
	defaultSQLiteMaxIdleConns      = 1
	defaultSQLiteOpenTimeout       = 10 * time.Second
	defaultSQLiteMigrationTimeout  = 60 * time.Second
)

// DB wraps sql.DB with migration support.
type DB struct {
	*sql.DB
}

// Open initializes a SQLite database, runs migrations, and configures the pool.
//
// dbPath must be the absolute or relative path to the SQLite database file.
// Parent directories are created automatically if they do not exist.
//
// SQLite runtime tuning intentionally lives here instead of migrations because
// PRAGMA settings such as busy_timeout, foreign_keys, and synchronous are
// connection-level settings. Migrations should only manage schema state.
func Open(dbPath string, maxOpen, maxIdle, maxLifetimeSec int) (*DB, error) {
	if strings.TrimSpace(dbPath) == "" {
		return nil, fmt.Errorf("database path is required")
	}

	if err := ensureDatabaseParentDir(dbPath); err != nil {
		return nil, err
	}

	dsn := sqliteDSN(dbPath)

	sqlDB, err := sql.Open(sqliteDriverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	configureSQLitePool(sqlDB, maxOpen, maxIdle, maxLifetimeSec)

	ctx, cancel := context.WithTimeout(context.Background(), defaultSQLiteOpenTimeout)
	defer cancel()

	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	if err := configureSQLite(ctx, sqlDB); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("configure sqlite: %w", err)
	}

	db := &DB{sqlDB}

	migrationCtx, migrationCancel := context.WithTimeout(context.Background(), defaultSQLiteMigrationTimeout)
	defer migrationCancel()

	if err := db.migrate(migrationCtx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return db, nil
}

// sqliteDSN builds a modernc.org/sqlite DSN.
//
// Do not HTML-escape this string. The query separator must be "&", not "&amp;".
// modernc.org/sqlite supports repeated _pragma query parameters.
func sqliteDSN(dbPath string) string {
	if strings.HasPrefix(dbPath, "file:") {
		return dbPath
	}

	u := &url.URL{
		Scheme: "file",
		Path:   dbPath,
	}

	q := u.Query()
	q.Add("_pragma", fmt.Sprintf("busy_timeout(%d)", defaultSQLiteBusyTimeoutMS))
	q.Add("_pragma", "foreign_keys(ON)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "synchronous(NORMAL)")
	q.Add("_pragma", fmt.Sprintf("wal_autocheckpoint(%d)", defaultSQLiteWALAutocheckpoint))
	u.RawQuery = q.Encode()

	return u.String()
}

// ensureDatabaseParentDir creates the parent directory for the SQLite database
// file when dbPath is a normal filesystem path.
//
// file: DSNs are left untouched because they may contain SQLite URI parameters.
func ensureDatabaseParentDir(dbPath string) error {
	if strings.HasPrefix(dbPath, "file:") {
		return nil
	}

	dir := filepath.Dir(dbPath)
	if dir == "." || dir == "" {
		return nil
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create database directory %q: %w", dir, err)
	}

	return nil
}

// configureSQLitePool sets conservative database/sql pool settings for SQLite.
//
// SQLite is not a client/server database. Multiple database/sql connections can
// amplify file-lock contention, especially when media streaming reads and
// playback progress writes happen at the same time. The safest default is one
// open connection unless callers intentionally pass stricter positive values.
func configureSQLitePool(db *sql.DB, maxOpen, maxIdle, maxLifetimeSec int) {
	if maxOpen <= 0 {
		maxOpen = defaultSQLiteMaxOpenConns
	}

	if maxIdle <= 0 {
		maxIdle = defaultSQLiteMaxIdleConns
	}

	if maxOpen > 1 {
		// Keep SQLite stable by default. Higher concurrency should be introduced
		// only with a deliberate read/write split and per-connection PRAGMAs.
		maxOpen = defaultSQLiteMaxOpenConns
	}

	if maxIdle > maxOpen {
		maxIdle = maxOpen
	}

	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)

	if maxLifetimeSec > 0 {
		db.SetConnMaxLifetime(time.Duration(maxLifetimeSec) * time.Second)
	} else {
		db.SetConnMaxLifetime(0)
	}

	db.SetConnMaxIdleTime(0)
}

// configureSQLite applies runtime SQLite settings and verifies the critical
// PRAGMAs that protect media streaming from transient write-lock failures.
func configureSQLite(ctx context.Context, db *sql.DB) error {
	pragmas := []string{
		"PRAGMA journal_mode = WAL;",
		"PRAGMA synchronous = NORMAL;",
		fmt.Sprintf("PRAGMA busy_timeout = %d;", defaultSQLiteBusyTimeoutMS),
		"PRAGMA foreign_keys = ON;",
		fmt.Sprintf("PRAGMA wal_autocheckpoint = %d;", defaultSQLiteWALAutocheckpoint),
	}

	for _, stmt := range pragmas {
		if err := WithSQLiteBusyRetry(ctx, func() error {
			_, execErr := db.ExecContext(ctx, stmt)
			return execErr
		}); err != nil {
			return fmt.Errorf("apply %q: %w", stmt, err)
		}
	}

	if err := verifySQLitePragmas(ctx, db); err != nil {
		return err
	}

	return nil
}

// verifySQLitePragmas fails fast when critical SQLite runtime settings are not
// active. This makes deployment issues such as read-only directories, invalid
// DSNs, or unsupported URI parameters visible during startup.
func verifySQLitePragmas(ctx context.Context, db *sql.DB) error {
	var journalMode string
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode;").Scan(&journalMode); err != nil {
		return fmt.Errorf("verify journal_mode: %w", err)
	}

	if strings.ToLower(journalMode) != "wal" {
		return fmt.Errorf("verify journal_mode: got %q, want %q", journalMode, "wal")
	}

	var busyTimeout int
	if err := db.QueryRowContext(ctx, "PRAGMA busy_timeout;").Scan(&busyTimeout); err != nil {
		return fmt.Errorf("verify busy_timeout: %w", err)
	}

	if busyTimeout < defaultSQLiteBusyTimeoutMS {
		return fmt.Errorf("verify busy_timeout: got %d, want at least %d", busyTimeout, defaultSQLiteBusyTimeoutMS)
	}

	var foreignKeys int
	if err := db.QueryRowContext(ctx, "PRAGMA foreign_keys;").Scan(&foreignKeys); err != nil {
		return fmt.Errorf("verify foreign_keys: %w", err)
	}

	if foreignKeys != 1 {
		return fmt.Errorf("verify foreign_keys: got %d, want 1", foreignKeys)
	}

	var synchronous int
	if err := db.QueryRowContext(ctx, "PRAGMA synchronous;").Scan(&synchronous); err != nil {
		return fmt.Errorf("verify synchronous: %w", err)
	}

	// SQLite returns 1 for NORMAL and 2 for FULL.
	if synchronous != 1 {
		return fmt.Errorf("verify synchronous: got %d, want 1", synchronous)
	}

	return nil
}

// IsSQLiteBusy reports whether err represents a transient SQLite lock failure.
func IsSQLiteBusy(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())

	return strings.Contains(msg, "database is locked") ||
		strings.Contains(msg, "database table is locked") ||
		strings.Contains(msg, "sqlite_busy") ||
		strings.Contains(msg, "sql logic error: database is locked") ||
		strings.Contains(msg, "constraint failed: database is locked")
}

// WithSQLiteBusyRetry retries a lightweight database operation when SQLite is
// temporarily busy. It is intended for short metadata reads/writes such as
// progress updates and migration bookkeeping, not for long-running operations.
func WithSQLiteBusyRetry(ctx context.Context, fn func() error) error {
	delays := []time.Duration{
		25 * time.Millisecond,
		50 * time.Millisecond,
		100 * time.Millisecond,
		200 * time.Millisecond,
		400 * time.Millisecond,
	}

	var lastErr error

	for attempt := 0; attempt <= len(delays); attempt++ {
		if err := fn(); err != nil {
			lastErr = err

			if !IsSQLiteBusy(err) {
				return err
			}

			if attempt == len(delays) {
				break
			}

			timer := time.NewTimer(delays[attempt])
			select {
			case <-ctx.Done():
				timer.Stop()
				return errors.Join(ctx.Err(), lastErr)
			case <-timer.C:
			}

			continue
		}

		return nil
	}

	return lastErr
}

// migrate reads SQL files from the migrations directory and applies pending ones.
func (db *DB) migrate(ctx context.Context) error {
	if err := WithSQLiteBusyRetry(ctx, func() error {
		_, execErr := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
            version INTEGER PRIMARY KEY,
            applied_at TEXT NOT NULL DEFAULT (datetime('now'))
        )`)
		return execErr
	}); err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}

	migrationDir := findMigrationsDir()
	if migrationDir == "" {
		return nil
	}

	entries, err := os.ReadDir(migrationDir)
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	migrations := collectMigrations(migrationDir, entries)

	for _, m := range migrations {
		applied, err := db.isMigrationApplied(ctx, m.version)
		if err != nil {
			return fmt.Errorf("check migration %d: %w", m.version, err)
		}

		if applied {
			continue
		}

		if err := db.applyMigration(ctx, m); err != nil {
			return err
		}
	}

	return nil
}

type migration struct {
	version int
	path    string
}

func collectMigrations(migrationDir string, entries []os.DirEntry) []migration {
	migrations := make([]migration, 0, len(entries))

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}

		versionPart := strings.SplitN(name, "_", 2)[0]
		version, err := strconv.Atoi(versionPart)
		if err != nil || version <= 0 {
			continue
		}

		migrations = append(migrations, migration{
			version: version,
			path:    filepath.Join(migrationDir, name),
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].version < migrations[j].version
	})

	return migrations
}

func (db *DB) isMigrationApplied(ctx context.Context, version int) (bool, error) {
	var dummy int

	err := db.QueryRowContext(ctx, "SELECT 1 FROM schema_migrations WHERE version = ?", version).Scan(&dummy)
	if err == nil {
		return true, nil
	}

	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}

	if IsSQLiteBusy(err) {
		var applied bool

		retryErr := WithSQLiteBusyRetry(ctx, func() error {
			retryErr := db.QueryRowContext(ctx, "SELECT 1 FROM schema_migrations WHERE version = ?", version).Scan(&dummy)
			if retryErr == nil {
				applied = true
				return nil
			}

			if errors.Is(retryErr, sql.ErrNoRows) {
				applied = false
				return nil
			}

			return retryErr
		})
		if retryErr != nil {
			return false, retryErr
		}

		return applied, nil
	}

	return false, err
}

func (db *DB) applyMigration(ctx context.Context, m migration) error {
	content, err := os.ReadFile(m.path)
	if err != nil {
		return fmt.Errorf("read migration %d: %w", m.version, err)
	}

	if err := WithSQLiteBusyRetry(ctx, func() error {
		conn, connErr := db.Conn(ctx)
		if connErr != nil {
			return connErr
		}
		defer func() {
			if cerr := conn.Close(); cerr != nil {
				_ = cerr // best-effort close; migration already committed or rolled back
			}
		}()

		committed := false

		// Acquire the write lock early so migration lock failures happen before
		// partially executing migration SQL.
		if _, execErr := conn.ExecContext(ctx, "BEGIN IMMEDIATE;"); execErr != nil {
			return execErr
		}

		defer func() {
			if !committed {
				_, _ = conn.ExecContext(context.Background(), "ROLLBACK;")
			}
		}()

		if _, execErr := conn.ExecContext(ctx, string(content)); execErr != nil {
			return execErr
		}

		if _, execErr := conn.ExecContext(ctx, "INSERT INTO schema_migrations (version) VALUES (?)", m.version); execErr != nil {
			return execErr
		}

		if _, commitErr := conn.ExecContext(ctx, "COMMIT;"); commitErr != nil {
			return commitErr
		}

		committed = true
		return nil
	}); err != nil {
		return fmt.Errorf("apply migration %d: %w", m.version, err)
	}

	return nil
}

// findMigrationsDir locates the migrations directory.
// It first tries relative paths, then falls back to searching from the executable's directory.
func findMigrationsDir() string {
	candidates := []string{
		"migrations",
		"../migrations",
		"../../migrations",
		"../../../migrations",
	}

	for _, dir := range candidates {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}

	exePath, err := os.Executable()
	if err == nil {
		exeDir := filepath.Dir(exePath)

		candidates := []string{
			filepath.Join(exeDir, "migrations"),
			filepath.Join(exeDir, "..", "migrations"),
			filepath.Join(exeDir, "..", "..", "migrations"),
		}

		for _, dir := range candidates {
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				return dir
			}
		}
	}

	return ""
}
