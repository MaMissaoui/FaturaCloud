package db

import (
	"fmt"
	"time"
)

// This file holds the "Reporting" tier of analytics: sales/purchasing
// reports computed directly from source documents (invoices,
// incoming_invoices, their line items) rather than from journal_lines. This
// is a deliberately different tier from db/gl_reports.go's GL-derived
// statutory reports (trial balance, P&L, balance sheet) — a document is
// counted here the moment it's sent/approved, long before (or in some cases
// instead of) whatever the GL recognizes at posting time, so numbers on this
// tier are not expected to reconcile against db/gl_reports.go's without the
// explanation in this comment. See CLAUDE.md's file-index entry for the
// tier split rationale and api/reporting.go's separate /reporting/ URL
// prefix, which exists specifically to keep that distinction visible.
//
// "Revenue" (sales side) is invoices in state sent or paid — draft isn't
// issued yet, cancelled is voided. "Committed spend" (purchases side) is the
// analogous incoming_invoices state set, approved or paid — draft isn't
// approved yet, cancelled is voided.
const revenueStates = `('sent', 'paid')`
const committedStates = `('approved', 'paid')`

// Every SUM/aggregate below multiplies by COALESCE(exchangeRate, 1) before
// summing, for the same reason db/dashboard.go's top-of-file comment
// explains: documents can each be in a different currency, but every number
// this package returns is rendered through the organization's own currency
// formatter, never the document's.

// dateRangeFilter appends "AND <column> >= ?" / "AND <column> <= ?" clauses
// (and their args, in order) only for the bounds actually supplied — 0 means
// unbounded on that side, not epoch. This is what lets GetDashboardData call
// through with endDate=0 and get the same no-upper-bound behavior the
// dashboard has always had.
func dateRangeFilter(column string, startDate, endDate int64, args *[]any) string {
	var clause string
	if startDate > 0 {
		clause += " AND " + column + " >= ?"
		*args = append(*args, startDate)
	}
	if endDate > 0 {
		clause += " AND " + column + " <= ?"
		*args = append(*args, endDate)
	}
	return clause
}

type MonthlyRevenue struct {
	Month   string `db:"month"   json:"month"`
	Revenue int64  `db:"revenue" json:"revenue"`
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

type VendorSpend struct {
	VendorID string `db:"vendorId" json:"vendorId"`
	Name     string `db:"name"     json:"name"`
	Spend    int64  `db:"spend"    json:"spend"`
}

// GetRevenueByMonth is the Revenue Trend report — and, with startDate
// computed from a rolling window and endDate left at 0, the dashboard's
// "revenue over time" widget. Grouped by calendar month, ordered oldest
// first.
func (d *Database) GetRevenueByMonth(organizationID string, startDate, endDate int64) ([]MonthlyRevenue, error) {
	args := []any{organizationID}
	rangeClause := dateRangeFilter("date", startDate, endDate, &args)
	rows := []MonthlyRevenue{}
	err := d.DB.Select(&rows, `
		SELECT strftime('%Y-%m', date / 1000, 'unixepoch') AS month,
		       CAST(ROUND(SUM(total * COALESCE(exchangeRate, 1))) AS INTEGER) AS revenue
		FROM invoices
		WHERE organizationId = ? AND state IN `+revenueStates+rangeClause+`
		GROUP BY month
		ORDER BY month ASC`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("get_revenue_by_month: %w", err)
	}
	return rows, nil
}

// GetSalesByClient is the Sales by Client report — and, with startDate
// computed from a rolling window, endDate left at 0, and limit=topN, the
// dashboard's "top clients" widget. limit <= 0 returns the full ranked list
// (no LIMIT clause), which is the report's actual point: the dashboard
// widget's top-10 cap is a display choice, not a query limitation.
func (d *Database) GetSalesByClient(organizationID string, startDate, endDate int64, limit int) ([]ClientRevenue, error) {
	args := []any{organizationID}
	rangeClause := dateRangeFilter("i.date", startDate, endDate, &args)
	query := `
		SELECT i.clientId AS clientId, COALESCE(c.name, '') AS name,
		       CAST(ROUND(SUM(i.total * COALESCE(i.exchangeRate, 1))) AS INTEGER) AS revenue
		FROM invoices i
		JOIN clients c ON i.clientId = c.id
		WHERE i.organizationId = ? AND i.state IN ` + revenueStates + rangeClause + `
		GROUP BY i.clientId
		ORDER BY revenue DESC`
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}
	rows := []ClientRevenue{}
	if err := d.DB.Select(&rows, query, args...); err != nil {
		return nil, fmt.Errorf("get_sales_by_client: %w", err)
	}
	return rows, nil
}

