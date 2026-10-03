package db

import (
	"testing"
	"time"
)

// The cases mirror src/components/cash-book/loan-register-model.test.ts, so
// the export's stalled rule and the screen's can't drift apart.

var loanTestLoc = time.UTC

func loanDay(iso string) int64 {
	t, err := time.ParseInLocation("2006-01-02 15:04", iso+" 10:00", loanTestLoc)
	if err != nil {
		panic(err)
	}
	return t.UnixMilli()
}

var loanTestNow = loanDay("2026-09-30")

func loanRow(over func(*LoanStatusRow)) LoanStatusRow {
	r := LoanStatusRow{
		LineID: "l1", InvoiceID: "i1", InvoiceNumber: "FAC-1", ClientID: "c1", ClientName: "Hédi Trabelsi",
		Date: loanDay("2026-03-12"), ProductName: "Climatiseur", Quantity: 1,
		Amount: 139000, Paid: 75000, Outstanding: 64000,
	}
	if over != nil {
		over(&r)
	}
	return r
}

func loanPay(client, invoice, iso string) loanRegisterPayment {
	return loanRegisterPayment{ClientID: client, InvoiceID: invoice, Date: loanDay(iso)}
}

func TestLoanRegisterTones(t *testing.T) {
	tone := func(rows []LoanStatusRow, pays []loanRegisterPayment) string {
		return loanRegisterTones(rows, pays, loanTestNow, loanTestLoc, LoanStaleAfterDays)["c1"]
	}

	t.Run("a recent payment keeps the customer open", func(t *testing.T) {
		if got := tone([]LoanStatusRow{loanRow(nil)}, []loanRegisterPayment{loanPay("c1", "i1", "2026-09-21")}); got != LoanRegisterOpen {
			t.Fatalf("tone = %q, want open", got)
		}
	})
	t.Run("no payment for more than the threshold is stale", func(t *testing.T) {
		if got := tone([]LoanStatusRow{loanRow(nil)}, []loanRegisterPayment{loanPay("c1", "i1", "2026-07-18")}); got != LoanRegisterStale {
			t.Fatalf("tone = %q, want stale", got)
		}
	})
	t.Run("counts from the latest sale when nothing was paid", func(t *testing.T) {
		rows := []LoanStatusRow{loanRow(func(r *LoanStatusRow) { r.Date = loanDay("2026-09-27"); r.Paid = 0 })}
		if got := tone(rows, nil); got != LoanRegisterOpen {
			t.Fatalf("tone = %q, want open", got)
		}
	})
	t.Run("open exactly at the threshold, stale the day after", func(t *testing.T) {
		at := loanTestNow - int64(LoanStaleAfterDays)*86_400_000
		at1 := loanRegisterPayment{ClientID: "c1", InvoiceID: "i1", Date: at}
		if got := tone([]LoanStatusRow{loanRow(nil)}, []loanRegisterPayment{at1}); got != LoanRegisterOpen {
			t.Fatalf("at threshold: tone = %q, want open", got)
		}
		at1.Date -= 86_400_000
		if got := tone([]LoanStatusRow{loanRow(nil)}, []loanRegisterPayment{at1}); got != LoanRegisterStale {
			t.Fatalf("day after: tone = %q, want stale", got)
		}
	})
	t.Run("ignores payments for other invoices and other customers", func(t *testing.T) {
		pays := []loanRegisterPayment{
			loanPay("c1", "i99", "2026-09-29"), // another invoice (e.g. a later cash sale)
			loanPay("c2", "i1", "2026-09-29"),  // another customer
			loanPay("c1", "i1", "2026-06-01"),
		}
		if got := tone([]LoanStatusRow{loanRow(nil)}, pays); got != LoanRegisterStale {
			t.Fatalf("tone = %q, want stale", got)
		}
	})
	t.Run("a fully repaid customer is settled however long ago they paid", func(t *testing.T) {
		rows := []LoanStatusRow{loanRow(func(r *LoanStatusRow) { r.Paid, r.Outstanding = 139000, 0 })}
		if got := tone(rows, []loanRegisterPayment{loanPay("c1", "i1", "2026-01-15")}); got != LoanRegisterSettled {
			t.Fatalf("tone = %q, want settled", got)
		}
	})
}

func TestFilterLoanRegisterRows(t *testing.T) {
	rows := []LoanStatusRow{
		loanRow(func(r *LoanStatusRow) { r.LineID, r.ClientID, r.Outstanding = "1", "small", 10000 }),
		loanRow(func(r *LoanStatusRow) { r.LineID, r.ClientID, r.InvoiceID, r.Outstanding = "2", "big", "i2", 90000 }),
		// big's second line is paid off: kept under settled only, never under the owing tabs
		loanRow(func(r *LoanStatusRow) { r.LineID, r.ClientID, r.InvoiceID, r.Outstanding = "2b", "big", "i2", 0 }),
		loanRow(func(r *LoanStatusRow) { r.LineID, r.ClientID, r.InvoiceID, r.Outstanding = "3", "late", "i3", 5000 }),
		loanRow(func(r *LoanStatusRow) {
			r.LineID, r.ClientID, r.InvoiceID, r.Paid, r.Outstanding = "4", "done", "i4", 139000, 0
		}),
	}
	pays := []loanRegisterPayment{
		loanPay("small", "i1", "2026-09-20"),
		loanPay("big", "i2", "2026-09-20"),
		loanPay("late", "i3", "2026-05-01"),
		loanPay("done", "i4", "2026-04-01"),
	}
	tones := loanRegisterTones(rows, pays, loanTestNow, loanTestLoc, LoanStaleAfterDays)
	lines := func(filter string) []string {
		var out []string
		for _, r := range filterLoanRegisterRows(rows, tones, filter) {
			out = append(out, r.LineID)
		}
		return out
	}
	for _, tc := range []struct {
		filter string
		want   []string
	}{
		{LoanRegisterOpen, []string{"1", "2", "3"}},
		{LoanRegisterStale, []string{"3"}},
		{LoanRegisterSettled, []string{"4"}},
	} {
		got := lines(tc.filter)
		if len(got) != len(tc.want) {
			t.Fatalf("%s: lines = %v, want %v", tc.filter, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("%s: lines = %v, want %v", tc.filter, got, tc.want)
			}
		}
	}
}

func TestCalendarDaysBetweenFollowsTheZone(t *testing.T) {
	tunis, err := time.LoadLocation("Africa/Tunis")
	if err != nil {
		t.Fatal(err)
	}
	// 23:30 UTC on the 1st is already the 2nd in Tunis (UTC+1).
	from := time.Date(2026, 3, 1, 23, 30, 0, 0, time.UTC).UnixMilli()
	to := time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC).UnixMilli()
	if got := calendarDaysBetween(from, to, time.UTC); got != 1 {
		t.Fatalf("UTC: %d days, want 1", got)
	}
	if got := calendarDaysBetween(from, to, tunis); got != 0 {
		t.Fatalf("Tunis: %d days, want 0", got)
	}
	if got := calendarDaysBetween(to, from, time.UTC); got != 0 {
		t.Fatalf("negative span: %d days, want 0", got)
	}
}
