package db

import "testing"

func TestCloseFiscalYearZeroesRevenueAndExpenseIntoRetainedEarnings(t *testing.T) {
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-close-happy")
	fyID := fecFixtureFiscalYearID(t, d, fx.orgID)

	inv := fx.createInvoice(t, d, "close-inv-1", 2, 1000) // 2000 revenue
	if _, err := d.UpdateInvoiceState(inv.ID, "sent"); err != nil {
		t.Fatalf("UpdateInvoiceState(sent): %v", err)
	}
	bill := fx.createIncomingInvoice(t, d, "close-bill-1", 1, 1200) // 1200 expense
	if _, err := d.UpdateIncomingInvoiceState(bill.ID, "approved"); err != nil {
		t.Fatalf("UpdateIncomingInvoiceState(approved): %v", err)
	}

	closed, err := d.CloseFiscalYear(fyID)
	if err != nil {
		t.Fatalf("CloseFiscalYear: %v", err)
	}
	if closed.Status != "closed" {
		t.Fatalf("status = %q, want closed", closed.Status)
	}
	if closed.ClosedAt == nil {
		t.Fatal("expected ClosedAt to be set")
	}

	entry, err := d.FindPostedEntryForSourceDocument("closing", fyID)
	if err != nil || entry == nil {
		t.Fatalf("expected a posted closing entry, err=%v entry=%v", err, entry)
	}

	tb, err := d.GetTrialBalance(fx.orgID, "")
	if err != nil {
		t.Fatalf("GetTrialBalance: %v", err)
	}
	balances := map[string]int64{}
	for _, row := range tb {
		balances[row.AccountID] = row.Debit - row.Credit
	}
	if balances[fx.revenueAccountID] != 0 {
		t.Fatalf("revenue account balance = %d, want 0 after closing", balances[fx.revenueAccountID])
	}
	if balances[fx.expenseAccountID] != 0 {
		t.Fatalf("expense account balance = %d, want 0 after closing", balances[fx.expenseAccountID])
	}

	org, err := d.GetOrganization(fx.orgID)
	if err != nil {
		t.Fatalf("GetOrganization: %v", err)
	}
	retainedBalance := -balances[*org.RetainedEarningsAccountID] // credit-normal: negate the debit-credit reading
	if retainedBalance != 800 {                                  // 2000 revenue - 1200 expense
		t.Fatalf("retained earnings balance = %d, want 800", retainedBalance)
	}
}

func TestCloseFiscalYearRejectsAlreadyClosed(t *testing.T) {
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-close-twice")
	fyID := fecFixtureFiscalYearID(t, d, fx.orgID)

	if _, err := d.CloseFiscalYear(fyID); err != nil {
		t.Fatalf("CloseFiscalYear: %v", err)
	}
	if _, err := d.CloseFiscalYear(fyID); err == nil {
		t.Fatal("expected an error when closing an already-closed year")
	}
}

func TestCloseFiscalYearBlocksNewEntriesAfterward(t *testing.T) {
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-close-blocks")
	fyID := fecFixtureFiscalYearID(t, d, fx.orgID)

	if _, err := d.CloseFiscalYear(fyID); err != nil {
		t.Fatalf("CloseFiscalYear: %v", err)
	}

	inv := fx.createInvoice(t, d, "close-blocked-inv", 1, 1000)
	_, err := d.UpdateInvoiceState(inv.ID, "sent")
	if err == nil {
		t.Fatal("expected posting into a closed fiscal year to be rejected")
	}
	// The message must say the year is closed, not "create one first" — a
	// year covering this date does exist, it's just closed, and telling the
	// user to create one would point them the wrong way.
	if got := err.Error(); got != "cannot post: the fiscal year covering this date is closed" {
		t.Fatalf("error = %q, want the closed-year message", got)
	}
}

