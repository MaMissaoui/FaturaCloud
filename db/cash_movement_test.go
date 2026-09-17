package db

import "testing"

func cashMovementTestAccounts(t *testing.T, d *Database, orgID string) (register, bank, expense Account) {
	t.Helper()
	return accountByCode(t, d, orgID, "1010"), // Cash
		accountByCode(t, d, orgID, "1020"), // Bank
		accountByCode(t, d, orgID, "5100")
}

func TestCreateCashMovementBankDeposit(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-cash-movement-deposit")
	register, bank, _ := cashMovementTestAccounts(t, d, fx.orgID)

	result, err := d.CreateCashMovement(CreateCashMovementRequest{
		OrganizationID: fx.orgID, AccountID: register.ID, Date: fx.date,
		CounterAccountType: "bank", CounterAccountID: bank.ID, Amount: 5000,
	})
	if err != nil {
		t.Fatalf("CreateCashMovement: %v", err)
	}
	if result.Balance != -5000 {
		t.Fatalf("register balance = %d, want -5000", result.Balance)
	}
	if result.CashMovement.JournalEntryID == nil {
		t.Fatal("expected a linked journal entry")
	}

	entry, err := d.FindPostedEntryForSourceDocument("cash_movement", result.CashMovement.ID)
	if err != nil || entry == nil {
		t.Fatalf("expected a posted entry for the cash movement, err=%v entry=%v", err, entry)
	}
	lines, err := d.GetJournalEntryLines(entry.ID)
	if err != nil {
		t.Fatalf("GetJournalEntryLines: %v", err)
	}
	bankDebit, bankCredit := sumLines(lines, bank.ID)
	registerDebit, registerCredit := sumLines(lines, register.ID)
	if bankDebit != 5000 || bankCredit != 0 {
		t.Fatalf("bank leg = debit %d credit %d, want debit 5000 credit 0", bankDebit, bankCredit)
	}
	if registerDebit != 0 || registerCredit != 5000 {
		t.Fatalf("register leg = debit %d credit %d, want debit 0 credit 5000", registerDebit, registerCredit)
	}

	bankBalance, err := d.GetAccountBalance(fx.orgID, bank.ID, 0)
	if err != nil {
		t.Fatalf("GetAccountBalance(bank): %v", err)
	}
	if bankBalance != 5000 {
		t.Fatalf("bank balance = %d, want 5000", bankBalance)
	}
}

func TestCreateCashMovementExpense(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-cash-movement-expense")
	register, _, expense := cashMovementTestAccounts(t, d, fx.orgID)

	note := "Bought cleaning supplies"
	result, err := d.CreateCashMovement(CreateCashMovementRequest{
		OrganizationID: fx.orgID, AccountID: register.ID, Date: fx.date,
		CounterAccountType: "expense", CounterAccountID: expense.ID, Amount: 1500, Note: &note,
	})
	if err != nil {
		t.Fatalf("CreateCashMovement: %v", err)
	}
	if result.Balance != -1500 {
		t.Fatalf("register balance = %d, want -1500", result.Balance)
	}
}

func TestCreateCashMovementDefaultsToOrganizationRegisterAccount(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-cash-movement-defaultacct")
	register, bank, _ := cashMovementTestAccounts(t, d, fx.orgID)

	if _, err := d.UpdateOrganization(fx.orgID, UpdateOrganizationRequest{
		DefaultCashRegisterAccountID: &register.ID,
	}); err != nil {
		t.Fatalf("UpdateOrganization: %v", err)
	}

	result, err := d.CreateCashMovement(CreateCashMovementRequest{
		OrganizationID: fx.orgID, Date: fx.date, // AccountID deliberately omitted
		CounterAccountType: "bank", CounterAccountID: bank.ID, Amount: 2000,
	})
	if err != nil {
		t.Fatalf("CreateCashMovement: %v", err)
	}
	if result.CashMovement.AccountID != register.ID {
		t.Fatalf("movement accountId = %q, want the organization's default register account %q",
			result.CashMovement.AccountID, register.ID)
	}
}

func TestCreateCashMovementNoRegisterAccountConfiguredRejected(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-cash-movement-noregister")
	_, bank, _ := cashMovementTestAccounts(t, d, fx.orgID)

	_, err := d.CreateCashMovement(CreateCashMovementRequest{
		OrganizationID: fx.orgID, Date: fx.date, // AccountID omitted, no org default either
		CounterAccountType: "bank", CounterAccountID: bank.ID, Amount: 2000,
	})
	if err == nil {
		t.Fatal("expected rejection with no register account configured or given")
	}
}

