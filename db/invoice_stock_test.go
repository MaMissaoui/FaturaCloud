package db

import (
	"errors"
	"strings"
	"testing"
)

// cashStockFixture is newGLPostingTestFixture plus stock-enabled products.
type cashStockFixture struct {
	glPostingTestFixture
	washer *Product // stock-enabled, uncosted until a test gives it stock
}

func newCashStockFixture(t *testing.T, d *Database, orgID string, quantityOnly bool) cashStockFixture {
	t.Helper()
	fx := newGLPostingTestFixture(t, d, orgID)
	if quantityOnly {
		if _, err := d.UpdateOrganization(fx.orgID, UpdateOrganizationRequest{
			InventoryValuation: ptr(InventoryValuationQuantityOnly),
		}); err != nil {
			t.Fatalf("UpdateOrganization(quantity_only): %v", err)
		}
	}
	washer, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: fx.orgID, Name: "Machine à laver HGE 9kg", SKU: ptr("MAL-001"),
		Type: "product", StockEnabled: 1, Price: 1000,
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	return cashStockFixture{glPostingTestFixture: fx, washer: washer}
}

// sellWasher records a Cash Book loan sale (nothing received, state sent)
// of qty washers at 10.00 each, no tax.
func (fx cashStockFixture) sellWasher(t *testing.T, d *Database, qty float64) (*CashSaleResult, error) {
	t.Helper()
	total := int64(qty * 1000)
	return d.CreateCashSale(CreateCashSaleRequest{
		OrganizationID: fx.orgID, ClientID: fx.clientID, Date: fx.date, Currency: "EUR",
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: qty, UnitPrice: 1000, ProductID: &fx.washer.ID},
		},
		SubTotal: total, TaxTotal: 0, Total: total,
	})
}

func stockOf(t *testing.T, d *Database, productID string) float64 {
	t.Helper()
	p, err := d.GetProduct(productID)
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	return p.StockQuantity
}

func requireValidationError(t *testing.T, err error, contains string) {
	t.Helper()
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected a *ValidationError containing %q, got %T: %v", contains, err, err)
	}
	if !strings.Contains(err.Error(), contains) {
		t.Fatalf("error %q does not contain %q", err.Error(), contains)
	}
}

// Perpetual valuation: the sale takes stock out and posts COGS at the
// average; cancelling puts both back; re-sending posts them again.
func TestCashSaleMovesStockAndFollowsInvoiceState(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newCashStockFixture(t, d, "org-cash-stock", false)
	requireFiscalYearCoveringNow(t, d, fx.orgID)
	if _, err := d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: fx.orgID, ProductID: fx.washer.ID, Type: "in", Quantity: 5, UnitCost: ptr(int64(60000)),
	}); err != nil {
		t.Fatalf("CreateStockMovement: %v", err)
	}
	org, err := d.GetOrganization(fx.orgID)
	if err != nil {
		t.Fatalf("GetOrganization: %v", err)
	}

	sale, err := fx.sellWasher(t, d, 2)
	if err != nil {
		t.Fatalf("CreateCashSale: %v", err)
	}
	invoiceID := sale.Invoice.ID
	if sale.Invoice.MovesStock != 1 {
		t.Fatalf("movesStock = %d, want 1", sale.Invoice.MovesStock)
	}
	if got := stockOf(t, d, fx.washer.ID); got != 3 {
		t.Fatalf("stock after sale = %v, want 3", got)
	}
	cogs, err := d.FindPostedEntryForSourceDocument(invoiceCOGSSourceType, invoiceID)
	if err != nil || cogs == nil {
		t.Fatalf("expected a posted COGS entry, err=%v entry=%v", err, cogs)
	}
	lines, err := d.GetJournalEntryLines(cogs.ID)
	if err != nil {
		t.Fatalf("GetJournalEntryLines: %v", err)
	}
	if dr, _ := sumLines(lines, *org.DefaultCOGSAccountID); dr != 120000 {
		t.Fatalf("COGS debit = %d, want 120000 (2 × 600.00)", dr)
	}
	if _, cr := sumLines(lines, *org.DefaultInventoryAccountID); cr != 120000 {
		t.Fatalf("Inventory credit = %d, want 120000", cr)
	}

	if _, err := d.UpdateInvoiceState(invoiceID, "cancelled"); err != nil {
		t.Fatalf("UpdateInvoiceState(cancelled): %v", err)
	}
	if got := stockOf(t, d, fx.washer.ID); got != 5 {
		t.Fatalf("stock after cancel = %v, want 5", got)
	}
	if entry, err := d.FindPostedEntryForSourceDocument(invoiceCOGSSourceType, invoiceID); err != nil || entry != nil {
		t.Fatalf("expected the COGS entry to be reversed, err=%v entry=%v", err, entry)
	}
	// Cancelled again is a no-op, not a second restore.
	if _, err := d.UpdateInvoiceState(invoiceID, "draft"); err != nil {
		t.Fatalf("UpdateInvoiceState(draft): %v", err)
	}
	if got := stockOf(t, d, fx.washer.ID); got != 5 {
		t.Fatalf("stock after draft = %v, want 5", got)
	}

	if _, err := d.UpdateInvoiceState(invoiceID, "sent"); err != nil {
		t.Fatalf("UpdateInvoiceState(sent): %v", err)
	}
	if got := stockOf(t, d, fx.washer.ID); got != 3 {
		t.Fatalf("stock after re-send = %v, want 3", got)
	}
	if entry, err := d.FindPostedEntryForSourceDocument(invoiceCOGSSourceType, invoiceID); err != nil || entry == nil {
		t.Fatalf("expected a fresh COGS entry after re-send, err=%v entry=%v", err, entry)
	}
	// sent -> paid needs stock out in both states: nothing moves.
	if _, err := d.UpdateInvoiceState(invoiceID, "paid"); err != nil {
		t.Fatalf("UpdateInvoiceState(paid): %v", err)
	}
	if got := stockOf(t, d, fx.washer.ID); got != 3 {
		t.Fatalf("stock after paid = %v, want 3", got)
	}

	// Its movements can't be deleted by hand.
	movements, err := d.GetProductStockMovements(fx.washer.ID)
	if err != nil {
		t.Fatalf("GetProductStockMovements: %v", err)
	}
	for _, m := range movements {
		if m.SourceDocumentID != nil && *m.SourceDocumentID == invoiceID {
			_, err := d.DeleteStockMovement(m.ID)
			requireValidationError(t, err, "cash sale")
			break
		}
	}
}

