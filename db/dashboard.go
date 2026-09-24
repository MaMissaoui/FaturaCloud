package db

import (
	"fmt"
	"math/big"
	"sort"
	"time"
)

// "Revenue" (revenueStates) is defined in db/sales_reports.go, shared with
// this file's getRevenueByMonth-turned-GetRevenueByMonth caller below.
//
// Every SUM/aggregate below multiplies by COALESCE(exchangeRate, 1) before
// summing: invoices can each be in a different currency (invoices.currency),
// but this whole page — every stat tile, every table cell — renders every
// number through the organization's own currency formatter
// (src/routes/dashboard.tsx's `money`), never the invoice's. Without the
// conversion, summing raw cents across currencies would silently produce a
// number in no currency at all. See db/exchange_rate.go for the
// exchangeRate direction/storage convention this relies on.

type OutstandingInvoice struct {
	ID         string `db:"id"         json:"id"`
	Number     string `db:"number"     json:"number"`
	ClientName string `db:"clientName" json:"clientName"`
	DueDate    *int64 `db:"dueDate"    json:"dueDate"`
	// Total is the remaining balance converted to the organization's
	// functional currency (× exchangeRate) — what every bucket/summary sum
	// below is expressed in. Currency/ForeignTotal (F116, multi-currency
	// reporting) carry the same balance in the document's own currency,
	// unconverted, so a foreign-currency AR/AP line stays visible in the
	// currency it's actually owed in instead of only its functional-currency
	// equivalent — a display addition, not a new bucketing dimension: the
	// buckets themselves stay functional-currency, since summing balances
	// across different currencies into one bucket wouldn't mean anything.
	Currency     string `db:"currency"     json:"currency"`
	ForeignTotal int64  `db:"foreignTotal" json:"foreignTotal"`
	Total        int64  `db:"total"        json:"total"`
	DaysOverdue  int    `json:"daysOverdue"`
}

type OutstandingSummary struct {
	Total      int64                `json:"total"`
	Current    int64                `json:"current"`
	Days1To30  int64                `json:"days1To30"`
	Days31To60 int64                `json:"days31To60"`
	Days61To90 int64                `json:"days61To90"`
	Days90Plus int64                `json:"days90Plus"`
	Invoices   []OutstandingInvoice `json:"invoices"`
}

type StockValuationItem struct {
	ProductID string  `db:"id"            json:"productId"`
	Name      string  `db:"name"          json:"name"`
	Quantity  float64 `db:"stockQuantity" json:"quantity"`
	Value     int64   `db:"value"         json:"value"`
}

type StockValuation struct {
	Total int64                `json:"total"`
	Items []StockValuationItem `json:"items"`
}

type DashboardData struct {
	RevenueByMonth []MonthlyRevenue   `json:"revenueByMonth"`
	Outstanding    OutstandingSummary `json:"outstanding"`
	StockValuation StockValuation     `json:"stockValuation"`
	TopClients     []ClientRevenue    `json:"topClients"`
	TopProducts    []ProductRevenue   `json:"topProducts"`
}

// topN is fixed rather than caller-configurable — this is a dashboard widget
// size, not a general-purpose reporting API.
const topN = 10

// GetDashboardData composes the Dashboard widget page from the same
// range-based report functions db/sales_reports.go exposes under the
// Reporting menu — startDate/endDate (either a rolling "last N months"
// cutoff with endDate=0/unbounded, via DashboardCutoff, or a calendar
// year's bounds, via DashboardYearRange — api/dashboard.go decides which)
// and topN as the ranked-list limit. This keeps the widget and the full
// reports as one source of truth instead of two copies of "revenue by
// month" that could drift.
func (d *Database) GetDashboardData(organizationID string, startDate, endDate int64) (DashboardData, error) {
	revenueByMonth, err := d.GetRevenueByMonth(organizationID, startDate, endDate)
	if err != nil {
		return DashboardData{}, err
	}
	outstanding, err := d.getOutstandingInvoices(organizationID)
	if err != nil {
		return DashboardData{}, err
	}
	stockValuation, err := d.getStockValuation(organizationID)
	if err != nil {
		return DashboardData{}, err
	}
	topClients, err := d.GetSalesByClient(organizationID, startDate, endDate, topN)
	if err != nil {
		return DashboardData{}, err
	}
	topProducts, err := d.GetSalesByProduct(organizationID, startDate, endDate, topN)
	if err != nil {
		return DashboardData{}, err
	}

	return DashboardData{
		RevenueByMonth: revenueByMonth,
		Outstanding:    outstanding,
		StockValuation: stockValuation,
		TopClients:     topClients,
		TopProducts:    topProducts,
	}, nil
}

