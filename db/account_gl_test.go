package db

import "testing"

// TestCreateOrganizationSeedsInventoryAccounts confirms a brand-new
// organization's chart of accounts includes Phase 7's four inventory/COGS
// accounts, wired to the matching default*AccountId columns, in the same
// seeding pass as the original nine.
func TestCreateOrganizationSeedsInventoryAccounts(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-inv-seed", Name: ptr("Inventory Seed Test Org")})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if org.DefaultInventoryAccountID == nil || org.DefaultGRNIAccountID == nil ||
		org.DefaultCOGSAccountID == nil || org.DefaultInventoryAdjustmentAccountID == nil {
		t.Fatalf("expected all four Phase 7 default accounts to be wired, got %+v", org)
	}

	accounts, err := d.GetAccounts(org.ID)
	if err != nil {
		t.Fatalf("GetAccounts: %v", err)
	}
	byCode := map[string]Account{}
	for _, a := range accounts {
		byCode[a.Code] = a
	}

	inventory, ok := byCode["1150"]
	if !ok || inventory.ID != *org.DefaultInventoryAccountID || inventory.Type != "asset" {
		t.Fatalf("expected 1150 Inventory (asset) wired as defaultInventoryAccountId, got %+v", inventory)
	}
	grni, ok := byCode["2150"]
	if !ok || grni.ID != *org.DefaultGRNIAccountID || grni.Type != "liability" {
		t.Fatalf("expected 2150 GRNI (liability) wired as defaultGRNIAccountId, got %+v", grni)
	}
	cogs, ok := byCode["5150"]
	if !ok || cogs.ID != *org.DefaultCOGSAccountID || cogs.Type != "expense" {
		t.Fatalf("expected 5150 COGS (expense) wired as defaultCOGSAccountId, got %+v", cogs)
	}
	adj, ok := byCode["5900"]
	if !ok || adj.ID != *org.DefaultInventoryAdjustmentAccountID || adj.Type != "expense" {
		t.Fatalf("expected 5900 Inventory Adjustment (expense) wired as defaultInventoryAdjustmentAccountId, got %+v", adj)
	}

	// Every new account is parented correctly under the existing top-level groups.
	assets, liabilities, expenses := byCode["1000"], byCode["2000"], byCode["5000"]
	if inventory.ParentID == nil || *inventory.ParentID != assets.ID {
		t.Fatalf("expected Inventory parented under Assets")
	}
	if grni.ParentID == nil || *grni.ParentID != liabilities.ID {
		t.Fatalf("expected GRNI parented under Liabilities")
	}
	if cogs.ParentID == nil || *cogs.ParentID != expenses.ID {
		t.Fatalf("expected COGS parented under Expenses")
	}
	if adj.ParentID == nil || *adj.ParentID != expenses.ID {
		t.Fatalf("expected Inventory Adjustment parented under Expenses")
	}

	// 5100 is now the general (non-inventory) purchases account, renamed.
	if byCode["5100"].Name != "General Purchases" {
		t.Fatalf("expected 5100 renamed to General Purchases, got %q", byCode["5100"].Name)
	}
}

