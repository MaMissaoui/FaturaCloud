package db

import (
	"database/sql"
	"fmt"
	"time"
)

// VendorSummaryList is the batch overview for the Vendors screen — the
// purchases mirror of ClientSummaryList.
type VendorSummaryList struct {
	TotalOwed    int64              `json:"totalOwed"`
	OwingCount   int                `json:"owingCount"`
	OverdueCount int                `json:"overdueCount"`
	Vendors      []VendorSummaryRow `json:"vendors"`
}

// VendorSummaryRow is one vendor's row in the list view.
type VendorSummaryRow struct {
	VendorID     string `json:"vendorId"`
	Owed         int64  `json:"owed"`
	Overdue      int64  `json:"overdue"`
	LastPurchase *int64 `json:"lastPurchase"`
	BillCount    int    `json:"billCount"`
}

// VendorSummary is the single-vendor detail view — the purchases mirror of
// ClientSummary. OpenBills carries the vendor's outstanding bills, oldest due
// date first, with DaysOverdue/Bucket filled exactly as bucketOutstandingBills
// does for the batch report.
type VendorSummary struct {
	VendorID      string             `json:"vendorId"`
	Owed          int64              `json:"owed"`
	Current       int64              `json:"current"`
	Days1To30     int64              `json:"days1To30"`
	Days31To60    int64              `json:"days31To60"`
	Days61To90    int64              `json:"days61To90"`
	Days90Plus    int64              `json:"days90Plus"`
	OpenBills     []OutstandingBill  `json:"openBills"`
	BillCount     int                `json:"billCount"`
	FirstPurchase *int64             `json:"firstPurchase"`
	LastPurchase  *int64             `json:"lastPurchase"`
	BilledTotal   int64              `json:"billedTotal"`
	PaidTotal     int64              `json:"paidTotal"`
	PaymentCount  int                `json:"paymentCount"`
	LastPayment   *VendorLastPayment `json:"lastPayment"`
}

// VendorLastPayment is the most recent posted, non-voided outbound payment.
type VendorLastPayment struct {
	Date   int64  `json:"date"`
	Amount int64  `json:"amount"`
	Method string `json:"method"`
}

// GetVendorSummaries mirrors GetClientSummaries: total owed, owing/overdue
// counts, one row per vendor who either owes or has at least one approved/paid
// bill. Reuses selectOutstandingBills (the same query the AP aging report
// reads) for the per-vendor owed totals — so the headline TotalOwed equals
// GetPayableAging's Total by construction, the same tripwire the client side
// has. errSummariesSwitchedOff is reused from client_summary.go.
func (d *Database) GetVendorSummaries(organizationID string) (VendorSummaryList, error) {
	org, err := d.GetOrganization(organizationID)
	if err != nil {
		return VendorSummaryList{}, err
	}
	if !org.MasterDataSummaries {
		return VendorSummaryList{}, errSummariesSwitchedOff
	}

	// Per-vendor outstanding, via the same shared query the AP aging uses.
	bills, err := d.selectOutstandingBills(organizationID, "")
	if err != nil {
		return VendorSummaryList{}, fmt.Errorf("get_vendor_summaries: %w", err)
	}

	// Aggregate owed and overdue per vendor. overdue is the sum of the
	// vendor's bills whose bucket is not "current" — what the list page's
	// "Overdue" chip counts and sorts by.
	type vendorOwed struct {
		owed    int64
		overdue int64
	}
	vendorMap := map[string]*vendorOwed{}
	now := time.Now()
	for _, bill := range bills {
		if _, ok := vendorMap[bill.VendorID]; !ok {
			vendorMap[bill.VendorID] = &vendorOwed{}
		}
		vendorMap[bill.VendorID].owed += bill.Total
		// The bucket, not daysOverdue: a bill past due by less than a day
		// is already late (days1To30) with daysOverdue still 0.
		if bucket, _ := agingBucketFor(bill.DueDate, now.UnixMilli()); bucket != "current" {
			vendorMap[bill.VendorID].overdue += bill.Total
		}
	}

	// LastPurchase and BillCount per vendor (state IN ('approved','paid')).
	type vendorBillInfo struct {
		VendorID     string `db:"vendorId"`
		LastPurchase *int64 `db:"lastPurchase"`
		BillCount    int    `db:"billCount"`
	}
	var billInfos []vendorBillInfo
	err = d.DB.Select(&billInfos, `
		SELECT vendorId, MAX(date) AS lastPurchase, COUNT(*) AS billCount
		FROM incoming_invoices
		WHERE organizationId = ? AND state IN ('approved', 'paid')
		GROUP BY vendorId`,
		organizationID,
	)
	if err != nil {
		return VendorSummaryList{}, fmt.Errorf("get_vendor_summaries bill info: %w", err)
	}

	// Merge into one list: vendors with either a balance or at least one
	// approved/paid bill.
	seen := map[string]bool{}
	rows := []VendorSummaryRow{}
	for _, info := range billInfos {
		seen[info.VendorID] = true
		owed := int64(0)
		overdue := int64(0)
		if vo, ok := vendorMap[info.VendorID]; ok {
			owed = vo.owed
			overdue = vo.overdue
		}
		rows = append(rows, VendorSummaryRow{
			VendorID:     info.VendorID,
			Owed:         owed,
			Overdue:      overdue,
			LastPurchase: info.LastPurchase,
			BillCount:    info.BillCount,
		})
	}
	for vid, vo := range vendorMap {
		if !seen[vid] {
			rows = append(rows, VendorSummaryRow{
				VendorID: vid,
				Owed:     vo.owed,
				Overdue:  vo.overdue,
			})
		}
	}

	var totalOwed int64
	owingCount := 0
	overdueCount := 0
	for _, r := range rows {
		totalOwed += r.Owed
		if r.Owed > 0 {
			owingCount++
		}
		if r.Overdue > 0 {
			overdueCount++
		}
	}

	return VendorSummaryList{
		TotalOwed:    totalOwed,
		OwingCount:   owingCount,
		OverdueCount: overdueCount,
		Vendors:      rows,
	}, nil
}