// The two payment-status predicates the outstanding/aging/loan queries
// below filter on. Deliberately spelled as SQL fragments (not placeholders)
// because they're inlined into the correlated subqueries built by
// documentPaidAmountExpr — these are fixed internal literals, never user
// input.
const (
	paymentsNonVoided = "p.status != 'voided'"
	paymentsPosted    = "p.status = 'posted'"
)

// documentPaidAmountExpr builds a correlated scalar subquery returning the
// total amount paid against the document row aliased by docAlias (invoices
// or incoming_invoices) by payments matching statusClause.
//
// This replaces the previous `LEFT JOIN (SELECT documentId, SUM(amount) ...
// GROUP BY documentId)` shape. That derived table had no organization scope
// (payment_applications carries no organizationId), so SQLite materialized
// every payment application in every organization on every call, then built
// a runtime AUTOMATIC COVERING INDEX just to join it back — cost growing
// with total tenant-wide payments, not the org's own. The correlated form
// instead seeks the existing payment_applications_document(documentType,
// documentId) index (migration 0056) once per row. Measured ~16× faster for
// the outstanding query and ~3.7× for loan status at 20 orgs / 500k
// invoices / 200k applications (see the 2026-09-20 DB audit).
func documentPaidAmountExpr(docAlias, documentType, statusClause string) string {
	return fmt.Sprintf(`(
		SELECT COALESCE(SUM(pa.amount), 0)
		FROM payment_applications pa
		JOIN payments p ON p.id = pa.paymentId
		WHERE pa.documentType = '%s' AND pa.documentId = %s.id AND %s
	)`, documentType, docAlias, statusClause)
}

// documentPaymentAppCountExpr is documentPaidAmountExpr's COUNT(*) twin,
// used by GetLoanStatus to tell a pure cash sale (exactly one full payment)
// from a loan — see LoanStatusRow's doc comment.
func documentPaymentAppCountExpr(docAlias, documentType, statusClause string) string {
	return fmt.Sprintf(`(
		SELECT COUNT(*)
		FROM payment_applications pa
		JOIN payments p ON p.id = pa.paymentId
		WHERE pa.documentType = '%s' AND pa.documentId = %s.id AND %s
	)`, documentType, docAlias, statusClause)
}

// getOutstandingInvoices keeps the pre-Phase-3 filter of state == 'sent'
// only — 'paid' is a manual, free-transitioning flag disconnected from real
// payments (see CLAUDE.md), and second-guessing it here would be a product
// decision this function shouldn't make silently. What Phase 3 payments do
// fix: a 'sent' invoice that already has a real partial (or full) payment
// applied now shows its actual remaining balance — previously this always
// showed the full total, and a fully-paid-via-real-payments invoice never
// dropped off the list at all.
func (d *Database) getOutstandingInvoices(organizationID string) (OutstandingSummary, error) {
	invoices := []OutstandingInvoice{}
	err := d.DB.Select(&invoices, fmt.Sprintf(`
		SELECT id, number, clientName, dueDate, currency,
		       CAST(ROUND(total - paid) AS INTEGER) AS foreignTotal,
		       CAST(ROUND((total - paid) * COALESCE(exchangeRate, 1)) AS INTEGER) AS total
		FROM (
			SELECT i.id, i.number, c.name AS clientName, i.dueDate, i.currency,
			       i.exchangeRate, i.total,
			       %s AS paid
			FROM invoices i
			JOIN clients c ON i.clientId = c.id
			WHERE i.organizationId = ? AND i.state = 'sent'
		)
		WHERE (total - paid) > 0
		ORDER BY dueDate ASC`,
		documentPaidAmountExpr("i", "invoice", paymentsNonVoided),
	),
		organizationID,
	)
	if err != nil {
		return OutstandingSummary{}, fmt.Errorf("get_outstanding_invoices: %w", err)
	}
	return bucketOutstanding(invoices, time.Now()), nil
}

