package db

import (
	"fmt"
	"testing"
)

// Documents with no number of their own still get a name in the history: a
// cash movement its journal entry's number, as the journal shows it ("#12"),
// a stock movement its product's name.
func TestAuditDocumentInfoNamesMovements(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-audit-names")
	register, bank, _ := cashMovementTestAccounts(t, d, fx.orgID)

	cash, err := d.CreateCashMovement(CreateCashMovementRequest{
		OrganizationID: fx.orgID, AccountID: register.ID, Date: fx.date,
		CounterAccountType: "bank", CounterAccountID: bank.ID, Amount: 5000,
	})
	if err != nil {
		t.Fatalf("CreateCashMovement: %v", err)
	}
	var entryNumber int
	if err := d.DB.Get(&entryNumber, `SELECT entryNumber FROM journal_entries WHERE id = ?`, *cash.CashMovement.JournalEntryID); err != nil {
		t.Fatalf("entry number: %v", err)
	}
	want := fmt.Sprintf("#%d", entryNumber)
	if label, _ := d.AuditDocumentInfo("cash-movements", cash.CashMovement.ID); label != want {
		t.Errorf("cash movement label = %q, want %q", label, want)
	}
	if label, _ := d.AuditDocumentInfo("journal-entries", *cash.CashMovement.JournalEntryID); label != want {
		t.Errorf("journal entry label = %q, want %q", label, want)
	}

	product, err := d.CreateProduct(CreateProductRequest{OrganizationID: fx.orgID, Name: "Huile d'olive 1L", Type: "product", StockEnabled: 1})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}
	stock, err := d.CreateStockMovement(CreateStockMovementRequest{OrganizationID: fx.orgID, ProductID: product.ID, Type: "in", Quantity: 4})
	if err != nil {
		t.Fatalf("CreateStockMovement: %v", err)
	}
	if label, _ := d.AuditDocumentInfo("stock-movements", stock.Movements[0].ID); label != "Huile d'olive 1L" {
		t.Errorf("stock movement label = %q, want the product's name", label)
	}
}
