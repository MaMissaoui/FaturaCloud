package db

import (
	"embed"
	"fmt"
	"os"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Database holds the SQLite connections: DB, the single connection every
// write and transaction goes through, and reads, a small read-only pool the
// heavy list and report reads use through ReadOnly (audit F177).
type Database struct {
	DB     *sqlx.DB
	DBPath string
	reads  *sqlx.DB
}

// readPoolSize is how many connections the read-only pool may open. The
// production Pi 4 has four cores and SQLite here is pure Go, so CPU-bound:
// three readers leave the writer a core.
const readPoolSize = 3

// NewDatabase opens (or creates) the SQLite database at dbPath, runs all
// pending migrations and returns a ready-to-use Database.
func NewDatabase(dbPath string) (*Database, error) {
	// Embed pragmas in the DSN so every connection gets them automatically,
	// regardless of pool size. busy_timeout avoids SQLITE_BUSY under load.
	//
	// synchronous=NORMAL is the standard WAL pairing: it still guarantees a
	// consistent (non-corrupt) database after a crash while skipping the
	// per-commit fsync, so only the last few committed transactions can be
	// lost on a power cut — a conscious trade for a single-file, single-writer
	// deployment (a Raspberry Pi on an SD card). temp_store=MEMORY keeps the
	// planner's many temp B-tree sorts in RAM; cache_size is in KiB (negative
	// = KiB) and mmap_size in bytes.
	pragmas := "?_pragma=foreign_keys(1)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=busy_timeout(5000)" +
		"&_pragma=synchronous(NORMAL)" +
		"&_pragma=temp_store(MEMORY)" +
		"&_pragma=cache_size(-64000)" +
		"&_pragma=mmap_size(268435456)"
	db, err := sqlx.Open("sqlite", dbPath+pragmas)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// SQLite performs best with a single writer connection.
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	d := &Database{DB: db, DBPath: dbPath}
	if err := d.runMigrations(); err != nil {
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	// The read pool opens after the migrations, on the same file and with the
	// same pragmas plus query_only, so a write through it fails at once
	// instead of slipping past the single writer. In WAL mode its readers run
	// alongside the writer and each other, so a counter sale no longer waits
	// behind someone's Dashboard.
	reads, err := sqlx.Open("sqlite", dbPath+pragmas+"&_pragma=query_only(1)")
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("open read pool: %w", err)
	}
	reads.SetMaxOpenConns(readPoolSize)
	if err := reads.Ping(); err != nil {
		reads.Close()
		db.Close()
		return nil, fmt.Errorf("ping read pool: %w", err)
	}
	d.reads = reads

	return d, nil
}

// ReadOnly returns a view of the database whose DB is the read-only pool. It
// is for heavy GET handlers that only read and open no transaction: their
// reads then run beside the writer instead of queueing on its single
// connection. Anything that writes, or reads inside a transaction, keeps using
// the Database itself; a write through the view fails (query_only). Never
// Close the view.
func (d *Database) ReadOnly() *Database {
	if d.reads == nil {
		return d
	}
	return &Database{DB: d.reads, DBPath: d.DBPath}
}

// BackupFileMode is the mode of every file the app writes into the backups
// directory (snapshots and the schedule's config.json). Never world-readable
// (F15: a snapshot is the full financial database), but group-readable rather
// than owner-only: on a host that grants a backup user read access through a
// POSIX ACL, the group bits are the ACL mask, so 0600 silently cancels that
// grant. That is how the production Pi's Syncthing (running as a different
// user than the container) stopped seeing every backup.
const BackupFileMode os.FileMode = 0640

// Backup creates a consistent single-file snapshot via VACUUM INTO,
// safe to call while the database is open in WAL mode.
func (d *Database) Backup(destPath string) error {
	_ = os.Remove(destPath) // VACUUM INTO fails if the destination already exists
	if _, err := d.DB.Exec("VACUUM INTO ?", destPath); err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	// VACUUM INTO creates the file with SQLite's default (world-readable) mode —
	// this is a full financial database, take away "other" (see BackupFileMode).
	if err := os.Chmod(destPath, BackupFileMode); err != nil {
		return fmt.Errorf("backup: chmod: %w", err)
	}
	return nil
}

// Close shuts down the connection pool.
func (d *Database) Close() error {
	if d.reads != nil {
		if err := d.reads.Close(); err != nil {
			d.DB.Close()
			return err
		}
	}
	return d.DB.Close()
}

func (d *Database) runMigrations() error {
	sourceDriver, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("create migration source: %w", err)
	}

	dbDriver, err := sqlite.WithInstance(d.DB.DB, &sqlite.Config{})
	if err != nil {
		return fmt.Errorf("create migration db driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", sourceDriver, "sqlite", dbDriver)
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("apply migrations: %w", err)
	}

	return nil
}
