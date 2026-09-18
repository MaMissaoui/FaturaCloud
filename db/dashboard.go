package db

import (
	"fmt"
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
	err := d.DB.Select(&invoices, `
		SELECT i.id, i.number, c.name AS clientName, i.dueDate, i.currency,
		       CAST(ROUND(i.total - COALESCE(paid.amount, 0)) AS INTEGER) AS foreignTotal,
		       CAST(ROUND((i.total - COALESCE(paid.amount, 0)) * COALESCE(i.exchangeRate, 1)) AS INTEGER) AS total
		FROM invoices i
		JOIN clients c ON i.clientId = c.id
		LEFT JOIN (
			SELECT pa.documentId, SUM(pa.amount) AS amount
			FROM payment_applications pa
			JOIN payments p ON p.id = pa.paymentId
			WHERE pa.documentType = 'invoice' AND p.status != 'voided'
			GROUP BY pa.documentId
		) paid ON paid.documentId = i.id
		WHERE i.organizationId = ? AND i.state = 'sent'
		      AND (i.total - COALESCE(paid.amount, 0)) > 0
		ORDER BY i.dueDate ASC`,
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
	err := d.DB.Select(&invoices, `
		SELECT i.id, i.number, c.name AS clientName, i.dueDate, i.currency,
		       CAST(ROUND(i.total - COALESCE(paid.amount, 0)) AS INTEGER) AS foreignTotal,
		       CAST(ROUND((i.total - COALESCE(paid.amount, 0)) * COALESCE(i.exchangeRate, 1)) AS INTEGER) AS total
		FROM invoices i
		JOIN clients c ON i.clientId = c.id
		LEFT JOIN (
			SELECT pa.documentId, SUM(pa.amount) AS amount
			FROM payment_applications pa
			JOIN payments p ON p.id = pa.paymentId
			WHERE pa.documentType = 'invoice' AND p.status != 'voided'
			GROUP BY pa.documentId
		) paid ON paid.documentId = i.id
		WHERE i.clientId = ? AND i.state IN ('sent', 'paid')
		      AND (i.total - COALESCE(paid.amount, 0)) > 0
		ORDER BY i.date ASC`,
		clientID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_client_open_invoices: %w", err)
	}
	return invoices, nil
}

// LoanStatusRow is one invoice's loan lifecycle: what it was originally
// billed for, what's been collected against it so far (every posted
// payment, not just the first), and what's still outstanding. Unlike
// getOutstandingInvoices/GetClientOpenInvoices, this does NOT filter to a
// currently-nonzero balance — a loan that's since been fully repaid still
// belongs here with Outstanding: 0, since selecting a specific customer
// who has settled up should show that, not render a table indistinguishable
// from "no data for this customer at all".
//
// The actual filter is "was ever a loan": excludes a pure cash sale — an
// invoice with exactly one payment application, for the full total, and
// nothing since — the same first-payment-application signal
// GetCashMovementDetails uses to tell a "sale" movement from a "loan"
// one. Everything else (no payment yet, a partial first payment, or more
// than one payment) is a loan by construction, whatever its current
// balance.
type LoanStatusRow struct {
	InvoiceID   string `db:"id"          json:"invoiceId"`
	Number      string `db:"number"      json:"number"`
	ClientID    string `db:"clientId"    json:"clientId"`
	ClientName  string `db:"clientName"  json:"clientName"`
	Date        int64  `db:"date"        json:"date"`
	Original    int64  `db:"original"    json:"original"`
	Paid        int64  `db:"paid"        json:"paid"`
	Outstanding int64  `db:"outstanding" json:"outstanding"`
}

// GetLoanStatus is the Cash Book screen's embedded loan tracker — org-wide
// by default, or scoped to one customer via clientID (empty means every
// customer). See LoanStatusRow's doc comment for the "was ever a loan"
// filter and why a settled loan still appears. Sorted outstanding-first so
// the customers who actually owe money surface before ones who've settled
// up, which the date-only ordering every other report here uses would bury.
func (d *Database) GetLoanStatus(organizationID, clientID string) ([]LoanStatusRow, error) {
	query := `
		SELECT i.id, i.number, i.clientId, c.name AS clientName, i.date,
		       i.total AS original,
		       COALESCE(paid.amount, 0) AS paid,
		       CAST(ROUND(i.total - COALESCE(paid.amount, 0)) AS INTEGER) AS outstanding
		FROM invoices i
		JOIN clients c ON i.clientId = c.id
		LEFT JOIN (
			SELECT pa.documentId, SUM(pa.amount) AS amount, COUNT(*) AS appCount
			FROM payment_applications pa
			JOIN payments p ON p.id = pa.paymentId
			WHERE pa.documentType = 'invoice' AND p.status = 'posted'
			GROUP BY pa.documentId
		) paid ON paid.documentId = i.id
		WHERE i.organizationId = ? AND i.state IN ('sent', 'paid')
		      AND (
		          COALESCE(paid.appCount, 0) = 0
		          OR paid.appCount > 1
		          OR paid.amount < i.total
		      )`
	args := []any{organizationID}
	if clientID != "" {
		query += " AND i.clientId = ?"
		args = append(args, clientID)
	}
	query += " ORDER BY outstanding DESC, i.date ASC"

	rows := []LoanStatusRow{}
	if err := d.DB.Select(&rows, query, args...); err != nil {
		return nil, fmt.Errorf("get_loan_status: %w", err)
	}
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
	err := d.DB.Select(&bills, `
		SELECT ii.id, ii.vendorInvoiceNumber AS number, v.name AS vendorName, ii.dueDate, ii.currency,
		       CAST(ROUND(ii.total - COALESCE(paid.amount, 0)) AS INTEGER) AS foreignTotal,
		       CAST(ROUND((ii.total - COALESCE(paid.amount, 0)) * COALESCE(ii.exchangeRate, 1)) AS INTEGER) AS total
		FROM incoming_invoices ii
		JOIN vendors v ON ii.vendorId = v.id
		LEFT JOIN (
			SELECT pa.documentId, SUM(pa.amount) AS amount
			FROM payment_applications pa
			JOIN payments p ON p.id = pa.paymentId
			WHERE pa.documentType = 'incoming_invoice' AND p.status != 'voided'
			GROUP BY pa.documentId
		) paid ON paid.documentId = ii.id
		WHERE ii.organizationId = ? AND ii.state = 'approved'
		      AND (ii.total - COALESCE(paid.amount, 0)) > 0
		ORDER BY ii.dueDate ASC`,
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
