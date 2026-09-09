package db

import "testing"

// The three-mode behavior of ResetOrganizationData (neither flag rejected,
// transactional-only leaves master data untouched, master data forces
// transactional data along with it) is already covered by
// TestResetOrganizationData in db_test.go. This file covers what that one
// doesn't: the two properties issue #154 flagged as untested and
// consequential enough to regress silently — organization_users surviving a
// reset, and one organization's reset never touching another's data.

func seedResetUser(t *testing.T, d *Database, id string) {
	t.Helper()
	if _, err := d.DB.Exec(
		`INSERT INTO users (id, email, passwordHash, displayName) VALUES (?, ?, ?, ?)`,
		id, id+"@test.local", "unused-hash", id,
	); err != nil {
		t.Fatalf("seed user %q: %v", id, err)
	}
}

// resetTestSeed mirrors TestResetOrganizationData's own seed closure in
// db_test.go (client + vendor + product + tax rate + invoice + stock
// movement) — kept as a local copy rather than exported, since it's small
// and this is the only other place that needs it.
func resetTestSeed(t *testing.T, d *Database, orgID string) {
	t.Helper()
	client, err := d.CreateClient(CreateClientRequest{
		ID: orgID + "-client", OrganizationID: orgID, Name: ptr("Client"),
	})
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	if _, err := d.CreateVendor(CreateVendorRequest{
		ID: orgID + "-vendor", OrganizationID: orgID, Name: ptr("Vendor"),
	}); err != nil {
		t.Fatalf("CreateVendor: %v", err)
	}
	product, err := d.CreateProduct(CreateProductRequest{
		ID: orgID + "-product", OrganizationID: orgID, Name: "Widget",
		Type: "product", StockEnabled: 1,
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	if _, err := d.CreateTaxRate(CreateTaxRateRequest{
		ID: orgID + "-tax", OrganizationID: orgID, Name: "VAT", Percentage: 20,
	}); err != nil {
		t.Fatalf("CreateTaxRate: %v", err)
	}
	if _, err := d.CreateInvoice(CreateInvoiceRequest{
		ID: orgID + "-inv", OrganizationID: orgID, Number: "INV-001",
		ClientID: client.ID, Date: 1700000000000, Currency: "EUR",
		Total: 5000, SubTotal: 5000,
		LineItems: []CreateInvoiceLineItemRequest{{Quantity: 1, UnitPrice: 5000}},
	}); err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}
	if _, err := d.CreateStockMovement(CreateStockMovementRequest{
		ID: orgID + "-move", OrganizationID: orgID, ProductID: product.ID,
		Type: "in", Quantity: 10,
	}); err != nil {
		t.Fatalf("CreateStockMovement: %v", err)
	}
}

// TestResetOrganizationDataPreservesOrganizationUsers pins the exact risk
// #154 named: organization_users is access control, not the organization's
// own data, and must survive even the maximal reset (master data, which
// forces transactional data along with it — see ResetOrganizationData's own
// comment). Worth contrasting with DeleteOrganization, which DOES cascade
// organization_users away (ON DELETE CASCADE on organizationId) — a reset
// must not have the same effect, or resetting an org's data would silently
// lock out every one of its members.
func TestResetOrganizationDataPreservesOrganizationUsers(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)

	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	resetTestSeed(t, d, org.ID)
	seedResetUser(t, d, "user-1")
	if _, err := d.AddOrganizationUser(org.ID, "user-1", "admin"); err != nil {
		t.Fatalf("AddOrganizationUser: %v", err)
	}

	if _, err := d.ResetOrganizationData(org.ID, ResetOrganizationDataRequest{ResetMasterData: true}); err != nil {
		t.Fatalf("ResetOrganizationData: %v", err)
	}

	role, isMember, err := d.GetOrganizationRole(org.ID, "user-1")
	if err != nil {
		t.Fatalf("GetOrganizationRole after reset: %v", err)
	}
	if !isMember || role != "admin" {
		t.Errorf("expected the membership to survive the reset with its role intact, got role=%q isMember=%v", role, isMember)
	}
}

// TestResetOrganizationDataIsolatesOtherOrganizations seeds two organizations
// with identical fixtures and resets only the first (master data, the
// maximal mode — clears transactional data, master data, GL account
// defaults, invoice_number_counter, and stock quantities). Every one of
// those operations is scoped by "WHERE organizationId = ?" or, for the
// account-defaults UPDATE, "WHERE id = ?" — a missing or wrong predicate on
// any of them would silently affect every organization instead of the one
// requested, which the existing single-organization subtests in
// TestResetOrganizationData can't catch.
func TestResetOrganizationDataIsolatesOtherOrganizations(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)

	orgA, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-a"})
	if err != nil {
		t.Fatalf("CreateOrganization org-a: %v", err)
	}
	orgB, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-b"})
	if err != nil {
		t.Fatalf("CreateOrganization org-b: %v", err)
	}
	resetTestSeed(t, d, orgA.ID)
	resetTestSeed(t, d, orgB.ID)

	// Give org B a GL account default too, so a reset that wrongly clears
	// every organization's defaults instead of just org A's would show up —
	// seedDefaultChartOfAccounts already wires this on creation, but assert
	// it's actually set before relying on it as a signal.
	orgBBefore, err := d.GetOrganization(orgB.ID)
	if err != nil {
		t.Fatalf("GetOrganization org-b before reset: %v", err)
	}
	if orgBBefore.DefaultRevenueAccountID == nil {
		t.Fatal("expected org-b to have a default revenue account seeded, got nil")
	}

	if _, err := d.ResetOrganizationData(orgA.ID, ResetOrganizationDataRequest{ResetMasterData: true}); err != nil {
		t.Fatalf("ResetOrganizationData org-a: %v", err)
	}

	for _, table := range []string{"invoices", "stockMovements", "clients", "vendors", "products", "taxRates"} {
		var n int64
		if err := d.DB.Get(&n, `SELECT COUNT(*) FROM `+table+` WHERE organizationId = ?`, orgB.ID); err != nil {
			t.Fatalf("count %s for org-b: %v", table, err)
		}
		if n == 0 {
			t.Errorf("org-b's %s: got 0 rows after resetting org-a, want its own data untouched", table)
		}
	}

	orgBAfter, err := d.GetOrganization(orgB.ID)
	if err != nil {
		t.Fatalf("GetOrganization org-b after reset: %v", err)
	}
	if orgBAfter.DefaultRevenueAccountID == nil {
		t.Error("org-b's defaultRevenueAccountId was cleared by resetting org-a")
	}
	if orgBAfter.InvoiceNumberCounter == nil || *orgBAfter.InvoiceNumberCounter == 0 {
		t.Errorf("org-b's invoice_number_counter was reset by resetting org-a: %v", orgBAfter.InvoiceNumberCounter)
	}

	product, err := d.GetProduct(orgB.ID + "-product")
	if err != nil {
		t.Fatalf("GetProduct org-b: %v", err)
	}
	if product.StockQuantity == 0 {
		t.Error("org-b's product stockQuantity was zeroed by resetting org-a")
	}
}
