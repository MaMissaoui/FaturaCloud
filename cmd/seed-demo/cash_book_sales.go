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
	scaled := scaleCountRange([2]int{lo, hi}, s.cfg.VolumeScale)
	count := s.rng.IntRange(scaled[0], scaled[1])
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
	lines := s.randomInvoiceLines(s.rng.IntRange(1, 4))
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
	if req.AmountReceived > 0 {
		s.stats.Payments++
	}
	if newClient {
		s.clients = append(s.clients, clientRef{id: result.Client.ID, name: clientName})
		s.stats.Clients++
	}

	if remaining := total - req.AmountReceived; remaining > 0 {
		s.scheduleLoanRepayment(day, result.Invoice.ID, remaining, result.Client.ID)
	}
	return nil
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
	switch {
	case s.rng.Chance(0.75):
		// The typical case: paid off within one further installment.
		payDay := businessDaysLater(saleDay, s.rng.IntRange(15, 90))
		s.schedulePayment(payDay, invoiceID, remaining, clientID, "invoice", s.cfg.Currency, nil, s.registerAccountID)

	case s.rng.Chance(0.6): // of the remaining 25% -> 15% overall
		first := remaining / 2
		second := remaining - first
		firstDay := businessDaysLater(saleDay, s.rng.IntRange(20, 60))
		secondDay := businessDaysLater(firstDay, s.rng.IntRange(20, 60))
		s.schedulePayment(firstDay, invoiceID, first, clientID, "invoice", s.cfg.Currency, nil, s.registerAccountID)
		s.schedulePayment(secondDay, invoiceID, second, clientID, "invoice", s.cfg.Currency, nil, s.registerAccountID)

	case s.rng.Chance(0.7): // of the remaining 10% -> 7% overall: slow-pay, well past due
		payDay := businessDaysLater(saleDay, s.rng.IntRange(120, 250))
		s.schedulePayment(payDay, invoiceID, remaining, clientID, "invoice", s.cfg.Currency, nil, s.registerAccountID)

	default:
		// Permanent bad debt tail, ~3% of loan sales — left outstanding for
		// the rest of the run, same small-and-deliberate share
		// createDirectInvoice's own default case documents.
	}
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