// GetReceivableAging is the AR aging report (Phase 4) — a thin public name
// for the same computation the dashboard's "Outstanding" widget already
// does, rather than rebuilding it under a different query.
func (d *Database) GetReceivableAging(organizationID string) (OutstandingSummary, error) {
	return d.getOutstandingInvoices(organizationID)
}

// GetClientOpenInvoices is the Cash Book screen's "does this client have any
// outstanding loan sales" lookup. Deliberately NOT a call to
// getOutstandingInvoices above: that query filters state = 'sent' only,
// which would drop a loan sale someone manually marked 'paid' while a
// balance still remains (invoices.state has no transition matrix — see
// db/CLAUDE.md). This filters on the actual balance instead, but still
// restricts to state IN ('sent', 'paid') — a 'draft' invoice has no posted
// GL entry yet (needsInvoiceGLPresence), so it can't accept a payment
// through this screen's PaymentPanel and must not be offered a "Pay" button.
func (d *Database) GetClientOpenInvoices(clientID string) ([]OutstandingInvoice, error) {
	invoices := []OutstandingInvoice{}
	err := d.DB.Select(&invoices, fmt.Sprintf(`
		SELECT id, number, clientName, dueDate, currency,
		       CAST(ROUND(total - paid) AS INTEGER) AS foreignTotal,
		       CAST(ROUND((total - paid) * COALESCE(exchangeRate, 1)) AS INTEGER) AS total
		FROM (
			SELECT i.id, i.number, c.name AS clientName, i.dueDate, i.currency,
			       i.exchangeRate, i.total, i.date AS docDate,
			       %s AS paid
			FROM invoices i
			JOIN clients c ON i.clientId = c.id
			WHERE i.clientId = ? AND i.state IN ('sent', 'paid')
		)
		WHERE (total - paid) > 0
		ORDER BY docDate ASC`,
		documentPaidAmountExpr("i", "invoice", paymentsNonVoided),
	),
		clientID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_client_open_invoices: %w", err)
	}
	return invoices, nil
}

// LoanStatusRow is one invoice line's contribution to the loan tracker:
// which customer, when, what product/quantity, how much that line accounts
// for, and how much of it has been paid. A line's Amount is its share of the
// invoice total (tax, discount and stamp included) by net line value, and its
// Paid is what's been applied to it: Cash Book loan payments name the line
// they settle (payment_applications.invoiceLineItemId, migration 0090) and
// count against that line only, while invoice-level payments (the upfront
// amount of a cash sale, anything recorded through the invoice page's
// payment panel) are spread across the lines — see allocateInvoiceLines.
//
// The tracker lists every invoice that was ever a loan, including one since
// settled (its lines show outstanding 0), and leaves out a pure cash sale —
// one paid in full by a single payment. An invoice counts as a loan when it
// has no payment yet, more than one, a balance still owing, or any Cash Book
// line settlement (lineAppCount): only POST /api/cash-sales/{id}/payments
// names a line, so a zero-deposit loan cleared by one line payment stays
// listed. What the filter still can't tell from a cash sale is a loan
// cleared by exactly one invoice-level payment (recorded from the invoice
// page, or a repayment made before migration 0090) — both are one full
// payment with no line named.
type LoanStatusRow struct {
	LineID    string `db:"lineId"      json:"lineId"`
	InvoiceID string `db:"invoiceId"   json:"invoiceId"`
	// InvoiceNumber doubles as the loan number on the Cash Book screen.
	InvoiceNumber string  `db:"invoiceNumber" json:"invoiceNumber"`
	ClientID      string  `db:"clientId"    json:"clientId"`
	ClientName    string  `db:"clientName"  json:"clientName"`
	Date          int64   `db:"date"        json:"date"`
	ProductName   string  `db:"productName" json:"productName"`
	Sku           string  `db:"sku"         json:"sku"`
	Quantity      float64 `db:"quantity"    json:"quantity"`
	Amount        int64   `db:"amount"      json:"amount"`
	Paid          int64   `db:"paid"        json:"paid"`
	Outstanding   int64   `db:"outstanding" json:"outstanding"`
}

