package main

import (
	"fmt"
	"time"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// importRef is this tool's own record of a created Import (F114 — a
// consolidated China shipment carrying several foreign vendors' purchase
// orders, see db/import.go), kept for the whole linking window so
// maybeCreateImportLinkedPO knows whether to place another PO against it,
// and finalizeImportCosts can later set a realistic freight/customs cost
// once every PO going into it has been placed.
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
// ~4.3 weeks) and makes it the "current" one for maybeCreateImportLinkedPO
// to place foreign-vendor purchase orders against. Freight/customs start
// as a small placeholder — the real cost is set once the linking window
// closes, see finalizeImportCosts. The shipment itself carries no
// Currency/ExchangeRate of its own — those are prefill-only fields on
// Import (db/import.go) for the linked POs to optionally copy, and each PO
// still freezes its own, per the F114 design.
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

// maybeCreateImportLinkedPO places 1-2 foreign-vendor purchase orders a
// week against the currently open import, while its linking window is
// still open and it hasn't already hit importLinkMaxPOs — the only path
// that ever creates a PO against an import now: a consolidated shipment
// realistically carries goods from overseas manufacturers (see
// setupForeignVendors), never a domestic restocking order, so linking is
// no longer a coin-flip on an otherwise-ordinary local PO (createPurchaseOrder,
// purchasing.go, draws only from the local vendor pool and never sets
// ImportID at all).
func (s *Seeder) maybeCreateImportLinkedPO(day time.Time) error {
	imp := s.currentImport
	if imp == nil || day.Weekday() != time.Monday {
		return nil
	}
	if imp.linkedPOs >= importLinkMaxPOs {
		return nil
	}
	if day.Sub(imp.createdOn) > importLinkWindowDays*24*time.Hour {
		return nil
	}
	n := s.rng.IntRange(1, 2)
	for i := 0; i < n && imp.linkedPOs < importLinkMaxPOs; i++ {
		placeDay := businessDaysLater(day, s.rng.IntRange(0, 4))
		if placeDay.After(s.cfg.EndDate) {
			continue
		}
		imp.linkedPOs++
		s.sched.Schedule(placeDay, func() error { return s.createImportLinkedPurchaseOrder(placeDay, imp) })
	}
	return nil
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
