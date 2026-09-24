package db

import (
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// TestMigration0089BackfillsTunisianLayout steps a migrated database back to
// 0088, inserts organizations with pre-0089 invoiceLayout values, and
// re-applies 0089: Tunisian organizations with no layout keep the Facture
// layout they were already getting, an explicit choice is never overridden,
// and the inert 'custom' value becomes NULL.
func TestMigration0089BackfillsTunisianLayout(t *testing.T) {
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
	if err := m.Migrate(88); err != nil {
		t.Fatalf("migrate down to 0088: %v", err)
	}

	for _, org := range []struct {
		id, country, countryCode string
		layout                   any
	}{
		{"tn-code", "", "TN", nil},
		{"tn-code-lower", "", " tn ", ""},
		{"tn-name", "Tunisia", "", nil},
		{"tn-name-fr", "Tunisie", "", nil},
		{"tn-explicit-default", "", "TN", "default"},
		{"tn-custom", "", "TN", "custom"},
		{"de", "Germany", "DE", nil},
		{"de-custom", "Germany", "DE", "custom"},
		{"de-tunisia", "Germany", "DE", "tunisia"},
	} {
		if _, err := d.DB.Exec(
			`INSERT INTO organizations (id, name, country, country_code, invoiceLayout) VALUES (?, ?, ?, ?, ?)`,
			org.id, org.id, org.country, org.countryCode, org.layout,
		); err != nil {
			t.Fatalf("insert %s: %v", org.id, err)
		}
	}

	if err := m.Migrate(89); err != nil {
		t.Fatalf("migrate up to 0089: %v", err)
	}

	want := map[string]*string{
		"tn-code":             ptr("tunisia"),
		"tn-code-lower":       ptr("tunisia"),
		"tn-name":             ptr("tunisia"),
		"tn-name-fr":          ptr("tunisia"),
		"tn-explicit-default": ptr("default"),
		// 'custom' is normalized to NULL first, then backfilled like any
		// other unset Tunisian organization.
		"tn-custom":  ptr("tunisia"),
		"de":         nil,
		"de-custom":  nil,
		"de-tunisia": ptr("tunisia"),
	}
	for id, w := range want {
		var got *string
		if err := d.DB.Get(&got, `SELECT documentLayout FROM organizations WHERE id = ?`, id); err != nil {
			t.Fatalf("read %s: %v", id, err)
		}
		if (got == nil) != (w == nil) || (got != nil && *got != *w) {
			t.Errorf("%s: documentLayout = %v, want %v", id, deref(got), deref(w))
		}
	}
}

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}
