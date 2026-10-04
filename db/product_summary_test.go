package db

import (
	"database/sql"
	"errors"
	"testing"
)

// productSummaryFixture inserts the documents and movements the product
// summary reads straight into the tables: the summary is a read path, and the
// cases it has to resolve (a delivery known only by its number, a cancelled
// delivery's "in" restore, a number two deliveries share) are easier to state
// as rows than to provoke through every document's status flow.
func productSummaryFixture(t *testing.T, d *Database) (orgID, productID string) {
	t.Helper()
	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-ps", Currency: ptr("TND")})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := d.DB.Exec(query, args...); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
	day := func(n int64) int64 { return 1767225600000 + n*86400000 } // 2026-01-01 + n days

	exec(`INSERT INTO clients (id, organizationId, name) VALUES ('ps-c1', ?, 'Invoice Client'), ('ps-c2', ?, 'Order Client')`, org.ID, org.ID)
	exec(`INSERT INTO vendors (id, organizationId, name) VALUES ('ps-v1', ?, 'Receipt Vendor'), ('ps-v2', ?, 'Draft Vendor')`, org.ID, org.ID)
	exec(`INSERT INTO products (id, organizationId, name, type, price, unitCost, stockEnabled, stockQuantity) VALUES
		('ps-p', ?, 'Fridge', 'product', 150000, 100000, 1, 5),
		('ps-out', ?, 'Gone', 'product', 1000, 500, 1, 0),
		('ps-low', ?, 'Nearly gone', 'product', 1000, 500, 1, 2),
		('ps-svc', ?, 'Install', 'service', 5000, NULL, 0, 0)`,
		org.ID, org.ID, org.ID, org.ID)

	exec(`INSERT INTO invoices (id, organizationId, number, state, clientId, date) VALUES ('ps-inv', ?, 'FAC-1', 'paid', 'ps-c1', ?)`, org.ID, day(7))
	exec(`INSERT INTO orders (id, organizationId, clientId, orderNumber, orderDate) VALUES ('ps-ord', ?, 'ps-c2', 'ORD-1', ?)`, org.ID, day(1))
	// BL-1 comes from an order: its client is the order's, its own is NULL.
	exec(`INSERT INTO outbound_deliveries (id, organizationId, orderId, deliveryNumber, deliveryDate) VALUES ('ps-bl1', ?, 'ps-ord', 'BL-1', ?)`, org.ID, day(5))
	exec(`INSERT INTO inbound_deliveries (id, organizationId, vendorId, deliveryNumber, deliveryDate) VALUES ('ps-br1', ?, 'ps-v1', 'BR-1', ?)`, org.ID, day(2))
	// DUP-1 is both a delivery and a receipt number; DUP-2 two deliveries'.
	exec(`INSERT INTO outbound_deliveries (id, organizationId, deliveryNumber, deliveryDate, clientId) VALUES
		('ps-dup1-out', ?, 'DUP-1', ?, 'ps-c1'), ('ps-dup2-a', ?, 'DUP-2', ?, 'ps-c1'), ('ps-dup2-b', ?, 'DUP-2', ?, 'ps-c1')`,
		org.ID, day(3), org.ID, day(3), org.ID, day(3))
	exec(`INSERT INTO inbound_deliveries (id, organizationId, vendorId, deliveryNumber, deliveryDate) VALUES ('ps-dup1-in', ?, 'ps-v1', 'DUP-1', ?)`, org.ID, day(3))

	exec(`INSERT INTO stockMovements (id, organizationId, productId, type, quantity, reference, sourceDocumentId, createdAt) VALUES
		('m-inv', ?, 'ps-p', 'out', -1, 'FAC-1', 'ps-inv', '2026-10-01 09:00:00'),
		('m-bl', ?, 'ps-p', 'out', -2, 'BL-1', NULL, '2026-10-01 09:00:00'),
		('m-bl-cancel', ?, 'ps-p', 'in', 2, 'BL-1', NULL, '2026-10-01 09:00:00'),
		('m-br', ?, 'ps-p', 'in', 12, 'BR-1', NULL, '2026-10-01 09:00:00'),
		('m-dup1', ?, 'ps-p', 'out', -1, 'DUP-1', NULL, '2026-01-04 08:00:00'),
		('m-dup2', ?, 'ps-p', 'out', -1, 'DUP-2', NULL, '2026-01-01 08:00:00'),
		('m-manual', ?, 'ps-p', 'adjustment', -1, NULL, NULL, '2026-01-09 12:00:00')`,
		org.ID, org.ID, org.ID, org.ID, org.ID, org.ID, org.ID)

	// The last vendor is the latest approved/paid bill with a line for the
	// product; a later draft bill doesn't count.
	exec(`INSERT INTO incoming_invoices (id, organizationId, vendorId, vendorInvoiceNumber, state, date, currency) VALUES
		('ps-bill', ?, 'ps-v1', 'V-1', 'approved', ?, 'TND'), ('ps-draft', ?, 'ps-v2', 'V-2', 'draft', ?, 'TND')`,
		org.ID, day(2), org.ID, day(20))
	exec(`INSERT INTO incoming_invoice_line_items (id, incomingInvoiceId, productId, description, quantity, unitPrice) VALUES
		('ps-bill-l', 'ps-bill', 'ps-p', 'Fridge', 12, 100000), ('ps-draft-l', 'ps-draft', 'ps-p', 'Fridge', 1, 100000)`)
	return org.ID, "ps-p"
}

