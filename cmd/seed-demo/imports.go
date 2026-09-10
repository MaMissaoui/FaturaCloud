package main

import (
	"fmt"
	"time"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// importRef is this tool's own record of a created Import (F114 — a
// consolidated China shipment carrying several vendors' purchase orders,
// see db/import.go), kept for the whole linking window so
// maybeLinkToImport can decide whether a freshly created purchase order
// should attach to it, and finalizeImportCosts can later set a realistic
// freight/customs cost once every PO that's going to link to it has.
type importRef struct {
	id, number string
	createdOn  time.Time
	linkedPOs  int
	// committedValueCents is the running sum (qty × unitCost) of every PO
	// this tool linked to the import — the base finalizeImportCosts derives
	// freight/customs from, since a fixed absolute range can't track the
	// ~5x spread between an import that happens to link mostly cheap
	// hardware and one that links an engine-core-heavy PO (catalog.go's
	// componentTiers).
	committedValueCents int64
}

// importLinkWindowDays/importLinkMaxPOs bound how long an import stays
// "current" for new purchase orders to attach to: a real consolidated
// shipment collects a handful of vendor POs over the few weeks it's being
// assembled before it ships, not forever and not unboundedly many.
const (
	importLinkWindowDays = 21
	importLinkMaxPOs     = 5
)

// maybeStartImport opens a new consolidated shipment roughly once a month
// (checked every Monday alongside maybeStartOrder/maybeStartPurchaseOrder
// in seeder.go's Run — ~23% chance per Monday averages one import every
// ~4.3 weeks) and makes it the "current" one purchase orders can link to.
// Freight/customs start as a small placeholder — the real cost is set once
// the linking window closes, see finalizeImportCosts. Currency/ExchangeRate
// are left nil, matching this tool's existing "no multi-currency
// documents" scope boundary (README.md) rather than expanding it.
func (s *Seeder) maybeStartImport(day time.Time) error {
	if day.Weekday() != time.Monday || !s.rng.Chance(0.23) {
		return nil
	}
	req := db.CreateImportRequest{
		OrganizationID: s.orgID,
		ImportNumber:   s.importNum.next(day.Year()),
		Date:           midnightUTC(day),
		FreightCost:    50000,
		CustomsCost:    20000,
		Notes:          strPtr("Consolidated component shipment"),
	}
	var imp db.Import
	if err := s.c.Post("/api/imports", req, &imp); err != nil {
		return fmt.Errorf("create import: %w", err)
	}
	s.stats.Imports++
	ref := &importRef{id: imp.ID, number: imp.ImportNumber, createdOn: day}
	s.currentImport = ref

	finalizeDay := businessDaysLater(day, importLinkWindowDays)
	if !finalizeDay.After(s.cfg.EndDate) {
		s.sched.Schedule(finalizeDay, func() error { return s.finalizeImportCosts(ref) })
	}
	// If finalizeDay would fall after --end-date, the import simply keeps
	// its creation-time placeholder cost — the same "genuinely still open
	// as of today" reasoning schedulePayment uses for a payment that would
	// land in the future.
	return nil
}

// maybeLinkToImport is called by createPurchaseOrder (purchasing.go) for
// every newly placed purchase order of stock-tracked components — the only
// kind of PO this demo's imports ever carry, since stockProducts already
// excludes "finished" goods (purchasing.go). poValueCents is that PO's own
// Σ(quantity × unitCost), accumulated into the import's committedValueCents
// so finalizeImportCosts has something real to base freight/customs on.
// Returns nil when there's no current, still-open import to attach to,
// which is the common case — most purchase orders are ordinary domestic
// restocking, not part of a consolidated shipment. A linked import is what
// lets db/gl_posting.go's applyLandedCost spread freight/customs across the
// receipt once this PO is actually received.
func (s *Seeder) maybeLinkToImport(day time.Time, poValueCents int64) *string {
	imp := s.currentImport
	if imp == nil || imp.linkedPOs >= importLinkMaxPOs {
		return nil
	}
	if day.Sub(imp.createdOn) > importLinkWindowDays*24*time.Hour {
		return nil
	}
	if !s.rng.Chance(0.45) {
		return nil
	}
	imp.linkedPOs++
	imp.committedValueCents += poValueCents
	return &imp.id
}

// finalizeImportCosts sets the shipment's real freight/customs cost once
// the linking window has closed and every PO that's going to link to it
// already has — freight lands at 3-7% of the accumulated committed value
// and customs at 2-4%, roughly matching a real China shipment's transport
// cost usually exceeding its customs duty. An import nothing ever linked
// to (committedValueCents stays 0 — linking is chance-based, not
// guaranteed) keeps its small creation-time placeholder rather than being
// zeroed out, since a shipment with genuinely nothing aboard yet still
// cost something to book.
func (s *Seeder) finalizeImportCosts(imp *importRef) error {
	if imp.committedValueCents <= 0 {
		return nil
	}
	freight := float64(imp.committedValueCents) * s.rng.Float64Range(0.03, 0.07)
	customs := float64(imp.committedValueCents) * s.rng.Float64Range(0.02, 0.04)
	req := db.UpdateImportRequest{FreightCost: &freight, CustomsCost: &customs}
	if err := s.c.Put("/api/imports/"+imp.id, req, nil); err != nil {
		return fmt.Errorf("finalize import %s costs: %w", imp.number, err)
	}
	return nil
}
