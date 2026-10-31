package db

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	gonanoid "github.com/matoous/go-nanoid/v2"
)

// findClientByPhone looks for an existing client of organizationID whose
// phone is the same number as phone under normalizePhone's digit-only
// comparison (so "+216 55 123 456" and "21655123456" match). It runs either
// against d.DB before the transaction or against tx afterwards, where the
// duplicate check is re-run so two simultaneous cash sales can't both
// create the client.
func findClientByPhone(exec sqlx.Queryer, organizationID, phone string) (*Client, error) {
	wanted := normalizePhone(phone)
	if wanted == "" {
		return nil, nil
	}
	var clients []struct {
		ID    string  `db:"id"`
		Name  *string `db:"name"`
		Phone *string `db:"phone"`
	}
	if err := sqlx.Select(exec, &clients,
		`SELECT id, name, phone FROM clients WHERE organizationId = ? AND phone IS NOT NULL AND phone != ''`,
		organizationID,
	); err != nil {
		return nil, err
	}
	for _, c := range clients {
		if c.Phone != nil && normalizePhone(*c.Phone) == wanted {
			return &Client{ID: c.ID, Name: c.Name}, nil
		}
	}
	return nil, nil
}

// CreateCashSaleRequest is the payload for the Cash Book screen's single
// atomic action: create-or-reuse a client, create an invoice, and (unless
// this is a zero-upfront-payment loan sale) record a payment against it —
// all in one transaction. See CreateCashSale's doc comment for why this
// can't safely be three separate API calls.
type CreateCashSaleRequest struct {
	OrganizationID string `json:"organizationId"`

	// Exactly one of ClientID (existing client) or NewClient (inline
	// creation) must be set.
	ClientID  string               `json:"clientId"`
	NewClient *CreateClientRequest `json:"newClient"`

	Date      int64                          `json:"date"`
	Currency  string                         `json:"currency"`
	LineItems []CreateInvoiceLineItemRequest `json:"lineItems"`

	SubTotal int64 `json:"subTotal"`
	TaxTotal int64 `json:"taxTotal"`
	Total    int64 `json:"total"`

	// AmountReceived is 0 for a pure loan sale (no upfront payment). It may
	// be less than Total (a deposit) or equal to it (a cash sale).
	AmountReceived int64  `json:"amountReceived"`
	PaymentMethod  string `json:"paymentMethod"` // defaults "cash"
	// BankAccountID is the register/till account the received amount is
	// debited to for a cash payment. Empty defaults to
	// organizations.defaultCashRegisterAccountId and 409s if that is unset —
	// deliberately never organizations.defaultCashAccountId, which every
	// chart template wires to Bank (see CreateCashSale's doc comment below).
	// For a card/transfer payment in cash_and_bank mode it is ignored: that
	// money goes to defaultCashAccountId (the bank account), never the till.
	BankAccountID string  `json:"bankAccountId"`
	Reference     *string `json:"reference"`
	Notes         *string `json:"notes"`
}

// CashSaleResult is what CreateCashSale returns: the resolved client (new or
// existing), the created invoice, and the payment if one was recorded.
type CashSaleResult struct {
	Client  *Client  `json:"client"`
	Invoice *Invoice `json:"invoice"`
	Payment *Payment `json:"payment"`
}

// generateDocumentNumber ports src/utils/invoice.ts's generateInvoiceNumber
// token-replace logic to Go, close but not byte-exact: this uses
// strings.NewReplacer, which substitutes every occurrence of each token,
// while the frontend's sequential .replace(string, string) calls only
// replace the first occurrence of each — the two diverge only for a format
// that repeats a token (e.g. "{year}-{number}-{year}"), which no format in
// this app's Settings UI does today. CreateCashSale derives the invoice
// number itself, inside its own transaction, instead of trusting a
// frontend-computed one the way every other document-creation path does —
// see CreateCashSale's doc comment for why that convention doesn't hold up
// on this particular screen.
func generateDocumentNumber(format string, counter int64, date time.Time, clientCode string) string {
	if format == "" {
		return ""
	}
	r := strings.NewReplacer(
		"{number}", strconv.FormatInt(counter, 10),
		"{year}", strconv.Itoa(date.Year()),
		"{y}", fmt.Sprintf("%02d", date.Year()%100),
		"{month}", fmt.Sprintf("%02d", int(date.Month())),
		"{m}", date.Format("Jan"),
		"{day}", fmt.Sprintf("%02d", date.Day()),
		"{clientCode}", clientCode,
	)
	return r.Replace(format)
}

