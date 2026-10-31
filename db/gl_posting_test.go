package db

import (
	"errors"
	"testing"
)

// glPostingTestFixture builds an organization via the normal path — so it
// gets the real auto-seeded chart of accounts and journals
// (seedDefaultChartOfAccounts/seedDefaultJournals) exactly as production
// does — plus a fiscal year, a client, a vendor, and a taxed product, so
// UpdateInvoiceState/UpdateIncomingInvoiceState can actually auto-post
// against it.
type glPostingTestFixture struct {
	orgID              string
	clientID           string
	vendorID           string
	productID          string
	taxRateID          string
	arAccountID        string
	apAccountID        string
	revenueAccountID   string
	expenseAccountID   string
	outputTaxAccountID string
	inputTaxAccountID  string
	date               int64
}

func newGLPostingTestFixture(t *testing.T, d *Database, orgID string) glPostingTestFixture {
	t.Helper()

	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: orgID, Name: ptr("GL Posting Test Org")})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	date := int64(1738368000000) // 2025-02-01 in ms
	if _, err := d.CreateFiscalYear(CreateFiscalYearRequest{
		OrganizationID: org.ID, Name: "2025", StartDate: 1735689600000, EndDate: 1767225599000,
	}); err != nil {
		t.Fatalf("CreateFiscalYear: %v", err)
	}

	client, err := d.CreateClient(CreateClientRequest{OrganizationID: org.ID, Name: ptr("Test Client")})
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	vendor, err := d.CreateVendor(CreateVendorRequest{OrganizationID: org.ID, Name: ptr("Test Vendor")})
	if err != nil {
		t.Fatalf("CreateVendor: %v", err)
	}

	accounts, err := d.GetAccounts(org.ID)
	if err != nil {
		t.Fatalf("GetAccounts: %v", err)
	}
	byCode := map[string]Account{}
	for _, a := range accounts {
		byCode[a.Code] = a
	}
	outputTax := byCode["2200"] // Output Tax Payable, seeded liability account
	inputTax := byCode["1200"]  // Input Tax Receivable, seeded asset account

	taxRate, err := d.CreateTaxRate(CreateTaxRateRequest{
		ID: orgID + "-vat", OrganizationID: org.ID, Name: "Standard VAT", Percentage: 20,
		OutputTaxAccountID: &outputTax.ID, InputTaxAccountID: &inputTax.ID,
	})
	if err != nil {
		t.Fatalf("CreateTaxRate: %v", err)
	}

	product, err := d.CreateProduct(CreateProductRequest{
		OrganizationID: org.ID, Name: "Widget", Type: "service", Price: 1000,
	})
	if err != nil {
		t.Fatalf("CreateProduct: %v", err)
	}

	return glPostingTestFixture{
		orgID:              org.ID,
		clientID:           client.ID,
		vendorID:           vendor.ID,
		productID:          product.ID,
		taxRateID:          taxRate.ID,
		arAccountID:        *org.DefaultArAccountID,
		apAccountID:        *org.DefaultApAccountID,
		revenueAccountID:   *org.DefaultRevenueAccountID,
		expenseAccountID:   *org.DefaultExpenseAccountID,
		outputTaxAccountID: outputTax.ID,
		inputTaxAccountID:  inputTax.ID,
		date:               date,
	}
}

func (fx glPostingTestFixture) createInvoice(t *testing.T, d *Database, id string, qty float64, unitPrice float64) *Invoice {
	t.Helper()
	subTotal := int64(qty * unitPrice)
	taxTotal := subTotal / 5 // 20%
	inv, err := d.CreateInvoice(CreateInvoiceRequest{
		ID: id, OrganizationID: fx.orgID, Number: id, ClientID: fx.clientID,
		Date: fx.date, Currency: "EUR",
		SubTotal: subTotal, TaxTotal: taxTotal, Total: subTotal + taxTotal,
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: qty, UnitPrice: unitPrice, TaxRate: &fx.taxRateID, ProductID: &fx.productID},
		},
	})
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}
	return inv
}

