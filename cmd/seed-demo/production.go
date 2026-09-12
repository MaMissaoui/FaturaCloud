package main

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// This file is the "assembly" step: the piece the original demo-data design
// was missing. Before this, purchasing.go restocked "component" products
// and sales.go only ever sold/shipped "finished" motorcycles when they had
// on-hand stock — but nothing anywhere ever produced a finished motorcycle
// in the first place, so onHand for every one of the 25 "finished" products
// stayed at zero for the life of a run. orderableLines (sales.go) always
// fell back to services, no outbound delivery ever shipped a stock-enabled
// finished good, and buildDeliveryCOGSGLLines (db/gl_posting.go) — the only
// thing that ever posts to an Expense account in this demo — never fired.
// That's why every seeded organization's Profit & Loss showed real revenue
// but permanently empty expenses.
//
// The app itself has no production/assembly/BOM feature (CLAUDE.md's
// cmd/seed-demo entry: no such module exists), so this can't call a
// dedicated endpoint — it simulates assembly the same way a real user would
// have to today: a pair of manual stock movements (POST /api/stock-movements)
// per batch, one consuming components and one producing the finished good.

// assemblyBOMComponentNames is a curated, representative subset of
// catalog.go's 55 componentTemplates — not all of them. Requiring every
// single one of the 55 (one instance per displacement class) turned out
// unworkable when first tried: purchasing.go's stockProducts() restocks by
// drawing uniformly from all 275 components, so getting all 55 of one
// class simultaneously in stock is a coupon-collector problem this tool's
// PO volume doesn't reliably clear even over a full 18-month "busy" run,
// let alone "small". This list instead names one or two parts per tier
// (engine core down to small hardware — see catalog.go's componentTier
// vars) that a real BOM couldn't skip, and maybeRestockAssemblyComponents
// below orders them directly and predictably rather than leaving their
// supply to chance the way ordinary restocking does for the other ~43
// components per class, which remain purchasable but play no role here.
var assemblyBOMComponentNames = []string{
	"Engine block", "Frame chassis", // tierEngineCore
	"Front fork assembly", "Rear shock absorber", "Wheel rim", "Fuel tank", "Wiring harness", // tierMajorAssembly
	"Battery",                             // tierMidComponent
	"Headlight assembly",                  // tierElectricalSmall
	"Brake pad set", "Tire", "Spark plug", // tierSmallHardware
}

var assemblyBOMComponentSet = func() map[string]bool {
	m := make(map[string]bool, len(assemblyBOMComponentNames))
	for _, n := range assemblyBOMComponentNames {
		m[n] = true
	}
	return m
}()

// assemblyBOM is how many units of each BOM component one finished unit
// consumes — flat across every part, chosen for simplicity over literal
// engineering accuracy (a real bike needs 2 tires, not 1) the same way
// catalog.go's own comments accept elsewhere in this tool.
const assemblyBOM = 1

// maxAssemblyBatch caps how many units one assembly run will build, so a
// well-stocked week doesn't drain a BOM component to zero in one shot —
// assembly draws down stock gradually, the same "restock happens in bulk,
// consumption happens gradually" shape purchasing.go/sales.go already have.
// It doubles as the buffer-stock target maybeRestockAssemblyComponents
// orders back up to.
const maxAssemblyBatch = 15

// maybeRestockAssemblyComponents runs once a week (Monday, alongside every
// other weekly-cadence decision in this tool) and tops up each
// displacement class's BOM components back up to maxAssemblyBatch — a
// dedicated procurement plan for the parts this tool specifically depends
// on, on top of whatever ordinary purchasing separately (and randomly) buys
// of the same products. Skips a class already holding a full buffer of
// every BOM component, so this doesn't place a purchase order every single
// week once the organization is established.
func (s *Seeder) maybeRestockAssemblyComponents(day time.Time) error {
	if day.Weekday() != time.Monday {
		return nil
	}
	for _, class := range displacementClasses {
		components := s.bomComponentsByDisplacement(class.label)
		if len(components) == 0 {
			continue
		}
		min := math.Inf(1)
		for _, c := range components {
			if h := s.onHand(c.id); h < min {
				min = h
			}
		}
		if min >= float64(maxAssemblyBatch) {
			continue
		}
		if err := s.restockAssemblyComponentsForClass(day, class.label, components); err != nil {
			return err
		}
	}
	return nil
}

