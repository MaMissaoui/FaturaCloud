package db

import "testing"

// journalEntryTestFixture sets up the minimum a journal entry needs to post:
// an organization, a fiscal year covering the test date, a journal, and two
// postable (non-group) accounts to balance a line against.
type journalEntryTestFixture struct {
	orgID          string
	journalID      string
	cashAccountID  string
	salesAccountID string
	groupAccountID string
	date           int64
}

func newJournalEntryTestFixture(t *testing.T, d *Database) journalEntryTestFixture {
	t.Helper()

	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "je-test-org", Name: ptr("GL Test Org")})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}

	if _, err := d.CreateFiscalYear(CreateFiscalYearRequest{
		ID: "je-test-fy", OrganizationID: org.ID, Name: "2025",
		StartDate: 1735689600000, EndDate: 1767225599000, // 2025-01-01 .. 2025-12-31 (ms)
	}); err != nil {
		t.Fatalf("CreateFiscalYear: %v", err)
	}

	journal, err := d.CreateJournal(CreateJournalRequest{
		ID: "je-test-journal", OrganizationID: org.ID, Code: "MISC", Name: "Miscellaneous", Type: "miscellaneous",
	})
	if err != nil {
		t.Fatalf("CreateJournal: %v", err)
	}

	group, err := d.CreateAccount(CreateAccountRequest{
		// CreateOrganization auto-seeds a default chart of accounts (codes
		// 1000-5200, see seedDefaultChartOfAccounts) — use a disjoint range
		// here so these test accounts never collide with it.
		ID: "je-test-group", OrganizationID: org.ID, Code: "9000", Name: "Test Assets", Type: "asset", IsGroup: 1,
	})
	if err != nil {
		t.Fatalf("CreateAccount(group): %v", err)
	}
	cash, err := d.CreateAccount(CreateAccountRequest{
		ID: "je-test-cash", OrganizationID: org.ID, ParentID: &group.ID, Code: "9010", Name: "Test Cash", Type: "asset",
	})
	if err != nil {
		t.Fatalf("CreateAccount(cash): %v", err)
	}
	sales, err := d.CreateAccount(CreateAccountRequest{
		ID: "je-test-sales", OrganizationID: org.ID, Code: "9400", Name: "Test Sales Revenue", Type: "revenue",
	})
	if err != nil {
		t.Fatalf("CreateAccount(sales): %v", err)
	}

	return journalEntryTestFixture{
		orgID:          org.ID,
		journalID:      journal.ID,
		cashAccountID:  cash.ID,
		salesAccountID: sales.ID,
		groupAccountID: group.ID,
		date:           1738368000000, // 2025-02-01 in ms — within the fiscal year above
	}
}

func TestJournalEntryPostReverseRepostAllocatesSequentialNumbers(t *testing.T) {
	d := newTestDB(t)
	fx := newJournalEntryTestFixture(t, d)
	date := fx.date

	balancedLines := func() []CreateJournalLineRequest {
		return []CreateJournalLineRequest{
			{AccountID: fx.cashAccountID, Debit: 1000},
			{AccountID: fx.salesAccountID, Credit: 1000},
		}
	}

	entryA, err := d.CreateJournalEntry(CreateJournalEntryRequest{
		OrganizationID: fx.orgID, JournalID: fx.journalID, Date: date,
		Description: "Sale A", Lines: balancedLines(),
	})
	if err != nil {
		t.Fatalf("CreateJournalEntry(A): %v", err)
	}
	entryA, err = d.PostJournalEntry(entryA.ID)
	if err != nil {
		t.Fatalf("PostJournalEntry(A): %v", err)
	}
	if entryA.EntryNumber == nil || *entryA.EntryNumber != 1 {
		t.Fatalf("entry A entryNumber = %v, want 1", entryA.EntryNumber)
	}

	reversal, err := d.ReverseJournalEntry(entryA.ID, "correction", date)
	if err != nil {
		t.Fatalf("ReverseJournalEntry: %v", err)
	}
	if reversal.EntryNumber == nil || *reversal.EntryNumber != 2 {
		t.Fatalf("reversal entryNumber = %v, want 2", reversal.EntryNumber)
	}

	original, err := d.GetJournalEntry(entryA.ID)
	if err != nil {
		t.Fatalf("GetJournalEntry(original): %v", err)
	}
	if original.Status != "reversed" {
		t.Fatalf("original status = %q, want reversed", original.Status)
	}

	// Posting a brand-new entry after the reversal must not collide with
	// either of the two numbers already issued — this is exactly what
	// scoping the MAX+1 allocator to `entryNumber IS NOT NULL` (rather than
	// `status = 'posted'`) is for for; scoping to status='posted' would have
	// tried to re-issue entryNumber 1 (freed up when entryA flipped to
	// 'reversed') and hit the unique index.
	entryB, err := d.CreateJournalEntry(CreateJournalEntryRequest{
		OrganizationID: fx.orgID, JournalID: fx.journalID, Date: date,
		Description: "Sale B", Lines: balancedLines(),
	})
	if err != nil {
		t.Fatalf("CreateJournalEntry(B): %v", err)
	}
	entryB, err = d.PostJournalEntry(entryB.ID)
	if err != nil {
		t.Fatalf("PostJournalEntry(B): %v", err)
	}
	if entryB.EntryNumber == nil || *entryB.EntryNumber != 3 {
		t.Fatalf("entry B entryNumber = %v, want 3", entryB.EntryNumber)
	}

	// Trial balance after A+reversal+B must reflect only B: A and its
	// reversal must net to exactly zero per account, not double-count or
	// sign-flip (see GetTrialBalance's status IN ('posted','reversed') note).
	rows, err := d.GetTrialBalance(fx.orgID, "")
	if err != nil {
		t.Fatalf("GetTrialBalance: %v", err)
	}
	totals := map[string]struct{ debit, credit int64 }{}
	for _, r := range rows {
		totals[r.AccountID] = struct{ debit, credit int64 }{r.Debit, r.Credit}
	}
	cash := totals[fx.cashAccountID]
	if cash.debit-cash.credit != 1000 {
		t.Fatalf("cash net = %d, want 1000 (A's reversal should have zeroed A itself, leaving only B)", cash.debit-cash.credit)
	}
	sales := totals[fx.salesAccountID]
	if sales.credit-sales.debit != 1000 {
		t.Fatalf("sales net = %d, want 1000", sales.credit-sales.debit)
	}
}

