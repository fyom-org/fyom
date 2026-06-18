package repository

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSQLiteDSNUsesPragmaParametersAndDoesNotHTMLEscapeAmpersands(t *testing.T) {
	t.Parallel()

	dsn, err := sqliteDSN(filepath.Join("tmp", "fyom.db"))
	if err != nil {
		t.Fatalf("sqliteDSN() error = %v", err)
	}

	if strings.Contains(dsn, "&amp;") {
		t.Fatalf("sqliteDSN() contains HTML-escaped ampersand: %q", dsn)
	}

	for _, want := range []string{
		"_pragma=busy_timeout%285000%29",
		"_pragma=foreign_keys%28ON%29",
		"_pragma=journal_mode%28WAL%29",
		"_pragma=synchronous%28NORMAL%29",
	} {
		if !strings.Contains(dsn, want) {
			t.Fatalf("sqliteDSN() = %q, missing %q", dsn, want)
		}
	}
}

func TestOpenAppliesSQLitePragmas(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "fyom.db")

	db, err := Open(dbPath, 0, 0, 0)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	ctx := context.Background()

	var journalMode string
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode;").Scan(&journalMode); err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}

	if strings.ToLower(journalMode) != "wal" {
		t.Fatalf("journal_mode = %q, want wal", journalMode)
	}

	var busyTimeout int
	if err := db.QueryRowContext(ctx, "PRAGMA busy_timeout;").Scan(&busyTimeout); err != nil {
		t.Fatalf("query busy_timeout: %v", err)
	}

	if busyTimeout < defaultSQLiteBusyTimeoutMS {
		t.Fatalf("busy_timeout = %d, want at least %d", busyTimeout, defaultSQLiteBusyTimeoutMS)
	}

	var foreignKeys int
	if err := db.QueryRowContext(ctx, "PRAGMA foreign_keys;").Scan(&foreignKeys); err != nil {
		t.Fatalf("query foreign_keys: %v", err)
	}

	if foreignKeys != 1 {
		t.Fatalf("foreign_keys = %d, want 1", foreignKeys)
	}

	var synchronous int
	if err := db.QueryRowContext(ctx, "PRAGMA synchronous;").Scan(&synchronous); err != nil {
		t.Fatalf("query synchronous: %v", err)
	}

	if synchronous != 1 {
		t.Fatalf("synchronous = %d, want 1", synchronous)
	}

	stats := db.Stats()
	if stats.MaxOpenConnections != 1 {
		t.Fatalf("MaxOpenConnections = %d, want 1", stats.MaxOpenConnections)
	}
}

func TestIsMigrationAppliedHandlesErrNoRows(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "fyom.db")

	db, err := Open(dbPath, 0, 0, 0)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer db.Close()

	applied, err := db.isMigrationApplied(context.Background(), 999999)
	if err != nil {
		t.Fatalf("isMigrationApplied() error = %v", err)
	}

	if applied {
		t.Fatalf("isMigrationApplied() = true, want false")
	}
}

func TestIsSQLiteBusy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil",
			err:  nil,
			want: false,
		},
		{
			name: "database is locked",
			err:  errors.New("database is locked (5) (SQLITE_BUSY)"),
			want: true,
		},
		{
			name: "database table is locked",
			err:  errors.New("database table is locked"),
			want: true,
		},
		{
			name: "sqlite busy",
			err:  errors.New("SQLITE_BUSY"),
			want: true,
		},
		{
			name: "other error",
			err:  sql.ErrNoRows,
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := IsSQLiteBusy(tt.err)
			if got != tt.want {
				t.Fatalf("IsSQLiteBusy() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWithSQLiteBusyRetryEventuallySucceeds(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	attempts := 0

	err := WithSQLiteBusyRetry(ctx, func() error {
		attempts++
		if attempts < 3 {
			return errors.New("database is locked (5) (SQLITE_BUSY)")
		}

		return nil
	})
	if err != nil {
		t.Fatalf("WithSQLiteBusyRetry() error = %v", err)
	}

	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
}