func (fx glPostingTestFixture) createIncomingInvoice(t *testing.T, d *Database, id string, qty float64, unitPrice float64) *IncomingInvoice {
	t.Helper()
	subTotal := int64(qty * unitPrice)
	taxTotal := subTotal / 5 // 20%
	inv, err := d.CreateIncomingInvoice(CreateIncomingInvoiceRequest{
		ID: id, OrganizationID: fx.orgID, VendorID: fx.vendorID, VendorInvoiceNumber: id,
		Date: fx.date, Currency: "EUR",
		SubTotal: subTotal, TaxTotal: taxTotal, Total: subTotal + taxTotal,
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: qty, UnitPrice: unitPrice, TaxRate: &fx.taxRateID, ProductID: &fx.productID},
		},
	})
	if err != nil {
		t.Fatalf("CreateIncomingInvoice: %v", err)
	}
	return inv
}

// sumLines returns (debit, credit) totals booked to accountID across lines.
func sumLines(lines []JournalLine, accountID string) (debit, credit int64) {
	for _, l := range lines {
		if l.AccountID == accountID {
			debit += l.Debit
			credit += l.Credit
		}
	}
	return
}

func TestInvoiceSentPostsBalancedGLEntry(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-inv-sent")
	inv := fx.createInvoice(t, d, "inv-1", 2, 1000) // 2000 subtotal, 400 tax, 2400 total

	if _, err := d.UpdateInvoiceState(inv.ID, "sent"); err != nil {
		t.Fatalf("UpdateInvoiceState(sent): %v", err)
	}

	entry, err := d.postedEntryForSourceDocument("invoice", inv.ID)
	if err != nil {
		t.Fatalf("FindPostedEntryForSourceDocument: %v", err)
	}
	if entry == nil {
		t.Fatal("expected a posted GL entry for the invoice, got none")
	}
	if entry.Status != "posted" {
		t.Fatalf("entry status = %q, want posted", entry.Status)
	}

	lines, err := d.GetJournalEntryLines(entry.ID)
	if err != nil {
		t.Fatalf("GetJournalEntryLines: %v", err)
	}

	arDebit, arCredit := sumLines(lines, fx.arAccountID)
	if arDebit != 2400 || arCredit != 0 {
		t.Fatalf("AR line = debit %d credit %d, want debit 2400 credit 0", arDebit, arCredit)
	}
	revDebit, revCredit := sumLines(lines, fx.revenueAccountID)
	if revDebit != 0 || revCredit != 2000 {
		t.Fatalf("revenue line = debit %d credit %d, want debit 0 credit 2000", revDebit, revCredit)
	}
	taxDebit, taxCredit := sumLines(lines, fx.outputTaxAccountID)
	if taxDebit != 0 || taxCredit != 400 {
		t.Fatalf("output tax line = debit %d credit %d, want debit 0 credit 400", taxDebit, taxCredit)
	}

	rows, err := d.GetTrialBalance(fx.orgID, "", "")
	if err != nil {
		t.Fatalf("GetTrialBalance: %v", err)
	}
	found := false
	for _, r := range rows {
		if r.AccountID == fx.arAccountID {
			found = true
			if r.Debit-r.Credit != 2400 {
				t.Fatalf("trial balance AR net = %d, want 2400", r.Debit-r.Credit)
			}
		}
	}
	if !found {
		t.Fatal("AR account missing from trial balance")
	}
}

func TestInvoiceStateBounceBackReversesGLEntry(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-inv-bounce")
	inv := fx.createInvoice(t, d, "inv-1", 1, 1000)

	if _, err := d.UpdateInvoiceState(inv.ID, "sent"); err != nil {
		t.Fatalf("UpdateInvoiceState(sent): %v", err)
	}
	original, err := d.postedEntryForSourceDocument("invoice", inv.ID)
	if err != nil || original == nil {
		t.Fatalf("expected a posted entry after sent, err=%v entry=%v", err, original)
	}

	// Bounce back to draft — the entry no longer needs to exist, so it must
	// be reversed, not left dangling.
	if _, err := d.UpdateInvoiceState(inv.ID, "draft"); err != nil {
		t.Fatalf("UpdateInvoiceState(draft): %v", err)
	}

	stillLive, err := d.postedEntryForSourceDocument("invoice", inv.ID)
	if err != nil {
		t.Fatalf("FindPostedEntryForSourceDocument: %v", err)
	}
	if stillLive != nil {
		t.Fatalf("expected no live posted entry after bouncing back to draft, got %+v", stillLive)
	}

	reversedOriginal, err := d.GetJournalEntry(original.ID)
	if err != nil {
		t.Fatalf("GetJournalEntry(original): %v", err)
	}
	if reversedOriginal.Status != "reversed" {
		t.Fatalf("original entry status = %q, want reversed", reversedOriginal.Status)
	}

	rows, err := d.GetTrialBalance(fx.orgID, "", "")
	if err != nil {
		t.Fatalf("GetTrialBalance: %v", err)
	}
	for _, r := range rows {
		if r.AccountID == fx.arAccountID && r.Debit-r.Credit != 0 {
			t.Fatalf("AR should net to zero after reversal, got %d", r.Debit-r.Credit)
		}
	}
}