func TestProductSummary(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	orgID, productID := productSummaryFixture(t, d)

	summary, err := d.GetProductSummary(orgID, productID)
	if err != nil {
		t.Fatalf("GetProductSummary: %v", err)
	}
	if summary.MovementCount != 7 {
		t.Fatalf("MovementCount = %d, want 7", summary.MovementCount)
	}
	if len(summary.RecentMovements) != productRecentMovements {
		t.Fatalf("got %d recent movements, want %d", len(summary.RecentMovements), productRecentMovements)
	}

	// Newest document date first: the manual adjustment (createdAt Jan 9),
	// the invoice (Jan 8), the delivery and its cancel (Jan 6), then the
	// ambiguous DUP-1 (createdAt Jan 4, since it resolves to no document).
	got := map[string]ProductMovement{}
	order := []string{}
	for _, m := range summary.RecentMovements {
		got[m.ID] = m
		order = append(order, m.ID)
	}
	if order[0] != "m-manual" || order[1] != "m-inv" {
		t.Errorf("order = %v, want m-manual then m-inv first", order)
	}
	if _, ok := got["m-dup1"]; !ok {
		t.Errorf("order = %v, want m-dup1 among the five newest", order)
	}

	inv := got["m-inv"]
	if inv.DocumentKind != "invoice" || inv.CounterpartyKind != "client" || inv.CounterpartyName == nil || *inv.CounterpartyName != "Invoice Client" {
		t.Errorf("invoice movement = %+v", inv)
	}
	// A delivery known only by its number resolves, and takes its order's
	// client; the cancelled delivery's "in" restore resolves to the same one.
	for _, id := range []string{"m-bl", "m-bl-cancel"} {
		m := got[id]
		if m.DocumentKind != "delivery" || m.DocumentID == nil || *m.DocumentID != "ps-bl1" {
			t.Errorf("%s: document = %q %v, want delivery ps-bl1", id, m.DocumentKind, m.DocumentID)
		}
		if m.CounterpartyName == nil || *m.CounterpartyName != "Order Client" {
			t.Errorf("%s: counterparty = %v, want Order Client", id, m.CounterpartyName)
		}
		if m.Date != 1767225600000+5*86400000 {
			t.Errorf("%s: date = %d, want the delivery date", id, m.Date)
		}
	}
	dup := got["m-dup1"]
	if dup.DocumentKind != "" || dup.CounterpartyKind != "" || dup.DocumentID != nil {
		t.Errorf("a number both a delivery and a receipt carry must resolve to nothing, got %+v", dup)
	}
	if dup.Date != 1767513600000 { // 2026-01-04 08:00:00 UTC
		t.Errorf("unresolved movement date = %d, want its createdAt", dup.Date)
	}

	if summary.LastVendor == nil || summary.LastVendor.VendorID != "ps-v1" || summary.LastVendor.IncomingInvoiceID != "ps-bill" {
		t.Errorf("LastVendor = %+v, want ps-v1 from the approved bill", summary.LastVendor)
	}
}

