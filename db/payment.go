package db

import (
	"database/sql"
	"errors"
	"fmt"

	gonanoid "github.com/matoous/go-nanoid/v2"
)

// Payment mirrors the payments table. Amount, and every PaymentApplication's
// Amount, are always in Currency — CreatePayment requires every applied
// document to already be in that same currency (no per-application FX), so
// there is exactly one currency in play per payment. journal_lines.debit/
// credit, by contrast, are always functional-currency cents; converting
// between the two only ever happens inside CreatePayment via the payment's
// own ExchangeRate (for the bank/cash line) or each document's frozen
// ExchangeRate (for its settlement line) — never mix the two up.
type Payment struct {
	ID               string  `db:"id"               json:"id"`
	OrganizationID   string  `db:"organizationId"   json:"organizationId"`
	Direction        string  `db:"direction"        json:"direction"`
	ClientID         *string `db:"clientId"         json:"clientId"`
	VendorID         *string `db:"vendorId"         json:"vendorId"`
	BankAccountID    string  `db:"bankAccountId"    json:"bankAccountId"`
	Amount           int64   `db:"amount"           json:"amount"`
	Currency         string  `db:"currency"         json:"currency"`
	ExchangeRate     *string `db:"exchangeRate"     json:"exchangeRate"`
	ExchangeRateDate *int64  `db:"exchangeRateDate" json:"exchangeRateDate"`
	Date             int64   `db:"date"             json:"date"`
	Method           string  `db:"method"           json:"method"`
	Reference        *string `db:"reference"        json:"reference"`
	Notes            *string `db:"notes"            json:"notes"`
	Status           string  `db:"status"           json:"status"`
	JournalEntryID   *string `db:"journalEntryId"   json:"journalEntryId"`
	VoidingEntryID   *string `db:"voidingEntryId"   json:"voidingEntryId"`
	CreatedAt        int64   `db:"createdAt"        json:"createdAt"`
}

// PaymentApplication mirrors the payment_applications table.
type PaymentApplication struct {
	ID           string `db:"id"           json:"id"`
	PaymentID    string `db:"paymentId"    json:"paymentId"`
	DocumentType string `db:"documentType" json:"documentType"`
	DocumentID   string `db:"documentId"   json:"documentId"`
	Amount       int64  `db:"amount"       json:"amount"`
	CreatedAt    int64  `db:"createdAt"    json:"createdAt"`
}

// CreatePaymentApplicationRequest is one line of a CreatePaymentRequest.
type CreatePaymentApplicationRequest struct {
	DocumentType string `json:"documentType"`
	DocumentID   string `json:"documentId"`
	Amount       int64  `json:"amount"`
}

// CreatePaymentRequest is the payload for CreatePayment.
type CreatePaymentRequest struct {
	ID               string                            `json:"id"`
	OrganizationID   string                            `json:"organizationId"`
	Direction        string                            `json:"direction"`
	ClientID         *string                           `json:"clientId"`
	VendorID         *string                           `json:"vendorId"`
	BankAccountID    string                            `json:"bankAccountId"`
	Amount           int64                             `json:"amount"`
	Currency         string                            `json:"currency"`
	ExchangeRate     *float64                          `json:"exchangeRate"`
	ExchangeRateDate *int64                            `json:"exchangeRateDate"`
	Date             int64                             `json:"date"`
	Method           string                            `json:"method"`
	Reference        *string                           `json:"reference"`
	Notes            *string                           `json:"notes"`
	Applications     []CreatePaymentApplicationRequest `json:"applications"`
}

var paymentMethods = map[string]bool{
	"bank_transfer": true, "cash": true, "card": true,
	"direct_debit": true, "check": true, "other": true,
}

// GetInvoiceAmountPaid sums everything applied to an invoice by non-voided
// payments — computed on read, like every other running balance in this
// codebase (stockQuantity, unitCost, 3-way match). Voided payments must be
// excluded or voiding one would permanently eat the invoice's balance.
func (d *Database) GetInvoiceAmountPaid(invoiceID string) (int64, error) {
	return d.getDocumentAmountPaid("invoice", invoiceID)
}