// loanLineRaw is one invoice line as read for the loan tracker, before its
// invoice's total and payments are allocated across the lines.
type loanLineRaw struct {
	LineID        string  `db:"lineId"`
	InvoiceID     string  `db:"invoiceId"`
	InvoiceNumber string  `db:"invoiceNumber"`
	ClientID      string  `db:"clientId"`
	ClientName    string  `db:"clientName"`
	Date          int64   `db:"docDate"`
	InvoiceTotal  int64   `db:"invoiceTotal"`
	InvoicePaid   int64   `db:"invoicePaid"`
	AppCount      int64   `db:"appCount"`
	LineAppCount  int64   `db:"lineAppCount"`
	LinePaid      int64   `db:"linePaid"`
	ProductName   string  `db:"productName"`
	Sku           string  `db:"sku"`
	Quantity      float64 `db:"quantity"`
	NetLine       int64   `db:"netLine"`
}

// loanLineOrder is the within-invoice line order every allocation over
// loanLineRaw rows assumes (GetLoanStatus and invoiceLineBalances must agree,
// or the rounding remainder would land on a different line in each).
const loanLineOrder = "netLine DESC, lineId ASC"

// loanLinesQuery reads every line of the invoices matching where (a
// condition on the invoice alias i), with the invoice's total, its paid
// amount, and the amount applied to each line specifically. Only posted
// payments count, the same clause the rest of the loan tracker uses.
func loanLinesQuery(where string) string {
	return fmt.Sprintf(`
		SELECT li.id AS lineId, i.id AS invoiceId, COALESCE(i.number, '') AS invoiceNumber, i.clientId AS clientId,
		       c.name AS clientName, i.date AS docDate,
		       i.total AS invoiceTotal,
		       %s AS invoicePaid,
		       %s AS appCount,
		       (
		           SELECT COUNT(*)
		           FROM payment_applications pa
		           JOIN payments p ON p.id = pa.paymentId
		           WHERE pa.documentType = 'invoice' AND pa.documentId = i.id
		             AND pa.invoiceLineItemId IS NOT NULL AND %s
		       ) AS lineAppCount,
		       (
		           SELECT COALESCE(SUM(pa.amount), 0)
		           FROM payment_applications pa
		           JOIN payments p ON p.id = pa.paymentId
		           WHERE pa.documentType = 'invoice' AND pa.invoiceLineItemId = li.id AND %s
		       ) AS linePaid,
		       COALESCE(pr.name, li.description, '') AS productName,
		       COALESCE(pr.sku, '') AS sku,
		       li.quantity AS quantity,
		       CAST(ROUND(li.unitPrice * li.quantity) AS INTEGER) AS netLine
		FROM invoices i
		JOIN clients c ON i.clientId = c.id
		JOIN invoiceLineItems li ON li.invoiceId = i.id
		LEFT JOIN products pr ON li.productId = pr.id
		WHERE %s`,
		documentPaidAmountExpr("i", "invoice", paymentsPosted),
		documentPaymentAppCountExpr("i", "invoice", paymentsPosted),
		paymentsPosted,
		paymentsPosted,
		where,
	)
}