func TestProductMovementResolution(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	orgID, _ := productSummaryFixture(t, d)

	// The receipt and the twice-used DUP-2 are past the five newest; resolve
	// every row directly through the same query by asking for a product
	// whose movements are just those.
	if _, err := d.DB.Exec(`UPDATE stockMovements SET productId = 'ps-low' WHERE id IN ('m-br', 'm-dup2')`); err != nil {
		t.Fatalf("move rows: %v", err)
	}
	summary, err := d.GetProductSummary(orgID, "ps-low")
	if err != nil {
		t.Fatalf("GetProductSummary: %v", err)
	}
	got := map[string]ProductMovement{}
	for _, m := range summary.RecentMovements {
		got[m.ID] = m
	}
	br := got["m-br"]
	if br.DocumentKind != "receipt" || br.CounterpartyKind != "vendor" || br.CounterpartyName == nil || *br.CounterpartyName != "Receipt Vendor" {
		t.Errorf("receipt movement = %+v", br)
	}
	if dup := got["m-dup2"]; dup.DocumentKind != "" || dup.DocumentID != nil {
		t.Errorf("a number two deliveries share must resolve to nothing, got %+v", dup)
	}
	if summary.LastVendor != nil {
		t.Errorf("ps-low has no bill line, LastVendor = %+v", summary.LastVendor)
	}
}

func TestProductSummaries(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	orgID, _ := productSummaryFixture(t, d)

	list, err := d.GetProductSummaries(orgID)
	if err != nil {
		t.Fatalf("GetProductSummaries: %v", err)
	}
	if list.ItemCount != 4 || list.StockTracked != 3 || list.Services != 1 {
		t.Errorf("counts = %+v, want 4 items, 3 tracked, 1 service", list)
	}
	if list.OutOfStock != 1 || list.LowStock != 2 {
		t.Errorf("out = %d low = %d, want 1 and 2 (out of stock counts as low)", list.OutOfStock, list.LowStock)
	}
	valuation, err := d.getStockValuation(orgID)
	if err != nil {
		t.Fatalf("getStockValuation: %v", err)
	}
	if list.StockValue != valuation.Total || list.Units != valuation.Units {
		t.Errorf("headline = %d / %v, want the Dashboard's %d / %v", list.StockValue, list.Units, valuation.Total, valuation.Units)
	}
	if list.StockValue != 5*100000+2*500 {
		t.Errorf("StockValue = %d, want %d", list.StockValue, 5*100000+2*500)
	}
	if list.InventoryValuation != InventoryValuationPerpetual {
		t.Errorf("InventoryValuation = %q", list.InventoryValuation)
	}

	// The list's stock filters use the same rule as the counts.
	low, total, err := d.GetProducts(orgID, ProductListOptions{Stock: "low"})
	if err != nil {
		t.Fatalf("GetProducts low: %v", err)
	}
	if total != 2 || len(low) != 2 || low[0].ID != "ps-out" || low[1].ID != "ps-low" {
		t.Errorf("low = %d %v, want ps-out then ps-low", total, productIDs(low))
	}
	out, total, err := d.GetProducts(orgID, ProductListOptions{Stock: "out"})
	if err != nil {
		t.Fatalf("GetProducts out: %v", err)
	}
	if total != 1 || out[0].ID != "ps-out" {
		t.Errorf("out = %d %v, want ps-out", total, productIDs(out))
	}
}

func productIDs(products []Product) []string {
	ids := make([]string, len(products))
	for i, p := range products {
		ids[i] = p.ID
	}
	return ids
}

func TestProductSummariesSwitchedOffAndCrossOrg(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	orgID, productID := productSummaryFixture(t, d)

	other, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-ps-other"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if _, err := d.GetProductSummary(other.ID, productID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("cross-org product: err = %v, want sql.ErrNoRows", err)
	}

	off := false
	if _, err := d.UpdateOrganization(orgID, UpdateOrganizationRequest{MasterDataSummaries: &off}); err != nil {
		t.Fatalf("UpdateOrganization: %v", err)
	}
	if _, err := d.GetProductSummaries(orgID); err != errSummariesSwitchedOff {
		t.Errorf("GetProductSummaries switched off: err = %v", err)
	}
	if _, err := d.GetProductSummary(orgID, productID); err != errSummariesSwitchedOff {
		t.Errorf("GetProductSummary switched off: err = %v", err)
	}
}