// GetSalesByProduct is GetSalesByClient's per-product counterpart, over
// invoiceLineItems. A line item with no productId (e.g. a free-text service
// line) is excluded, same as before.
func (d *Database) GetSalesByProduct(organizationID string, startDate, endDate int64, limit int) ([]ProductRevenue, error) {
	args := []any{organizationID}
	rangeClause := dateRangeFilter("i.date", startDate, endDate, &args)
	query := `
		SELECT ili.productId AS productId, COALESCE(p.name, '') AS name,
		       CAST(ROUND(SUM(ili.quantity * ili.unitPrice * COALESCE(i.exchangeRate, 1))) AS INTEGER) AS revenue
		FROM invoiceLineItems ili
		JOIN invoices i ON ili.invoiceId = i.id
		JOIN products p ON ili.productId = p.id
		WHERE i.organizationId = ? AND i.state IN ` + revenueStates + rangeClause + `
		      AND ili.productId IS NOT NULL
		GROUP BY ili.productId
		ORDER BY revenue DESC`
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}
	rows := []ProductRevenue{}
	if err := d.DB.Select(&rows, query, args...); err != nil {
		return nil, fmt.Errorf("get_sales_by_product: %w", err)
	}
	return rows, nil
}

// GetPurchasesByVendor is GetSalesByClient's purchasing-side counterpart,
// over incoming_invoices/vendors, filtered by committedStates.
func (d *Database) GetPurchasesByVendor(organizationID string, startDate, endDate int64, limit int) ([]VendorSpend, error) {
	args := []any{organizationID}
	rangeClause := dateRangeFilter("ii.date", startDate, endDate, &args)
	query := `
		SELECT ii.vendorId AS vendorId, COALESCE(v.name, '') AS name,
		       CAST(ROUND(SUM(ii.total * COALESCE(ii.exchangeRate, 1))) AS INTEGER) AS spend
		FROM incoming_invoices ii
		JOIN vendors v ON ii.vendorId = v.id
		WHERE ii.organizationId = ? AND ii.state IN ` + committedStates + rangeClause + `
		GROUP BY ii.vendorId
		ORDER BY spend DESC`
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}
	rows := []VendorSpend{}
	if err := d.DB.Select(&rows, query, args...); err != nil {
		return nil, fmt.Errorf("get_purchases_by_vendor: %w", err)
	}
	return rows, nil
}

// TaxSummaryLine is one tax-rate bucket of the Tax Summary report. TaxRateID
// "" (and Name/CategoryCode "") is the "Unrated" bucket — line items with no
// taxRate set — kept as its own row rather than dropped, so Base sums back
// to the document's subTotal.
type TaxSummaryLine struct {
	TaxRateID    string  `db:"taxRateId"    json:"taxRateId"`
	Name         string  `db:"name"         json:"name"`
	CategoryCode string  `db:"categoryCode" json:"categoryCode"`
	Percentage   float64 `db:"percentage"   json:"percentage"`
	Base         int64   `db:"base"         json:"base"`
	Tax          int64   `db:"tax"          json:"tax"`
}

