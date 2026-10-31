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

	// BankAccountID is now only a compatibility field: the server derives the
	// destination from the method and setting, so the caller doesn't send it.
	req := cashSaleRequestFor(fx, "card", 1200)
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

// cashAndBankWithRegister switches an organization to cash_and_bank and wires
// its register, returning the seeded bank and the register ids.
func cashAndBankWithRegister(t *testing.T, d *Database, orgID string) (bankID, registerID string) {
	t.Helper()
	register := accountByCode(t, d, orgID, "1010")
	org, err := d.GetOrganization(orgID)
	if err != nil {
		t.Fatalf("GetOrganization: %v", err)
	}
	if org.DefaultCashAccountID == nil {
		t.Fatal("fixture: no default cash (bank) account")
	}
	bankID = *org.DefaultCashAccountID
	if bankID == register.ID {
		t.Fatal("fixture: bank and register are the same account")
	}
	mode := CashBookPaymentMethodsCashAndBank
	if _, err := d.UpdateOrganization(orgID, UpdateOrganizationRequest{
		DefaultCashRegisterAccountID: &register.ID,
		CashBookPaymentMethods:       &mode,
	}); err != nil {
		t.Fatalf("UpdateOrganization(cash_and_bank): %v", err)
	}
	return bankID, register.ID
}

