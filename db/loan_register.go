package db

import (
	"time"
)

// LoanStaleAfterDays mirrors STALE_AFTER_DAYS in
// src/components/cash-book/loan-register-model.ts: a customer who still owes
// and has paid nothing on their loans for longer than this is "stalled" on the
// Cash Book's loan register. Change both together.
const LoanStaleAfterDays = 60

// The loan register's filter tabs, which its loan status export follows:
// customers who still owe (stalled ones included), only the stalled ones, and
// the fully repaid ones.
const (
	LoanRegisterOpen    = "open"
	LoanRegisterStale   = "stale"
	LoanRegisterSettled = "settled"
)

// IsLoanRegisterFilter reports whether s names one of the register's tabs.
func IsLoanRegisterFilter(s string) bool {
	return s == LoanRegisterOpen || s == LoanRegisterStale || s == LoanRegisterSettled
}

// loanRegisterPayment is one inbound, non-voided payment applied to an
// invoice: who paid, which invoice, when.
type loanRegisterPayment struct {
	ClientID  string `db:"clientId"`
	InvoiceID string `db:"invoiceId"`
	Date      int64  `db:"date"`
}

// loanRegisterPayments loads every inbound, non-voided payment application to
// an invoice in the organization, the input the stalled rule reads.
func (d *Database) loanRegisterPayments(organizationID string) ([]loanRegisterPayment, error) {
	var out []loanRegisterPayment
	err := d.DB.Select(&out, `
		SELECT p.clientId, pa.documentId AS invoiceId, p.date
		FROM payments p
		JOIN payment_applications pa ON pa.paymentId = p.id
		WHERE p.organizationId = ? AND p.direction = 'inbound' AND p.status != 'voided'
		  AND pa.documentType = 'invoice' AND p.clientId IS NOT NULL`, organizationID)
	return out, err
}

// loanRegisterTones is the Go twin of buildLoanRegister's tone in
// loan-register-model.ts: per customer, "settled" when nothing is owed,
// "stale" when the last payment on one of their loan invoices (or, with none,
// their latest loan sale) is more than staleAfterDays calendar days before
// now, else "open". A payment counts only when it is the customer's own and
// was applied to one of their loan invoices, so a later cash purchase doesn't
// make a stalled loan look active. Days are counted in the organization's
// zone (the screen counts in the browser's, the same for a till in that zone).
func loanRegisterTones(rows []LoanStatusRow, payments []loanRegisterPayment, now int64, loc *time.Location, staleAfterDays int) map[string]string {
	type customer struct {
		outstanding  int64
		lastSale     int64
		lastPayment  int64
		hasPayment   bool
		loanInvoices map[string]bool
	}
	byClient := map[string]*customer{}
	for _, r := range rows {
		c := byClient[r.ClientID]
		if c == nil {
			c = &customer{loanInvoices: map[string]bool{}}
			byClient[r.ClientID] = c
		}
		c.outstanding += r.Outstanding
		if r.Date > c.lastSale {
			c.lastSale = r.Date
		}
		c.loanInvoices[r.InvoiceID] = true
	}
	for _, p := range payments {
		c := byClient[p.ClientID]
		if c == nil || !c.loanInvoices[p.InvoiceID] {
			continue
		}
		if !c.hasPayment || p.Date > c.lastPayment {
			c.lastPayment, c.hasPayment = p.Date, true
		}
	}

	tones := make(map[string]string, len(byClient))
	for id, c := range byClient {
		from := c.lastSale
		if c.hasPayment {
			from = c.lastPayment
		}
		switch {
		case c.outstanding <= 0:
			tones[id] = LoanRegisterSettled
		case calendarDaysBetween(from, now, loc) > staleAfterDays:
			tones[id] = LoanRegisterStale
		default:
			tones[id] = LoanRegisterOpen
		}
	}
	return tones
}

// calendarDaysBetween counts whole calendar days from one instant's date to
// another's in loc (0 or more), ignoring the time of day and daylight saving.
func calendarDaysBetween(fromMs, toMs int64, loc *time.Location) int {
	day := func(ms int64) time.Time {
		y, m, d := time.UnixMilli(ms).In(loc).Date()
		return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	}
	n := int(day(toMs).Sub(day(fromMs)).Hours() / 24)
	if n < 0 {
		return 0
	}
	return n
}

// filterLoanRegisterRows keeps the rows of the customers one register tab
// lists: open = everyone who still owes (stalled included), stale = the
// stalled ones, settled = the fully repaid ones. For the two owing tabs only
// the lines still owed are kept, as on screen; a settled customer's lines are
// all paid off and all kept.
func filterLoanRegisterRows(rows []LoanStatusRow, tones map[string]string, filter string) []LoanStatusRow {
	out := make([]LoanStatusRow, 0, len(rows))
	for _, r := range rows {
		tone := tones[r.ClientID]
		var keep bool
		switch filter {
		case LoanRegisterOpen:
			keep = tone != LoanRegisterSettled && r.Outstanding != 0
		case LoanRegisterStale:
			keep = tone == LoanRegisterStale && r.Outstanding != 0
		case LoanRegisterSettled:
			keep = tone == LoanRegisterSettled
		}
		if keep {
			out = append(out, r)
		}
	}
	return out
}
