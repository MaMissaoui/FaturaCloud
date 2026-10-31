package db

import (
	"errors"
	"testing"
)

// cashSaleRequestFor builds a one-line 12.00 sale at the fixture's usual
// 20% rate, with the given method and amount received.
func cashSaleRequestFor(fx glPostingTestFixture, method string, amountReceived int64) CreateCashSaleRequest {
	return CreateCashSaleRequest{
		OrganizationID: fx.orgID, ClientID: fx.clientID, Date: fx.date, Currency: "EUR",
		LineItems: []CreateInvoiceLineItemRequest{
			{Quantity: 1, UnitPrice: 1000, TaxRate: &fx.taxRateID, ProductID: &fx.productID},
		},
		SubTotal: 1000, TaxTotal: 200, Total: 1200,
		AmountReceived: amountReceived, PaymentMethod: method,
	}
}

// TestCashSaleCashOnlyRefusesCard is the default organization: the counter
// accepts cash only, so a card payment with money received is a 409. With
// nothing received there is no payment to post, so the method is irrelevant
// and the sale (a loan) is still accepted.
func TestCashSaleCashOnlyRefusesCard(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-cash-only-card")

	org, err := d.GetOrganization(fx.orgID)
	if err != nil {
		t.Fatalf("GetOrganization: %v", err)
	}
	if org.CashBookPaymentMethods != CashBookPaymentMethodsCashOnly {
		t.Fatalf("default cashBookPaymentMethods = %q, want cash_only", org.CashBookPaymentMethods)
	}

	_, err = d.CreateCashSale(cashSaleRequestFor(fx, "card", 1200))
	assertValidationErrorContains(t, err, "this organization's Cash Book accepts cash only")

	result, err := d.CreateCashSale(cashSaleRequestFor(fx, "card", 0))
	if err != nil {
		t.Fatalf("a zero-deposit sale with method card should be accepted: %v", err)
	}
	if result.Payment != nil {
		t.Fatal("a zero-deposit sale must not record a payment")
	}
}

// TestCashSaleCashAndBankPostsCardToBank is the opt-in mode: a card sale
// debits the organization's bank account (defaultCashAccountId), leaves the
// register untouched and never shows as cash "in" on the register's daily
// movements report.
func TestCashSaleCashAndBankPostsCardToBank(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-cash-and-bank-card")
	register := accountByCode(t, d, fx.orgID, "1010")
	org, err := d.GetOrganization(fx.orgID)
	if err != nil {
		t.Fatalf("GetOrganization: %v", err)
	}
	if org.DefaultCashAccountID == nil {
		t.Fatal("fixture: no default cash (bank) account")
	}
	bankID := *org.DefaultCashAccountID
	if bankID == register.ID {
		t.Fatal("fixture: bank and register are the same account")
	}
	mode := CashBookPaymentMethodsCashAndBank
	if _, err := d.UpdateOrganization(fx.orgID, UpdateOrganizationRequest{
		DefaultCashRegisterAccountID: &register.ID,
		CashBookPaymentMethods:       &mode,
	}); err != nil {
		t.Fatalf("UpdateOrganization: %v", err)
	}

	// BankAccountID is the old counter convention (it always sent the
	// register). A card payment must ignore it and still use the bank.
	req := cashSaleRequestFor(fx, "card", 1200)
	req.BankAccountID = register.ID
	result, err := d.CreateCashSale(req)
	if err != nil {
		t.Fatalf("CreateCashSale(card): %v", err)
	}
	if result.Payment == nil {
		t.Fatal("expected a card payment to be recorded")
	}
	if result.Payment.BankAccountID != bankID {
		t.Fatalf("payment bankAccountId = %q, want bank %q", result.Payment.BankAccountID, bankID)
	}
	if result.Payment.Method != "card" {
		t.Fatalf("payment method = %q, want card", result.Payment.Method)
	}

	registerBalance, err := d.GetAccountBalance(fx.orgID, register.ID, fx.date)
	if err != nil {
		t.Fatalf("GetAccountBalance(register): %v", err)
	}
	if registerBalance != 0 {
		t.Fatalf("register balance = %d after a card sale, want 0", registerBalance)
	}
	bankBalance, err := d.GetAccountBalance(fx.orgID, bankID, fx.date)
	if err != nil {
		t.Fatalf("GetAccountBalance(bank): %v", err)
	}
	if bankBalance != 1200 {
		t.Fatalf("bank balance = %d after a card sale, want 1200", bankBalance)
	}

	rows, err := d.GetDailyCashMovements(fx.orgID, register.ID, fx.date, fx.date)
	if err != nil {
		t.Fatalf("GetDailyCashMovements(register): %v", err)
	}
	for _, row := range rows {
		if row.In != 0 {
			t.Fatalf("register daily movements show %d in for a card sale, want 0", row.In)
		}
	}

	entry, err := d.postedEntryForSourceDocument("payment", result.Payment.ID)
	if err != nil || entry == nil {
		t.Fatalf("expected the card payment's entry, err=%v entry=%v", err, entry)
	}
	lines, err := d.GetJournalEntryLines(entry.ID)
	if err != nil {
		t.Fatalf("GetJournalEntryLines: %v", err)
	}
	bankDebit, bankCredit := sumLines(lines, bankID)
	if bankDebit != 1200 || bankCredit != 0 {
		t.Fatalf("bank line = debit %d credit %d, want debit 1200 credit 0", bankDebit, bankCredit)
	}
	registerDebit, registerCredit := sumLines(lines, register.ID)
	if registerDebit != 0 || registerCredit != 0 {
		t.Fatalf("register line = debit %d credit %d, want no line", registerDebit, registerCredit)
	}
}

