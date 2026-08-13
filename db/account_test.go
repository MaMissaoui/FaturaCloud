package db

import "testing"

// TestUpdateAccountRejectsRetypeWithPostedHistory is F51's regression test:
// once an account has posted journal_lines, UpdateAccount must refuse to
// change its type or isGroup — either would retroactively reclassify past
// P&L/balance sheet numbers, or (for isGroup) contradict postings that
// already exist against what allocateAndFinalizeEntryTx treats as a leaf.
func TestUpdateAccountRejectsRetypeWithPostedHistory(t *testing.T) {
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-acct-retype")
	inv := fx.createInvoice(t, d, "acct-retype-inv-1", 1, 1000)
	if _, err := d.UpdateInvoiceState(inv.ID, "sent"); err != nil {
		t.Fatalf("UpdateInvoiceState(sent): %v", err)
	}

	revenueAccount, err := d.GetAccount(fx.revenueAccountID)
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}

	// Changing type is rejected.
	if _, err := d.UpdateAccount(revenueAccount.ID, UpdateAccountRequest{
		ParentID: revenueAccount.ParentID, Code: revenueAccount.Code, Name: revenueAccount.Name,
		Type: "expense", IsGroup: revenueAccount.IsGroup, IsActive: revenueAccount.IsActive,
	}); err == nil {
		t.Fatal("expected retyping an account with posted history to be rejected")
	}

	// Changing isGroup is rejected too, type held constant.
	if _, err := d.UpdateAccount(revenueAccount.ID, UpdateAccountRequest{
		ParentID: revenueAccount.ParentID, Code: revenueAccount.Code, Name: revenueAccount.Name,
		Type: revenueAccount.Type, IsGroup: 1, IsActive: revenueAccount.IsActive,
	}); err == nil {
		t.Fatal("expected regrouping an account with posted history to be rejected")
	}

	// An unrelated field (name) still updates freely.
	updated, err := d.UpdateAccount(revenueAccount.ID, UpdateAccountRequest{
		ParentID: revenueAccount.ParentID, Code: revenueAccount.Code, Name: "Renamed Revenue",
		Type: revenueAccount.Type, IsGroup: revenueAccount.IsGroup, IsActive: revenueAccount.IsActive,
	})
	if err != nil {
		t.Fatalf("expected a non-retyping update to succeed, got: %v", err)
	}
	if updated.Name != "Renamed Revenue" {
		t.Fatalf("name = %q, want %q", updated.Name, "Renamed Revenue")
	}
}

// TestUpdateAccountAllowsRetypeWithoutHistory confirms the guard is scoped
// to accounts with actual usage — a freshly created, never-posted account
// must still be freely retypable (e.g. fixing a mis-categorized new account).
func TestUpdateAccountAllowsRetypeWithoutHistory(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-acct-retype-free"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	acct, err := d.CreateAccount(CreateAccountRequest{
		OrganizationID: org.ID, Code: "9999", Name: "Scratch", Type: "asset", IsGroup: 0,
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}

	updated, err := d.UpdateAccount(acct.ID, UpdateAccountRequest{
		Code: acct.Code, Name: acct.Name, Type: "expense", IsGroup: 0, IsActive: 1,
	})
	if err != nil {
		t.Fatalf("expected retyping an unused account to succeed, got: %v", err)
	}
	if updated.Type != "expense" {
		t.Fatalf("type = %q, want expense", updated.Type)
	}
}

// TestUpdateAccountRejectsMakingAnAccountWithChildrenALeaf covers the other
// half of F51: an account that already has child accounts under it can't be
// turned into isGroup=0 (a leaf), which would leave those children pointing
// at a parent that's no longer structurally a group.
func TestUpdateAccountRejectsMakingAnAccountWithChildrenALeaf(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-acct-children"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	parent, err := d.CreateAccount(CreateAccountRequest{
		OrganizationID: org.ID, Code: "9000", Name: "Group", Type: "asset", IsGroup: 1,
	})
	if err != nil {
		t.Fatalf("CreateAccount (parent): %v", err)
	}
	if _, err := d.CreateAccount(CreateAccountRequest{
		OrganizationID: org.ID, ParentID: &parent.ID, Code: "9001", Name: "Child", Type: "asset", IsGroup: 0,
	}); err != nil {
		t.Fatalf("CreateAccount (child): %v", err)
	}

	if _, err := d.UpdateAccount(parent.ID, UpdateAccountRequest{
		Code: parent.Code, Name: parent.Name, Type: parent.Type, IsGroup: 0, IsActive: 1,
	}); err == nil {
		t.Fatal("expected making a parent-with-children account a leaf to be rejected")
	}
}

// TestUpdateAccountValidatesParentID covers F51's parentId check: a
// nonexistent parent must be a clean validation error, not an opaque FK
// 500, and an account can't be set as its own parent.
func TestUpdateAccountValidatesParentID(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-acct-parent"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	acct, err := d.CreateAccount(CreateAccountRequest{
		OrganizationID: org.ID, Code: "9100", Name: "Leaf", Type: "asset", IsGroup: 0,
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}

	bogus := "does-not-exist"
	if _, err := d.UpdateAccount(acct.ID, UpdateAccountRequest{
		ParentID: &bogus, Code: acct.Code, Name: acct.Name, Type: acct.Type, IsGroup: acct.IsGroup, IsActive: 1,
	}); err == nil {
		t.Fatal("expected a nonexistent parentId to be rejected")
	}

	if _, err := d.UpdateAccount(acct.ID, UpdateAccountRequest{
		ParentID: &acct.ID, Code: acct.Code, Name: acct.Name, Type: acct.Type, IsGroup: acct.IsGroup, IsActive: 1,
	}); err == nil {
		t.Fatal("expected an account being set as its own parent to be rejected")
	}
}

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