// Quantities only: no COGS, no cost needed, and — by decision — the sale
// goes through even with no stock recorded, driving stock negative.
func TestCashSaleQuantityOnlyAllowsNegativeStockAndPostsNoCOGS(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newCashStockFixture(t, d, "org-cash-stock-qo", true)

	sale, err := fx.sellWasher(t, d, 3)
	if err != nil {
		t.Fatalf("CreateCashSale: %v", err)
	}
	if got := stockOf(t, d, fx.washer.ID); got != -3 {
		t.Fatalf("stock = %v, want -3", got)
	}
	if entry, err := d.FindPostedEntryForSourceDocument(invoiceCOGSSourceType, sale.Invoice.ID); err != nil || entry != nil {
		t.Fatalf("expected no COGS entry in quantity-only mode, err=%v entry=%v", err, entry)
	}

	// A count bringing it back to zero, then a costed inflow: the average
	// replay handles a running quantity at or below zero without dividing
	// by it, and restarts from the inflow's own cost.
	if _, err := d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: fx.orgID, ProductID: fx.washer.ID, Type: "count_addition", Quantity: 3,
	}); err != nil {
		t.Fatalf("CreateStockMovement (count to zero): %v", err)
	}
	if _, err := d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: fx.orgID, ProductID: fx.washer.ID, Type: "in", Quantity: 2, UnitCost: ptr(int64(55000)),
	}); err != nil {
		t.Fatalf("CreateStockMovement (costed in): %v", err)
	}
	p, err := d.GetProduct(fx.washer.ID)
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	if p.StockQuantity != 2 || p.UnitCost == nil || *p.UnitCost != 55000 {
		t.Fatalf("stock=%v unitCost=%v, want 2 and 55000", p.StockQuantity, p.UnitCost)
	}

	// A costed inflow landing exactly on zero from below doesn't panic either.
	sale2, err := fx.sellWasher(t, d, 4) // 2 -> -2
	if err != nil {
		t.Fatalf("CreateCashSale 2: %v", err)
	}
	if _, err := d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: fx.orgID, ProductID: fx.washer.ID, Type: "in", Quantity: 2, UnitCost: ptr(int64(50000)),
	}); err != nil {
		t.Fatalf("CreateStockMovement (costed in to zero): %v", err)
	}
	if got := stockOf(t, d, fx.washer.ID); got != 0 {
		t.Fatalf("stock = %v, want 0", got)
	}

	// Cancelling the second sale restores exactly what it took.
	if _, err := d.UpdateInvoiceState(sale2.Invoice.ID, "cancelled"); err != nil {
		t.Fatalf("UpdateInvoiceState(cancelled): %v", err)
	}
	if got := stockOf(t, d, fx.washer.ID); got != 4 {
		t.Fatalf("stock after cancel = %v, want 4", got)
	}
}

// Perpetual valuation keeps the delivery rule: no cost basis, no sale —
// refused before anything is written.
func TestCashSalePerpetualWithoutCostBasisIsRejectedBeforeAnyWrite(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newCashStockFixture(t, d, "org-cash-stock-nocost", false)

	_, err := fx.sellWasher(t, d, 1)
	requireValidationError(t, err, "cost basis")
	invoices, err := d.GetInvoices(fx.orgID)
	if err != nil {
		t.Fatalf("GetInvoices: %v", err)
	}
	if len(invoices) != 0 {
		t.Fatalf("expected no invoice to be written, got %d", len(invoices))
	}
	if got := stockOf(t, d, fx.washer.ID); got != 0 {
		t.Fatalf("stock = %v, want 0", got)
	}
}

