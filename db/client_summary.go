package db

import (
	"database/sql"
	"fmt"
	"time"
)

// ClientSummaryList is the batch overview for the Clients screen.
type ClientSummaryList struct {
	TotalOwed  int64              `json:"totalOwed"`
	OwingCount int                `json:"owingCount"`
	Clients    []ClientSummaryRow `json:"clients"`
}

// ClientSummaryRow is one client's row in the list view.
type ClientSummaryRow struct {
	ClientID     string `json:"clientId"`
	Owed         int64  `json:"owed"`
	LastPurchase *int64 `json:"lastPurchase"`
	InvoiceCount int    `json:"invoiceCount"`
}

// ClientSummary is the single-client detail view.
type ClientSummary struct {
	ClientID string `json:"clientId"`
	Owed     int64  `json:"owed"`
	// Aging buckets, same boundaries as bucketOutstanding / agingBucketFor.
	Current    int64 `json:"current"`
	Days1To30  int64 `json:"days1To30"`
	Days31To60 int64 `json:"days31To60"`
	Days61To90 int64 `json:"days61To90"`
	Days90Plus int64 `json:"days90Plus"`
	// OpenInvoices are this client's outstanding invoices, oldest due date
	// first, with DaysOverdue/Bucket filled exactly as bucketOutstanding does.
	OpenInvoices  []OutstandingInvoice `json:"openInvoices"`
	InvoiceCount  int                  `json:"invoiceCount"`
	FirstPurchase *int64               `json:"firstPurchase"`
	LastPurchase  *int64               `json:"lastPurchase"`
	BilledTotal   int64                `json:"billedTotal"`
	PaidTotal     int64                `json:"paidTotal"`
	PaymentCount  int                  `json:"paymentCount"`
	LastPayment   *ClientLastPayment   `json:"lastPayment"`
}

// ClientLastPayment is the most recent posted, non-voided inbound payment.
type ClientLastPayment struct {
	Date   int64  `json:"date"`
	Amount int64  `json:"amount"`
	Method string `json:"method"`
}

// errSummariesSwitchedOff is returned when masterDataSummaries is off.
var errSummariesSwitchedOff = newValidationError("Summaries are switched off for this organization")

func (d *Database) GetClientSummaries(organizationID string) (ClientSummaryList, error) {
	// Check masterDataSummaries setting
	org, err := d.GetOrganization(organizationID)
	if err != nil {
		return ClientSummaryList{}, err
	}
	if !org.MasterDataSummaries {
		return ClientSummaryList{}, errSummariesSwitchedOff
	}

	// Get outstanding invoices grouped by client — same query the dashboard
	// and the receivable-aging report use (selectOutstandingInvoices).
	invoices, err := d.selectOutstandingInvoices(organizationID, "")
	if err != nil {
		return ClientSummaryList{}, fmt.Errorf("get_client_summaries: %w", err)
	}

	// Aggregate by clientId
	type clientOwed struct {
		owed int64
	}
	clientMap := map[string]*clientOwed{}
	for _, inv := range invoices {
		if _, ok := clientMap[inv.ClientID]; !ok {
			clientMap[inv.ClientID] = &clientOwed{}
		}
		clientMap[inv.ClientID].owed += inv.Total
	}

	// Get lastPurchase and invoiceCount per client (state IN ('sent','paid'))
	type clientInvoiceInfo struct {
		ClientID     string `db:"clientId"`
		LastPurchase *int64 `db:"lastPurchase"`
		InvoiceCount int    `db:"invoiceCount"`
	}
	var invoiceInfos []clientInvoiceInfo
	err = d.DB.Select(&invoiceInfos, `
		SELECT clientId, MAX(date) AS lastPurchase, COUNT(*) AS invoiceCount
		FROM invoices
		WHERE organizationId = ? AND state IN ('sent', 'paid')
		GROUP BY clientId`,
		organizationID,
	)
	if err != nil {
		return ClientSummaryList{}, fmt.Errorf("get_client_summaries invoice info: %w", err)
	}

	// Merge into one list: clients with either a balance or at least one sent/paid invoice
	seen := map[string]bool{}
	rows := []ClientSummaryRow{}
	for _, info := range invoiceInfos {
		seen[info.ClientID] = true
		owed := int64(0)
		if co, ok := clientMap[info.ClientID]; ok {
			owed = co.owed
		}
		rows = append(rows, ClientSummaryRow{
			ClientID:     info.ClientID,
			Owed:         owed,
			LastPurchase: info.LastPurchase,
			InvoiceCount: info.InvoiceCount,
		})
	}
	// Add clients that have a balance but no sent/paid invoices (shouldn't normally happen but be safe)
	for cid, co := range clientMap {
		if !seen[cid] {
			rows = append(rows, ClientSummaryRow{
				ClientID: cid,
				Owed:     co.owed,
			})
		}
	}

	// Totals
	var totalOwed int64
	owingCount := 0
	for _, r := range rows {
		totalOwed += r.Owed
		if r.Owed > 0 {
			owingCount++
		}
	}

	return ClientSummaryList{
		TotalOwed:  totalOwed,
		OwingCount: owingCount,
		Clients:    rows,
	}, nil
}

