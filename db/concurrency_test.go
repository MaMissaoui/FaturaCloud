package db

import (
	"sync"
	"testing"
)

// concurrentlyRun fires fn from n goroutines released simultaneously via a
// closed channel (rather than launched sequentially) so their pre-transaction
// reads are as likely as possible to race each other before any of their
// transactions commit — the exact window F48 identifies. It returns each
// goroutine's error, indexed by call order.
func concurrentlyRun(n int, fn func(i int) error) []error {
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			errs[i] = fn(i)
		}(i)
	}
	close(start)
	wg.Wait()
	return errs
}

// TestConcurrentUpdateInvoiceStateDoesNotDoublePost is F48's regression test
// for the invoice auto-posting path: several concurrent draft->sent calls on
// the same invoice must all still leave exactly one posted GL entry for it,
// never one per call. Before the fix, FindPostedEntryForSourceDocument ran
// once before any of the calls' transactions began, so every goroutine could
// see "no posted entry" and each would post its own.
func TestConcurrentUpdateInvoiceStateDoesNotDoublePost(t *testing.T) {
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-concurrent-post")
	inv := fx.createInvoice(t, d, "concurrent-inv-1", 1, 1000)

	const n = 8
	errs := concurrentlyRun(n, func(i int) error {
		_, err := d.UpdateInvoiceState(inv.ID, "sent")
		return err
	})
	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: UpdateInvoiceState(sent): %v", i, err)
		}
	}

	var count int
	if err := d.DB.Get(&count, `
		SELECT COUNT(*) FROM journal_entries
		WHERE sourceDocumentType = 'invoice' AND sourceDocumentId = ?
		      AND status = 'posted' AND reversalOfEntryId IS NULL`,
		inv.ID,
	); err != nil {
		t.Fatalf("count posted entries: %v", err)
	}
	if count != 1 {
		t.Fatalf("posted entry count = %d, want exactly 1 — concurrent draft->sent calls double-posted", count)
	}
}