// CreateCashSale is the Cash Book screen's single write path. Chaining the
// equivalent CreateClient/CreateInvoice/UpdateInvoiceState("sent")/
// CreatePayment calls from the frontend is unsafe: CreatePayment requires
// the invoice already have a posted (and, once posted, immutable — only
// reversible) GL entry, so a failure between posting the AR entry and
// posting the payment settlement would leave an orphaned, unbalanced AR
// entry nothing in the UI could clean up. This function does every read
// before opening a transaction (required by db.SetMaxOpenConns(1): a d.DB
// read while a *sqlx.Tx is open deadlocks, it doesn't error — see
// UpdateInvoiceState/CreatePayment for the same shape) and then does every
// write — client, invoice, line items, the sale's GL entry, and (if any
// upfront amount was received) the payment and its GL entry — inside one
// transaction.
func (d *Database) CreateCashSale(req CreateCashSaleRequest) (*CashSaleResult, error) {
	if req.OrganizationID == "" {
		return nil, newValidationError("organizationId is required")
	}
	if len(req.LineItems) == 0 {
		return nil, newValidationError("a sale needs at least one line item")
	}
	if req.AmountReceived < 0 {
		return nil, newValidationError("amount received cannot be negative")
	}
	if req.AmountReceived > req.Total {
		return nil, newValidationError("amount received %d exceeds the sale total %d", req.AmountReceived, req.Total)
	}
	if req.PaymentMethod == "" {
		req.PaymentMethod = "cash"
	}
	if req.AmountReceived > 0 && !paymentMethods[req.PaymentMethod] {
		return nil, newValidationError("invalid payment method %q", req.PaymentMethod)
	}
	if req.ClientID == "" && req.NewClient == nil {
		return nil, newValidationError("a cash sale needs either an existing clientId or a newClient")
	}

	org, err := d.GetOrganization(req.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("create_cash_sale organization: %w", err)
	}

	// The organization setting decides whether the counter accepts anything
	// but cash (migration 0111, default cash_only). A card/transfer payment is
	// only meaningful when an amount is actually received; with 0 there is no
	// payment to post and the method stays irrelevant.
	cashAndBank := normalizeCashBookPaymentMethods(org.CashBookPaymentMethods) == CashBookPaymentMethodsCashAndBank
	if req.AmountReceived > 0 && req.PaymentMethod != "cash" && !cashAndBank {
		return nil, newValidationError("this organization's Cash Book accepts cash only")
	}

	bankAccountID := req.BankAccountID
	accountLabel := "cash register"
	if req.AmountReceived > 0 {
		if req.PaymentMethod == "cash" {
			if bankAccountID == "" {
				// F101: a cash sale must land in the dedicated register/till
				// account, named by defaultCashRegisterAccountId. It must NOT
				// fall back to defaultCashAccountId: every chart-of-accounts
				// template wires the "cash" role to the Bank account
				// (db/account.go), so that fallback silently credited Bank
				// while the Cash Book screen's register balance/report watched
				// the till — a real production report. A register account is a
				// configuration precondition of this screen, surfaced as a 409
				// naming the exact column rather than quietly posting to Bank.
				if org.DefaultCashRegisterAccountID == nil {
					return nil, newValidationError(
						"cannot record payment: organization has no default cash register account configured — set defaultCashRegisterAccountId in the organization's Accounting settings",
					)
				}
				bankAccountID = *org.DefaultCashRegisterAccountID
			}
		} else {
			// cash_and_bank: card, transfer and the other non-cash methods
			// settle into the bank account, never the physical drawer. An
			// explicit req.BankAccountID is the register convention the old
			// counter always sent, so it is not honored here — only the
			// organization's configured bank account is consistent with this
			// payment.
			if org.DefaultCashAccountID == nil {
				return nil, newValidationError(
					"cannot record a card or transfer payment: organization has no default bank account configured",
				)
			}
			bankAccountID = *org.DefaultCashAccountID
			accountLabel = "bank"
		}
		bankAccount, err := d.GetAccount(bankAccountID)
		if err != nil {
			return nil, newValidationError("cash/bank account not found")
		}
		if err := requireSameOrg(req.OrganizationID, bankAccount.OrganizationID, "cash/bank account"); err != nil {
			return nil, err
		}
		// The destination is where the money physically lands: it must be an
		// asset account, never a revenue/expense/liability/equity row.
		if bankAccount.Type != "asset" {
			return nil, newValidationError("the %s account must be an asset account", accountLabel)
		}
	}

	// Resolve or validate the client. For a new client this only prepares
	// req.NewClient (id assigned, duplicate-phone checked) — the row itself
	// is inserted inside the transaction below.
	var client *Client
	if req.ClientID != "" {
		client, err = d.GetClient(req.ClientID)
		if err != nil {
			return nil, newValidationError("client not found")
		}
		if err := requireSameOrg(req.OrganizationID, client.OrganizationID, "client"); err != nil {
			return nil, err
		}
	} else {
		nc := *req.NewClient
		nc.OrganizationID = req.OrganizationID
		if nc.Name == nil || strings.TrimSpace(*nc.Name) == "" {
			return nil, newValidationError("a new client needs a name")
		}
		if nc.Phone != nil && *nc.Phone != "" {
			existing, err := findClientByPhone(d.DB, req.OrganizationID, *nc.Phone)
			if err != nil {
				return nil, fmt.Errorf("create_cash_sale duplicate_phone_check: %w", err)
			}
			if existing != nil {
				name := ""
				if existing.Name != nil {
					name = *existing.Name
				}
				return nil, newValidationError(
					"a client with this phone number already exists (%s) — search for them instead of creating a new one", name,
				)
			}
		}
		if nc.ID == "" {
			nc.ID, err = gonanoid.New()
			if err != nil {
				return nil, fmt.Errorf("create_cash_sale client id: %w", err)
			}
		}
		req.NewClient = &nc
		// An in-memory Client (no DB row yet) — enough for the GL-line
		// building and response shaping below, which only need ID/Name/Code.
		client = &Client{ID: nc.ID, OrganizationID: req.OrganizationID, Name: nc.Name, Code: nc.Code}
	}

	normalizeInvoiceLineItemIDs(req.LineItems)
	if err := d.checkInvoiceLineItemsFKOwnership(req.OrganizationID, req.LineItems); err != nil {
		return nil, err
	}
	if err := d.validateInvoiceTotals(req.LineItems, req.SubTotal, req.TaxTotal, req.Total, 0, 0); err != nil {
		return nil, err
	}
	// Resolve-only, for an early 409 before any work happens — postAutoEntryTx
	// re-resolves inside the transaction regardless (see its own doc comment).
	if _, _, err := resolveFiscalPeriodForDate(d.DB, req.OrganizationID, req.Date); err != nil {
		return nil, err
	}

	// This screen only ever sends the organization's own currency (see
	// src/routes/cash-book.tsx) and CreateCashSaleRequest has no field to
	// supply a rate for anything else, so a foreign currency is rejected
	// outright with a clear reason rather than falling through to
	// resolveExchangeRateForSave's generic "exchange rate is required" —
	// that message reads as if the caller could supply one here, and can't.
	orgCurrency := orgCurrencyOrDefault(org)
	if req.Currency != "" && req.Currency != orgCurrency {
		return nil, newValidationError(
			"cash sales must be recorded in the organization's currency (%s) — foreign-currency sales aren't supported on this screen",
			orgCurrency,
		)
	}
	req.Currency = orgCurrency

	invoiceID, err := gonanoid.New()
	if err != nil {
		return nil, fmt.Errorf("create_cash_sale invoice id: %w", err)
	}

	invoiceLineItems := make([]InvoiceLineItem, len(req.LineItems))
	for i, item := range req.LineItems {
		id, err := gonanoid.New()
		if err != nil {
			return nil, fmt.Errorf("create_cash_sale line_item id: %w", err)
		}
		invoiceLineItems[i] = InvoiceLineItem{
			ID: id, InvoiceID: invoiceID, Description: item.Description, Quantity: item.Quantity,
			UnitPrice: roundCents(item.UnitPrice), TaxRate: item.TaxRate, ProductID: item.ProductID, Position: i,
		}
	}

	state := "sent"
	if req.AmountReceived == req.Total {
		state = "paid"
	}

	// The sale takes its own stock out (db/invoice_stock.go) — resolved and,
	// in perpetual valuation, costed here, so a serialized product or a
	// missing cost basis is a 409 before anything is written.
	stockItems := make([]invoiceStockItem, len(req.LineItems))
	for i, item := range req.LineItems {
		stockItems[i] = invoiceStockItem{ProductID: item.ProductID, Quantity: item.Quantity}
	}
	stockLines, err := d.resolveInvoiceStockLines(stockItems)
	if err != nil {
		return nil, err
	}
	stockPlan, err := d.planInvoiceStockOut(req.OrganizationID, stockLines)
	if err != nil {
		return nil, err
	}

	invoice := &Invoice{
		ID: invoiceID, OrganizationID: req.OrganizationID, State: state, ClientID: client.ID,
		Date: req.Date, Currency: req.Currency, ExchangeRate: nil,
		Total: req.Total, TaxTotal: req.TaxTotal, SubTotal: req.SubTotal, FiscalStampAmount: 0,
		MovesStock: 1,
	}

	// needsInvoiceGLPresence's own rule (db/invoice.go): a zero-total
	// invoice has no economic event to record, and buildInvoiceGLLines'
	// unconditional AR line would otherwise emit a debit=0/credit=0 row that
	// violates journal_lines' CHECK constraint.
	needsGL := req.Total != 0
	var saleGLLines []CreateJournalLineRequest
	var saleJournal *Journal
	if needsGL {
		saleGLLines, saleJournal, err = d.buildInvoiceGLLines(invoice, invoiceLineItems)
		if err != nil {
			return nil, err
		}
	}

	var settlementGLLines []CreateJournalLineRequest
	var settlementJournal *Journal
	var paymentID string
	if req.AmountReceived > 0 {
		if org.DefaultArAccountID == nil {
			return nil, newValidationError("cannot post payment: organization has no default AR account configured")
		}
		paymentID, err = gonanoid.New()
		if err != nil {
			return nil, fmt.Errorf("create_cash_sale payment id: %w", err)
		}
		journalType := "bank"
		if req.PaymentMethod == "cash" {
			journalType = "cash"
		}
		settlementJournal, err = getJournalByTypeTx(d.DB, req.OrganizationID, journalType)
		if err != nil {
			return nil, err
		}

		// A cash sale is always in the organization's own currency (checked
		// above), so the invoice and this payment share that currency with no
		// exchange rate and no FX plug: the bank and AR legs are equal in
		// functional currency.
		bankFunctional := req.AmountReceived
		settlementGLLines = []CreateJournalLineRequest{
			glLine(bankAccountID, bankFunctional, 0, req.Currency, req.AmountReceived, nil, false, nil, nil, nil),
			glLine(*org.DefaultArAccountID, 0, bankFunctional, req.Currency, req.AmountReceived, nil, false, &client.ID, nil, nil),
		}
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("create_cash_sale begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if req.NewClient != nil {
		nc := req.NewClient
		// Re-run the duplicate-phone check through the transaction: the
		// pre-transaction read above could have raced another cash sale
		// for the same new phone, and both would otherwise create a client.
		if nc.Phone != nil && *nc.Phone != "" {
			existing, err := findClientByPhone(tx, req.OrganizationID, *nc.Phone)
			if err != nil {
				return nil, fmt.Errorf("create_cash_sale duplicate_phone_check: %w", err)
			}
			if existing != nil {
				name := ""
				if existing.Name != nil {
					name = *existing.Name
				}
				return nil, newValidationError(
					"a client with this phone number already exists (%s) — search for them instead of creating a new one", name,
				)
			}
		}
		if err := insertClientTx(tx, *nc); err != nil {
			return nil, fmt.Errorf("create_cash_sale insert_client: %w", err)
		}
	}

	// Invoice number derived here, inside the transaction, skipping any
	// number an invoice already has (db/invoice_number.go).
	clientCode := ""
	if client.Code != nil {
		clientCode = *client.Code
	}
	number, counter, err := nextInvoiceNumberTx(tx, req.OrganizationID, req.Date, clientCode)
	if err != nil {
		return nil, err
	}

	// dueDate defaults to the sale's own date, not organizations.due_days —
	// a counter sale (cash or a loan settled later via CreatePayment, not a
	// term the invoice itself tracks) has no invoicing due-date concept, and
	// leaving it nil rendered as a bare "-" in the Invoices list with no
	// explanation.
	_, err = tx.Exec(`
		INSERT INTO invoices (
			id, organizationId, number, state, clientId, date, dueDate,
			currency, exchangeRate, exchangeRateDate, customerNotes, overdueCharge, total, taxTotal, subTotal,
			buyerReference, paymentTerms, fiscalStampAmount, withholdingTaxRate, withholdingTaxAmount,
			movesStock
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`,
		invoiceID, req.OrganizationID, number, state, client.ID, req.Date, req.Date,
		req.Currency, nil, nil, nil, nil, req.Total, req.TaxTotal, req.SubTotal,
		nil, nil, 0, nil, nil,
	)
	if err != nil {
		if isDuplicateInvoiceNumber(err) {
			return nil, refuseTakenInvoiceNumber(number)
		}
		return nil, fmt.Errorf("create_cash_sale insert_invoice: %w", err)
	}

	for _, item := range invoiceLineItems {
		_, err = tx.Exec(`
			INSERT INTO invoiceLineItems (id, invoiceId, description, quantity, unitPrice, taxRate, productId, position)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			item.ID, invoiceID, item.Description, item.Quantity, item.UnitPrice, item.TaxRate, item.ProductID, item.Position,
		)
		if err != nil {
			return nil, fmt.Errorf("create_cash_sale insert_line_item: %w", err)
		}
	}

	if _, err := tx.Exec(`UPDATE organizations SET invoice_number_counter = ? WHERE id = ?`, counter, req.OrganizationID); err != nil {
		return nil, fmt.Errorf("create_cash_sale bump_counter: %w", err)
	}

	if needsGL {
		if _, err := postAutoEntryTx(
			tx, req.OrganizationID, saleJournal.ID, SourceDocInvoice, invoiceID, req.Date, number, "Invoice "+number, saleGLLines,
		); err != nil {
			return nil, err
		}
	}

	invoice.Number = number
	if err := applyInvoiceStockOutTx(tx, invoice, stockPlan); err != nil {
		return nil, err
	}

	if req.AmountReceived > 0 {
		_, err = tx.Exec(`
			INSERT INTO payments (
				id, organizationId, direction, clientId, bankAccountId,
				amount, currency, exchangeRate, date, method, reference, notes
			) VALUES (?, ?, 'inbound', ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			paymentID, req.OrganizationID, client.ID, bankAccountID,
			req.AmountReceived, req.Currency, nil, req.Date, req.PaymentMethod, req.Reference, req.Notes,
		)
		if err != nil {
			return nil, fmt.Errorf("create_cash_sale insert_payment: %w", err)
		}

		appID, err := gonanoid.New()
		if err != nil {
			return nil, fmt.Errorf("create_cash_sale application id: %w", err)
		}
		if _, err := tx.Exec(
			`INSERT INTO payment_applications (id, paymentId, documentType, documentId, amount) VALUES (?, ?, 'invoice', ?, ?)`,
			appID, paymentID, invoiceID, req.AmountReceived,
		); err != nil {
			return nil, fmt.Errorf("create_cash_sale insert_application: %w", err)
		}

		description := fmt.Sprintf("Payment %s", paymentID)
		entryID, err := postAutoEntryTx(
			tx, req.OrganizationID, settlementJournal.ID, SourceDocPayment, paymentID, req.Date,
			derefString(req.Reference), description, settlementGLLines,
		)
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(`UPDATE payments SET journalEntryId = ? WHERE id = ?`, entryID, paymentID); err != nil {
			return nil, fmt.Errorf("create_cash_sale link_entry: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("create_cash_sale commit: %w", err)
	}

	result := &CashSaleResult{}
	if result.Client, err = d.GetClient(client.ID); err != nil {
		return nil, err
	}
	if result.Invoice, err = d.GetInvoice(invoiceID); err != nil {
		return nil, err
	}
	if req.AmountReceived > 0 {
		if result.Payment, err = d.GetPayment(paymentID); err != nil {
			return nil, err
		}
	}
	return result, nil
}