// restockAssemblyComponentsForClass places one purchase order covering
// every BOM component of one displacement class in generous bulk, then
// schedules its receipt (and, from there, its bill) via
// receivePurchaseOrder — the exact same downstream chain purchasing.go's
// own createPurchaseOrder uses, so GRNI/GL/AP posting is identical; only
// which products get ordered and how the order is decided differs.
func (s *Seeder) restockAssemblyComponentsForClass(day time.Time, class string, components []productRef) error {
	vendor := Pick(s.rng, s.vendors)

	var reqLines []db.CreatePurchaseOrderLineItemRequest
	var localLines []purchaseLine
	for _, p := range components {
		qty := float64(s.rng.IntRange(30, 90)) // generous restock — this tool depends on these staying in stock
		reqLines = append(reqLines, db.CreatePurchaseOrderLineItemRequest{
			ProductID:   strPtr(p.id),
			Description: p.name,
			Quantity:    qty,
			UnitPrice:   float64(p.costCents),
			Unit:        strPtr(p.unit),
		})
		localLines = append(localLines, purchaseLine{productID: p.id, productName: p.name, unit: p.unit, quantity: qty, unitCost: p.costCents})
	}

	req := db.CreatePurchaseOrderRequest{
		OrganizationID: s.orgID,
		VendorID:       &vendor.id,
		OrderNumber:    s.poNum.next(day.Year()),
		Status:         "draft",
		OrderDate:      midnightUTC(day),
		LineItems:      reqLines,
	}
	var po db.PurchaseOrder
	if err := s.c.Post("/api/purchase-orders", req, &po); err != nil {
		return fmt.Errorf("assembly restock PO (%s) for %s: %w", class, vendor.name, err)
	}
	s.stats.PurchaseOrders++
	if err := s.c.Patch("/api/purchase-orders/"+po.ID+"/status", map[string]string{"status": "confirmed"}, nil); err != nil {
		return fmt.Errorf("confirm assembly restock PO %s: %w", po.OrderNumber, err)
	}

	var serverLines []db.PurchaseOrderLineItem
	if err := s.c.Get("/api/purchase-orders/"+po.ID+"/line-items", &serverLines); err != nil {
		return fmt.Errorf("read back assembly restock PO %s line items: %w", po.OrderNumber, err)
	}
	if len(serverLines) != len(localLines) {
		return fmt.Errorf("assembly restock PO %s: expected %d line items back, got %d", po.OrderNumber, len(localLines), len(serverLines))
	}
	for i := range localLines {
		localLines[i].poLineID = serverLines[i].ID
	}

	// Shorter, tighter lead time than ordinary restocking (purchasing.go
	// waits 3-14 business days) — assembly's own weekly cadence depends on
	// these actually arriving promptly, not on the same wide realistic
	// spread a generic restock can afford.
	receiveDay := businessDaysLater(day, s.rng.IntRange(2, 6))
	if receiveDay.After(s.cfg.EndDate) {
		return nil
	}
	s.sched.Schedule(receiveDay, func() error { return s.receivePurchaseOrder(receiveDay, po, vendor, localLines) })
	return nil
}

// maybeAssembleFinishedGoods runs once a week (Wednesday — after Monday's
// restock has had a couple of days to be scheduled, though the actual
// receipt lands via the scheduler independently) and attempts one assembly
// batch per displacement class. Cheap to call even when nothing is
// buildable yet: the buildable-count check is pure local arithmetic over
// s.products, and no HTTP call happens unless a batch actually clears the
// ">= 1 of every BOM component" bar — true for a run's first couple of
// weeks, before the new dedicated restocking above has delivered anything.
func (s *Seeder) maybeAssembleFinishedGoods(day time.Time) error {
	if day.Weekday() != time.Wednesday {
		return nil
	}
	for _, class := range displacementClasses {
		if err := s.assembleBatch(day, class.label); err != nil {
			return err
		}
	}
	return nil
}