// GetIncomingInvoiceAmountPaid is GetInvoiceAmountPaid's purchases counterpart.
func (d *Database) GetIncomingInvoiceAmountPaid(billID string) (int64, error) {
	return d.getDocumentAmountPaid("incoming_invoice", billID)
}

func (d *Database) getDocumentAmountPaid(documentType, documentID string) (int64, error) {
	var amount int64
	err := d.DB.Get(&amount, `
		SELECT COALESCE(SUM(pa.amount), 0)
		FROM payment_applications pa
		JOIN payments p ON p.id = pa.paymentId
		WHERE pa.documentType = ? AND pa.documentId = ? AND p.status != 'voided'`,
		documentType, documentID,
	)
	if err != nil {
		return 0, fmt.Errorf("get_document_amount_paid: %w", err)
	}
	return amount, nil
}

// GetPayment returns a single payment by id.
func (d *Database) GetPayment(id string) (*Payment, error) {
	var payment Payment
	err := d.DB.Get(&payment, `SELECT * FROM payments WHERE id = ? LIMIT 1`, id)
	if err != nil {
		return nil, fmt.Errorf("get_payment: %w", err)
	}
	return &payment, nil
}

// GetPayments lists every payment for an organization, newest first.
func (d *Database) GetPayments(organizationID string) ([]Payment, error) {
	payments := []Payment{}
	err := d.DB.Select(&payments,
		`SELECT * FROM payments WHERE organizationId = ? ORDER BY date DESC, createdAt DESC`,
		organizationID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_payments: %w", err)
	}
	return payments, nil
}

// GetPaymentApplications lists the applications belonging to one payment.
func (d *Database) GetPaymentApplications(paymentID string) ([]PaymentApplication, error) {
	applications := []PaymentApplication{}
	err := d.DB.Select(&applications,
		`SELECT * FROM payment_applications WHERE paymentId = ? ORDER BY createdAt ASC`,
		paymentID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_payment_applications: %w", err)
	}
	return applications, nil
}

// GetDocumentPaymentApplications lists every application (across all
// payments) recorded against one invoice/incoming invoice, newest first —
// used by the invoice/bill detail page's payment history.
func (d *Database) GetDocumentPaymentApplications(documentType, documentID string) ([]PaymentApplication, error) {
	applications := []PaymentApplication{}
	err := d.DB.Select(&applications, `
		SELECT pa.* FROM payment_applications pa
		JOIN payments p ON p.id = pa.paymentId
		WHERE pa.documentType = ? AND pa.documentId = ?
		ORDER BY pa.createdAt DESC`,
		documentType, documentID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_document_payment_applications: %w", err)
	}
	return applications, nil
}

// settlementLine is one application's contribution to the payment's journal
// entry, computed from a read against the applied document before any
// transaction opens (db.SetMaxOpenConns(1) means a read against d.DB while a
// *sqlx.Tx is open would block forever, not error).
type settlementLine struct {
	accountID  string
	functional int64 // AR/AP clearing amount, in the organization's functional currency
	docCents   int64 // the application's amount, in the document's own currency (== payment currency)
	docRate    *string
	docForeign bool
	clientID   *string
	vendorID   *string
}

