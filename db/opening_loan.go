package db

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/jmoiron/sqlx"
	gonanoid "github.com/matoous/go-nanoid/v2"
)

// Loans migrated from a paper loan register (docs/loan-register-migration-plan.md,
// migration 0097). A migrated loan is a receivable brought forward at the
// cutover date, not a sale made in the app, so it is an invoice with
// origin = 'opening' that:
//
//   - is dated with its original sale date, for the customer's history and
//     the Loan status, but posts nothing on that date;
//   - never moves stock and posts no revenue, VAT or COGS;
//   - if still owing at cutover, posts ONE entry dated the cutover for its
//     outstanding balance — Dr Accounts Receivable, Cr Retained Earnings
//     (the owner chose the simplest treatment: real accounting is done
//     elsewhere, so there's no dedicated opening-balance account). Later
//     Cash Book collections post as usual and run that AR down to zero;
//   - if settled, posts nothing at all — it's history only;
//   - records what was paid before the cutover as one payment (origin =
//     'opening', method "other") applied to the whole invoice, with no GL
//     entry (the invoice's entry already carries only the outstanding) and
//     no register: its bankAccountId is the Retained Earnings account, so no
//     register report ever sees it.
//
// It is frozen: only Cash Book collections (and a direct payment) change it.
// Editing, deleting, exporting, voiding its paid-to-date, and any state
// change other than sent<->paid are refused (see the guards calling
// isOpeningLoan). It is left out of the sales analytics, the Tax Summary and
// the Dashboard's revenue figures (openingLoanExclusion), and always listed
// in the Loan status, settled ones included.

// OpeningOrigin is invoices.origin / payments.origin for a migrated loan.
const OpeningOrigin = "opening"

// openingLoanSourceType is the journal_entries.sourceDocumentType of a
// migrated loan's outstanding-balance entry — distinct from "invoice", so
// UpdateInvoiceState's sale-entry logic never mistakes it for one.
const openingLoanSourceType = "invoice_opening"

// openingLoanExclusion is the SQL predicate the sales analytics, Tax Summary
// and Dashboard revenue queries add for invoice alias i: a migrated loan is
// a sale the app never made.
const openingLoanExclusion = "i.origin IS NULL"

func isOpeningLoan(invoice *Invoice) bool {
	return invoice.Origin != nil && *invoice.Origin == OpeningOrigin
}

func openingLoanFrozenError(invoice *Invoice, action string) error {
	return newValidationError(
		"loan %s was brought forward from the paper loan register and can't be %s — only Cash Book collections change it",
		invoice.Number, action,
	)
}

// refuseOpeningLoanChange is the guard for paths that only have the id. A
// missing invoice passes: the caller reports that itself.
func (d *Database) refuseOpeningLoanChange(invoiceID, action string) error {
	invoice, err := d.GetInvoice(invoiceID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("opening_loan guard: %w", err)
	}
	if isOpeningLoan(invoice) {
		return openingLoanFrozenError(invoice, action)
	}
	return nil
}

// findInvoiceReceivableEntry returns the posted entry that put an invoice's
// receivable on the books — its sale entry, or for a migrated loan its
// outstanding-balance entry — which a payment must have to settle against.
func (d *Database) findInvoiceReceivableEntry(invoice *Invoice) (*JournalEntry, error) {
	if isOpeningLoan(invoice) {
		return d.FindPostedEntryForSourceDocument(openingLoanSourceType, invoice.ID)
	}
	return d.FindPostedEntryForSourceDocument("invoice", invoice.ID)
}