func TestInvoiceDraftDirectlyToPaidPosts(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-inv-direct-paid")
	inv := fx.createInvoice(t, d, "inv-1", 1, 1000)

	// Skips "sent" entirely — invoiceStates has no transition matrix, so
	// this must post exactly as if it had passed through sent first.
	if _, err := d.UpdateInvoiceState(inv.ID, "paid"); err != nil {
		t.Fatalf("UpdateInvoiceState(paid): %v", err)
	}
	entry, err := d.postedEntryForSourceDocument("invoice", inv.ID)
	if err != nil || entry == nil {
		t.Fatalf("expected a posted entry after draft->paid, err=%v entry=%v", err, entry)
	}
}

func TestInvoicePostingRejectedWithoutDefaultARAccount(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-inv-no-ar")
	// UpdateOrganization's SQL COALESCEs every field (nil = keep unchanged),
	// so there's no way to clear defaultArAccountId back to NULL through the
	// normal API — reach into the DB directly to exercise this edge case.
	if _, err := d.DB.Exec(`UPDATE organizations SET defaultArAccountId = NULL WHERE id = ?`, fx.orgID); err != nil {
		t.Fatalf("clear defaultArAccountId: %v", err)
	}
	inv := fx.createInvoice(t, d, "inv-1", 1, 1000)

	_, err := d.UpdateInvoiceState(inv.ID, "sent")
	if err == nil {
		t.Fatal("expected UpdateInvoiceState(sent) to be rejected without a default AR account")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected a *ValidationError, got %T: %v", err, err)
	}
	current, getErr := d.GetInvoice(inv.ID)
	if getErr != nil {
		t.Fatalf("GetInvoice: %v", getErr)
	}
	if current.State != "draft" {
		t.Fatalf("state changed to %q despite rejected posting — state and GL entry must move atomically", current.State)
	}
}

func TestDeleteInvoiceBlockedByPostedGLEntry(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-inv-delete-guard")
	inv := fx.createInvoice(t, d, "inv-1", 1, 1000)

	if _, err := d.UpdateInvoiceState(inv.ID, "sent"); err != nil {
		t.Fatalf("UpdateInvoiceState(sent): %v", err)
	}
	if _, err := d.DeleteInvoice(inv.ID); err == nil {
		t.Fatal("expected deleting an invoice with a posted GL entry to be rejected")
	}

	// Cancelling reverses the entry, which clears the way to delete.
	if _, err := d.UpdateInvoiceState(inv.ID, "cancelled"); err != nil {
		t.Fatalf("UpdateInvoiceState(cancelled): %v", err)
	}
	ok, err := d.DeleteInvoice(inv.ID)
	if err != nil || !ok {
		t.Fatalf("DeleteInvoice(cancelled): ok=%v, err=%v", ok, err)
	}
}

