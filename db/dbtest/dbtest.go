// Package dbtest gives tests outside the db package a fully migrated
// database without paying for the migrations on every test.
//
// It mirrors db/db_test.go's goldenMigratedDB/newTestDB: the full migration
// set runs once per test binary, the result is snapshotted with Backup
// (VACUUM INTO), and every test gets its own copy of those bytes, against
// which NewDatabase's migrate.Up() is a no-op. Under -race, migrating from
// scratch costs ~2.4s per database (every migration statement is
// race-instrumented); copying the snapshot costs milliseconds. The db
// package can't use this package itself — dbtest imports db, so db's own
// tests importing it would be an import cycle — hence the two copies.
package dbtest

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/MaMissaoui/fatura-cloud/db"
)

var goldenMigratedDB = sync.OnceValues(func() ([]byte, error) {
	dir, err := os.MkdirTemp("", "fatura-dbtest-golden-*")
	if err != nil {
		return nil, fmt.Errorf("golden migrated db: mkdir: %w", err)
	}
	defer os.RemoveAll(dir)

	source, err := db.NewDatabase(filepath.Join(dir, "golden.db"))
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

	// Same guard as db/db_test.go: a snapshot missing schema must fail here,
	// loudly, rather than let a test that reads the schema pass against it.
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

	return os.ReadFile(snapshotPath)
})

func listSchemaTables(conn *sql.DB) ([]string, error) {
	rows, err := conn.Query(`SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name`)
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

// NewDatabase writes a fully migrated database to dbPath and opens it, as
// db.NewDatabase(dbPath) would on an empty path. The caller owns closing
// it, and chooses dbPath because some tests (restore, backups) need to know
// where the file lives.
func NewDatabase(t testing.TB, dbPath string) *db.Database {
	t.Helper()
	golden, err := goldenMigratedDB()
	if err != nil {
		t.Fatalf("build golden migrated database: %v", err)
	}
	if err := os.WriteFile(dbPath, golden, 0600); err != nil {
		t.Fatalf("write golden database copy: %v", err)
	}
	database, err := db.NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("new database: %v", err)
	}
	return database
}