// TestCloseFiscalYearClosesItsOpenPeriodsToo is the regression test for a
// display-consistency gap: CloseFiscalYear used to leave every fiscal_periods
// row at status='open' under a closed year. Posting was still correctly
// blocked either way (resolveFiscalPeriodForDate gates on the year first),
// but the fiscal periods page would keep showing "Open" periods, with
// working close/reopen buttons, under a year that no longer accepts entries.
func TestCloseFiscalYearClosesItsOpenPeriodsToo(t *testing.T) {
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-close-periods")
	fyID := fecFixtureFiscalYearID(t, d, fx.orgID)

	period, err := d.CreateFiscalPeriod(CreateFiscalPeriodRequest{
		OrganizationID: fx.orgID, FiscalYearID: fyID, Name: "Q1",
		StartDate: 1735689600000, EndDate: 1743465599000,
	})
	if err != nil {
		t.Fatalf("CreateFiscalPeriod: %v", err)
	}

	if _, err := d.CloseFiscalYear(fyID); err != nil {
		t.Fatalf("CloseFiscalYear: %v", err)
	}

	closedPeriod, err := d.GetFiscalPeriod(period.ID)
	if err != nil {
		t.Fatalf("GetFiscalPeriod: %v", err)
	}
	if closedPeriod.Status != "closed" {
		t.Fatalf("period status = %q, want closed", closedPeriod.Status)
	}
	if closedPeriod.ClosedAt == nil {
		t.Fatal("expected the period's ClosedAt to be set")
	}
}

func TestCloseFiscalYearWithNoActivityPostsNoEntry(t *testing.T) {
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-close-empty")
	fyID := fecFixtureFiscalYearID(t, d, fx.orgID)

	closed, err := d.CloseFiscalYear(fyID)
	if err != nil {
		t.Fatalf("CloseFiscalYear: %v", err)
	}
	if closed.Status != "closed" {
		t.Fatalf("status = %q, want closed", closed.Status)
	}

	entry, err := d.FindPostedEntryForSourceDocument("closing", fyID)
	if err != nil {
		t.Fatalf("FindPostedEntryForSourceDocument: %v", err)
	}
	if entry != nil {
		t.Fatalf("expected no closing entry when there was no revenue/expense activity, got %+v", entry)
	}
}

// TestCloseFiscalYearBlocksPostingAStrandedDraft is the regression test for
// the one gap allocateAndFinalizeEntryTx's fiscal-year-open check closes:
// unlike auto-posting/payments/reversal, PostJournalEntry doesn't re-resolve
// the entry's fiscal year — a draft created while the year was open still
// carries that fiscalYearId after the year closes. Without the check, this
// draft could still be posted afterward, landing revenue/expense into a
// year whose closing entry was already computed.
func TestCloseFiscalYearBlocksPostingAStrandedDraft(t *testing.T) {
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-close-stranded-draft")
	fyID := fecFixtureFiscalYearID(t, d, fx.orgID)

	entry, err := d.CreateJournalEntry(CreateJournalEntryRequest{
		OrganizationID: fx.orgID, JournalID: mustJournalID(t, d, fx.orgID, "OD"),
		Date: fx.date, Description: "drafted before close, posted after",
		Lines: []CreateJournalLineRequest{
			{AccountID: fx.revenueAccountID, Credit: 500},
			{AccountID: fx.arAccountID, Debit: 500},
		},
	})
	if err != nil {
		t.Fatalf("CreateJournalEntry: %v", err)
	}

	if _, err := d.CloseFiscalYear(fyID); err != nil {
		t.Fatalf("CloseFiscalYear: %v", err)
	}

	if _, err := d.PostJournalEntry(entry.ID); err == nil {
		t.Fatal("expected posting a stranded draft into a closed fiscal year to be rejected")
	}
}

func TestCloseFiscalYearRejectsMissingRetainedEarningsAccount(t *testing.T) {
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-close-no-re")
	fyID := fecFixtureFiscalYearID(t, d, fx.orgID)

	inv := fx.createInvoice(t, d, "close-no-re-inv", 1, 1000)
	if _, err := d.UpdateInvoiceState(inv.ID, "sent"); err != nil {
		t.Fatalf("UpdateInvoiceState(sent): %v", err)
	}
	if _, err := d.DB.Exec(`UPDATE organizations SET retainedEarningsAccountId = NULL WHERE id = ?`, fx.orgID); err != nil {
		t.Fatalf("clear retainedEarningsAccountId: %v", err)
	}

	if _, err := d.CloseFiscalYear(fyID); err == nil {
		t.Fatal("expected an error when no retained earnings account is configured")
	}

	year, err := d.GetFiscalYear(fyID)
	if err != nil {
		t.Fatalf("GetFiscalYear: %v", err)
	}
	if year.Status != "open" {
		t.Fatalf("year status = %q, want open — the failed close must not have partially applied", year.Status)
	}
}