func TestIncomingInvoiceApprovedPostsBalancedGLEntry(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-bill-approved")
	bill := fx.createIncomingInvoice(t, d, "bill-1", 2, 1000) // 2000 subtotal, 400 tax, 2400 total

	if _, err := d.UpdateIncomingInvoiceState(bill.ID, "approved"); err != nil {
		t.Fatalf("UpdateIncomingInvoiceState(approved): %v", err)
	}

	entry, err := d.postedEntryForSourceDocument("incoming_invoice", bill.ID)
	if err != nil || entry == nil {
		t.Fatalf("expected a posted entry, err=%v entry=%v", err, entry)
	}
	lines, err := d.GetJournalEntryLines(entry.ID)
	if err != nil {
		t.Fatalf("GetJournalEntryLines: %v", err)
	}

	apDebit, apCredit := sumLines(lines, fx.apAccountID)
	if apDebit != 0 || apCredit != 2400 {
		t.Fatalf("AP line = debit %d credit %d, want debit 0 credit 2400", apDebit, apCredit)
	}
	expDebit, expCredit := sumLines(lines, fx.expenseAccountID)
	if expDebit != 2000 || expCredit != 0 {
		t.Fatalf("expense line = debit %d credit %d, want debit 2000 credit 0", expDebit, expCredit)
	}
	taxDebit, taxCredit := sumLines(lines, fx.inputTaxAccountID)
	if taxDebit != 400 || taxCredit != 0 {
		t.Fatalf("input tax line = debit %d credit %d, want debit 400 credit 0", taxDebit, taxCredit)
	}
}

// Zero-total documents (e.g. a placeholder draft with no line items) have no
// economic event to record — needsInvoiceGLPresence must skip them rather
// than hand buildInvoiceGLLines a set of lines that would insert a
// debit=0/credit=0 row and fail journal_lines' CHECK constraint.
func TestZeroTotalInvoiceSkipsGLPosting(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-inv-zero")
	inv, err := d.CreateInvoice(CreateInvoiceRequest{
		OrganizationID: fx.orgID, Number: "inv-zero", ClientID: fx.clientID,
		Date: fx.date, Currency: "EUR",
		LineItems: []CreateInvoiceLineItemRequest{},
	})
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}

	if _, err := d.UpdateInvoiceState(inv.ID, "sent"); err != nil {
		t.Fatalf("UpdateInvoiceState(sent): %v", err)
	}
	entry, err := d.postedEntryForSourceDocument("invoice", inv.ID)
	if err != nil {
		t.Fatalf("FindPostedEntryForSourceDocument: %v", err)
	}
	if entry != nil {
		t.Fatalf("expected no GL entry for a zero-total invoice, got %+v", entry)
	}
}

// A 0%-percentage tax rate (e.g. an export exemption) with an output tax
// account configured computes to a zero-amount tax group — that group must
// not emit a journal line at all, since journal_lines' CHECK((debit=0) <>
// (credit=0)) rejects a debit=0/credit=0 row outright (caught live: sending
// an invoice under a real 0% VAT rate 500'd instead of posting).
func TestInvoiceWithZeroPercentTaxRateSkipsZeroTaxLine(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-inv-zero-pct-tax")

	zeroRate, err := d.CreateTaxRate(CreateTaxRateRequest{
		OrganizationID: fx.orgID, Name: "VAT 0% (Export)", Percentage: 0,
		OutputTaxAccountID: &fx.outputTaxAccountID,
	})
	if err != nil {
		t.Fatalf("CreateTaxRate: %v", err)
	}

	inv, err := d.CreateInvoice(CreateInvoiceRequest{
		OrganizationID: fx.orgID, Number: "inv-zero-pct", ClientID: fx.clientID,
		Date: fx.date, Currency: "EUR",
		SubTotal: 1000, TaxTotal: 0, Total: 1000,
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 1, UnitPrice: 1000, TaxRate: &zeroRate.ID, ProductID: &fx.productID},
		},
	})
	if err != nil {
		t.Fatalf("CreateInvoice: %v", err)
	}

	if _, err := d.UpdateInvoiceState(inv.ID, "sent"); err != nil {
		t.Fatalf("UpdateInvoiceState(sent): %v", err)
	}
	entry, err := d.postedEntryForSourceDocument("invoice", inv.ID)
	if err != nil || entry == nil {
		t.Fatalf("expected a posted entry, err=%v entry=%v", err, entry)
	}
	lines, err := d.GetJournalEntryLines(entry.ID)
	if err != nil {
		t.Fatalf("GetJournalEntryLines: %v", err)
	}
	taxDebit, taxCredit := sumLines(lines, fx.outputTaxAccountID)
	if taxDebit != 0 || taxCredit != 0 {
		t.Fatalf("expected no output tax line at all for a 0%% rate, found debit %d credit %d", taxDebit, taxCredit)
	}
	arDebit, _ := sumLines(lines, fx.arAccountID)
	if arDebit != 1000 {
		t.Fatalf("AR debit = %d, want 1000", arDebit)
	}
}

