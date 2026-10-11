package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// MigratedSnapshot builds one fully-migrated SQLite database in a temporary
// directory and returns its raw bytes. It exists for tests: newTestDB
// (db_test.go) and db/dbtest each call it once per test binary and stamp
// out a fresh copy for each test instead of running every migration
// through golang-migrate each time. Measured under -race: NewDatabase
// (open + migrate) costs ~1.44s/call — every migration statement is
// race-instrumented — versus ~20ms/call to copy these bytes into a fresh
// file and let migrate.Up() no-op against an already-current schema
// (runMigrations already treats migrate.ErrNoChange as success, so this
// needs zero changes to db.go). Across 250+ top-level db package tests
// that's the dominant cost in the suite, not disk I/O — an earlier
// synchronous=OFF experiment on the test DSN made no measurable difference,
// which is what pointed at -race's per-statement instrumentation instead of
// fsync as the actual bottleneck.
//
// Reuses Backup (VACUUM INTO), the same primitive db.go already uses for
// production backups, rather than a raw file copy of a WAL-mode database —
// VACUUM INTO produces a single consistent file safe to read even while the
// source is open in WAL mode.
//
// One function shared by both helpers (audit F166), so a stronger check
// here can't reach one copy and miss the other.
func MigratedSnapshot() ([]byte, error) {
	dir, err := os.MkdirTemp("", "fatura-golden-*")
	if err != nil {
		return nil, fmt.Errorf("golden migrated db: mkdir: %w", err)
	}
	defer os.RemoveAll(dir)

	source, err := NewDatabase(filepath.Join(dir, "golden.db"))
	if err != nil {
		return nil, fmt.Errorf("golden migrated db: source: %w", err)
	}
	defer source.Close()
	wantTables, err := listSchemaTables(source.DB.DB)
	if err != nil {
		return nil, fmt.Errorf("golden migrated db: list source tables: %w", err)
	}

	snapshotPath := filepath.Join(dir, "golden-snapshot.db")
	if err := source.Backup(snapshotPath); err != nil {
		return nil, fmt.Errorf("golden migrated db: snapshot: %w", err)
	}

	// Guard against VACUUM INTO ever silently dropping schema: several
	// tests in this file (e.g.
	// TestResetOrganizationDataCoversEveryOrganizationScopedTable,
	// TestVendorDocumentCountCoversEveryReference) are tripwires that read
	// sqlite_master/pragma_table_info against whatever newTestDB hands
	// them — a schema-incomplete snapshot would make those pass while
	// checking nothing, silently, instead of failing loudly here once.
	snapshot, err := sql.Open("sqlite", snapshotPath)
	if err != nil {
		return nil, fmt.Errorf("golden migrated db: open snapshot: %w", err)
	}
	defer snapshot.Close()
	gotTables, err := listSchemaTables(snapshot)
	if err != nil {
		return nil, fmt.Errorf("golden migrated db: list snapshot tables: %w", err)
	}
	if !slices.Equal(wantTables, gotTables) {
		return nil, fmt.Errorf(
			"golden migrated db: snapshot schema mismatch — want tables %v, got %v",
			wantTables, gotTables,
		)
	}

	snapshotBytes, err := os.ReadFile(snapshotPath)
	if err != nil {
		return nil, fmt.Errorf("golden migrated db: read snapshot: %w", err)
	}
	return snapshotBytes, nil
}

func listSchemaTables(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, name)
	}
	return tables, rows.Err()
}
