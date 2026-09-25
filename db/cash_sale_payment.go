package db

import (
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	gonanoid "github.com/matoous/go-nanoid/v2"
)

// CreateCashSalePaymentRequest settles part or all of one line of a loan
// sale from the Cash Book. InvoiceID comes from the route, not the body.
type CreateCashSalePaymentRequest struct {
	InvoiceID         string  `json:"-"`
	InvoiceLineItemID string  `json:"invoiceLineItemId"`
	Amount            int64   `json:"amount"`
	Date              int64   `json:"date"`
	Reference         *string `json:"reference"`
	Notes             *string `json:"notes"`
}

// CashSalePaymentResult is what CreateCashSalePayment returns: the recorded
// payment and the invoice as it stands afterwards (its state flips to "paid"
// when this payment cleared the balance).
type CashSalePaymentResult struct {
	Payment *Payment `json:"payment"`
	Invoice *Invoice `json:"invoice"`
}

// invoiceLineBalances reads one invoice's lines with their allocated amount
// and paid figures — the same allocation GetLoanStatus shows, so what the
// cashier sees as a line's outstanding balance is exactly what this checks
// against. q is either d.DB or an open *sqlx.Tx (the in-transaction
// re-check below).
func invoiceLineBalances(q sqlx.Queryer, invoiceID string) (lines []loanLineRaw, amounts, paid []int64, err error) {
	if err := sqlx.Select(q, &lines, loanLinesQuery("i.id = ?")+" ORDER BY "+loanLineOrder, invoiceID); err != nil {
		return nil, nil, nil, fmt.Errorf("invoice_line_balances: %w", err)
	}
	amounts, paid = allocateInvoiceLines(lines)
	return lines, amounts, paid, nil
}

// lineOutstanding returns the outstanding balance of lineID among lines, and
// whether the line belongs to the invoice at all.
func lineOutstanding(lines []loanLineRaw, amounts, paid []int64, lineID string) (int64, bool) {
	for k, l := range lines {
		if l.LineID == lineID {
			return amounts[k] - paid[k], true
		}
	}
	return 0, false
}