func TestPostJournalEntryRejectsUnbalancedEntry(t *testing.T) {
	d := newTestDB(t)
	fx := newJournalEntryTestFixture(t, d)

	// CreateJournalEntry only validates each line individually (exactly one
	// of debit/credit set) — header balance is asserted at post time by
	// allocateAndFinalizeEntryTx, so build an intentionally-unbalanced draft
	// directly rather than going through the balanced-by-construction helper.
	entry, err := d.CreateJournalEntry(CreateJournalEntryRequest{
		OrganizationID: fx.orgID, JournalID: fx.journalID, Date: fx.date,
		Description: "Unbalanced",
		Lines: []CreateJournalLineRequest{
			{AccountID: fx.cashAccountID, Debit: 1000},
			{AccountID: fx.salesAccountID, Credit: 500},
		},
	})
	if err != nil {
		t.Fatalf("CreateJournalEntry: %v", err)
	}

	if _, err := d.PostJournalEntry(entry.ID); err == nil {
		t.Fatal("PostJournalEntry: expected an error for an unbalanced entry, got nil")
	}
}

func TestPostJournalEntryRejectsGroupAccount(t *testing.T) {
	d := newTestDB(t)
	fx := newJournalEntryTestFixture(t, d)

	entry, err := d.CreateJournalEntry(CreateJournalEntryRequest{
		OrganizationID: fx.orgID, JournalID: fx.journalID, Date: fx.date,
		Description: "Posted to a header account",
		Lines: []CreateJournalLineRequest{
			{AccountID: fx.groupAccountID, Debit: 1000},
			{AccountID: fx.salesAccountID, Credit: 1000},
		},
	})
	if err != nil {
		t.Fatalf("CreateJournalEntry: %v", err)
	}

	if _, err := d.PostJournalEntry(entry.ID); err == nil {
		t.Fatal("PostJournalEntry: expected an error for posting to a group account, got nil")
	}
}

func TestDeleteJournalEntryOnlyAllowsDraft(t *testing.T) {
	d := newTestDB(t)
	fx := newJournalEntryTestFixture(t, d)

	entry, err := d.CreateJournalEntry(CreateJournalEntryRequest{
		OrganizationID: fx.orgID, JournalID: fx.journalID, Date: fx.date,
		Description: "To be posted",
		Lines: []CreateJournalLineRequest{
			{AccountID: fx.cashAccountID, Debit: 1000},
			{AccountID: fx.salesAccountID, Credit: 1000},
		},
	})
	if err != nil {
		t.Fatalf("CreateJournalEntry: %v", err)
	}
	if _, err := d.PostJournalEntry(entry.ID); err != nil {
		t.Fatalf("PostJournalEntry: %v", err)
	}

	if _, err := d.DeleteJournalEntry(entry.ID); err == nil {
		t.Fatal("DeleteJournalEntry: expected an error for a posted entry, got nil")
	}

	reversal, err := d.ReverseJournalEntry(entry.ID, "undo", fx.date)
	if err != nil {
		t.Fatalf("ReverseJournalEntry: %v", err)
	}
	if _, err := d.DeleteJournalEntry(reversal.ID); err == nil {
		t.Fatal("DeleteJournalEntry: expected an error for a posted reversal entry, got nil")
	}
}