// TaxSummary is the Tax Summary report: output VAT (sales side, what the
// organization collected) and input VAT (purchases side, what it paid),
// each grouped by tax rate over a date range.
//
// This deliberately diverges from db/gl_posting.go's buildInvoiceGLLines/
// buildIncomingInvoiceGLLines in two ways:
//
//  1. The GL skips a tax group that rounds to exactly 0 — journal_lines'
//     CHECK forbids a zero-amount row. This report must NOT skip a 0%-rate
//     group: zero-rated/exempt turnover (category codes Z/E/AE/O) is still
//     reportable, so it appears here with Tax=0 and a real Base.
//  2. Exact-cent agreement with journal_lines' posted tax amounts isn't
//     guaranteed for foreign-currency documents — the GL's rounding-residual
//     absorption can shift a cent onto the AR/AP line rather than the tax
//     line, which this report doesn't replicate. A tiny (single-cent)
//     mismatch against the GL for such a document is expected and benign,
//     not a bug to chase.
//
// There is no discount column on invoiceLineItems/incoming_invoice_line_items,
// so quantity*unitPrice is the correct taxable base — the same base the GL
// itself computes.
type TaxSummary struct {
	Output []TaxSummaryLine `json:"output"` // sales / output VAT
	Input  []TaxSummaryLine `json:"input"`  // purchases / input VAT
}

func (d *Database) GetTaxSummary(organizationID string, startDate, endDate int64) (*TaxSummary, error) {
	output, err := d.getOutputTaxSummary(organizationID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	input, err := d.getInputTaxSummary(organizationID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	return &TaxSummary{Output: output, Input: input}, nil
}

func (d *Database) getOutputTaxSummary(organizationID string, startDate, endDate int64) ([]TaxSummaryLine, error) {
	args := []any{organizationID}
	rangeClause := dateRangeFilter("i.date", startDate, endDate, &args)
	rows := []TaxSummaryLine{}
	err := d.DB.Select(&rows, `
		SELECT COALESCE(tr.id, '') AS taxRateId, COALESCE(tr.name, '') AS name,
		       COALESCE(tr.category_code, '') AS categoryCode, COALESCE(tr.percentage, 0) AS percentage,
		       CAST(ROUND(SUM(ili.quantity * ili.unitPrice * COALESCE(i.exchangeRate, 1))) AS INTEGER) AS base,
		       CAST(ROUND(SUM(ili.quantity * ili.unitPrice * COALESCE(tr.percentage, 0) / 100.0 * COALESCE(i.exchangeRate, 1))) AS INTEGER) AS tax
		FROM invoiceLineItems ili
		JOIN invoices i ON ili.invoiceId = i.id
		LEFT JOIN taxRates tr ON ili.taxRate = tr.id
		WHERE i.organizationId = ? AND i.state IN `+revenueStates+rangeClause+`
		GROUP BY tr.id
		ORDER BY tr.percentage DESC`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("get_output_tax_summary: %w", err)
	}
	return rows, nil
}

func (d *Database) getInputTaxSummary(organizationID string, startDate, endDate int64) ([]TaxSummaryLine, error) {
	args := []any{organizationID}
	rangeClause := dateRangeFilter("ii.date", startDate, endDate, &args)
	rows := []TaxSummaryLine{}
	err := d.DB.Select(&rows, `
		SELECT COALESCE(tr.id, '') AS taxRateId, COALESCE(tr.name, '') AS name,
		       COALESCE(tr.category_code, '') AS categoryCode, COALESCE(tr.percentage, 0) AS percentage,
		       CAST(ROUND(SUM(iili.quantity * iili.unitPrice * COALESCE(ii.exchangeRate, 1))) AS INTEGER) AS base,
		       CAST(ROUND(SUM(iili.quantity * iili.unitPrice * COALESCE(tr.percentage, 0) / 100.0 * COALESCE(ii.exchangeRate, 1))) AS INTEGER) AS tax
		FROM incoming_invoice_line_items iili
		JOIN incoming_invoices ii ON iili.incomingInvoiceId = ii.id
		LEFT JOIN taxRates tr ON iili.taxRate = tr.id
		WHERE ii.organizationId = ? AND ii.state IN `+committedStates+rangeClause+`
		GROUP BY tr.id
		ORDER BY tr.percentage DESC`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("get_input_tax_summary: %w", err)
	}
	return rows, nil
}

// dashboardCutoff turns a rolling "months" window into a startDate for the
// range-based functions above — the shape GetDashboardData's `months`
// parameter has always had, kept here so it isn't duplicated at each call
// site.
func dashboardCutoff(months int) int64 {
	return time.Now().AddDate(0, -months, 0).UnixMilli()
}
