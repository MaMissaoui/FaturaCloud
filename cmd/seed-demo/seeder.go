package main

import (
	"fmt"
	"log"
	"math"
	"time"
)

// volumeProfile is a named bundle of "how much activity per day/week"
// knobs. Adding a third profile (or tuning the busy one) only ever means
// editing this map — nothing downstream hardcodes a number.
type volumeProfile struct {
	Clients, Vendors int
	// Sales invoices created directly (not via an order), per business day.
	InvoicesPerWeekday    [2]int
	InvoicesPerWeekendDay [2]int
	// Sales orders started per week (each becomes a delivery, then an invoice).
	OrdersPerWeek [2]int
	// Purchase orders placed per week.
	PurchaseOrdersPerWeek [2]int
}

var volumeProfiles = map[string]volumeProfile{
	"small": {
		Clients: 40, Vendors: 12,
		InvoicesPerWeekday: [2]int{3, 6}, InvoicesPerWeekendDay: [2]int{0, 1},
		OrdersPerWeek: [2]int{1, 3}, PurchaseOrdersPerWeek: [2]int{2, 4},
	},
	"busy": {
		Clients: 100, Vendors: 25,
		InvoicesPerWeekday: [2]int{8, 15}, InvoicesPerWeekendDay: [2]int{0, 2},
		OrdersPerWeek: [2]int{3, 6}, PurchaseOrdersPerWeek: [2]int{5, 8},
	},
}

// taxRateRef is what the rest of the tool needs to know about a tax rate it
// created — the id to reference on a line item, and the percentage to
// replicate the server's own totals math locally (see money.go).
type taxRateRef struct {
	id      string
	percent float64
}

type clientRef struct {
	id, name string
}

type vendorRef struct {
	id, name string
}

// productRef is a catalog entry plus the id the server assigned it and,
// for a stock-enabled product, the tool's own running estimate of on-hand
// quantity — kept in lockstep with every receipt/shipment this tool creates
// so it never asks a delivery to ship more than a real UpdateDeliveryStatus
// call would allow.
type productRef struct {
	id           string
	name         string
	unit         string
	stockEnabled bool
	priceCents   int64
	costCents    int64
	taxRateID    string // "" = untaxed
	onHand       float64
	qtyLo, qtyHi int    // plausible line-item quantity range, from catalog.go
	category     string // "finished" | "component" | "" — see catalog.go's productCatalogEntry
	displacement string // "50cc".."650cc" for "finished"/"component", "" otherwise — see catalog.go's productCatalogEntry
}

// Stats tallies what actually got created, printed as a summary at the end
// — the only proof-of-work for a run that otherwise just logs progress
// lines for several minutes.
type Stats struct {
	Clients, Vendors, Products                          int
	Invoices, Orders, Deliveries                        int
	PurchaseOrders, InboundDeliveries, IncomingInvoices int
	Imports                                             int
	Payments                                            int
	// AssemblyBatches counts production.go's maybeAssembleFinishedGoods runs
	// that actually built something (skipped attempts with insufficient
	// component stock don't count); AssembledUnits is the total finished
	// goods produced across every batch.
	AssemblyBatches, AssembledUnits int
	Errors                          int
}

// Seeder holds every piece of shared state a generator (sales.go,
// purchasing.go) needs: the API client, the deterministic RNG, the
// organization's master data and its ids, a day-keyed task queue for
// multi-step workflows (order -> deliver -> invoice, PO -> receive -> bill
// -> pay), and running counters for human-readable document numbers.
type Seeder struct {
	c   *Client
	rng *Rand
	cfg Config
	log *log.Logger

	profile volumeProfile

	orgID         string
	cashAccountID string
	// orgProfile is resolved once in setupOrganization from cfg.Country and
	// reused by setupTaxRates (VAT account codes) and setupVendors/
	// setupClients (CountryCode) — see masterdata.go's orgProfiles.
	orgProfile orgProfile

	standardTax, reducedTax, zeroTax taxRateRef

	clients  []clientRef
	vendors  []vendorRef
	products []productRef

	sched *Scheduler

	// currentImport is the most recently created consolidated shipment
	// (F114) still open for new purchase orders to link to — see
	// imports.go's maybeStartImport/maybeLinkToImport.
	currentImport *importRef

	invoiceNum, orderNum, deliveryNum, poNum, inboundNum, incomingNum, importNum *numberer

	start time.Time
	stats Stats
}