// GetLoanStatus is the Cash Book screen's embedded loan tracker — org-wide
// by default, or scoped to one customer via clientID (empty means every
// customer). See LoanStatusRow's doc comment for the "was ever a loan"
// filter and why a settled loan still appears, and for how each invoice's
// payments are split across its lines. Sorted oldest-first by date — the
// longest-outstanding loan surfaces first — with the largest outstanding
// amount and then line id breaking ties so the order is deterministic.
func (d *Database) GetLoanStatus(organizationID, clientID string) ([]LoanStatusRow, error) {
	query := `SELECT * FROM (` + loanLinesQuery(`i.organizationId = ? AND i.state IN ('sent', 'paid')`) + `)
		WHERE (appCount = 0 OR appCount > 1 OR invoicePaid < invoiceTotal OR lineAppCount > 0)`
	args := []any{organizationID}
	if clientID != "" {
		query += " AND clientId = ?"
		args = append(args, clientID)
	}
	query += " ORDER BY docDate ASC, invoiceId ASC, " + loanLineOrder

	raw := []loanLineRaw{}
	if err := d.DB.Select(&raw, query, args...); err != nil {
		return nil, fmt.Errorf("get_loan_status: %w", err)
	}

	// The rows are contiguous per invoice (ORDER BY invoiceId), so a single
	// forward pass groups them.
	rows := make([]LoanStatusRow, 0, len(raw))
	for i := 0; i < len(raw); {
		j := i
		for j < len(raw) && raw[j].InvoiceID == raw[i].InvoiceID {
			j++
		}
		amounts, paid := allocateInvoiceLines(raw[i:j])
		for k := i; k < j; k++ {
			r := raw[k]
			rows = append(rows, LoanStatusRow{
				LineID: r.LineID, InvoiceID: r.InvoiceID, InvoiceNumber: r.InvoiceNumber, ClientID: r.ClientID,
				ClientName: r.ClientName, Date: r.Date,
				ProductName: r.ProductName, Sku: r.Sku, Quantity: r.Quantity,
				Amount: amounts[k-i], Paid: paid[k-i], Outstanding: amounts[k-i] - paid[k-i],
			})
		}
		i = j
	}

	sort.SliceStable(rows, func(a, b int) bool {
		// Oldest first: the longest-outstanding debt is what a cashier should
		// chase first, which matters more than its size. Largest outstanding
		// then breaks a same-day tie, and line id makes the order stable.
		if rows[a].Date != rows[b].Date {
			return rows[a].Date < rows[b].Date
		}
		if rows[a].Outstanding != rows[b].Outstanding {
			return rows[a].Outstanding > rows[b].Outstanding
		}
		return rows[a].LineID < rows[b].LineID
	})
	return rows, nil
}

// allocateInvoiceLines splits one invoice's total and paid amount across its
// lines (all rows of the same invoice, in loanLineOrder). It returns each
// line's amount and paid, both summing exactly to the invoice's figures.
//
// A line's amount is the invoice total weighted by net line value, the last
// line absorbing the rounding remainder. A line's paid is what was applied to
// it directly (LinePaid) plus its share of the invoice-level payments: those
// are spread in proportion to the line amounts, but never beyond what a line
// still owes after its own direct payments — see allocateCapped. That's what
// makes paying one line leave every other line's balance untouched.
func allocateInvoiceLines(lines []loanLineRaw) (amounts, paid []int64) {
	n := len(lines)
	amounts = make([]int64, n)
	paid = make([]int64, n)
	if n == 0 {
		return amounts, paid
	}
	total, invoicePaid := lines[0].InvoiceTotal, lines[0].InvoicePaid

	weights := make([]int64, n)
	var weightSum int64
	for k, l := range lines {
		weights[k] = l.NetLine
		if weights[k] <= 0 {
			// A free (or negative) line still gets a share rather than
			// dividing by zero or pulling another line's share negative.
			weights[k] = 1
		}
		weightSum += weights[k]
	}
	var allocated int64
	for k := range lines {
		if k == n-1 {
			amounts[k] = total - allocated
		} else {
			amounts[k] = mulDiv(total, weights[k], weightSum)
		}
		allocated += amounts[k]
	}

	var direct int64
	caps := make([]int64, n)
	for k, l := range lines {
		paid[k] = l.LinePaid
		direct += l.LinePaid
		caps[k] = amounts[k] - l.LinePaid
		if caps[k] < 0 {
			caps[k] = 0
		}
	}
	undirected := invoicePaid - direct
	if undirected < 0 {
		undirected = 0
	}
	for k, share := range allocateCapped(undirected, amounts, caps) {
		paid[k] += share
	}
	return amounts, paid
}

