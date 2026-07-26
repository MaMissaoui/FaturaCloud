package db

import (
	"fmt"
	"time"
)

// "Revenue" is defined consistently across every metric on this page as
// invoices in state sent or paid — draft isn't issued yet, cancelled is
// voided. Neither counts.
const revenueStates = `('sent', 'paid')`

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
		SELECT strftime('%Y-%m', date / 1000, 'unixepoch') AS month, SUM(total) AS revenue
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

func (d *Database) getOutstandingInvoices(organizationID string) (OutstandingSummary, error) {
	invoices := []OutstandingInvoice{}
	err := d.DB.Select(&invoices, `
		SELECT i.id, i.number, c.name AS clientName, i.dueDate, i.total
		FROM invoices i
		JOIN clients c ON i.clientId = c.id
		WHERE i.organizationId = ? AND i.state = 'sent'
		ORDER BY i.dueDate ASC`,
		organizationID,
	)
	if err != nil {
		return OutstandingSummary{}, fmt.Errorf("get_outstanding_invoices: %w", err)
	}
	return bucketOutstanding(invoices, time.Now()), nil
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

		if inv.DueDate == nil || *inv.DueDate >= nowMillis {
			inv.DaysOverdue = 0
			summary.Current += inv.Total
			continue
		}

		daysOverdue := int((nowMillis - *inv.DueDate) / 86400000)
		inv.DaysOverdue = daysOverdue

		switch {
		case daysOverdue <= 30:
			summary.Days1To30 += inv.Total
		case daysOverdue <= 60:
			summary.Days31To60 += inv.Total
		case daysOverdue <= 90:
			summary.Days61To90 += inv.Total
		default:
			summary.Days90Plus += inv.Total
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
		SELECT i.clientId AS clientId, c.name AS name, SUM(i.total) AS revenue
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
		       CAST(ROUND(SUM(ili.quantity * ili.unitPrice)) AS INTEGER) AS revenue
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