// numberer hands out human-looking, per-calendar-year document numbers
// ("INV-2026-0001", "PO-2025-0142", ...) — a monotonic-forever counter would
// still be structurally valid (these fields have no uniqueness constraint,
// per CLAUDE.md's schema notes) but wouldn't look like what a real
// organization's numbering scheme produces.
type numberer struct {
	prefix string
	counts map[int]int
}

func newNumberer(prefix string) *numberer { return &numberer{prefix: prefix, counts: map[int]int{}} }

func (n *numberer) next(year int) string {
	n.counts[year]++
	return fmt.Sprintf("%s-%d-%04d", n.prefix, year, n.counts[year])
}

func NewSeeder(c *Client, cfg Config) *Seeder {
	return &Seeder{
		c:       c,
		rng:     NewRand(cfg.Seed, cfg.Country),
		cfg:     cfg,
		log:     log.New(log.Writer(), "", log.LstdFlags),
		profile: volumeProfiles[cfg.Volume],
		sched:   NewScheduler(),

		invoiceNum:  newNumberer("INV"),
		orderNum:    newNumberer("SO"),
		deliveryNum: newNumberer("DN"),
		poNum:       newNumberer("PO"),
		inboundNum:  newNumberer("GR"),
		incomingNum: newNumberer("BILL"),
		importNum:   newNumberer("IMP"),
	}
}

// Run is the whole pipeline: master data, fiscal coverage, then one pass a
// day from (EndDate - Months) through EndDate. It's intentionally linear
// and easy to read top to bottom — a seeding tool that's hard to follow is
// worse than useless the first time its output looks wrong and someone has
// to find out why.
func (s *Seeder) Run() error {
	runStart := time.Now()
	startDate := s.cfg.EndDate.AddDate(0, -s.cfg.Months, 0)
	s.start = startDate

	s.log.Printf("seed-demo: range %s .. %s (%s profile, seed %d)",
		startDate.Format("2006-01-02"), s.cfg.EndDate.Format("2006-01-02"), s.cfg.Volume, s.cfg.Seed)

	if err := s.setupOrganization(); err != nil {
		return fmt.Errorf("organization setup: %w", err)
	}
	if err := s.setupMasterData(); err != nil {
		return fmt.Errorf("master data: %w", err)
	}
	if err := s.setupFiscalCoverage(startDate, s.cfg.EndDate); err != nil {
		return fmt.Errorf("fiscal years/periods: %w", err)
	}

	if s.cfg.DryRun {
		s.log.Printf("seed-demo: --dry-run, skipping document generation")
		return nil
	}

	totalDays := int(math.Round(s.cfg.EndDate.Sub(startDate).Hours()/24)) + 1
	dayNum := 0
	for day := startDate; !day.After(s.cfg.EndDate); day = day.AddDate(0, 0, 1) {
		dayNum++

		// Multi-step workflows scheduled from earlier days (a payment due
		// today, a receipt for a PO placed last week, ...) run before
		// today's own new activity, matching the order a real business day
		// unfolds: yesterday's paperwork lands on your desk, then you make
		// today's.
		s.sched.RunDue(day, func(err error) { s.onTaskError(day, err) })

		weekend := day.Weekday() == time.Saturday || day.Weekday() == time.Sunday
		if err := s.generateSalesForDay(day, weekend); err != nil {
			s.onTaskError(day, fmt.Errorf("sales: %w", err))
		}
		if !weekend {
			// Assembly runs before orders so a batch produced today is
			// already on hand for orderableLines to pick from the same day.
			if err := s.maybeAssembleFinishedGoods(day); err != nil {
				s.onTaskError(day, fmt.Errorf("assembly: %w", err))
			}
			if err := s.maybeStartOrder(day); err != nil {
				s.onTaskError(day, fmt.Errorf("order: %w", err))
			}
			// maybeStartImport runs before maybeStartPurchaseOrder so a
			// newly opened import is already s.currentImport by the time
			// this same day's purchase orders decide whether to link to
			// it (see purchasing.go's createPurchaseOrder).
			if err := s.maybeStartImport(day); err != nil {
				s.onTaskError(day, fmt.Errorf("import: %w", err))
			}
			if err := s.maybeStartPurchaseOrder(day); err != nil {
				s.onTaskError(day, fmt.Errorf("purchase order: %w", err))
			}
			if err := s.maybeRestockAssemblyComponents(day); err != nil {
				s.onTaskError(day, fmt.Errorf("assembly restock: %w", err))
			}
		}

		if s.cfg.ProgressEvery > 0 && dayNum%s.cfg.ProgressEvery == 0 {
			s.log.Printf("seed-demo: %s (%d/%d days) — invoices=%d orders=%d deliveries=%d POs=%d receipts=%d bills=%d imports=%d assembled=%d payments=%d errors=%d",
				day.Format("2006-01-02"), dayNum, totalDays,
				s.stats.Invoices, s.stats.Orders, s.stats.Deliveries,
				s.stats.PurchaseOrders, s.stats.InboundDeliveries, s.stats.IncomingInvoices,
				s.stats.Imports, s.stats.AssembledUnits, s.stats.Payments, s.stats.Errors)
		}
	}

	// Anything still scheduled beyond EndDate is deliberately left
	// unresolved — those are today's genuinely-still-outstanding invoices
	// and bills, exactly what an aging report should have something to show.

	elapsed := time.Since(runStart)
	s.log.Printf("seed-demo: done in %s", elapsed.Round(time.Second))
	s.log.Printf("seed-demo: clients=%d vendors=%d products=%d", s.stats.Clients, s.stats.Vendors, s.stats.Products)
	s.log.Printf("seed-demo: invoices=%d orders=%d deliveries=%d", s.stats.Invoices, s.stats.Orders, s.stats.Deliveries)
	s.log.Printf("seed-demo: purchase_orders=%d inbound_deliveries=%d incoming_invoices=%d imports=%d", s.stats.PurchaseOrders, s.stats.InboundDeliveries, s.stats.IncomingInvoices, s.stats.Imports)
	s.log.Printf("seed-demo: assembly_batches=%d assembled_units=%d", s.stats.AssemblyBatches, s.stats.AssembledUnits)
	s.log.Printf("seed-demo: payments=%d errors=%d", s.stats.Payments, s.stats.Errors)
	if s.stats.Errors > 0 {
		s.log.Printf("seed-demo: WARNING — %d step(s) failed; see the log above for which day/kind. The rest of the run continued.", s.stats.Errors)
	}
	return nil
}