// TestInvoiceDiscountPostsReducedRevenueAndTax proves the remise reaches the
// GL: a 100.00 discount on a 1000.00 subtotal at 20% books 900.00 revenue and
// 180.00 output tax, against a 1080.00 AR — matching what
// validateInvoiceTotals accepts.
func TestInvoiceDiscountPostsReducedRevenueAndTax(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-inv-discount")

	// 1000.00 subtotal, 100.00 discount -> net 900.00, tax 180.00,
	// total 1080.00.
	inv, err := d.CreateInvoice(CreateInvoiceRequest{
		OrganizationID: fx.orgID, Number: "inv-discount", ClientID: fx.clientID,
		Date: fx.date, Currency: "EUR",
		SubTotal: 100000, TaxTotal: 18000, Total: 108000, DiscountAmount: 10000,
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 1, UnitPrice: 100000, TaxRate: &fx.taxRateID},
		},
	})
	if err != nil {
		t.Fatalf("CreateInvoice with discount: %v", err)
	}
	if inv.DiscountAmount != 10000 {
		t.Fatalf("persisted discountAmount = %d, want 10000", inv.DiscountAmount)
	}

	if _, err := d.UpdateInvoiceState(inv.ID, "sent"); err != nil {
		t.Fatalf("UpdateInvoiceState(sent): %v", err)
	}
	entry, err := d.postedEntryForSourceDocument("invoice", inv.ID)
	if err != nil || entry == nil {
		t.Fatalf("FindPostedEntryForSourceDocument: entry=%v err=%v", entry, err)
	}
	lines, err := d.GetJournalEntryLines(entry.ID)
	if err != nil {
		t.Fatalf("GetJournalEntryLines: %v", err)
	}

	arDebit, _ := sumLines(lines, fx.arAccountID)
	if arDebit != 108000 {
		t.Fatalf("AR debit = %d, want 108000", arDebit)
	}
	_, revCredit := sumLines(lines, fx.revenueAccountID)
	if revCredit != 90000 {
		t.Fatalf("revenue credit = %d, want 90000", revCredit)
	}
	_, taxCredit := sumLines(lines, fx.outputTaxAccountID)
	if taxCredit != 18000 {
		t.Fatalf("output tax credit = %d, want 18000", taxCredit)
	}
}

// F191: a live entry in another organization naming this invoice neither
// stops the invoice posting nor gets reversed by its state changes.
func TestInvoicePostingIgnoresAnotherOrganizationsEntryNamingIt(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fxA := newGLPostingTestFixture(t, d, "org-claim-a")
	fxB := newGLPostingTestFixture(t, d, "org-claim-b")
	invA := fxA.createInvoice(t, d, "inv-claim-a", 1, 1000)
	invB := fxB.createInvoice(t, d, "inv-claim-b", 2, 1000)

	if _, err := d.UpdateInvoiceState(invA.ID, "sent"); err != nil {
		t.Fatalf("UpdateInvoiceState(org-a sent): %v", err)
	}
	claim, err := d.FindPostedEntryForSourceDocument(fxA.orgID, "invoice", invA.ID)
	if err != nil || claim == nil {
		t.Fatalf("org-a entry: entry=%v err=%v", claim, err)
	}
	// Retag org A's live entry as org B's invoice, as a manual entry could
	// before F191's first step.
	if _, err := d.DB.Exec(`UPDATE journal_entries SET sourceDocumentId = ? WHERE id = ?`, invB.ID, claim.ID); err != nil {
		t.Fatalf("retag: %v", err)
	}

	if _, err := d.UpdateInvoiceState(invB.ID, "sent"); err != nil {
		t.Fatalf("UpdateInvoiceState(org-b sent): %v", err)
	}
	entry, err := d.FindPostedEntryForSourceDocument(fxB.orgID, "invoice", invB.ID)
	if err != nil || entry == nil {
		t.Fatalf("org-b entry: entry=%v err=%v", entry, err)
	}
	lines, err := d.GetJournalEntryLines(entry.ID)
	if err != nil {
		t.Fatalf("GetJournalEntryLines: %v", err)
	}
	if debit, _ := sumLines(lines, fxB.arAccountID); debit != invB.Total {
		t.Fatalf("org-b AR debit = %d, want %d", debit, invB.Total)
	}

	if _, err := d.UpdateInvoiceState(invB.ID, "draft"); err != nil {
		t.Fatalf("UpdateInvoiceState(org-b draft): %v", err)
	}
	var status string
	if err := d.DB.Get(&status, `SELECT status FROM journal_entries WHERE id = ?`, claim.ID); err != nil {
		t.Fatal(err)
	}
	if status != "posted" {
		t.Fatalf("org-a entry status after org-b's draft = %q, want posted", status)
	}
}

