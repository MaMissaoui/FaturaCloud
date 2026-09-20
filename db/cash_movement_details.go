package db

import (
	"fmt"
	"sort"
)

// CashMovementDetail is one register transaction on a given day — the
// per-line detail behind GetDailyCashMovements' daily In/Out aggregate.
//
// Kind classifies each inbound payment by its position against the
// invoice it settles, computed from *when the payment happened*, not the
// invoice's current state. State is retroactive and would get this
// backwards: a three-month-old loan repaid today flips that invoice to
// "paid" the moment the balance clears, so a state-based label would
// relabel *today's* repayment — and, navigating back, the *original
// deposit* too — as a plain "sale". The first-payment-application signal
// below is recorded history, not a current value, so it never changes
// once written:
//   - "sale": the invoice's first-ever payment application, for the full
//     total — a cash sale settled in one shot.
//   - "loan": the invoice's first-ever payment application, for less
//     than the total — a loan sale's upfront deposit (a zero-deposit
//     loan has no payment row at all, so it never appears here — see
//     GetLoanStatus for those).
//   - "repayment": any later payment against an invoice that already had
//     one — collecting an outstanding loan sale's balance.
//
// Voided payments are excluded. GetDailyCashMovements' aggregate sums
// journal_lines for status IN ('posted','reversed'), so a payment voided
// on a *later* day still contributes its original amount to the *earlier*
// day's In total there — this list, scoped to currently-posted payments
// only, can therefore undercount relative to the aggregate panel above it
// for a day that had a since-voided payment. Deliberately not reconciled
// further: the same "visibility, not a guarantee" stance
// GetInventoryValuation's Difference already takes elsewhere.
type CashMovementDetail struct {
	ID            string  `json:"id"`
	Date          int64   `json:"date"`
	Direction     string  `json:"direction"` // "in" | "out"
	Kind          string  `json:"kind"`      // "sale" | "loan" | "repayment" | "withdrawal"
	Amount        int64   `json:"amount"`
	ClientName    *string `json:"clientName"`
	InvoiceID     *string `json:"invoiceId"`
	InvoiceNumber *string `json:"invoiceNumber"`
	Note          *string `json:"note"`
}

// GetCashMovementDetails is GetDailyCashMovements' per-transaction drill
// down for accountID covering the UTC calendar days that startDate and
// endDate fall in, inclusive of both. Two sources, merged and sorted by
// date: inbound payments applied to an invoice (the "in" side — a
// withdrawal is never an application, see CreateCashMovement) and
// cash_movements withdrawals (the "out" side, always kind "withdrawal" —
// cash_movements has no client and no invoice to attach).
//
// Both endpoints are interpreted as *days*, not as literal instants: each
// is floored to its UTC day (floorToUTCDay, the same bucketing
// GetDailyCashMovements uses) and the filter is
// [floor(startDate), floor(endDate) + 24h). This is deliberate, because
// every caller today passes the same UTC-midnight value twice to mean "this
// whole day" (src/routes/cash-book.tsx and db/report_export.go), while the
// stored timestamps carry real time-of-day — a live sale or withdrawal is
// stamped at the moment it happened — so a literal >= / <= on that instant
// silently matched only a row stamped exactly 00:00:00.000. Flooring here
// instead of expanding at the call site keeps that same-day call working and
// also treats a genuine multi-day range as covering every day from start's
// day through end's day, rather than degrading to a zero-width window
// whenever both endpoints share an instant.
func (d *Database) GetCashMovementDetails(organizationID, accountID string, startDate, endDate int64) ([]CashMovementDetail, error) {
	if _, err := d.resolveCashReportAccount(organizationID, accountID); err != nil {
		return nil, err
	}
	if endDate < startDate {
		return nil, newValidationError("endDate must not be before startDate")
	}

	startMs := floorToUTCDay(startDate).UnixMilli()
	endExclusiveMs := floorToUTCDay(endDate).AddDate(0, 0, 1).UnixMilli()

	type paymentRow struct {
		ID            string  `db:"id"`
		Date          int64   `db:"date"`
		Amount        int64   `db:"amount"`
		ClientName    *string `db:"clientName"`
		InvoiceID     string  `db:"invoiceId"`
		InvoiceNumber string  `db:"invoiceNumber"`
		Kind          string  `db:"kind"`
		Note          *string `db:"note"`
	}
	payments := []paymentRow{}
	// rn is ranked over EVERY payment ever applied to the invoice,
	// regardless of which account or date range it landed in — "is this
	// the first payment" is a fact about the invoice's whole history, not
	// something the caller's own date/account filter should be able to
	// change. Only the outer query restricts to this account and range.
	err := d.DB.Select(&payments, `
		WITH ranked AS (
			SELECT pa.paymentId, pa.documentId AS invoiceId, pa.amount AS appliedAmount,
			       ROW_NUMBER() OVER (
			           PARTITION BY pa.documentId ORDER BY p.date ASC, pa.rowid ASC
			       ) AS rn
			FROM payment_applications pa
			JOIN payments p ON p.id = pa.paymentId
			WHERE pa.documentType = 'invoice' AND p.status = 'posted' AND p.organizationId = ?
		)
		SELECT p.id, p.date, r.appliedAmount AS amount, c.name AS clientName,
		       i.id AS invoiceId, i.number AS invoiceNumber, p.notes AS note,
		       CASE WHEN r.rn = 1 AND r.appliedAmount >= i.total THEN 'sale'
		            WHEN r.rn = 1 THEN 'loan'
		            ELSE 'repayment' END AS kind
		FROM ranked r
		JOIN payments p ON p.id = r.paymentId
		JOIN invoices i ON i.id = r.invoiceId
		LEFT JOIN clients c ON c.id = p.clientId
		WHERE p.bankAccountId = ? AND p.date >= ? AND p.date < ?
		ORDER BY p.date ASC`,
		organizationID, accountID, startMs, endExclusiveMs,
	)
	if err != nil {
		return nil, fmt.Errorf("get_cash_movement_details payments: %w", err)
	}

	type withdrawalRow struct {
		ID     string  `db:"id"`
		Date   int64   `db:"date"`
		Amount int64   `db:"amount"`
		Note   *string `db:"note"`
	}
	withdrawals := []withdrawalRow{}
	err = d.DB.Select(&withdrawals, `
		SELECT id, date, amount, note
		FROM cash_movements
		WHERE organizationId = ? AND accountId = ? AND date >= ? AND date < ?
		ORDER BY date ASC`,
		organizationID, accountID, startMs, endExclusiveMs,
	)
	if err != nil {
		return nil, fmt.Errorf("get_cash_movement_details withdrawals: %w", err)
	}

	details := make([]CashMovementDetail, 0, len(payments)+len(withdrawals))
	for _, p := range payments {
		invoiceID, invoiceNumber := p.InvoiceID, p.InvoiceNumber
		details = append(details, CashMovementDetail{
			ID: p.ID, Date: p.Date, Direction: "in", Kind: p.Kind, Amount: p.Amount,
			ClientName: p.ClientName, InvoiceID: &invoiceID, InvoiceNumber: &invoiceNumber, Note: p.Note,
		})
	}
	for _, w := range withdrawals {
		details = append(details, CashMovementDetail{
			ID: w.ID, Date: w.Date, Direction: "out", Kind: "withdrawal", Amount: w.Amount, Note: w.Note,
		})
	}
	sort.Slice(details, func(i, j int) bool { return details[i].Date < details[j].Date })
	return details, nil
}