// allocateCapped spreads total across slots in proportion to weights, with
// slot k never receiving more than caps[k]: whatever a capped slot can't take
// is redistributed over the remaining slots (water-filling), and leftover
// cents from the integer division go one at a time to slots with room. If
// total exceeds the sum of the caps (only possible with inconsistent data),
// the excess lands on the last slot so nothing is silently dropped.
func allocateCapped(total int64, weights, caps []int64) []int64 {
	n := len(caps)
	shares := make([]int64, n)
	if n == 0 || total <= 0 {
		return shares
	}
	remaining := total
	active := make([]bool, n)
	for k := range caps {
		active[k] = caps[k] > 0
	}
	for remaining > 0 {
		var weightSum int64
		for k := range caps {
			if active[k] {
				w := weights[k]
				if w <= 0 {
					w = 1
				}
				weightSum += w
			}
		}
		if weightSum == 0 {
			break
		}
		pool := remaining
		progressed, capped := false, false
		for k := range caps {
			if !active[k] {
				continue
			}
			w := weights[k]
			if w <= 0 {
				w = 1
			}
			share := mulDiv(pool, w, weightSum)
			if room := caps[k] - shares[k]; share >= room {
				share = room
				active[k] = false
				capped = true
			}
			if share > 0 {
				shares[k] += share
				remaining -= share
				progressed = true
			}
		}
		if !capped {
			// Every active slot took its full proportional share; only the
			// integer-division leftover (fewer cents than slots) remains.
			break
		}
		if !progressed {
			break
		}
	}
	for k := 0; k < n && remaining > 0; k++ {
		if shares[k] < caps[k] {
			shares[k]++
			remaining--
		}
	}
	for remaining > 0 {
		// Still left after a full pass: every slot has one cent of room at
		// most per pass, so keep filling in order until the caps run out.
		progressed := false
		for k := 0; k < n && remaining > 0; k++ {
			if shares[k] < caps[k] {
				shares[k]++
				remaining--
				progressed = true
			}
		}
		if !progressed {
			shares[n-1] += remaining
			remaining = 0
		}
	}
	return shares
}

// mulDiv returns a*b/c truncated toward zero, computed without int64
// overflow (two cent amounts multiplied together easily exceed it).
func mulDiv(a, b, c int64) int64 {
	if c == 0 {
		return 0
	}
	r := new(big.Int).Mul(big.NewInt(a), big.NewInt(b))
	r.Quo(r, big.NewInt(c))
	return r.Int64()
}

// OutstandingBill is getOutstandingInvoices' purchases counterpart. See
// OutstandingInvoice above for what Currency/ForeignTotal carry.
type OutstandingBill struct {
	ID           string `db:"id"           json:"id"`
	Number       string `db:"number"       json:"number"`
	VendorName   string `db:"vendorName"   json:"vendorName"`
	DueDate      *int64 `db:"dueDate"      json:"dueDate"`
	Currency     string `db:"currency"     json:"currency"`
	ForeignTotal int64  `db:"foreignTotal" json:"foreignTotal"`
	Total        int64  `db:"total"        json:"total"`
	DaysOverdue  int    `json:"daysOverdue"`
}

// OutstandingBillSummary mirrors OutstandingSummary for bills.
type OutstandingBillSummary struct {
	Total      int64             `json:"total"`
	Current    int64             `json:"current"`
	Days1To30  int64             `json:"days1To30"`
	Days31To60 int64             `json:"days31To60"`
	Days61To90 int64             `json:"days61To90"`
	Days90Plus int64             `json:"days90Plus"`
	Bills      []OutstandingBill `json:"bills"`
}

// GetPayableAging is the AP aging report (Phase 4) — getOutstandingInvoices'
// purchases counterpart. Filter choices mirror it exactly: state == 'approved'
// only (not 'paid', same manual-flag reasoning), remaining balance computed
// against real, non-voided payments. A bill bounced back to 'approved' from
// 'paid' with a real remaining balance (e.g. a reversed/bounced payment) is
// picked up here since it's back in scope; a bill still flagged 'paid' with
// an actual outstanding balance is not — same intentional gap as AR aging's
// 'sent'-only filter, not second-guessed here either.
func (d *Database) GetPayableAging(organizationID string) (OutstandingBillSummary, error) {
	bills := []OutstandingBill{}
	err := d.DB.Select(&bills, fmt.Sprintf(`
		SELECT id, number, vendorName, dueDate, currency,
		       CAST(ROUND(total - paid) AS INTEGER) AS foreignTotal,
		       CAST(ROUND((total - paid) * COALESCE(exchangeRate, 1)) AS INTEGER) AS total
		FROM (
			SELECT ii.id, ii.vendorInvoiceNumber AS number, v.name AS vendorName,
			       ii.dueDate, ii.currency, ii.exchangeRate, ii.total,
			       %s AS paid
			FROM incoming_invoices ii
			JOIN vendors v ON ii.vendorId = v.id
			WHERE ii.organizationId = ? AND ii.state = 'approved'
		)
		WHERE (total - paid) > 0
		ORDER BY dueDate ASC`,
		documentPaidAmountExpr("ii", "incoming_invoice", paymentsNonVoided),
	),
		organizationID,
	)
	if err != nil {
		return OutstandingBillSummary{}, fmt.Errorf("get_payable_aging: %w", err)
	}
	return bucketOutstandingBills(bills, time.Now()), nil
}