// F191 follow-up: DeleteInvoice's atomic backstop used to look for a live
// posted entry by sourceDocumentId alone. Another organization's entry
// retagged with this draft invoice's id made the scoped checks pass and the
// final DELETE match nothing, yet the transaction still committed — losing
// the line items while the invoice survived.
func TestDeleteInvoiceIgnoresAnotherOrganizationsEntryNamingIt(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fxA := newGLPostingTestFixture(t, d, "org-del-claim-a")
	fxB := newGLPostingTestFixture(t, d, "org-del-claim-b")
	invA := fxA.createInvoice(t, d, "inv-del-claim-a", 1, 1000)
	invB := fxB.createInvoice(t, d, "inv-del-claim-b", 2, 1000)

	if _, err := d.UpdateInvoiceState(invA.ID, "sent"); err != nil {
		t.Fatalf("UpdateInvoiceState(org-a sent): %v", err)
	}
	claim, err := d.FindPostedEntryForSourceDocument(fxA.orgID, "invoice", invA.ID)
	if err != nil || claim == nil {
		t.Fatalf("org-a entry: entry=%v err=%v", claim, err)
	}
	// Retag org A's live entry as org B's draft invoice.
	if _, err := d.DB.Exec(`UPDATE journal_entries SET sourceDocumentId = ? WHERE id = ?`, invB.ID, claim.ID); err != nil {
		t.Fatalf("retag: %v", err)
	}

	deleted, err := d.DeleteInvoice(invB.ID)
	if err != nil {
		t.Fatalf("DeleteInvoice(org-b draft named by org-a's entry): %v", err)
	}
	if !deleted {
		t.Fatal("DeleteInvoice = false, want true")
	}
	if _, err := d.GetInvoice(invB.ID); err == nil {
		t.Fatal("org-b invoice still exists after a successful delete")
	}
	items, err := d.GetInvoiceLineItems(invB.ID)
	if err != nil {
		t.Fatalf("GetInvoiceLineItems: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("org-b line items still exist after the invoice was deleted: %+v", items)
	}
}

// The same backstop exists for bills, with sourceDocumentType
// "incoming_invoice".
func TestDeleteIncomingInvoiceIgnoresAnotherOrganizationsEntryNamingIt(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fxA := newGLPostingTestFixture(t, d, "org-bill-del-claim-a")
	fxB := newGLPostingTestFixture(t, d, "org-bill-del-claim-b")
	billA := fxA.createIncomingInvoice(t, d, "bill-del-claim-a", 1, 1000)
	billB := fxB.createIncomingInvoice(t, d, "bill-del-claim-b", 2, 1000)

	if _, err := d.UpdateIncomingInvoiceState(billA.ID, "approved"); err != nil {
		t.Fatalf("UpdateIncomingInvoiceState(org-a approved): %v", err)
	}
	claim, err := d.FindPostedEntryForSourceDocument(fxA.orgID, "incoming_invoice", billA.ID)
	if err != nil || claim == nil {
		t.Fatalf("org-a entry: entry=%v err=%v", claim, err)
	}
	if _, err := d.DB.Exec(`UPDATE journal_entries SET sourceDocumentId = ? WHERE id = ?`, billB.ID, claim.ID); err != nil {
		t.Fatalf("retag: %v", err)
	}

	deleted, err := d.DeleteIncomingInvoice(billB.ID)
	if err != nil {
		t.Fatalf("DeleteIncomingInvoice(org-b draft named by org-a's entry): %v", err)
	}
	if !deleted {
		t.Fatal("DeleteIncomingInvoice = false, want true")
	}
	if _, err := d.GetIncomingInvoice(billB.ID); err == nil {
		t.Fatal("org-b bill still exists after a successful delete")
	}
	items, err := d.GetIncomingInvoiceLineItems(billB.ID)
	if err != nil {
		t.Fatalf("GetIncomingInvoiceLineItems: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("org-b bill line items still exist after the bill was deleted: %+v", items)
	}
}

// The organization scoping must not weaken the guard itself: an invoice
// named by its OWN live posted entry is still refused, and nothing is lost.
func TestDeleteInvoiceStillRefusedWithItsOwnPostedEntry(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-del-own")
	inv := fx.createInvoice(t, d, "inv-del-own", 1, 1000)

	if _, err := d.UpdateInvoiceState(inv.ID, "sent"); err != nil {
		t.Fatalf("UpdateInvoiceState(sent): %v", err)
	}

	_, err := d.DeleteInvoice(inv.ID)
	if err == nil {
		t.Fatal("expected deleting an invoice with its own posted GL entry to be refused")
	}
	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected a *ValidationError, got %T: %v", err, err)
	}
	if _, err := d.GetInvoice(inv.ID); err != nil {
		t.Fatalf("invoice should still exist: %v", err)
	}
	items, err := d.GetInvoiceLineItems(inv.ID)
	if err != nil {
		t.Fatalf("GetInvoiceLineItems: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("invoice line items = %d, want 1 still stored", len(items))
	}
}

// DeleteStockMovement's delivery/receipt guards matched deliveryNumber
// globally. Another organization's shipped delivery numbered like this
// movement's free-text reference must not block deleting it.
func TestDeleteStockMovementIgnoresAnotherOrganizationsDeliveryNumber(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	orgA, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-mv-ref-a"})
	if err != nil {
		t.Fatalf("CreateOrganization(org-a): %v", err)
	}
	orgB, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-mv-ref-b"})
	if err != nil {
		t.Fatalf("CreateOrganization(org-b): %v", err)
	}
	productA, err := d.CreateProduct(CreateProductRequest{
		ID: "prod-mv-ref-a", OrganizationID: orgA.ID, Name: "Widget A", Type: "product", StockEnabled: 1,
	})
	if err != nil {
		t.Fatalf("CreateProduct(org-a): %v", err)
	}
	productB, err := d.CreateProduct(CreateProductRequest{
		ID: "prod-mv-ref-b", OrganizationID: orgB.ID, Name: "Widget B", Type: "product", StockEnabled: 1,
	})
	if err != nil {
		t.Fatalf("CreateProduct(org-b): %v", err)
	}

	// An uncosted inflow posts no GL entry, so it stays deletable.
	created, err := d.CreateStockMovement(CreateStockMovementRequest{
		OrganizationID: orgA.ID, ProductID: productA.ID, Type: "in", Quantity: 10, Reference: ptr("DN-X-1"),
	})
	if err != nil {
		t.Fatalf("CreateStockMovement(org-a): %v", err)
	}
	if len(created.Movements) != 1 {
		t.Fatalf("org-a movements = %d, want 1", len(created.Movements))
	}

	delivery, err := d.CreateDelivery(CreateDeliveryRequest{
		ID: "del-mv-ref-b", OrganizationID: orgB.ID, DeliveryNumber: "DN-X-1", DeliveryDate: 1700000000000,
		LineItems: []CreateDeliveryLineItemRequest{
			{ProductID: &productB.ID, Description: "Widget B", Quantity: 1},
		},
	})
	if err != nil {
		t.Fatalf("CreateDelivery(org-b): %v", err)
	}
	// Shipping the delivery would need stock and COGS; the guard only reads
	// the status, so set it directly.
	if _, err := d.DB.Exec(`UPDATE outbound_deliveries SET status = 'shipped' WHERE id = ?`, delivery.ID); err != nil {
		t.Fatalf("mark org-b delivery shipped: %v", err)
	}

	deleted, err := d.DeleteStockMovement(created.Movements[0].ID)
	if err != nil {
		t.Fatalf("DeleteStockMovement(org-a movement referencing org-b's delivery number): %v", err)
	}
	if !deleted {
		t.Fatal("DeleteStockMovement = false, want true")
	}
}