// onTaskError logs and counts a failed step but never aborts the run — a
// hiccup on one invoice out of several thousand shouldn't cost the rest of
// an unattended multi-minute run. abortAfter in scheduler.go/sales.go still
// applies for genuinely structural failures (see maybeAbort).
func (s *Seeder) onTaskError(day time.Time, err error) {
	s.stats.Errors++
	s.log.Printf("seed-demo: %s: %v", day.Format("2006-01-02"), err)
	s.maybeAbort()
}

// maybeAbort stops the whole run once errors are clearly structural (wrong
// credentials, server down mid-run, a schema this tool's assumptions no
// longer match) rather than grinding through a multi-minute run racking up
// thousands of identical failures.
func (s *Seeder) maybeAbort() {
	const abortThreshold = 25
	if s.stats.Errors == abortThreshold {
		s.log.Fatalf("seed-demo: aborting — %d errors in this run, which looks structural rather than incidental. Check the messages above.", abortThreshold)
	}
}

// --- small shared helpers -------------------------------------------------

func strPtr(s string) *string       { return &s }
func int64Ptr(v int64) *int64       { return &v }
func float64Ptr(v float64) *float64 { return &v }

// nonEmptyStrPtr is strPtr, except "" becomes nil — for genericOrgProfile's
// (masterdata.go) blank placeholder fields, where an actual empty string on
// the wire would set e.g. City to "" instead of leaving it unset.
func nonEmptyStrPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func midnightUTC(t time.Time) int64 {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC).UnixMilli()
}

// businessDaysLater adds n calendar days then snaps forward to the nearest
// weekday — every follow-up task (a payment, a receipt, a bill) lands on a
// day the "business" is actually open, matching how the day-stepper in Run
// only fires weekday-only generators.
func businessDaysLater(day time.Time, n int) time.Time {
	d := day.AddDate(0, 0, n)
	for d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
		d = d.AddDate(0, 0, 1)
	}
	return d
}
