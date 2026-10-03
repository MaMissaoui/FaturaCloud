package main

import (
	"fmt"
	"time"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// newCustomerProbability is how often a Cash Book sale, while the client
// base is still under targetClientCount, goes to a brand-new walk-in
// customer rather than a repeat one. It doesn't need precise tuning: the
// hard cap in createCashBookSale (len(s.clients) < s.targetClientCount)
// is what guarantees the run lands on *exactly* targetClientCount by the
// time enough sales have happened — this probability only controls how
// front-loaded that growth is.
const newCustomerProbability = 0.35

// generateCashBookSalesForDay is --scenario retail's only sales channel —
// every sale goes through db.CreateCashSale, the atomic client+invoice+
// payment endpoint the Cash Book screen itself calls. Unlike the B2B
// channel in sales.go (near-zero weekend volume), a retail counter is open
// and busiest on weekends — see cashBookSalesRangeFor's day-of-week
// weighting.
func (s *Seeder) generateCashBookSalesForDay(day time.Time) error {
	lo, hi := cashBookSalesRangeFor(day.Weekday())
	// The weekday range, scaled, then shaped by the season and the business's
	// growth (retail_season.go) so the months and years differ.
	expected := s.rng.Float64Range(float64(lo), float64(hi)) * s.cfg.VolumeScale * seasonFactor(day) * s.growthFactor(day)
	count := s.poissonish(expected)
	if count == 0 && s.cfg.EndDate.Sub(day) < 48*time.Hour {
		count = 1 // the Dashboard's till shows today and yesterday: keep both moving
	}
	for i := 0; i < count; i++ {
		if err := s.createCashBookSale(day); err != nil {
			return err
		}
	}
	return nil
}

// cashBookSalesRangeFor is tuned, together with newCustomerProbability and
// the 18-month default range, to comfortably clear targetClientCount
// (400-450) well before the run ends, leaving a realistic repeat-customer
// tail — verified via --dry-run and a full run's final Stats line, not
// just by inspection.
func cashBookSalesRangeFor(weekday time.Weekday) (lo, hi int) {
	switch weekday {
	case time.Sunday:
		return 2, 5 // many small Tunisian retailers are half-day/closed-ish on Sunday
	case time.Thursday, time.Friday, time.Saturday:
		return 8, 16 // payday/weekend shopping bump
	default:
		return 5, 11
	}
}

// createCashBookSale builds one counter sale: 1-4 line items from the
// scenario's product+service catalog (randomInvoiceLines, sales.go — fully
// reused, unmodified), a resolved-or-new client, and an AmountReceived
// tiered by the sale's own size (decideAmountReceived) — then posts it via
// POST /api/cash-sales with BankAccountID explicitly set to the register
// account. Never left empty: this dataset must be correct regardless of
// CreateCashSale's own server-side default (see CLAUDE.md's cash register
// account note — the exact bug this dataset exists partly to catch a
// regression of).
func (s *Seeder) createCashBookSale(day time.Time) error {
	lines := s.retailSaleLines(day, s.rng.IntRange(1, 3))
	subTotal, taxTotal, total := computeTotals(lines.totals)

	req := db.CreateCashSaleRequest{
		OrganizationID: s.orgID,
		Date:           midnightUTC(day),
		Currency:       s.cfg.Currency,
		LineItems:      lines.items,
		SubTotal:       subTotal,
		TaxTotal:       taxTotal,
		Total:          total,
		PaymentMethod:  "cash",
		BankAccountID:  s.registerAccountID,
		AmountReceived: s.decideAmountReceived(total),
	}

	var clientName string
	newClient := len(s.clients) < s.targetClientCount && (len(s.clients) == 0 || s.rng.Chance(newCustomerProbability))
	if newClient {
		name, phone := s.newRetailCustomer()
		req.NewClient = &db.CreateClientRequest{Name: strPtr(name), Phone: phone}
		clientName = name
	} else {
		existing := Pick(s.rng, s.clients)
		req.ClientID = existing.id
		clientName = existing.name
	}

	var result db.CashSaleResult
	if err := s.c.Post("/api/cash-sales", req, &result); err != nil {
		return fmt.Errorf("cash sale for %s: %w", clientName, err)
	}
	s.stats.Invoices++
	s.takeRetailSaleStock(req.LineItems)
	if req.AmountReceived > 0 {
		s.stats.Payments++
	}
	if newClient {
		s.clients = append(s.clients, clientRef{id: result.Client.ID, name: clientName})
		s.stats.Clients++
	}

	if remaining := total - req.AmountReceived; remaining > 0 {
		// Now and then a customer who took an appliance on credit with
		// nothing down brings it back unused within the week: the sale is
		// cancelled, which puts its stock back (db/invoice_stock.go).
		if req.AmountReceived == 0 && s.rng.Chance(0.06) {
			s.scheduleCashSaleCancellation(day, result.Invoice.ID, result.Invoice.Number, req.LineItems)
			return nil
		}
		s.scheduleLoanRepayment(day, result.Invoice.ID, remaining, result.Client.ID)
	}
	return nil
}

func (s *Seeder) scheduleCashSaleCancellation(saleDay time.Time, invoiceID, number string, items []db.CreateInvoiceLineItemRequest) {
	cancelDay := saleDay.AddDate(0, 0, s.rng.IntRange(1, 6))
	if cancelDay.After(s.cfg.EndDate) {
		return
	}
	s.sched.Schedule(cancelDay, func() error {
		if err := s.c.Patch("/api/invoices/"+invoiceID+"/state", map[string]string{"state": "cancelled"}, nil); err != nil {
			return fmt.Errorf("cancel cash sale %s: %w", number, err)
		}
		for _, item := range items {
			if item.ProductID == nil {
				continue
			}
			if p, ok := s.productByID(*item.ProductID); ok && p.stockEnabled {
				s.adjustOnHand(p.id, item.Quantity)
			}
		}
		s.stats.CancelledSales++
		return nil
	})
}

// decideAmountReceived biases full-cash-at-the-counter vs. a deposit vs. a
// zero-deposit loan by ticket size — real Tunisian retail practice: small
// items are paid in full, big-ticket ones (fridges, TVs, washing machines,
// ACs — priced above bigTicketThresholdCents in catalog_retail.go) are
// commonly bought on informal installment ("vente à tempérament").
const bigTicketThresholdCents = 60000 // 600 TND

func (s *Seeder) decideAmountReceived(total int64) int64 {
	if total < bigTicketThresholdCents {
		if s.rng.Chance(0.90) {
			return total
		}
		return total * int64(s.rng.IntRange(50, 80)) / 100
	}
	switch {
	case s.rng.Chance(0.40):
		return total // paid in full even for a big-ticket item
	case s.rng.Chance(0.6): // of the remaining 60% -> 36% overall: a deposit
		return total * int64(s.rng.IntRange(20, 60)) / 100
	default: // remaining ~24% overall: zero-deposit loan
		return 0
	}
}

// scheduleLoanRepayment collects a loan sale's remaining balance via
// sales.go's schedulePayment (reused unchanged, just pointed at the
// register account instead of Bank — an installment is paid back in cash
// at the counter, not wired in) — a tiered fate distribution in the same
// spirit as createDirectInvoice's (sales.go), adapted for a sale that's
// already happened rather than an invoice that might still be cancelled.
func (s *Seeder) scheduleLoanRepayment(saleDay time.Time, invoiceID string, remaining int64, clientID string) {
	// pay collects share of what's left on the loan on payDay. Most
	// collections go through the Cash Book's own line-by-line settlement
	// (POST /api/cash-sales/{id}/payments) — what the counter actually does
	// — and the rest through an ordinary payment into the register, the way
	// an accountant records one from the invoice page.
	pay := func(payDay time.Time, share float64, amount int64) {
		if s.rng.Chance(0.7) {
			s.scheduleLoanCollection(payDay, invoiceID, clientID, share)
			return
		}
		s.schedulePayment(payDay, invoiceID, amount, clientID, "invoice", s.cfg.Currency, nil, s.registerAccountID)
	}
	switch {
	case s.rng.Chance(0.75):
		// The typical case: paid off within one further installment.
		pay(businessDaysLater(saleDay, s.rng.IntRange(15, 90)), 1, remaining)

	case s.rng.Chance(0.6): // of the remaining 25% -> 15% overall
		first := remaining / 2
		firstDay := businessDaysLater(saleDay, s.rng.IntRange(20, 60))
		secondDay := businessDaysLater(firstDay, s.rng.IntRange(20, 60))
		pay(firstDay, 0.5, first)
		pay(secondDay, 1, remaining-first)

	case s.rng.Chance(0.7): // of the remaining 10% -> 7% overall: slow-pay, well past due
		pay(businessDaysLater(saleDay, s.rng.IntRange(120, 250)), 1, remaining)

	default:
		// Permanent bad debt tail, ~3% of loan sales — left outstanding for
		// the rest of the run, same small-and-deliberate share
		// createDirectInvoice's own default case documents.
	}
}

// scheduleLoanCollection settles share (0..1] of an invoice's outstanding
// balance on payDay through the Cash Book, one payment per line: the lines
// are paid in the order the loan register lists them, the last one only in
// part when share runs out. share 1 clears the loan. The line balances are
// read from the loan register (GET .../reports/loan-status), the figures the
// cashier sees — and read again after every payment, because a payment on
// one line re-spreads the invoice's undirected deposit and can move a cent of
// rounding onto another line.
func (s *Seeder) scheduleLoanCollection(payDay time.Time, invoiceID, clientID string, share float64) {
	if payDay.After(s.cfg.EndDate) {
		return
	}
	s.sched.Schedule(payDay, func() error {
		lines, outstanding, err := s.loanLines(invoiceID, clientID)
		if err != nil {
			return err
		}
		target := outstanding
		if share < 1 {
			target = int64(float64(outstanding) * share)
		}
		for i := 0; target > 0 && len(lines) > 0 && i < 10; i++ {
			l := lines[0]
			amount := min(l.Outstanding, target)
			req := db.CreateCashSalePaymentRequest{InvoiceLineItemID: l.LineID, Amount: amount, Date: midnightUTC(payDay)}
			if err := s.c.Post("/api/cash-sales/"+invoiceID+"/payments", req, nil); err != nil {
				return fmt.Errorf("collect loan %s line %s: %w", l.InvoiceNumber, l.ProductName, err)
			}
			s.stats.Payments++
			target -= amount
			if lines, _, err = s.loanLines(invoiceID, clientID); err != nil {
				return err
			}
		}
		return nil
	})
}

// loanLines reads an invoice's lines that still have something outstanding,
// in the loan register's order, and their total outstanding.
func (s *Seeder) loanLines(invoiceID, clientID string) ([]db.LoanStatusRow, int64, error) {
	var rows []db.LoanStatusRow
	path := fmt.Sprintf("/api/organizations/%s/reports/loan-status?clientId=%s", s.orgID, clientID)
	if err := s.c.Get(path, &rows); err != nil {
		return nil, 0, fmt.Errorf("loan status for %s: %w", invoiceID, err)
	}
	var lines []db.LoanStatusRow
	var outstanding int64
	for _, r := range rows {
		if r.InvoiceID == invoiceID && r.Outstanding > 0 {
			lines = append(lines, r)
			outstanding += r.Outstanding
		}
	}
	return lines, outstanding, nil
}

// newRetailCustomer generates a walk-in customer's name and (70% of the
// time — plenty leave one for search/identification, some don't) a unique
// phone number, retried against usedCustomerPhones the same way
// masterdata.go's uniqueSKU avoids a SKU collision — CreateCashSale 409s a
// duplicate phone within the same organization (see db/cash_sale.go).
func (s *Seeder) newRetailCustomer() (name string, phone *string) {
	name = s.rng.RetailCustomerName()
	if !s.rng.Chance(0.7) {
		return name, nil
	}
	for i := 0; i < 20; i++ {
		candidate := s.rng.Phone()
		if !s.usedCustomerPhones[candidate] {
			s.usedCustomerPhones[candidate] = true
			return name, &candidate
		}
	}
	return name, nil // exhausted retries — sell without a phone rather than fail the sale
}