func openLoanForPayment(t *testing.T, d *Database, fx glPostingTestFixture) (invoiceID, lineID string) {
	t.Helper()
	result, err := d.CreateCashSale(cashSaleRequestFor(fx, "cash", 0))
	if err != nil {
		t.Fatalf("CreateCashSale(loan): %v", err)
	}
	items, err := d.GetInvoiceLineItems(result.Invoice.ID)
	if err != nil {
		t.Fatalf("GetInvoiceLineItems: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("loan line items = %d, want 1", len(items))
	}
	return result.Invoice.ID, items[0].ID
}

func journalIDByType(t *testing.T, d *Database, orgID, journalType string) string {
	t.Helper()
	journal, err := getJournalByTypeTx(d.DB, orgID, journalType)
	if err != nil {
		t.Fatalf("getJournalByTypeTx(%s): %v", journalType, err)
	}
	return journal.ID
}

// TestCashSaleCashAndBankRequiresBankAccount: cash_and_bank without a default
// bank account refuses a card sale instead of silently falling back to the
// register.
func TestCashSaleCashAndBankRequiresBankAccount(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-cbpm-no-bank")
	register := accountByCode(t, d, fx.orgID, "1010")
	mode := CashBookPaymentMethodsCashAndBank
	cleared := ""
	if _, err := d.UpdateOrganization(fx.orgID, UpdateOrganizationRequest{
		DefaultCashRegisterAccountID: &register.ID,
		CashBookPaymentMethods:       &mode,
		DefaultCashAccountID:         &cleared,
	}); err != nil {
		t.Fatalf("UpdateOrganization: %v", err)
	}

	_, err := d.CreateCashSale(cashSaleRequestFor(fx, "card", 1200))
	assertValidationErrorContains(t, err, "no default bank account configured")
}

// TestCashSaleNonCashMethodsPostToBank: every non-cash method goes to
// defaultCashAccountId on the "bank" journal, not the till.
func TestCashSaleNonCashMethodsPostToBank(t *testing.T) {
	t.Parallel()
	for _, method := range []string{"card", "check", "bank_transfer", "direct_debit", "other"} {
		t.Run(method, func(t *testing.T) {
			t.Parallel()
			d := newTestDB(t)
			fx := newGLPostingTestFixture(t, d, "org-cbpm-bank-"+method)
			bankID, registerID := cashAndBankWithRegister(t, d, fx.orgID)
			bankJournalID := journalIDByType(t, d, fx.orgID, "bank")

			result, err := d.CreateCashSale(cashSaleRequestFor(fx, method, 1200))
			if err != nil {
				t.Fatalf("CreateCashSale(%s): %v", method, err)
			}
			if result.Payment == nil {
				t.Fatal("expected a payment")
			}
			if result.Payment.BankAccountID != bankID {
				t.Fatalf("payment bankAccountId = %q, want bank %q", result.Payment.BankAccountID, bankID)
			}
			if result.Payment.Method != method {
				t.Fatalf("payment method = %q, want %q", result.Payment.Method, method)
			}
			entry, err := d.postedEntryForSourceDocument("payment", result.Payment.ID)
			if err != nil || entry == nil {
				t.Fatalf("payment entry: err=%v entry=%v", err, entry)
			}
			if entry.JournalID != bankJournalID {
				t.Fatalf("payment journal = %q, want bank journal %q", entry.JournalID, bankJournalID)
			}
			if balance, err := d.GetAccountBalance(fx.orgID, bankID, fx.date); err != nil || balance != 1200 {
				t.Fatalf("bank balance = %d (err %v), want 1200", balance, err)
			}
			if balance, err := d.GetAccountBalance(fx.orgID, registerID, fx.date); err != nil || balance != 0 {
				t.Fatalf("register balance = %d (err %v), want 0", balance, err)
			}
		})
	}
}

// TestCashSalePartialCardDepositPostsToBank: a deposit smaller than the total
// is still non-cash money and lands in the bank.
func TestCashSalePartialCardDepositPostsToBank(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-cbpm-partial-card")
	bankID, registerID := cashAndBankWithRegister(t, d, fx.orgID)

	result, err := d.CreateCashSale(cashSaleRequestFor(fx, "card", 400))
	if err != nil {
		t.Fatalf("CreateCashSale(partial card): %v", err)
	}
	if result.Payment == nil || result.Payment.BankAccountID != bankID {
		t.Fatalf("payment = %+v, want the bank %q", result.Payment, bankID)
	}
	if result.Payment.Amount != 400 {
		t.Fatalf("payment amount = %d, want 400", result.Payment.Amount)
	}
	if result.Invoice.State != "sent" {
		t.Fatalf("invoice state = %q after a partial deposit, want sent", result.Invoice.State)
	}
	if balance, err := d.GetAccountBalance(fx.orgID, bankID, fx.date); err != nil || balance != 400 {
		t.Fatalf("bank balance = %d (err %v), want 400", balance, err)
	}
	if balance, err := d.GetAccountBalance(fx.orgID, registerID, fx.date); err != nil || balance != 0 {
		t.Fatalf("register balance = %d (err %v), want 0", balance, err)
	}
}

// TestCashSalePaymentUsesCashBookPaymentMethods covers the loan collection
// path: cash goes to the register, card is refused in cash_only, and card in
// cash_and_bank goes to the bank with the bank journal and method stored.
func TestCashSalePaymentUsesCashBookPaymentMethods(t *testing.T) {
	t.Parallel()

	t.Run("cash goes to the register", func(t *testing.T) {
		t.Parallel()
		d := newTestDB(t)
		fx := newGLPostingTestFixture(t, d, "org-cbpm-pay-cash")
		register := accountByCode(t, d, fx.orgID, "1010")
		if _, err := d.UpdateOrganization(fx.orgID, UpdateOrganizationRequest{
			DefaultCashRegisterAccountID: &register.ID,
		}); err != nil {
			t.Fatalf("UpdateOrganization: %v", err)
		}
		invoiceID, lineID := openLoanForPayment(t, d, fx)
		result, err := d.CreateCashSalePayment(CreateCashSalePaymentRequest{
			InvoiceID: invoiceID, InvoiceLineItemID: lineID, Amount: 1200, Date: fx.date,
		})
		if err != nil {
			t.Fatalf("CreateCashSalePayment(cash): %v", err)
		}
		if result.Payment.Method != "cash" || result.Payment.BankAccountID != register.ID {
			t.Fatalf("payment = %s/%s, want cash/register %s", result.Payment.Method, result.Payment.BankAccountID, register.ID)
		}
		entry, err := d.postedEntryForSourceDocument("payment", result.Payment.ID)
		if err != nil || entry == nil {
			t.Fatalf("payment entry: err=%v entry=%v", err, entry)
		}
		if entry.JournalID != journalIDByType(t, d, fx.orgID, "cash") {
			t.Fatal("cash loan payment did not post to the cash journal")
		}
	})

	t.Run("card refused in cash_only", func(t *testing.T) {
		t.Parallel()
		d := newTestDB(t)
		fx := newGLPostingTestFixture(t, d, "org-cbpm-pay-card-only")
		register := accountByCode(t, d, fx.orgID, "1010")
		if _, err := d.UpdateOrganization(fx.orgID, UpdateOrganizationRequest{
			DefaultCashRegisterAccountID: &register.ID,
		}); err != nil {
			t.Fatalf("UpdateOrganization: %v", err)
		}
		invoiceID, lineID := openLoanForPayment(t, d, fx)
		_, err := d.CreateCashSalePayment(CreateCashSalePaymentRequest{
			InvoiceID: invoiceID, InvoiceLineItemID: lineID, Amount: 1200,
			Date: fx.date, PaymentMethod: "card",
		})
		assertValidationErrorContains(t, err, "this organization's Cash Book accepts cash only")
	})

	t.Run("card in cash_and_bank goes to the bank", func(t *testing.T) {
		t.Parallel()
		d := newTestDB(t)
		fx := newGLPostingTestFixture(t, d, "org-cbpm-pay-card")
		bankID, registerID := cashAndBankWithRegister(t, d, fx.orgID)
		invoiceID, lineID := openLoanForPayment(t, d, fx)
		result, err := d.CreateCashSalePayment(CreateCashSalePaymentRequest{
			InvoiceID: invoiceID, InvoiceLineItemID: lineID, Amount: 1200,
			Date: fx.date, PaymentMethod: "card",
		})
		if err != nil {
			t.Fatalf("CreateCashSalePayment(card): %v", err)
		}
		if result.Payment.Method != "card" || result.Payment.BankAccountID != bankID {
			t.Fatalf("payment = %s/%s, want card/bank %s", result.Payment.Method, result.Payment.BankAccountID, bankID)
		}
		entry, err := d.postedEntryForSourceDocument("payment", result.Payment.ID)
		if err != nil || entry == nil {
			t.Fatalf("payment entry: err=%v entry=%v", err, entry)
		}
		if entry.JournalID != journalIDByType(t, d, fx.orgID, "bank") {
			t.Fatal("card loan payment did not post to the bank journal")
		}
		if balance, err := d.GetAccountBalance(fx.orgID, bankID, fx.date); err != nil || balance != 1200 {
			t.Fatalf("bank balance = %d (err %v), want 1200", balance, err)
		}
		if balance, err := d.GetAccountBalance(fx.orgID, registerID, fx.date); err != nil || balance != 0 {
			t.Fatalf("register balance = %d (err %v), want 0", balance, err)
		}
	})
}

// TestCashSaleExplicitBankAccountIDMustMatchTheDerivedAccount: the field is
// kept for compatibility, but it can't redirect cash away from the register.
func TestCashSaleExplicitBankAccountIDMustMatchTheDerivedAccount(t *testing.T) {
	t.Parallel()
	d := newTestDB(t)
	fx := newGLPostingTestFixture(t, d, "org-cbpm-explicit-bank")
	register := accountByCode(t, d, fx.orgID, "1010")
	org, err := d.GetOrganization(fx.orgID)
	if err != nil {
		t.Fatalf("GetOrganization: %v", err)
	}
	if org.DefaultCashAccountID == nil {
		t.Fatal("fixture: no default cash (bank) account")
	}
	bankID := *org.DefaultCashAccountID
	if _, err := d.UpdateOrganization(fx.orgID, UpdateOrganizationRequest{
		DefaultCashRegisterAccountID: &register.ID,
	}); err != nil {
		t.Fatalf("UpdateOrganization: %v", err)
	}

	req := cashSaleRequestFor(fx, "cash", 1200)
	req.BankAccountID = bankID
	_, err = d.CreateCashSale(req)
	assertValidationErrorContains(t, err, "bankAccountId can't change that")

	// The derived account itself is accepted.
	req.BankAccountID = register.ID
	result, err := d.CreateCashSale(req)
	if err != nil {
		t.Fatalf("CreateCashSale(equal bankAccountId): %v", err)
	}
	if result.Payment == nil || result.Payment.BankAccountID != register.ID {
		t.Fatalf("payment = %+v, want the register %q", result.Payment, register.ID)
	}
}

// TestCashBookBankAndRegisterMustDiffer covers both sides of the rule: a
// stored equal pair refuses a sale, UpdateOrganization refuses creating it,
// and cash_only still allows both defaults to point at the same account.
func TestCashBookBankAndRegisterMustDiffer(t *testing.T) {
	t.Parallel()

	t.Run("sale refused", func(t *testing.T) {
		t.Parallel()
		d := newTestDB(t)
		fx := newGLPostingTestFixture(t, d, "org-cbpm-same-sale")
		register := accountByCode(t, d, fx.orgID, "1010")
		mode := CashBookPaymentMethodsCashAndBank
		// Bypass UpdateOrganization's guard to simulate a legacy/bad row.
		if _, err := d.DB.Exec(
			`UPDATE organizations SET cashBookPaymentMethods = ?, defaultCashRegisterAccountId = ?, defaultCashAccountId = ? WHERE id = ?`,
			mode, register.ID, register.ID, fx.orgID,
		); err != nil {
			t.Fatalf("direct update: %v", err)
		}
		_, err := d.CreateCashSale(cashSaleRequestFor(fx, "card", 1200))
		assertValidationErrorContains(t, err, "must be different accounts")
	})

	t.Run("update refused", func(t *testing.T) {
		t.Parallel()
		d := newTestDB(t)
		fx := newGLPostingTestFixture(t, d, "org-cbpm-same-update")
		register := accountByCode(t, d, fx.orgID, "1010")
		mode := CashBookPaymentMethodsCashAndBank
		if _, err := d.UpdateOrganization(fx.orgID, UpdateOrganizationRequest{
			DefaultCashRegisterAccountID: &register.ID,
			CashBookPaymentMethods:       &mode,
		}); err != nil {
			t.Fatalf("UpdateOrganization(cash_and_bank): %v", err)
		}
		_, err := d.UpdateOrganization(fx.orgID, UpdateOrganizationRequest{
			DefaultCashAccountID: &register.ID,
		})
		assertValidationErrorContains(t, err, "must be different accounts")
	})

	t.Run("cash_only allows equal accounts", func(t *testing.T) {
		t.Parallel()
		d := newTestDB(t)
		fx := newGLPostingTestFixture(t, d, "org-cbpm-same-cash-only")
		register := accountByCode(t, d, fx.orgID, "1010")
		mode := CashBookPaymentMethodsCashOnly
		if _, err := d.UpdateOrganization(fx.orgID, UpdateOrganizationRequest{
			DefaultCashRegisterAccountID: &register.ID,
			DefaultCashAccountID:         &register.ID,
			CashBookPaymentMethods:       &mode,
		}); err != nil {
			t.Fatalf("cash_only with equal accounts should be allowed: %v", err)
		}
	})
}
