package db

import (
	"fmt"
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
// from a loan.
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
// for, and how much of it has been paid. Payments are recorded at the
// invoice level, so a line's Paid is the invoice's paid amount allocated
// proportionally by net line amount — with the rounding remainder landing
// on the invoice's last line, so the line amounts always sum back to the
// invoice total.
type LoanStatusRow struct {
	LineID      string  `db:"lineId"      json:"lineId"`
	InvoiceID   string  `db:"invoiceId"   json:"invoiceId"`
	ClientID    string  `db:"clientId"    json:"clientId"`
	ClientName  string  `db:"clientName"  json:"clientName"`
	Date        int64   `db:"date"        json:"date"`
	ProductName string  `db:"productName" json:"productName"`
	Sku         string  `db:"sku"         json:"sku"`
	Quantity    float64 `db:"quantity"    json:"quantity"`
	Amount      int64   `db:"amount"      json:"amount"`
	Paid        int64   `db:"paid"        json:"paid"`
	Outstanding int64   `db:"outstanding" json:"outstanding"`
}

// GetLoanStatus is the Cash Book screen's embedded loan tracker — org-wide
// by default, or scoped to one customer via clientID (empty means every
// customer). See LoanStatusRow's doc comment for the "was ever a loan"
// filter and why a settled loan still appears, and for how each invoice's
// payments are split across its lines. Sorted outstanding-first so the
// customers who actually owe money surface before ones who've settled up.
func (d *Database) GetLoanStatus(organizationID, clientID string) ([]LoanStatusRow, error) {
	query := fmt.Sprintf(`
		SELECT lineId, invoiceId, clientId, clientName, docDate,
		       invoiceTotal, invoicePaid, productName, sku, quantity, netLine
		FROM (
			SELECT li.id AS lineId, i.id AS invoiceId, i.clientId AS clientId,
			       c.name AS clientName, i.date AS docDate,
			       i.total AS invoiceTotal,
			       %s AS invoicePaid,
			       %s AS appCount,
			       COALESCE(pr.name, li.description, '') AS productName,
			       COALESCE(pr.sku, '') AS sku,
			       li.quantity AS quantity,
			       CAST(ROUND(li.unitPrice * li.quantity) AS INTEGER) AS netLine
			FROM invoices i
			JOIN clients c ON i.clientId = c.id
			JOIN invoiceLineItems li ON li.invoiceId = i.id
			LEFT JOIN products pr ON li.productId = pr.id
			WHERE i.organizationId = ? AND i.state IN ('sent', 'paid')
		)
		WHERE (appCount = 0 OR appCount > 1 OR invoicePaid < invoiceTotal)`,
		documentPaidAmountExpr("i", "invoice", paymentsPosted),
		documentPaymentAppCountExpr("i", "invoice", paymentsPosted),
	)
	args := []any{organizationID}
	if clientID != "" {
		query += " AND clientId = ?"
		args = append(args, clientID)
	}
	query += " ORDER BY docDate ASC, invoiceId ASC, netLine DESC, lineId ASC"

	raw := []struct {
		LineID       string  `db:"lineId"`
		InvoiceID    string  `db:"invoiceId"`
		ClientID     string  `db:"clientId"`
		ClientName   string  `db:"clientName"`
		Date         int64   `db:"docDate"`
		InvoiceTotal int64   `db:"invoiceTotal"`
		InvoicePaid  int64   `db:"invoicePaid"`
		ProductName  string  `db:"productName"`
		Sku          string  `db:"sku"`
		Quantity     float64 `db:"quantity"`
		NetLine      int64   `db:"netLine"`
	}{}
	if err := d.DB.Select(&raw, query, args...); err != nil {
		return nil, fmt.Errorf("get_loan_status: %w", err)
	}

	// Allocate each invoice's total and paid amount across its lines by net
	// line weight. The rows are contiguous per invoice (ORDER BY invoiceId),
	// so a single forward pass groups them; the last line of each invoice
	// absorbs the rounding remainder so the parts always sum to the whole.
	rows := make([]LoanStatusRow, 0, len(raw))
	for i := 0; i < len(raw); {
		j := i
		var netSum int64
		for j < len(raw) && raw[j].InvoiceID == raw[i].InvoiceID {
			netSum += raw[j].NetLine
			j++
		}
		if netSum <= 0 {
			// All-zero (e.g. a free line) — split evenly rather than divide by zero.
			netSum = int64(j - i)
		}
		var allocatedAmount, allocatedPaid int64
		for k := i; k < j; k++ {
			r := raw[k]
			weight := r.NetLine
			if weight <= 0 {
				weight = 1
			}
			var amount, paid int64
			if k == j-1 {
				amount = r.InvoiceTotal - allocatedAmount
				paid = r.InvoicePaid - allocatedPaid
			} else {
				amount = r.InvoiceTotal * weight / netSum
				paid = r.InvoicePaid * weight / netSum
			}
			allocatedAmount += amount
			allocatedPaid += paid
			rows = append(rows, LoanStatusRow{
				LineID: r.LineID, InvoiceID: r.InvoiceID, ClientID: r.ClientID,
				ClientName: r.ClientName, Date: r.Date,
				ProductName: r.ProductName, Sku: r.Sku, Quantity: r.Quantity,
				Amount: amount, Paid: paid, Outstanding: amount - paid,
			})
		}
		i = j
	}

	sort.SliceStable(rows, func(a, b int) bool {
		if rows[a].Outstanding != rows[b].Outstanding {
			return rows[a].Outstanding > rows[b].Outstanding
		}
		return rows[a].Date < rows[b].Date
	})
	return rows, nil
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