func TestCashSaleRefusesSerializedAndNonPositiveStockLines(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newCashStockFixture(t, d, "org-cash-stock-serial", true)
	tv, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: fx.orgID, Name: "TV 55 TCL", Type: "product", StockEnabled: 1, Serialized: 1,
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}

	_, err = d.CreateCashSale(CreateCashSaleRequest{
		OrganizationID: fx.orgID, ClientID: fx.clientID, Date: fx.date, Currency: "EUR",
		LineItems: []CreateInvoiceLineItemRequest{{Quantity: 1, UnitPrice: 1000, ProductID: &tv.ID}},
		SubTotal: 1000, Total: 1000,
	})
	requireValidationError(t, err, "serialized")

	_, err = d.CreateCashSale(CreateCashSaleRequest{
		OrganizationID: fx.orgID, ClientID: fx.clientID, Date: fx.date, Currency: "EUR",
		// Overall total stays positive (a service line), so this reaches
		// the stock check rather than the sale-total validation.
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: -1, UnitPrice: 1000, ProductID: &fx.washer.ID},
			{Quantity: 2, UnitPrice: 1000, ProductID: &fx.productID},
		},
		SubTotal: 1000, Total: 1000,
	})
	requireValidationError(t, err, "quantity above zero")
}

// A zero-total sale has no GL entry, so only the stock guards stand
// between it and an edit/delete that would orphan its stock-out.
func TestCashSaleZeroTotalMovesStockAndIsGuarded(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newCashStockFixture(t, d, "org-cash-stock-zero", true)

	sale, err := d.CreateCashSale(CreateCashSaleRequest{
		OrganizationID: fx.orgID, ClientID: fx.clientID, Date: fx.date, Currency: "EUR",
		LineItems: []CreateInvoiceLineItemRequest{{Quantity: 1, UnitPrice: 0, ProductID: &fx.washer.ID}},
	})
	if err != nil {
		t.Fatalf("CreateCashSale: %v", err)
	}
	if got := stockOf(t, d, fx.washer.ID); got != -1 {
		t.Fatalf("stock = %v, want -1", got)
	}

	lines := []CreateInvoiceLineItemRequest{{Quantity: 5, UnitPrice: 0, ProductID: &fx.washer.ID}}
	_, err = d.UpdateInvoice(sale.Invoice.ID, UpdateInvoiceRequest{LineItems: &lines})
	requireValidationError(t, err, "stock is taken out")

	// Moved to sent (still needs stock), then deletion is refused on stock.
	if _, err := d.UpdateInvoiceState(sale.Invoice.ID, "sent"); err != nil {
		t.Fatalf("UpdateInvoiceState(sent): %v", err)
	}
	_, err = d.DeleteInvoice(sale.Invoice.ID)
	requireValidationError(t, err, "stock is taken out")

	// Cancelled, the stock is back and the sale can go.
	if _, err := d.UpdateInvoiceState(sale.Invoice.ID, "cancelled"); err != nil {
		t.Fatalf("UpdateInvoiceState(cancelled): %v", err)
	}
	if got := stockOf(t, d, fx.washer.ID); got != 0 {
		t.Fatalf("stock after cancel = %v, want 0", got)
	}
	if ok, err := d.DeleteInvoice(sale.Invoice.ID); err != nil || !ok {
		t.Fatalf("DeleteInvoice after cancel = %v, %v; want true, nil", ok, err)
	}
}

// An ordinary invoice (not a Cash Book sale) never moves stock — its
// delivery does — so the two never deduct twice.
func TestOrdinaryInvoiceNeverMovesStock(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newCashStockFixture(t, d, "org-cash-stock-ordinary", true)

	inv, err := d.CreateInvoice(CreateInvoiceRequest{
		OrganizationID: fx.orgID, Number: "INV-0001", State: "draft", ClientID: fx.clientID,
		Date: fx.date, Currency: "EUR", SubTotal: 2000, Total: 2000,
		LineItems: []CreateInvoiceLineItemRequest{{Quantity: 2, UnitPrice: 1000, ProductID: &fx.washer.ID}},
	})
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}
	for _, state := range []string{"sent", "paid", "cancelled", "sent"} {
		if _, err := d.UpdateInvoiceState(inv.ID, state); err != nil {
			t.Fatalf("UpdateInvoiceState(%s): %v", state, err)
		}
		if got := stockOf(t, d, fx.washer.ID); got != 0 {
			t.Fatalf("stock after %s = %v, want 0", state, got)
		}
	}
}