// GetVendorSummary mirrors GetClientSummary. Outstanding bills, the
// vendor's aging buckets, bill/payment history, last payment. Returns
// sql.ErrNoRows when the vendor belongs to another organization (uses the
// existing GetVendor — every other handler that resolves a vendor already
// reads it).
func (d *Database) GetVendorSummary(organizationID, vendorID string) (*VendorSummary, error) {
	org, err := d.GetOrganization(organizationID)
	if err != nil {
		return nil, err
	}
	if !org.MasterDataSummaries {
		return nil, errSummariesSwitchedOff
	}

	vendor, err := d.GetVendor(vendorID)
	if err != nil {
		return nil, err
	}
	if vendor.OrganizationID != organizationID {
		return nil, sql.ErrNoRows
	}

	bills, err := d.selectOutstandingBills(organizationID, vendorID)
	if err != nil {
		return nil, fmt.Errorf("get_vendor_summary: %w", err)
	}

	now := time.Now()
	summary := bucketOutstandingBills(bills, now)

	// Bill stats (state IN ('approved','paid')).
	type billStats struct {
		BillCount     int    `db:"billCount"`
		FirstPurchase *int64 `db:"firstPurchase"`
		LastPurchase  *int64 `db:"lastPurchase"`
		BilledTotal   int64  `db:"billedTotal"`
	}
	var stats billStats
	err = d.DB.Get(&stats, `
		SELECT COUNT(*) AS billCount,
		       MIN(date) AS firstPurchase,
		       MAX(date) AS lastPurchase,
		       CAST(COALESCE(SUM(ROUND(total * COALESCE(exchangeRate, 1))), 0) AS INTEGER) AS billedTotal
		FROM incoming_invoices
		WHERE organizationId = ? AND vendorId = ? AND state IN ('approved', 'paid')`,
		organizationID, vendorID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_vendor_summary bill stats: %w", err)
	}

	// Payment stats (posted, non-voided outbound payments of this vendor).
	type paymentStats struct {
		PaidTotal    int64 `db:"paidTotal"`
		PaymentCount int   `db:"paymentCount"`
	}
	var ps paymentStats
	err = d.DB.Get(&ps, fmt.Sprintf(`
		SELECT CAST(COALESCE(SUM(ROUND(p.amount * COALESCE(p.exchangeRate, 1))), 0) AS INTEGER) AS paidTotal,
		       COUNT(*) AS paymentCount
		FROM payments p
		WHERE p.organizationId = ? AND p.vendorId = ? AND p.direction = 'outbound' AND %s`,
		paymentsNonVoided,
	),
		organizationID, vendorID,
	)
	if err != nil {
		return nil, fmt.Errorf("get_vendor_summary payment stats: %w", err)
	}

	// Last payment.
	var lastPayment *VendorLastPayment
	var lp struct {
		Date   int64  `db:"date"`
		Amount int64  `db:"amount"`
		Method string `db:"method"`
	}
	err = d.DB.Get(&lp, fmt.Sprintf(`
		SELECT p.date, CAST(ROUND(p.amount * COALESCE(p.exchangeRate, 1)) AS INTEGER) AS amount, p.method
		FROM payments p
		WHERE p.organizationId = ? AND p.vendorId = ? AND p.direction = 'outbound' AND %s
		ORDER BY p.date DESC
		LIMIT 1`,
		paymentsNonVoided,
	),
		organizationID, vendorID,
	)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("get_vendor_summary last payment: %w", err)
	}
	if err == nil {
		lastPayment = &VendorLastPayment{Date: lp.Date, Amount: lp.Amount, Method: lp.Method}
	}

	return &VendorSummary{
		VendorID:      vendorID,
		Owed:          summary.Total,
		Current:       summary.Current,
		Days1To30:     summary.Days1To30,
		Days31To60:    summary.Days31To60,
		Days61To90:    summary.Days61To90,
		Days90Plus:    summary.Days90Plus,
		OpenBills:     summary.Bills,
		BillCount:     stats.BillCount,
		FirstPurchase: stats.FirstPurchase,
		LastPurchase:  stats.LastPurchase,
		BilledTotal:   stats.BilledTotal,
		PaidTotal:     ps.PaidTotal,
		PaymentCount:  ps.PaymentCount,
		LastPayment:   lastPayment,
	}, nil
}
