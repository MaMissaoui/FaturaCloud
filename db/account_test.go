package db

import "testing"

// TestGetAccountUsageCountCoversEveryReference is the same schema-
// introspection tripwire shape as TestVendorDocumentCountCoversEveryReference
// and TestTaxRateUsageCountCoversEveryReference, but for accounts:
// GetAccountUsageCount hand-assembles its query from several differently-
// named columns rather than one shared column name, so nothing else catches
// a future migration adding another *AccountId column (e.g. a new Phase 2/3
// org default, or a new per-product/per-document account override) without
// it being wired into DeleteAccount's guard.
func TestGetAccountUsageCountCoversEveryReference(t *testing.T) {
	d := newTestDB(t)

	// Every {table, column} pair GetAccountUsageCount's hand-built query
	// actually reads. Keep this in sync with db/account.go by hand — there's
	// no way to introspect the query string itself, so this list is the
	// thing that must be updated alongside GetAccountUsageCount.
	covered := map[string]map[string]bool{
		"journal_lines": {"accountId": true},
		"taxRates":      {"outputTaxAccountId": true, "inputTaxAccountId": true},
		"products":      {"revenueAccountId": true, "expenseAccountId": true},
		"payments":      {"bankAccountId": true},
		"organizations": {},
	}
	for _, col := range accountReferencingOrganizationColumns {
		covered["organizations"][col] = true
	}
	// accounts.parentId is a self-reference handled separately by
	// DeleteAccount's child-count check (ErrAccountHasChildren), not by
	// GetAccountUsageCount — it's a structural link, not a "this account is
	// in use elsewhere" reference.
	covered["accounts"] = map[string]bool{"parentId": true}

	tables := []string{}
	if err := d.DB.Select(&tables,
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`,
	); err != nil {
		t.Fatalf("list tables: %v", err)
	}

	for _, table := range tables {
		columns := []struct {
			Name string `db:"name"`
		}{}
		if err := d.DB.Select(&columns, `SELECT name FROM pragma_table_info(?)`, table); err != nil {
			t.Fatalf("pragma_table_info(%s): %v", table, err)
		}
		for _, col := range columns {
			if len(col.Name) <= len("AccountId") || col.Name[len(col.Name)-len("AccountId"):] != "AccountId" {
				continue
			}
			if !covered[table][col.Name] {
				t.Errorf(
					"table %q has a %q column but it's not accounted for in GetAccountUsageCount — "+
						"DeleteAccount's guard would not see its rows; wire it into db/account.go and "+
						"update the covered map in this test",
					table, col.Name,
				)
			}
		}
	}
}
