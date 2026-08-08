package db

import (
	"fmt"
	"time"
)

// "Revenue" is defined consistently across every metric on this page as
// invoices in state sent or paid — draft isn't issued yet, cancelled is
// voided. Neither counts.
const revenueStates = `('sent', 'paid')`

// Every SUM/aggregate below multiplies by COALESCE(exchangeRate, 1) before
// summing: invoices can each be in a different currency (invoices.currency),
// but this whole page — every stat tile, every table cell — renders every
// number through the organization's own currency formatter
// (src/routes/dashboard.tsx's `money`), never the invoice's. Without the
// conversion, summing raw cents across currencies would silently produce a
// number in no currency at all. See db/exchange_rate.go for the
// exchangeRate direction/storage convention this relies on.

type MonthlyRevenue struct {
	Month   string `db:"month"   json:"month"`
	Revenue int64  `db:"revenue" json:"revenue"`
}

type OutstandingInvoice struct {
	ID          string `db:"id"         json:"id"`
	Number      string `db:"number"     json:"number"`
	ClientName  string `db:"clientName" json:"clientName"`
	DueDate     *int64 `db:"dueDate"    json:"dueDate"`
	Total       int64  `db:"total"      json:"total"`
	DaysOverdue int    `json:"daysOverdue"`
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

type ClientRevenue struct {
	ClientID string `db:"clientId" json:"clientId"`
	Name     string `db:"name"     json:"name"`
	Revenue  int64  `db:"revenue"  json:"revenue"`
}

type ProductRevenue struct {
	ProductID string `db:"productId" json:"productId"`
	Name      string `db:"name"      json:"name"`
	Revenue   int64  `db:"revenue"   json:"revenue"`
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

func (d *Database) GetDashboardData(organizationID string, months int) (DashboardData, error) {
	revenueByMonth, err := d.getRevenueByMonth(organizationID, months)
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
	topClients, err := d.getTopClients(organizationID, months, topN)
	if err != nil {
		return DashboardData{}, err
	}
	topProducts, err := d.getTopProducts(organizationID, months, topN)
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

func (d *Database) getRevenueByMonth(organizationID string, months int) ([]MonthlyRevenue, error) {
	cutoff := time.Now().AddDate(0, -months, 0).UnixMilli()
	rows := []MonthlyRevenue{}
	err := d.DB.Select(&rows, `
		SELECT strftime('%Y-%m', date / 1000, 'unixepoch') AS month,
		       CAST(ROUND(SUM(total * COALESCE(exchangeRate, 1))) AS INTEGER) AS revenue
		FROM invoices
		WHERE organizationId = ? AND state IN `+revenueStates+` AND date >= ?
		GROUP BY month
		ORDER BY month ASC`,
		organizationID, cutoff,
	)
	if err != nil {
		return nil, fmt.Errorf("get_revenue_by_month: %w", err)
	}
	return rows, nil
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
		SELECT i.id, i.number, c.name AS clientName, i.dueDate,
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

// OutstandingBill is getOutstandingInvoices' purchases counterpart.
type OutstandingBill struct {
	ID          string `db:"id"         json:"id"`
	Number      string `db:"number"     json:"number"`
	VendorName  string `db:"vendorName" json:"vendorName"`
	DueDate     *int64 `db:"dueDate"    json:"dueDate"`
	Total       int64  `db:"total"      json:"total"`
	DaysOverdue int    `json:"daysOverdue"`
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
// against real, non-voided payments.
func (d *Database) GetPayableAging(organizationID string) (OutstandingBillSummary, error) {
	bills := []OutstandingBill{}
	err := d.DB.Select(&bills, `
		SELECT ii.id, ii.vendorInvoiceNumber AS number, v.name AS vendorName, ii.dueDate,
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

func (d *Database) getTopClients(organizationID string, months, limit int) ([]ClientRevenue, error) {
	cutoff := time.Now().AddDate(0, -months, 0).UnixMilli()
	rows := []ClientRevenue{}
	err := d.DB.Select(&rows, `
		SELECT i.clientId AS clientId, c.name AS name,
		       CAST(ROUND(SUM(i.total * COALESCE(i.exchangeRate, 1))) AS INTEGER) AS revenue
		FROM invoices i
		JOIN clients c ON i.clientId = c.id
		WHERE i.organizationId = ? AND i.state IN `+revenueStates+` AND i.date >= ?
		GROUP BY i.clientId
		ORDER BY revenue DESC
		LIMIT ?`,
		organizationID, cutoff, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("get_top_clients: %w", err)
	}
	return rows, nil
}

func (d *Database) getTopProducts(organizationID string, months, limit int) ([]ProductRevenue, error) {
	cutoff := time.Now().AddDate(0, -months, 0).UnixMilli()
	rows := []ProductRevenue{}
	err := d.DB.Select(&rows, `
		SELECT ili.productId AS productId, p.name AS name,
		       CAST(ROUND(SUM(ili.quantity * ili.unitPrice * COALESCE(i.exchangeRate, 1))) AS INTEGER) AS revenue
		FROM invoiceLineItems ili
		JOIN invoices i ON ili.invoiceId = i.id
		JOIN products p ON ili.productId = p.id
		WHERE i.organizationId = ? AND i.state IN `+revenueStates+` AND i.date >= ?
		      AND ili.productId IS NOT NULL
		GROUP BY ili.productId
		ORDER BY revenue DESC
		LIMIT ?`,
		organizationID, cutoff, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("get_top_products: %w", err)
	}
	return rows, nil
}