// TestSeedInventoryAccountingDefaultsForAllOrganizationsBackfillsExistingOrgs
// is the regression test for Finding A: an organization that ran Phases 1-6
// before Phase 7 shipped already has a non-empty chart, so
// SeedAccountingDefaultsForAllOrganizations's account-count gate never
// re-enters seeding for it. SeedInventoryAccountingDefaultsForAllOrganizations
// must independently backfill the four new accounts for such an org.
func TestSeedInventoryAccountingDefaultsForAllOrganizationsBackfillsExistingOrgs(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-inv-backfill", Name: ptr("Backfill Test Org")})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	// Simulate a pre-Phase-7 organization: clear the default wiring first
	// (or the accounts delete below would fail its own FOREIGN KEY
	// constraint), then strip the four new accounts, as if this org's chart
	// predates their existence.
	if _, err := d.DB.Exec(`
		UPDATE organizations SET defaultInventoryAccountId = NULL, defaultGRNIAccountId = NULL,
		    defaultCOGSAccountId = NULL, defaultInventoryAdjustmentAccountId = NULL
		WHERE id = ?`, org.ID,
	); err != nil {
		t.Fatalf("simulate pre-phase-7 clear defaults: %v", err)
	}
	if _, err := d.DB.Exec(`DELETE FROM accounts WHERE organizationId = ? AND code IN ('1150','2150','5150','5900')`, org.ID); err != nil {
		t.Fatalf("simulate pre-phase-7 delete: %v", err)
	}

	if err := d.SeedInventoryAccountingDefaultsForAllOrganizations(); err != nil {
		t.Fatalf("SeedInventoryAccountingDefaultsForAllOrganizations: %v", err)
	}

	backfilled, err := d.GetOrganization(org.ID)
	if err != nil {
		t.Fatalf("GetOrganization: %v", err)
	}
	if backfilled.DefaultInventoryAccountID == nil || backfilled.DefaultGRNIAccountID == nil ||
		backfilled.DefaultCOGSAccountID == nil || backfilled.DefaultInventoryAdjustmentAccountID == nil {
		t.Fatalf("expected backfill to wire all four defaults, got %+v", backfilled)
	}

	// The original chart (e.g. 1100 Accounts Receivable) is untouched.
	accounts, err := d.GetAccounts(org.ID)
	if err != nil {
		t.Fatalf("GetAccounts: %v", err)
	}
	found1100 := false
	for _, a := range accounts {
		if a.Code == "1100" {
			found1100 = true
		}
	}
	if !found1100 {
		t.Fatal("expected the organization's original chart to remain untouched by the backfill")
	}

	// Idempotent: running it again is a no-op (the gate excludes this org now).
	if err := d.SeedInventoryAccountingDefaultsForAllOrganizations(); err != nil {
		t.Fatalf("SeedInventoryAccountingDefaultsForAllOrganizations (second run): %v", err)
	}
	accountsAfter, err := d.GetAccounts(org.ID)
	if err != nil {
		t.Fatalf("GetAccounts: %v", err)
	}
	if len(accountsAfter) != len(accounts) {
		t.Fatalf("expected a second backfill run to be a no-op, account count changed from %d to %d", len(accounts), len(accountsAfter))
	}
}

// TestSeedInventoryAccountsSkipsCodeCollision confirms a backfill doesn't
// fail an organization's whole seeding pass just because it already has its
// own account at one of the four new codes — it skips that one insert and
// leaves the matching default*AccountId column unset (NULL), rather than
// silently overwriting the organization's own account or aborting entirely.
func TestSeedInventoryAccountsSkipsCodeCollision(t *testing.T) {
	d := newTestDB(t)
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-inv-collision", Name: ptr("Collision Test Org")})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	// Simulate: this org predates Phase 7, and its own chart happens to
	// already use code "1150" for something else entirely. Clear the
	// default wiring before deleting the seeded accounts (FOREIGN KEY).
	if _, err := d.DB.Exec(`
		UPDATE organizations SET defaultInventoryAccountId = NULL, defaultGRNIAccountId = NULL,
		    defaultCOGSAccountId = NULL, defaultInventoryAdjustmentAccountId = NULL
		WHERE id = ?`, org.ID,
	); err != nil {
		t.Fatalf("simulate pre-phase-7 clear defaults: %v", err)
	}
	if _, err := d.DB.Exec(`DELETE FROM accounts WHERE organizationId = ? AND code IN ('1150','2150','5150','5900')`, org.ID); err != nil {
		t.Fatalf("simulate pre-phase-7 delete: %v", err)
	}
	customAccount, err := d.CreateAccount(CreateAccountRequest{
		OrganizationID: org.ID, Code: "1150", Name: "Custom Prepaid Expenses", Type: "asset",
	})
	if err != nil {
		t.Fatalf("CreateAccount (custom 1150): %v", err)
	}

	if err := d.SeedInventoryAccountingDefaultsForAllOrganizations(); err != nil {
		t.Fatalf("SeedInventoryAccountingDefaultsForAllOrganizations: %v", err)
	}

	backfilled, err := d.GetOrganization(org.ID)
	if err != nil {
		t.Fatalf("GetOrganization: %v", err)
	}
	if backfilled.DefaultInventoryAccountID != nil {
		t.Fatalf("expected defaultInventoryAccountId to stay unset after a code collision, got %v", *backfilled.DefaultInventoryAccountID)
	}
	// The other three (no collision) still get wired.
	if backfilled.DefaultGRNIAccountID == nil || backfilled.DefaultCOGSAccountID == nil || backfilled.DefaultInventoryAdjustmentAccountID == nil {
		t.Fatalf("expected the three non-colliding defaults to still be wired, got %+v", backfilled)
	}

	// The organization's own custom 1150 account is untouched.
	stillThere, err := d.GetAccount(customAccount.ID)
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	if stillThere.Name != "Custom Prepaid Expenses" {
		t.Fatalf("expected the organization's own 1150 account to be untouched, got %+v", stillThere)
	}
}