// updateOpeningLoanState allows a migrated loan only between sent and paid —
// what collecting it (or reopening it after a mistaken "paid") needs. Neither
// state touches the GL: the loan's only entry is its outstanding balance,
// posted once at cutover.
func (d *Database) updateOpeningLoanState(invoice *Invoice, state string) (*Invoice, error) {
	if state == invoice.State {
		return invoice, nil
	}
	if (state != "sent" && state != "paid") || (invoice.State != "sent" && invoice.State != "paid") {
		return nil, openingLoanFrozenError(invoice, "moved to "+state)
	}
	res, err := d.DB.Exec(`UPDATE invoices SET state = ? WHERE id = ? AND state = ?`, state, invoice.ID, invoice.State)
	if err != nil {
		return nil, fmt.Errorf("update_opening_loan_state: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, newValidationError("the loan's state changed while updating it — reload and try again")
	}
	return d.GetInvoice(invoice.ID)
}

// OpeningLoanLine is one item of a migrated loan. Amount is the line's gross
// total in cents, as written in the register (there is no tax on a migrated
// loan). ProductID is optional: a line the import can't match to a product
// keeps its free-text Description.
type OpeningLoanLine struct {
	ProductID   *string `json:"productId"`
	Description string  `json:"description"`
	Quantity    float64 `json:"quantity"`
	Amount      int64   `json:"amount"`
}

// OpeningLoanRequest is one loan from the paper register.
type OpeningLoanRequest struct {
	OrganizationID string `json:"organizationId"`
	ClientID       string `json:"clientId"`
	// Number is the loan's reference in the paper register (e.g. its
	// book/page/line). It becomes the invoice number — migrated loans don't
	// consume the organization's invoice counter — and must be unique among
	// the organization's migrated loans.
	Number      string            `json:"number"`
	Date        int64             `json:"date"`        // the original sale date
	CutoverDate int64             `json:"cutoverDate"` // when the app takes over; dates the GL entry
	Lines       []OpeningLoanLine `json:"lines"`
	PaidToDate  int64             `json:"paidToDate"`
	// LastPaymentDate dates the paid-to-date record (it posts nothing, so any
	// date up to the cutover is fine); nil means the cutover date.
	LastPaymentDate *int64  `json:"lastPaymentDate"`
	Notes           *string `json:"notes"`
	ImportBatchID   *string `json:"importBatchId"`
}

type openingLoanPlanLine struct {
	productID   *string
	description string
	quantity    float64
	unitPrice   int64
}

// openingLoanPlan is everything applyOpeningLoanTx writes, resolved before
// the transaction opens (db.SetMaxOpenConns(1): no d.DB read between Beginx
// and Commit) — so an import can plan every loan, then apply them all in one
// transaction.
type openingLoanPlan struct {
	req                  OpeningLoanRequest
	currency             string
	total                int64
	state                string
	lines                []openingLoanPlanLine
	retainedEarningsID   string
	outstandingGL        []CreateJournalLineRequest
	outstandingJournalID string
}

// planOpeningLoan validates a loan against its organization and builds its
// writes. It doesn't check req.ClientID: CreateOpeningLoan does, and an
// import may create the client in the same transaction.
func (d *Database) planOpeningLoan(org *Organization, req OpeningLoanRequest) (*openingLoanPlan, error) {
	req.Number = strings.TrimSpace(req.Number)
	switch {
	case req.Number == "":
		return nil, newValidationError("a migrated loan needs its register reference")
	case req.ClientID == "":
		return nil, newValidationError("loan %s: a customer is required", req.Number)
	case req.Date <= 0:
		return nil, newValidationError("loan %s: the sale date is required", req.Number)
	case req.CutoverDate <= 0:
		return nil, newValidationError("loan %s: the cutover date is required", req.Number)
	case req.Date > req.CutoverDate:
		return nil, newValidationError("loan %s: the sale date is after the cutover date", req.Number)
	case len(req.Lines) == 0:
		return nil, newValidationError("loan %s: at least one item is required", req.Number)
	case req.PaidToDate < 0:
		return nil, newValidationError("loan %s: paid to date can't be negative", req.Number)
	}
	if req.LastPaymentDate != nil && (*req.LastPaymentDate < req.Date || *req.LastPaymentDate > req.CutoverDate) {
		return nil, newValidationError("loan %s: the last payment date must fall between the sale date and the cutover date", req.Number)
	}
	if org.RetainedEarningsAccountID == nil {
		return nil, newValidationError("the organization has no Retained Earnings account configured")
	}

	plan := &openingLoanPlan{
		req:                req,
		currency:           orgCurrencyOrDefault(org),
		retainedEarningsID: *org.RetainedEarningsAccountID,
	}
	for i, line := range req.Lines {
		if line.Quantity <= 0 || math.IsNaN(line.Quantity) || math.IsInf(line.Quantity, 0) {
			return nil, newValidationError("loan %s, item %d: the quantity must be above zero", req.Number, i+1)
		}
		if line.Amount <= 0 {
			return nil, newValidationError("loan %s, item %d: the amount must be above zero", req.Number, i+1)
		}
		description := strings.TrimSpace(line.Description)
		productID := nilIfEmptyID(line.ProductID)
		if productID != nil {
			product, err := d.GetProduct(*productID)
			if err != nil {
				return nil, newValidationError("loan %s, item %d: product not found", req.Number, i+1)
			}
			if err := requireSameOrg(org.ID, product.OrganizationID, "product"); err != nil {
				return nil, err
			}
			if description == "" {
				description = product.Name
			}
		}
		if description == "" {
			return nil, newValidationError("loan %s, item %d: a product or a description is required", req.Number, i+1)
		}
		plan.lines = append(plan.lines, openingLoanPlanLine{
			productID:   productID,
			description: description,
			quantity:    line.Quantity,
			// Stored net of nothing — a migrated loan has no tax — so the
			// unit price is the gross amount per unit. A split that doesn't
			// divide exactly leaves the invoice total authoritative: the Loan
			// status allocates it across lines by their share.
			unitPrice: int64(math.Round(float64(line.Amount) / line.Quantity)),
		})
		plan.total += line.Amount
	}
	if req.PaidToDate > plan.total {
		return nil, newValidationError("loan %s: paid to date (%d) is more than the loan's total (%d)", req.Number, req.PaidToDate, plan.total)
	}
	plan.state = "sent"
	outstanding := plan.total - req.PaidToDate
	if outstanding == 0 {
		plan.state = "paid"
		return plan, nil
	}

	if org.DefaultArAccountID == nil {
		return nil, newValidationError("the organization has no default AR account configured")
	}
	journal, err := getJournalByTypeTx(d.DB, org.ID, "miscellaneous")
	if err != nil {
		return nil, err
	}
	clientID := req.ClientID
	plan.outstandingJournalID = journal.ID
	plan.outstandingGL = []CreateJournalLineRequest{
		glLine(*org.DefaultArAccountID, outstanding, 0, plan.currency, outstanding, nil, false, &clientID, nil, nil),
		glLine(plan.retainedEarningsID, 0, outstanding, plan.currency, outstanding, nil, false, nil, nil, nil),
	}
	return plan, nil
}

// applyOpeningLoanTx writes a planned loan: the invoice and its lines, the
// paid-to-date record, and (if still owing) the outstanding-balance entry.
// It returns the new invoice's id.
func applyOpeningLoanTx(tx *sqlx.Tx, plan *openingLoanPlan) (string, error) {
	req := plan.req
	var existing int
	if err := tx.Get(&existing,
		`SELECT COUNT(*) FROM invoices WHERE organizationId = ? AND origin = ? AND number = ?`,
		req.OrganizationID, OpeningOrigin, req.Number,
	); err != nil {
		return "", fmt.Errorf("apply_opening_loan duplicate check: %w", err)
	}
	if existing > 0 {
		return "", newValidationError("loan %s has already been brought forward", req.Number)
	}

	invoiceID, err := gonanoid.New()
	if err != nil {
		return "", fmt.Errorf("apply_opening_loan new_id: %w", err)
	}
	if _, err := tx.Exec(`
		INSERT INTO invoices (
			id, organizationId, number, state, clientId, date, dueDate, currency,
			customerNotes, total, taxTotal, subTotal, origin, importBatchId
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?)`,
		// Due on its sale date, like a Cash Book sale (CreateCashSale): a
		// counter loan has no invoicing terms, and AR aging then shows how
		// old the loan really is.
		invoiceID, req.OrganizationID, req.Number, plan.state, req.ClientID, req.Date, req.Date, plan.currency,
		req.Notes, plan.total, plan.total, OpeningOrigin, req.ImportBatchID,
	); err != nil {
		return "", fmt.Errorf("apply_opening_loan insert_invoice: %w", err)
	}
	for i, line := range plan.lines {
		lineID, _ := gonanoid.New()
		if _, err := tx.Exec(`
			INSERT INTO invoiceLineItems (id, invoiceId, description, quantity, unitPrice, taxRate, productId, position)
			VALUES (?, ?, ?, ?, ?, NULL, ?, ?)`,
			lineID, invoiceID, line.description, line.quantity, line.unitPrice, line.productID, i,
		); err != nil {
			return "", fmt.Errorf("apply_opening_loan insert_line: %w", err)
		}
	}

	if req.PaidToDate > 0 {
		paymentID, _ := gonanoid.New()
		paidOn := req.CutoverDate
		if req.LastPaymentDate != nil {
			paidOn = *req.LastPaymentDate
		}
		reference := "Opening balance"
		if _, err := tx.Exec(`
			INSERT INTO payments (
				id, organizationId, direction, clientId, bankAccountId,
				amount, currency, date, method, reference, origin
			) VALUES (?, ?, 'inbound', ?, ?, ?, ?, ?, 'other', ?, ?)`,
			paymentID, req.OrganizationID, req.ClientID, plan.retainedEarningsID,
			req.PaidToDate, plan.currency, paidOn, reference, OpeningOrigin,
		); err != nil {
			return "", fmt.Errorf("apply_opening_loan insert_payment: %w", err)
		}
		appID, _ := gonanoid.New()
		if _, err := tx.Exec(
			`INSERT INTO payment_applications (id, paymentId, documentType, documentId, amount) VALUES (?, ?, 'invoice', ?, ?)`,
			appID, paymentID, invoiceID, req.PaidToDate,
		); err != nil {
			return "", fmt.Errorf("apply_opening_loan insert_application: %w", err)
		}
	}

	if plan.outstandingGL != nil {
		if _, err := postAutoEntryTx(
			tx, req.OrganizationID, plan.outstandingJournalID, openingLoanSourceType, invoiceID,
			req.CutoverDate, req.Number, "Loan brought forward from the paper register: "+req.Number,
			plan.outstandingGL,
		); err != nil {
			return "", err
		}
	}
	return invoiceID, nil
}

// CreateOpeningLoan brings one loan forward from the paper register in its
// own transaction — the single-loan entry point (and what the import's tests
// build on). The import itself plans every loan and applies them together.
func (d *Database) CreateOpeningLoan(req OpeningLoanRequest) (*Invoice, error) {
	org, err := d.GetOrganization(req.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("create_opening_loan organization: %w", err)
	}
	client, err := d.GetClient(req.ClientID)
	if err != nil {
		return nil, newValidationError("customer not found")
	}
	if err := requireSameOrg(org.ID, client.OrganizationID, "client"); err != nil {
		return nil, err
	}
	plan, err := d.planOpeningLoan(org, req)
	if err != nil {
		return nil, err
	}

	tx, err := d.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("create_opening_loan begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	invoiceID, err := applyOpeningLoanTx(tx, plan)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("create_opening_loan commit: %w", err)
	}
	return d.GetInvoice(invoiceID)
}
