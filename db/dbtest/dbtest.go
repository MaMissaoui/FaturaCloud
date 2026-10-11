// Package dbtest gives tests outside the db package a fully migrated
// database without paying for the migrations on every test.
//
// The full migration set runs once per test binary (db.MigratedSnapshot,
// the same builder db's own newTestDB uses), and every test gets its own
// copy of those bytes, against which NewDatabase's migrate.Up() is a no-op.
// Under -race, migrating from scratch costs ~2.4s per database (every
// migration statement is race-instrumented); copying the snapshot costs
// milliseconds. The db package can't use this package itself — dbtest
// imports db, so db's own tests importing it would be an import cycle.
package dbtest

import (
	"os"
	"sync"
	"testing"

	"github.com/MaMissaoui/fatura-cloud/db"
)

var goldenMigratedDB = sync.OnceValues(db.MigratedSnapshot)

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