// agingBucketFor classifies a single outstanding amount by days overdue
// against its due date — the one piece of bucketing logic bucketOutstanding
// and bucketOutstandingBills actually share.
func agingBucketFor(dueDate *int64, nowMillis int64) (bucket string, daysOverdue int) {
	if dueDate == nil || *dueDate >= nowMillis {
		return "current", 0
	}
	daysOverdue = int((nowMillis - *dueDate) / 86400000)
	switch {
	case daysOverdue <= 30:
		return "days1To30", daysOverdue
	case daysOverdue <= 60:
		return "days31To60", daysOverdue
	case daysOverdue <= 90:
		return "days61To90", daysOverdue
	default:
		return "days90Plus", daysOverdue
	}
}

// bucketOutstanding computes each invoice's days-overdue against now and
// rolls the results into aging buckets. Pure function — no DB, no clock
// dependency baked into the query — so it's unit-testable at exact bucket
// boundaries without a database.
func bucketOutstanding(invoices []OutstandingInvoice, now time.Time) OutstandingSummary {
	summary := OutstandingSummary{Invoices: invoices}
	nowMillis := now.UnixMilli()
	for i := range invoices {
		inv := &invoices[i]
		summary.Total += inv.Total

		bucket, daysOverdue := agingBucketFor(inv.DueDate, nowMillis)
		inv.DaysOverdue = daysOverdue
		switch bucket {
		case "current":
			summary.Current += inv.Total
		case "days1To30":
			summary.Days1To30 += inv.Total
		case "days31To60":
			summary.Days31To60 += inv.Total
		case "days61To90":
			summary.Days61To90 += inv.Total
		default:
			summary.Days90Plus += inv.Total
		}
	}
	return summary
}

// bucketOutstandingBills is bucketOutstanding's purchases counterpart.
func bucketOutstandingBills(bills []OutstandingBill, now time.Time) OutstandingBillSummary {
	summary := OutstandingBillSummary{Bills: bills}
	nowMillis := now.UnixMilli()
	for i := range bills {
		bill := &bills[i]
		summary.Total += bill.Total

		bucket, daysOverdue := agingBucketFor(bill.DueDate, nowMillis)
		bill.DaysOverdue = daysOverdue
		switch bucket {
		case "current":
			summary.Current += bill.Total
		case "days1To30":
			summary.Days1To30 += bill.Total
		case "days31To60":
			summary.Days31To60 += bill.Total
		case "days61To90":
			summary.Days61To90 += bill.Total
		default:
			summary.Days90Plus += bill.Total
		}
	}
	return summary
}

func (d *Database) getStockValuation(organizationID string) (StockValuation, error) {
	items := []StockValuationItem{}
	err := d.DB.Select(&items, `
		SELECT id, name, stockQuantity, CAST(ROUND(stockQuantity * COALESCE(unitCost, 0)) AS INTEGER) AS value
		FROM products
		WHERE organizationId = ? AND stockEnabled = 1
		ORDER BY value DESC`,
		organizationID,
	)
	if err != nil {
		return StockValuation{}, fmt.Errorf("get_stock_valuation: %w", err)
	}

	var total int64
	for _, item := range items {
		total += item.Value
	}
	if len(items) > topN {
		items = items[:topN]
	}
	return StockValuation{Total: total, Items: items}, nil
}