// TestCashSaleCashStillPostsToRegister covers both modes: choosing cash
// always lands in the physical till with the cash journal.
func TestCashSaleCashStillPostsToRegister(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{CashBookPaymentMethodsCashOnly, CashBookPaymentMethodsCashAndBank} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			d := newTestDB(t)
			fx := newGLPostingTestFixture(t, d, "org-cash-still-register-"+mode)
			register := accountByCode(t, d, fx.orgID, "1010")
			org, err := d.GetOrganization(fx.orgID)
			if err != nil {
				t.Fatalf("GetOrganization: %v", err)
			}
			if _, err := d.UpdateOrganization(fx.orgID, UpdateOrganizationRequest{
				DefaultCashRegisterAccountID: &register.ID,
				CashBookPaymentMethods:       &mode,
			}); err != nil {
				t.Fatalf("UpdateOrganization: %v", err)
			}

			result, err := d.CreateCashSale(cashSaleRequestFor(fx, "cash", 1200))
			if err != nil {
				t.Fatalf("CreateCashSale(cash): %v", err)
			}
			if result.Payment == nil || result.Payment.BankAccountID != register.ID {
				t.Fatalf("payment = %+v, want the register %q", result.Payment, register.ID)
			}
			if result.Payment.Method != "cash" {
				t.Fatalf("payment method = %q, want cash", result.Payment.Method)
			}

			balance, err := d.GetAccountBalance(fx.orgID, register.ID, fx.date)
			if err != nil {
				t.Fatalf("GetAccountBalance: %v", err)
			}
			if balance != 1200 {
				t.Fatalf("register balance = %d, want 1200", balance)
			}
			if org.DefaultCashAccountID != nil {
				bankBalance, err := d.GetAccountBalance(fx.orgID, *org.DefaultCashAccountID, fx.date)
				if err != nil {
					t.Fatalf("GetAccountBalance(bank): %v", err)
				}
				if bankBalance != 0 {
					t.Fatalf("bank balance = %d after a cash sale, want 0", bankBalance)
				}
			}

			rows, err := d.GetDailyCashMovements(fx.orgID, register.ID, fx.date, fx.date)
			if err != nil {
				t.Fatalf("GetDailyCashMovements: %v", err)
			}
			var in int64
			for _, row := range rows {
				in += row.In
			}
			if in != 1200 {
				t.Fatalf("register daily movements in = %d, want 1200", in)
			}
		})
	}
}

// TestOrganizationCashBookPaymentMethodsValidation covers the setting
// itself: default cash_only on create, accepted values, omitted keeps, and
// a 409 (ValidationError) for anything else.
func TestOrganizationCashBookPaymentMethodsValidation(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)

	org, err := d.CreateOrganization(CreateOrganizationRequest{ID: "org-cbpm-default"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if org.CashBookPaymentMethods != CashBookPaymentMethodsCashOnly {
		t.Fatalf("default cashBookPaymentMethods = %q, want cash_only", org.CashBookPaymentMethods)
	}

	explicit, err := d.CreateOrganization(CreateOrganizationRequest{
		ID: "org-cbpm-explicit", CashBookPaymentMethods: ptr(CashBookPaymentMethodsCashAndBank),
	})
	if err != nil {
		t.Fatalf("CreateOrganization(cash_and_bank): %v", err)
	}
	if explicit.CashBookPaymentMethods != CashBookPaymentMethodsCashAndBank {
		t.Fatalf("created cashBookPaymentMethods = %q, want cash_and_bank", explicit.CashBookPaymentMethods)
	}

	updated, err := d.UpdateOrganization(org.ID, UpdateOrganizationRequest{
		CashBookPaymentMethods: ptr(CashBookPaymentMethodsCashAndBank),
	})
	if err != nil {
		t.Fatalf("UpdateOrganization(cash_and_bank): %v", err)
	}
	if updated.CashBookPaymentMethods != CashBookPaymentMethodsCashAndBank {
		t.Fatalf("updated cashBookPaymentMethods = %q, want cash_and_bank", updated.CashBookPaymentMethods)
	}

	// Omitting it from another update keeps the stored mode.
	kept, err := d.UpdateOrganization(org.ID, UpdateOrganizationRequest{Name: ptr("Renamed")})
	if err != nil {
		t.Fatalf("UpdateOrganization(omitted): %v", err)
	}
	if kept.CashBookPaymentMethods != CashBookPaymentMethodsCashAndBank {
		t.Fatalf("an update that omits cashBookPaymentMethods changed it to %q", kept.CashBookPaymentMethods)
	}

	bad := "card_only"
	var verr *ValidationError
	if _, err := d.UpdateOrganization(org.ID, UpdateOrganizationRequest{
		CashBookPaymentMethods: &bad,
	}); !errors.As(err, &verr) {
		t.Fatalf("a bad cashBookPaymentMethods should be a *ValidationError, got %T (%v)", err, err)
	}
	if _, err := d.CreateOrganization(CreateOrganizationRequest{
		ID: "org-cbpm-bad", CashBookPaymentMethods: &bad,
	}); !errors.As(err, &verr) {
		t.Fatalf("CreateOrganization with a bad cashBookPaymentMethods should be a *ValidationError, got %T (%v)", err, err)
	}
}