// assembleBatch builds as many finished units of one displacement class as
// the scarcest BOM component of that class currently allows (capped at
// maxAssemblyBatch), all assigned to one randomly chosen model line for
// this batch — a different Wednesday will likely pick a different line, so
// variety comes from many batches over the run rather than splitting one
// batch across several SKUs.
func (s *Seeder) assembleBatch(day time.Time, class string) error {
	components := s.bomComponentsByDisplacement(class)
	finished := s.finishedByDisplacement(class)
	if len(components) == 0 || len(finished) == 0 {
		return nil
	}

	buildable := math.Inf(1)
	for _, c := range components {
		if h := s.onHand(c.id); h < buildable {
			buildable = h
		}
	}
	batch := int(math.Floor(buildable / assemblyBOM))
	if batch < 1 {
		return nil // some BOM component in this class isn't stocked yet — nothing to build
	}
	if batch > maxAssemblyBatch {
		batch = maxAssemblyBatch
	}

	product := Pick(s.rng, finished)
	ref := fmt.Sprintf("ASM-%s-%s", class, day.Format("20060102"))

	// Consume assemblyBOM units of every BOM component. Each is a plain
	// "out" adjustment — no UnitCost is sent, since a negative-quantity
	// movement is always costed from the product's own average
	// (db/gl_posting.go's buildStockAdjustmentGLLines), which is exactly
	// componentRef.costCents here (every receipt for a given component
	// always uses that same fixed catalog cost — see
	// restockAssemblyComponentsForClass/purchasing.go's createPurchaseOrder
	// — so its weighted average never actually varies).
	var partsCostPerUnit int64
	for _, c := range components {
		qty := float64(batch * assemblyBOM)
		req := db.CreateStockMovementRequest{
			OrganizationID: s.orgID,
			ProductID:      c.id,
			Type:           "adjustment",
			Quantity:       -qty,
			Note:           strPtr(fmt.Sprintf("Assembly: consumed for %d x %s (%s)", batch, product.name, class)),
			Reference:      strPtr(ref),
		}
		if err := s.c.Post("/api/stock-movements", req, nil); err != nil {
			return fmt.Errorf("assembly %s: consume %s: %w", ref, c.name, err)
		}
		s.adjustOnHand(c.id, -qty)
		partsCostPerUnit += c.costCents * assemblyBOM
	}

	// Produce the finished good at exactly the parts cost just consumed —
	// no assembly-labor markup invented on top of it. That keeps this
	// transformation's net effect on the organization's Inventory
	// Adjustment account at ~zero (the only account a manual movement can
	// target — see db/gl_posting.go): the credit side from consuming
	// components and the debit side from producing the finished good are
	// the same total value, just moved between SKUs, not a fabricated gain.
	// This also supersedes the finished good's initial catalog-random
	// UnitCost (masterdata.go) with a real, components-based cost the
	// first time any batch for its displacement class runs —
	// db/product_cost.go's recomputeAverageCostTx only ever averages a
	// product's own costed inflows, and before this, a finished good had
	// none.
	produceQty := float64(batch)
	produceReq := db.CreateStockMovementRequest{
		OrganizationID: s.orgID,
		ProductID:      product.id,
		Type:           "adjustment",
		Quantity:       produceQty,
		UnitCost:       int64Ptr(partsCostPerUnit),
		Note:           strPtr(fmt.Sprintf("Assembly: %d unit(s) produced from %s components", batch, class)),
		Reference:      strPtr(ref),
	}
	if err := s.c.Post("/api/stock-movements", produceReq, nil); err != nil {
		return fmt.Errorf("assembly %s: produce %s: %w", ref, product.name, err)
	}
	s.adjustOnHand(product.id, produceQty)

	s.stats.AssemblyBatches++
	s.stats.AssembledUnits += batch
	return nil
}

// bomComponentsByDisplacement returns this class's instances of
// assemblyBOMComponentNames — e.g. "Engine block (125cc)" for class
// "125cc" — by stripping the "(<class>)" suffix catalog.go's
// buildComponentCatalog appends and checking the base name against the set.
// finishedByDisplacement is the finished-goods equivalent, unfiltered by
// name since every "finished" entry for a class is an equally valid build
// target (just a different model line — see catalog.go's motorcycleModelLines).
func (s *Seeder) bomComponentsByDisplacement(class string) []productRef {
	suffix := fmt.Sprintf(" (%s)", class)
	var out []productRef
	for _, p := range s.products {
		if p.category != "component" || p.displacement != class {
			continue
		}
		if assemblyBOMComponentSet[strings.TrimSuffix(p.name, suffix)] {
			out = append(out, p)
		}
	}
	return out
}

func (s *Seeder) finishedByDisplacement(class string) []productRef {
	var out []productRef
	for _, p := range s.products {
		if p.category == "finished" && p.displacement == class {
			out = append(out, p)
		}
	}
	return out
}
