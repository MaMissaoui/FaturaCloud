package db

import (
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// TestMigration0091BackfillsDocumentLanguage steps a migrated database back
// to 0090, inserts organizations, and re-applies 0091: Tunisian
// organizations (by layout or country) keep the French amount-in-words line
// they were already printing, German- and French-speaking countries get
// their language, and everyone else stays NULL (English on the default
// layout).
func TestMigration0091BackfillsDocumentLanguage(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)

	sourceDriver, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		t.Fatalf("migration source: %v", err)
	}
	dbDriver, err := sqlite.WithInstance(d.DB.DB, &sqlite.Config{})
	if err != nil {
		t.Fatalf("migration db driver: %v", err)
	}
	m, err := migrate.NewWithInstance("iofs", sourceDriver, "sqlite", dbDriver)
	if err != nil {
		t.Fatalf("migrator: %v", err)
	}
	if err := m.Migrate(90); err != nil {
		t.Fatalf("migrate down to 0090: %v", err)
	}

	for _, org := range []struct {
		id, country, countryCode string
		layout                   any
	}{
		{"tn-layout", "", "", "tunisia"},
		{"tn-code", "", " tn ", nil},
		{"tn-name", "Tunisie", "", nil},
		{"fr", "France", "FR", nil},
		{"de", "", "DE", nil},
		{"at-name", "Österreich", "", nil},
		{"de-tunisia", "Germany", "DE", "tunisia"},
		{"us", "United States", "US", nil},
		{"blank", "", "", nil},
	} {
		if _, err := d.DB.Exec(
			`INSERT INTO organizations (id, name, country, country_code, documentLayout) VALUES (?, ?, ?, ?, ?)`,
			org.id, org.id, org.country, org.countryCode, org.layout,
		); err != nil {
			t.Fatalf("insert %s: %v", org.id, err)
		}
	}

	if err := m.Migrate(91); err != nil {
		t.Fatalf("migrate up to 0091: %v", err)
	}

	want := map[string]*string{
		"tn-layout":  ptr("fr"),
		"tn-code":    ptr("fr"),
		"tn-name":    ptr("fr"),
		"fr":         ptr("fr"),
		"de":         ptr("de"),
		"at-name":    ptr("de"),
		"de-tunisia": ptr("fr"), // the Tunisian layout's labels are French
		"us":         nil,
		"blank":      nil,
	}
	for id, w := range want {
		var got *string
		if err := d.DB.Get(&got, `SELECT documentLanguage FROM organizations WHERE id = ?`, id); err != nil {
			t.Fatalf("read %s: %v", id, err)
		}
		if (got == nil) != (w == nil) || (got != nil && *got != *w) {
			t.Errorf("%s: documentLanguage = %v, want %v", id, deref(got), deref(w))
		}
	}
}