// CreatePayment records a payment and settles it against one or more
// invoices/incoming invoices in a single balanced journal entry: one bank/
// cash line, one AR/AP clearing line per application (converted at the
// *document's own* frozen exchange rate, not the payment's), and — when the
// payment's rate differs from a settled document's rate — a realized FX
// gain/loss plug so the entry still balances by construction, the same
// "absorb the residual" idiom buildInvoiceGLLines uses for rounding.
//
// Lettrage (tagging the original AR/AP line and this settlement line with a
// shared reconciliation_groups code) is deliberately not implemented yet —
// nothing before the FEC exporter (Phase 5) consumes it, and it requires
// mutating a previously posted journal_lines row, so it's deferred until the
// exporter's exact needs are known rather than guessed at now.
func (d *Database) CreatePayment(req CreatePaymentRequest) (*Payment, error) {
	if req.ID == "" {
		req.ID, _ = gonanoid.New()
	}
	if req.Direction != "inbound" && req.Direction != "outbound" {
		return nil, newValidationError("invalid payment direction %q", req.Direction)
	}
	if !paymentMethods[req.Method] {
		return nil, newValidationError("invalid payment method %q", req.Method)
	}
	if req.BankAccountID == "" {
		return nil, newValidationError("a payment requires a bank/cash account")
	}
	if req.Direction == "inbound" {
		if req.ClientID == nil || *req.ClientID == "" {
			return nil, newValidationError("an inbound payment requires a client")
		}
		if req.VendorID != nil && *req.VendorID != "" {
			return nil, newValidationError("an inbound payment cannot have a vendor")
		}
	} else {
		if req.VendorID == nil || *req.VendorID == "" {
			return nil, newValidationError("an outbound payment requires a vendor")
		}
		if req.ClientID != nil && *req.ClientID != "" {
			return nil, newValidationError("an outbound payment cannot have a client")
		}
	}
	if len(req.Applications) == 0 {
		return nil, newValidationError("a payment needs at least one application")
	}
	if req.Amount <= 0 {
		return nil, newValidationError("payment amount must be positive")
	}
	var appSum int64
	for _, app := range req.Applications {
		appSum += app.Amount
	}
	if appSum != req.Amount {
		return nil, newValidationError("payment amount %d does not match the sum of its applications %d", req.Amount, appSum)
	}

	org, err := d.GetOrganization(req.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("create_payment organization: %w", err)
	}
	paymentExchangeRate, err := resolveExchangeRateForSave(
		orgCurrencyOrDefault(org), "", nil, &req.Currency, req.ExchangeRate,
	)
	if err != nil {
		return nil, err
	}
	paymentRate, err := parseExchangeRate(paymentExchangeRate)
	if err != nil {
		return nil, err
	}
	paymentForeign := paymentExchangeRate != nil
	convertPayment := func(cents int64) int64 {
		if !paymentForeign {
			return cents
		}
		return convertCents(cents, paymentRate)
	}

	var settlements []settlementLine

	for i, app := range req.Applications {
		if app.Amount <= 0 {
			return nil, newValidationError("application %d: amount must be positive", i+1)
		}
		switch app.DocumentType {
		case "invoice":
			if req.Direction != "inbound" {
				return nil, newValidationError("application %d: an invoice can only be settled by an inbound payment", i+1)
			}
			invoice, err := d.GetInvoice(app.DocumentID)
			if err != nil {
				return nil, newValidationError("application %d: invoice not found", i+1)
			}
			if invoice.OrganizationID != req.OrganizationID {
				return nil, newValidationError("application %d: invoice belongs to a different organization", i+1)
			}
			if invoice.Currency != req.Currency {
				return nil, newValidationError(
					"application %d: invoice currency %q does not match payment currency %q", i+1, invoice.Currency, req.Currency,
				)
			}
			if postedEntry, err := d.FindPostedEntryForSourceDocument("invoice", app.DocumentID); err != nil {
				return nil, err
			} else if postedEntry == nil {
				return nil, newValidationError("application %d: invoice has no posted GL entry — send it first", i+1)
			}
			paid, err := d.GetInvoiceAmountPaid(app.DocumentID)
			if err != nil {
				return nil, err
			}
			if remaining := invoice.Total - paid; app.Amount > remaining {
				return nil, newValidationError("application %d: amount %d exceeds remaining balance %d", i+1, app.Amount, remaining)
			}
			if org.DefaultArAccountID == nil {
				return nil, newValidationError("cannot post payment: organization has no default AR account configured")
			}
			docRate, err := parseExchangeRate(invoice.ExchangeRate)
			if err != nil {
				return nil, err
			}
			docForeign := invoice.ExchangeRate != nil
			functional := app.Amount
			if docForeign {
				functional = convertCents(app.Amount, docRate)
			}
			settlements = append(settlements, settlementLine{
				accountID: *org.DefaultArAccountID, functional: functional,
				docCents: app.Amount, docRate: invoice.ExchangeRate, docForeign: docForeign,
				clientID: &invoice.ClientID,
			})
		case "incoming_invoice":
			if req.Direction != "outbound" {
				return nil, newValidationError("application %d: a bill can only be settled by an outbound payment", i+1)
			}
			bill, err := d.GetIncomingInvoice(app.DocumentID)
			if err != nil {
				return nil, newValidationError("application %d: incoming invoice not found", i+1)
			}
			if bill.OrganizationID != req.OrganizationID {
				return nil, newValidationError("application %d: incoming invoice belongs to a different organization", i+1)
			}
			if bill.Currency != req.Currency {
				return nil, newValidationError(
					"application %d: bill currency %q does not match payment currency %q", i+1, bill.Currency, req.Currency,
				)
			}
			if postedEntry, err := d.FindPostedEntryForSourceDocument("incoming_invoice", app.DocumentID); err != nil {
				return nil, err
			} else if postedEntry == nil {
				return nil, newValidationError("application %d: bill has no posted GL entry — approve it first", i+1)
			}
			paid, err := d.GetIncomingInvoiceAmountPaid(app.DocumentID)
			if err != nil {
				return nil, err
			}
			if remaining := bill.Total - paid; app.Amount > remaining {
				return nil, newValidationError("application %d: amount %d exceeds remaining balance %d", i+1, app.Amount, remaining)
			}
			if org.DefaultApAccountID == nil {
				return nil, newValidationError("cannot post payment: organization has no default AP account configured")
			}
			docRate, err := parseExchangeRate(bill.ExchangeRate)
			if err != nil {
				return nil, err
			}
			docForeign := bill.ExchangeRate != nil
			functional := app.Amount
			if docForeign {
				functional = convertCents(app.Amount, docRate)
			}
			settlements = append(settlements, settlementLine{
				accountID: *org.DefaultApAccountID, functional: functional,
				docCents: app.Amount, docRate: bill.ExchangeRate, docForeign: docForeign,
				vendorID: &bill.VendorID,
			})
		default:
			return nil, newValidationError("application %d: invalid document type %q", i+1, app.DocumentType)
		}
	}

	journalType := "bank"
	if req.Method == "cash" {
		journalType = "cash"
	}
	journal, err := getJournalByTypeTx(d.DB, req.OrganizationID, journalType)
	if err != nil {
		return nil, err
	}

	bankFunctional := convertPayment(req.Amount)

	var lines []CreateJournalLineRequest
	var debitSum, creditSum int64
	if req.Direction == "inbound" {
		lines = append(lines, glLine(req.BankAccountID, bankFunctional, 0, req.Currency, req.Amount, paymentExchangeRate, paymentForeign, nil, nil, nil))
		debitSum += bankFunctional
		for _, s := range settlements {
			lines = append(lines, glLine(s.accountID, 0, s.functional, req.Currency, s.docCents, s.docRate, s.docForeign, s.clientID, s.vendorID, nil))
			creditSum += s.functional
		}
	} else {
		lines = append(lines, glLine(req.BankAccountID, 0, bankFunctional, req.Currency, req.Amount, paymentExchangeRate, paymentForeign, nil, nil, nil))
		creditSum += bankFunctional
		for _, s := range settlements {
			lines = append(lines, glLine(s.accountID, s.functional, 0, req.Currency, s.docCents, s.docRate, s.docForeign, s.clientID, s.vendorID, nil))
			debitSum += s.functional
		}
	}

	// Realized FX gain/loss plug: the settlement lines value each application
	// at its document's frozen rate, while the bank line values the same cash
	// at the payment's own rate — any difference is a real realized gain or
	// loss, derived (not independently recomputed) from the lines already
	// built so the entry balances by construction, the same "absorb the
	// residual" move buildInvoiceGLLines uses for rounding.
	if diff := debitSum - creditSum; diff != 0 {
		var clientID, vendorID *string
		if req.Direction == "inbound" {
			clientID = req.ClientID
		} else {
			vendorID = req.VendorID
		}
		if diff > 0 {
			if org.FxGainAccountID == nil {
				return nil, newValidationError("cannot post payment: organization has no FX gain account configured")
			}
			lines = append(lines, glLine(*org.FxGainAccountID, 0, diff, req.Currency, 0, nil, false, clientID, vendorID, nil))
		} else {
			if org.FxLossAccountID == nil {
				return nil, newValidationError("cannot post payment: organization has no FX loss account configured")
			}
			lines = append(lines, glLine(*org.FxLossAccountID, -diff, 0, req.Currency, 0, nil, false, clientID, vendorID, nil))
		}
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("create_payment begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	_, err = tx.Exec(`
		INSERT INTO payments (
			id, organizationId, direction, clientId, vendorId, bankAccountId,
			amount, currency, exchangeRate, exchangeRateDate, date, method, reference, notes
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.ID, req.OrganizationID, req.Direction, req.ClientID, req.VendorID, req.BankAccountID,
		req.Amount, req.Currency, paymentExchangeRate, req.ExchangeRateDate, req.Date, req.Method, req.Reference, req.Notes,
	)
	if err != nil {
		return nil, fmt.Errorf("create_payment insert: %w", err)
	}

	for i, app := range req.Applications {
		appID, err := gonanoid.New()
		if err != nil {
			return nil, fmt.Errorf("create_payment application new_id: %w", err)
		}
		if _, err := tx.Exec(
			`INSERT INTO payment_applications (id, paymentId, documentType, documentId, amount) VALUES (?, ?, ?, ?, ?)`,
			appID, req.ID, app.DocumentType, app.DocumentID, app.Amount,
		); err != nil {
			return nil, fmt.Errorf("create_payment application %d insert: %w", i+1, err)
		}
	}

	description := fmt.Sprintf("Payment %s", req.ID)
	entryID, err := postAutoEntryTx(tx, req.OrganizationID, journal.ID, "payment", req.ID, req.Date, derefString(req.Reference), description, lines)
	if err != nil {
		return nil, err
	}

	if _, err := tx.Exec(`UPDATE payments SET journalEntryId = ? WHERE id = ?`, entryID, req.ID); err != nil {
		return nil, fmt.Errorf("create_payment link_entry: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("create_payment commit: %w", err)
	}

	return d.GetPayment(req.ID)
}

// VoidPayment reverses a posted payment's journal entry (via the same
// reverseEntryTx every other reversal path uses) and marks the payment
// itself voided so GetInvoiceAmountPaid/GetIncomingInvoiceAmountPaid stop
// counting it. Payments are never deleted, only voided — same immutability
// rule as journal entries themselves.
func (d *Database) VoidPayment(paymentID string, reversalDate int64) (*Payment, error) {
	payment, err := d.GetPayment(paymentID)
	if err != nil {
		return nil, err
	}
	if payment.Status != "posted" {
		return nil, newValidationError("only a posted payment can be voided")
	}
	if payment.JournalEntryID == nil {
		return nil, fmt.Errorf("void_payment: posted payment %s has no journal entry", paymentID)
	}

	var entry JournalEntry
	if err := d.DB.Get(&entry, `SELECT * FROM journal_entries WHERE id = ?`, *payment.JournalEntryID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("void_payment: journal entry %s not found", *payment.JournalEntryID)
		}
		return nil, fmt.Errorf("void_payment: %w", err)
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("void_payment begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	reversalID, err := reverseEntryTx(tx, &entry, "payment voided", reversalDate)
	if err != nil {
		return nil, err
	}

	if _, err := tx.Exec(`UPDATE payments SET status = 'voided', voidingEntryId = ? WHERE id = ?`, reversalID, paymentID); err != nil {
		return nil, fmt.Errorf("void_payment update: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("void_payment commit: %w", err)
	}

	return d.GetPayment(paymentID)
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