// CreateCashSalePayment is the Cash Book's loan-settlement write: a cash
// payment into the organization's register that settles one line of an
// invoice — any sent or paid invoice of the organization, not only one
// CreateCashSale made (audit F150: the counter may collect any open
// receivable, by decision). It exists alongside CreatePayment rather than going through it
// for three reasons:
//
//   - It targets a line (payment_applications.invoiceLineItemId, migration
//     0090), with the amount capped at that line's outstanding balance — the
//     cashier collects the balance of one item of a loan sale, and the other
//     lines' balances must not move.
//   - It is the cashbook role's write path. POST /api/payments stays an
//     accounting-tier action (audit F104); this one can only ever receive
//     cash into the register against a customer invoice, never pay a vendor
//     or void anything.
//   - It moves the invoice to "paid" in the same transaction once its whole
//     balance clears, instead of a follow-up PATCH …/state from the browser
//     (which the cashbook role can't make, and which could fail after the
//     payment had already been recorded).
//
// Same shape as CreateCashSale: every read before Beginx() (db.SetMaxOpenConns(1)
// — a d.DB read while a *sqlx.Tx is open deadlocks), then one transaction
// that re-checks the line's balance against a fresh read (a concurrent
// payment on the same line would otherwise let both through) and writes the
// payment, its application, its GL entry and the state change.
func (d *Database) CreateCashSalePayment(req CreateCashSalePaymentRequest) (*CashSalePaymentResult, error) {
	if req.InvoiceLineItemID == "" {
		return nil, newValidationError("invoiceLineItemId is required")
	}
	if req.Amount <= 0 {
		return nil, newValidationError("payment amount must be positive")
	}
	if req.Date == 0 {
		req.Date = time.Now().UnixMilli()
	}

	invoice, err := d.GetInvoice(req.InvoiceID)
	if err != nil {
		return nil, err
	}
	if invoice.State != "sent" && invoice.State != "paid" {
		return nil, newValidationError("only a sent invoice can receive a payment")
	}
	// Cash Book payments are always in the organization's own currency (the
	// same rule CreateCashSale enforces), so a foreign-currency invoice —
	// which would need a payment rate and an FX plug — is refused here and
	// settled through the invoice page instead.
	if invoice.ExchangeRate != nil {
		return nil, newValidationError("a foreign-currency invoice can't be settled from the Cash Book — record the payment from the invoice page")
	}
	if postedEntry, err := d.FindPostedEntryForSourceDocument("invoice", invoice.ID); err != nil {
		return nil, err
	} else if postedEntry == nil {
		return nil, newValidationError("invoice has no posted GL entry — send it first")
	}

	org, err := d.GetOrganization(invoice.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("create_cash_sale_payment organization: %w", err)
	}
	// F101: always the register, never defaultCashAccountId (wired to Bank).
	if org.DefaultCashRegisterAccountID == nil {
		return nil, newValidationError(
			"cannot record payment: organization has no default cash register account configured — set defaultCashRegisterAccountId in the organization's Accounting settings",
		)
	}
	registerAccountID := *org.DefaultCashRegisterAccountID
	if org.DefaultArAccountID == nil {
		return nil, newValidationError("cannot post payment: organization has no default AR account configured")
	}

	lines, amounts, paid, err := invoiceLineBalances(d.DB, invoice.ID)
	if err != nil {
		return nil, err
	}
	outstanding, ok := lineOutstanding(lines, amounts, paid, req.InvoiceLineItemID)
	if !ok {
		return nil, newValidationError("line item not found on this invoice")
	}
	if req.Amount > outstanding {
		return nil, newValidationError("amount %d exceeds the line's outstanding balance %d", req.Amount, outstanding)
	}

	journal, err := getJournalByTypeTx(d.DB, invoice.OrganizationID, "cash")
	if err != nil {
		return nil, err
	}
	paymentID, err := gonanoid.New()
	if err != nil {
		return nil, fmt.Errorf("create_cash_sale_payment id: %w", err)
	}
	appID, err := gonanoid.New()
	if err != nil {
		return nil, fmt.Errorf("create_cash_sale_payment application id: %w", err)
	}
	glLines := []CreateJournalLineRequest{
		glLine(registerAccountID, req.Amount, 0, invoice.Currency, req.Amount, nil, false, nil, nil, nil),
		glLine(*org.DefaultArAccountID, 0, req.Amount, invoice.Currency, req.Amount, nil, false, &invoice.ClientID, nil, nil),
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("create_cash_sale_payment begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	lines, amounts, paid, err = invoiceLineBalances(tx, invoice.ID)
	if err != nil {
		return nil, err
	}
	outstanding, _ = lineOutstanding(lines, amounts, paid, req.InvoiceLineItemID)
	if req.Amount > outstanding {
		return nil, newValidationError(
			"amount %d exceeds the line's outstanding balance %d — another payment was recorded concurrently, reload and try again",
			req.Amount, outstanding,
		)
	}
	var invoicePaid int64
	if len(lines) > 0 {
		invoicePaid = lines[0].InvoicePaid
	}

	if _, err := tx.Exec(`
		INSERT INTO payments (
			id, organizationId, direction, clientId, bankAccountId,
			amount, currency, date, method, reference, notes
		) VALUES (?, ?, 'inbound', ?, ?, ?, ?, ?, 'cash', ?, ?)`,
		paymentID, invoice.OrganizationID, invoice.ClientID, registerAccountID,
		req.Amount, invoice.Currency, req.Date, req.Reference, req.Notes,
	); err != nil {
		return nil, fmt.Errorf("create_cash_sale_payment insert_payment: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO payment_applications (id, paymentId, documentType, documentId, invoiceLineItemId, amount)
		 VALUES (?, ?, 'invoice', ?, ?, ?)`,
		appID, paymentID, invoice.ID, req.InvoiceLineItemID, req.Amount,
	); err != nil {
		return nil, fmt.Errorf("create_cash_sale_payment insert_application: %w", err)
	}
	entryID, err := postAutoEntryTx(
		tx, invoice.OrganizationID, journal.ID, "payment", paymentID, req.Date,
		derefString(req.Reference), fmt.Sprintf("Payment %s", paymentID), glLines,
	)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`UPDATE payments SET journalEntryId = ? WHERE id = ?`, entryID, paymentID); err != nil {
		return nil, fmt.Errorf("create_cash_sale_payment link_entry: %w", err)
	}
	// Clearing the balance marks the invoice paid. Both "sent" and "paid"
	// need the same GL presence (needsInvoiceGLPresence), so this is a pure
	// state flag change with no posting to bring in line.
	if invoicePaid+req.Amount >= invoice.Total {
		if _, err := tx.Exec(`UPDATE invoices SET state = 'paid' WHERE id = ? AND state = 'sent'`, invoice.ID); err != nil {
			return nil, fmt.Errorf("create_cash_sale_payment mark_paid: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("create_cash_sale_payment commit: %w", err)
	}

	result := &CashSalePaymentResult{}
	if result.Payment, err = d.GetPayment(paymentID); err != nil {
		return nil, err
	}
	if result.Invoice, err = d.GetInvoice(invoice.ID); err != nil {
		return nil, err
	}
	return result, nil
}