// TestConcurrentVoidPaymentDoesNotDoubleReverse is F48's regression test for
// VoidPayment: firing several concurrent voids against the same payment must
// let exactly one succeed and reverse the underlying entry exactly once.
// Before the fix, reverseEntryTx's final UPDATE had no `WHERE status =
// 'posted'` guard, so a second concurrent void could re-reverse an
// already-reversed entry.
func TestConcurrentVoidPaymentDoesNotDoubleReverse(t *testing.T) {
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-concurrent-void")
	inv := fx.createInvoice(t, d, "concurrent-void-inv-1", 1, 1000) // total 1200
	if _, err := d.UpdateInvoiceState(inv.ID, "sent"); err != nil {
		t.Fatalf("UpdateInvoiceState(sent): %v", err)
	}
	bank := accountByCode(t, d, fx.orgID, "1020")

	payment, err := d.CreatePayment(CreatePaymentRequest{
		OrganizationID: fx.orgID, Direction: "inbound", ClientID: &fx.clientID,
		BankAccountID: bank.ID, Amount: 1200, Currency: "EUR", Date: fx.date, Method: "bank_transfer",
		Applications: []CreatePaymentApplicationRequest{{DocumentType: "invoice", DocumentID: inv.ID, Amount: 1200}},
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}

	const n = 8
	errs := concurrentlyRun(n, func(i int) error {
		_, err := d.VoidPayment(payment.ID, fx.date)
		return err
	})
	successes := 0
	for _, err := range errs {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("VoidPayment succeeded %d times, want exactly 1", successes)
	}

	var reversalCount int
	if err := d.DB.Get(&reversalCount, `
		SELECT COUNT(*) FROM journal_entries WHERE reversalOfEntryId = ?`,
		*payment.JournalEntryID,
	); err != nil {
		t.Fatalf("count reversal entries: %v", err)
	}
	if reversalCount != 1 {
		t.Fatalf("reversal entry count = %d, want exactly 1 — concurrent voids double-reversed", reversalCount)
	}

	paid, err := d.GetInvoiceAmountPaid(inv.ID)
	if err != nil {
		t.Fatalf("GetInvoiceAmountPaid: %v", err)
	}
	if paid != 0 {
		t.Fatalf("GetInvoiceAmountPaid after concurrent voids = %d, want 0", paid)
	}
}

// TestConcurrentCreatePaymentDoesNotOverpay is F48's regression test for the
// CreatePayment overpay guard: two concurrent full-amount payments against
// the same invoice must let only one through. Before the fix, both requests
// computed "remaining balance" from a pre-transaction SUM, so both could pass
// the check and together over-settle the invoice.
func TestConcurrentCreatePaymentDoesNotOverpay(t *testing.T) {
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-concurrent-overpay")
	inv := fx.createInvoice(t, d, "concurrent-overpay-inv-1", 1, 1000) // total 1200
	if _, err := d.UpdateInvoiceState(inv.ID, "sent"); err != nil {
		t.Fatalf("UpdateInvoiceState(sent): %v", err)
	}
	bank := accountByCode(t, d, fx.orgID, "1020")

	const n = 5
	errs := concurrentlyRun(n, func(i int) error {
		_, err := d.CreatePayment(CreatePaymentRequest{
			OrganizationID: fx.orgID, Direction: "inbound", ClientID: &fx.clientID,
			BankAccountID: bank.ID, Amount: 1200, Currency: "EUR", Date: fx.date, Method: "bank_transfer",
			Applications: []CreatePaymentApplicationRequest{{DocumentType: "invoice", DocumentID: inv.ID, Amount: 1200}},
		})
		return err
	})
	successes := 0
	for _, err := range errs {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("CreatePayment succeeded %d times, want exactly 1 — invoice was over-settled", successes)
	}

	paid, err := d.GetInvoiceAmountPaid(inv.ID)
	if err != nil {
		t.Fatalf("GetInvoiceAmountPaid: %v", err)
	}
	if paid != 1200 {
		t.Fatalf("GetInvoiceAmountPaid = %d, want exactly 1200", paid)
	}
}

// TestConcurrentCloseFiscalYearPostsExactlyOneClosingEntry is F49's
// regression test: several concurrent close calls against the same fiscal
// year must let exactly one through and post exactly one closing entry.
// Before the fix, the final status flip had no `WHERE status = 'open'`
// re-check, so every concurrent call that read the year as still-open before
// any of them committed would each post its own closing entry.
func TestConcurrentCloseFiscalYearPostsExactlyOneClosingEntry(t *testing.T) {
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-concurrent-close")
	fyID := fecFixtureFiscalYearID(t, d, fx.orgID)

	inv := fx.createInvoice(t, d, "concurrent-close-inv-1", 2, 1000) // 2000 revenue
	if _, err := d.UpdateInvoiceState(inv.ID, "sent"); err != nil {
		t.Fatalf("UpdateInvoiceState(sent): %v", err)
	}

	const n = 8
	errs := concurrentlyRun(n, func(i int) error {
		_, err := d.CloseFiscalYear(fyID)
		return err
	})
	successes := 0
	for _, err := range errs {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("CloseFiscalYear succeeded %d times, want exactly 1", successes)
	}

	var closingCount int
	if err := d.DB.Get(&closingCount, `
		SELECT COUNT(*) FROM journal_entries
		WHERE sourceDocumentType = 'closing' AND sourceDocumentId = ? AND status = 'posted'`,
		fyID,
	); err != nil {
		t.Fatalf("count closing entries: %v", err)
	}
	if closingCount != 1 {
		t.Fatalf("closing entry count = %d, want exactly 1 — concurrent closes double-posted", closingCount)
	}

	year, err := d.GetFiscalYear(fyID)
	if err != nil {
		t.Fatalf("GetFiscalYear: %v", err)
	}
	if year.Status != "closed" {
		t.Fatalf("year status = %q, want closed", year.Status)
	}
}

// TestConcurrentDeleteJournalEntryDoesNotDeleteAPostedEntry is F50's
// regression test: racing DeleteJournalEntry against PostJournalEntry on the
// same draft must never end with a posted entry deleted. Before the fix, the
// DELETE had no `AND status = 'draft'` re-check, so a post that committed
// between the delete's pre-check read and its DELETE statement would still
// be removed. Note: unlike the other tests in this file, this one does not
// reliably fail against the pre-fix code — the vulnerable window is a single
// statement gap inside DeleteJournalEntry, too narrow for goroutine
// scheduling alone to hit consistently (confirmed empirically: 50 runs
// against the reverted code, zero failures). It's kept as a regression guard
// for the invariant regardless, and the fix itself (WHERE status = 'draft'
// plus a RowsAffected check) matches the same choke-point pattern already
// proven to close the race in every other test here.
func TestConcurrentDeleteJournalEntryDoesNotDeleteAPostedEntry(t *testing.T) {
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-concurrent-delete-post")

	entry, err := d.CreateJournalEntry(CreateJournalEntryRequest{
		OrganizationID: fx.orgID, JournalID: mustJournalID(t, d, fx.orgID, "OD"),
		Date: fx.date, Description: "raced by delete and post",
		Lines: []CreateJournalLineRequest{
			{AccountID: fx.revenueAccountID, Credit: 500},
			{AccountID: fx.arAccountID, Debit: 500},
		},
	})
	if err != nil {
		t.Fatalf("CreateJournalEntry: %v", err)
	}

	var postErr, deleteErr error
	var wg sync.WaitGroup
	start := make(chan struct{})
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_, postErr = d.PostJournalEntry(entry.ID)
	}()
	go func() {
		defer wg.Done()
		<-start
		_, deleteErr = d.DeleteJournalEntry(entry.ID)
	}()
	close(start)
	wg.Wait()

	if postErr == nil && deleteErr == nil {
		t.Fatal("expected exactly one of PostJournalEntry/DeleteJournalEntry to fail, both succeeded")
	}

	var status string
	err = d.DB.Get(&status, `SELECT status FROM journal_entries WHERE id = ?`, entry.ID)
	if postErr == nil {
		// Post won the race: the entry must still exist and be posted.
		if err != nil {
			t.Fatalf("expected the posted entry to still exist, got: %v", err)
		}
		if status != "posted" {
			t.Fatalf("entry status = %q, want posted", status)
		}
	} else {
		// Delete won the race: the entry must be gone entirely — never
		// left behind half-posted.
		if err == nil {
			t.Fatalf("expected the deleted entry to be gone, found status %q", status)
		}
	}
}