func TestCreateCashMovementWrongCounterAccountTypeRejected(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-cash-movement-wrongtype")
	register, bank, expense := cashMovementTestAccounts(t, d, fx.orgID)

	if _, err := d.CreateCashMovement(CreateCashMovementRequest{
		OrganizationID: fx.orgID, AccountID: register.ID, Date: fx.date,
		CounterAccountType: "bank", CounterAccountID: expense.ID, Amount: 1000,
	}); err == nil {
		t.Fatal("expected rejection: expense account picked for a 'bank' deposit")
	}
	if _, err := d.CreateCashMovement(CreateCashMovementRequest{
		OrganizationID: fx.orgID, AccountID: register.ID, Date: fx.date,
		CounterAccountType: "expense", CounterAccountID: bank.ID, Amount: 1000,
	}); err == nil {
		t.Fatal("expected rejection: bank account picked for an 'expense' spend")
	}
}

func TestCreateCashMovementInvalidCounterAccountTypeRejected(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-cash-movement-badtype")
	register, bank, _ := cashMovementTestAccounts(t, d, fx.orgID)

	_, err := d.CreateCashMovement(CreateCashMovementRequest{
		OrganizationID: fx.orgID, AccountID: register.ID, Date: fx.date,
		CounterAccountType: "vault", CounterAccountID: bank.ID, Amount: 1000,
	})
	if err == nil {
		t.Fatal("expected rejection of an unrecognized counter account type")
	}
}

func TestCreateCashMovementSameAccountBothSidesRejected(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-cash-movement-samesame")
	register, _, _ := cashMovementTestAccounts(t, d, fx.orgID)

	_, err := d.CreateCashMovement(CreateCashMovementRequest{
		OrganizationID: fx.orgID, AccountID: register.ID, Date: fx.date,
		CounterAccountType: "bank", CounterAccountID: register.ID, Amount: 1000,
	})
	if err == nil {
		t.Fatal("expected rejection when the counter account is the register account itself")
	}
}

func TestCreateCashMovementCrossOrgAccountsRejected(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-cash-movement-crossorg-a")
	other := newGLPostingTestFixture(t, d, "org-cash-movement-crossorg-b")
	register, bank, _ := cashMovementTestAccounts(t, d, fx.orgID)
	_, otherBank, _ := cashMovementTestAccounts(t, d, other.orgID)

	if _, err := d.CreateCashMovement(CreateCashMovementRequest{
		OrganizationID: fx.orgID, AccountID: register.ID, Date: fx.date,
		CounterAccountType: "bank", CounterAccountID: otherBank.ID, Amount: 1000,
	}); err == nil {
		t.Fatal("expected rejection: counter account belongs to a different organization")
	}

	otherRegister, _, _ := cashMovementTestAccounts(t, d, other.orgID)
	if _, err := d.CreateCashMovement(CreateCashMovementRequest{
		OrganizationID: fx.orgID, AccountID: otherRegister.ID, Date: fx.date,
		CounterAccountType: "bank", CounterAccountID: bank.ID, Amount: 1000,
	}); err == nil {
		t.Fatal("expected rejection: register account belongs to a different organization")
	}
}

func TestCreateCashMovementClosedFiscalYearRejectsBeforeAnyWrite(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-cash-movement-noyear")
	register, bank, _ := cashMovementTestAccounts(t, d, fx.orgID)

	farFutureDate := int64(1893456000000) // 2030-01-01, outside fx's 2025-only fiscal year
	_, err := d.CreateCashMovement(CreateCashMovementRequest{
		OrganizationID: fx.orgID, AccountID: register.ID, Date: farFutureDate,
		CounterAccountType: "bank", CounterAccountID: bank.ID, Amount: 1000,
	})
	if err == nil {
		t.Fatal("expected a date outside any open fiscal year to be rejected")
	}

	var count int
	if err := d.DB.Get(&count, `SELECT COUNT(*) FROM cash_movements WHERE organizationId = ?`, fx.orgID); err != nil {
		t.Fatalf("count cash_movements: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no cash_movements row committed on rejection, got %d", count)
	}
}

func TestGetAccountBalanceRejectsGroupAccount(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-cash-movement-groupacct")
	assets := accountByCode(t, d, fx.orgID, "1000") // "Assets" — isGroup=1

	if _, err := d.GetAccountBalance(fx.orgID, assets.ID, 0); err == nil {
		t.Fatal("expected a group account to be rejected")
	}
}