func (d *Database) GetClientSummary(organizationID, clientID string) (*ClientSummary, error) {
	// Check masterDataSummaries setting
	org, err := d.GetOrganization(organizationID)
	if err != nil {
		return nil, err
	}
	if !org.MasterDataSummaries {
		return nil, errSummariesSwitchedOff
	}

	// Verify client belongs to this organization
	client, err := d.GetClient(clientID)
	if err != nil {
		return nil, err
	}
	if client.OrganizationID != organizationID {
		return nil, sql.ErrNoRows
	}

	// Get this client's outstanding invoices — same query the dashboard uses.
	invoices, err := d.selectOutstandingInvoices(organizationID, clientID)
	if err != nil {
		return nil, fmt.Errorf("get_client_summary: %w", err)
	}

	// Fill days overdue / bucket using the shared bucketOutstanding
	now := time.Now()
	summary := bucketOutstanding(invoices, now)

	// Invoice stats (state IN ('sent','paid'))
	type invoiceStats struct {
		InvoiceCount  int    `db:"invoiceCount"`
		FirstPurchase *int64 `db:"firstPurchase"`
		LastPurchase  *int64 `db:"lastPurchase"`
		BilledTotal   int64  `db:"billedTotal"`
	}
	var stats invoiceStats
	err = d.DB.Get(&stats, `
		SELECT COUNT(*) AS invoiceCount,
		       MIN(date) AS firstPurchase,
		       MAX(date) AS lastPurchase,
		       CAST(COALESCE(SUM(ROUND(total * COALESCE(exchangeRate, 1))), 0) AS INTEGER) AS billedTotal
		FROM invoices
		WHERE organizationId = ? AND clientId = ? AND state IN ('sent', 'paid')`,
		organizationID, clientID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_client_summary invoice stats: %w", err)
	}

	// Payment stats (posted, non-voided inbound payments of this client)
	type paymentStats struct {
		PaidTotal    int64 `db:"paidTotal"`
		PaymentCount int   `db:"paymentCount"`
	}
	var ps paymentStats
	err = d.DB.Get(&ps, fmt.Sprintf(`
		SELECT COALESCE(SUM(ROUND(p.amount * COALESCE(p.exchangeRate, 1))), 0) AS paidTotal,
		       COUNT(*) AS paymentCount
		FROM payments p
		WHERE p.organizationId = ? AND p.clientId = ? AND p.direction = 'inbound' AND %s`,
		paymentsNonVoided,
	),
		organizationID, clientID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_client_summary payment stats: %w", err)
	}

	// Last payment
	var lastPayment *ClientLastPayment
	var lp struct {
		Date   int64  `db:"date"`
		Amount int64  `db:"amount"`
		Method string `db:"method"`
	}
	err = d.DB.Get(&lp, fmt.Sprintf(`
		SELECT p.date, CAST(ROUND(p.amount * COALESCE(p.exchangeRate, 1)) AS INTEGER) AS amount, p.method
		FROM payments p
		WHERE p.organizationId = ? AND p.clientId = ? AND p.direction = 'inbound' AND %s
		ORDER BY p.date DESC
		LIMIT 1`,
		paymentsNonVoided,
	),
		organizationID, clientID,
	)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("get_client_summary last payment: %w", err)
	}
	if err == nil {
		lastPayment = &ClientLastPayment{Date: lp.Date, Amount: lp.Amount, Method: lp.Method}
	}

	return &ClientSummary{
		ClientID:      clientID,
		Owed:          summary.Total,
		Current:       summary.Current,
		Days1To30:     summary.Days1To30,
		Days31To60:    summary.Days31To60,
		Days61To90:    summary.Days61To90,
		Days90Plus:    summary.Days90Plus,
		OpenInvoices:  summary.Invoices,
		InvoiceCount:  stats.InvoiceCount,
		FirstPurchase: stats.FirstPurchase,
		LastPurchase:  stats.LastPurchase,
		BilledTotal:   stats.BilledTotal,
		PaidTotal:     ps.PaidTotal,
		PaymentCount:  ps.PaymentCount,
		LastPayment:   lastPayment,
	}, nil
}
