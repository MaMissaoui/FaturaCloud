package main

import (
	"errors"
	"fmt"
	"math"
	"net/http"
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
// assembleBatch (below) drives the real Production Order feature
// (db/production_order.go, PR #238/#240/#241) — create a draft order for
// the batch, then mark it completed, the same two-call flow the frontend's
// "Create" + "Mark as completed" buttons drive. Before Production Orders
// existed, this file simulated assembly with a pair of raw manual stock
// movements (POST /api/stock-movements) instead; that's gone now that
// there's a dedicated endpoint to call.

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

// setupBillsOfMaterials defines a real Bill of Materials (db/product_bom.go,
// PUT /api/products/{id}/bom) for every finished-good product, once, right
// after setupProducts (masterdata.go) has created them — the same curated
// per-displacement-class component set assemblyBOMComponentNames already
// names, now actually stored via the app's own BOM feature instead of only
// ever living as this tool's local assumption. assembleBatch below reads it
// back at consumption time (one GET per batch), so a seeded organization's
// Products page shows real, populated recipes, and the simulation is
// genuinely driven by that stored data rather than a parallel hardcoded
// list nothing else in the app can see.
func (s *Seeder) setupBillsOfMaterials() error {
	for _, finished := range s.products {
		if finished.category != "finished" {
			continue
		}
		components := s.bomComponentsByDisplacement(finished.displacement)
		if len(components) == 0 {
			continue
		}
		lines := make([]db.CreateBillOfMaterialsLineRequest, len(components))
		for i, c := range components {
			lines[i] = db.CreateBillOfMaterialsLineRequest{
				ComponentProductID: c.id,
				QuantityPerUnit:    assemblyBOM, // flat 1 per component — see assemblyBOM's own comment
			}
		}
		if err := s.c.Put("/api/products/"+finished.id+"/bom", map[string]any{"lines": lines}, nil); err != nil {
			return fmt.Errorf("bill of materials for %q: %w", finished.name, err)
		}
		s.stats.BillsOfMaterialsDefined++
	}
	return nil
}

// getProductBOM reads back a finished product's stored recipe — the real
// source assembleBatch consumes against, instead of re-deriving it locally
// the way this tool did before setupBillsOfMaterials existed.
func (s *Seeder) getProductBOM(productID string) ([]db.BillOfMaterialsLine, error) {
	var lines []db.BillOfMaterialsLine
	if err := s.c.Get("/api/products/"+productID+"/bom", &lines); err != nil {
		return nil, err
	}
	return lines, nil
}

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
	finished := s.finishedByDisplacement(class)
	if len(finished) == 0 {
		return nil
	}
	product := Pick(s.rng, finished)

	// Read this specific product's real BOM back (setupBillsOfMaterials
	// defined it once, at startup) instead of assuming every finished good
	// in the class shares the same local, hardcoded component list — this
	// tells us `buildable` below, even though the server independently
	// snapshots the same BOM into the order it creates a moment later.
	bom, err := s.getProductBOM(product.id)
	if err != nil {
		return fmt.Errorf("assembly %s: read BOM for %s: %w", class, product.name, err)
	}
	if len(bom) == 0 {
		return nil // no BOM defined for this product — nothing to build against
	}

	buildable := math.Inf(1)
	for _, line := range bom {
		if b := s.onHand(line.ComponentProductID) / line.QuantityPerUnit; b < buildable {
			buildable = b
		}
	}
	batch := int(math.Floor(buildable))
	if batch < 1 {
		return nil // some BOM component isn't stocked yet — nothing to build
	}
	if batch > maxAssemblyBatch {
		batch = maxAssemblyBatch
	}

	// Create the draft order, then complete it in the same call — mirrors
	// the frontend's "Create" followed by "Mark as completed" (there's no
	// reason for this tool to leave a batch sitting in draft). The server
	// snapshots product's own BOM (already read above just to compute
	// `batch`) into the order's component lines itself; this doesn't send
	// them.
	createReq := db.CreateProductionOrderRequest{
		OrganizationID:    s.orgID,
		OrderNumber:       s.productionOrderNum.next(day.Year()),
		FinishedProductID: product.id,
		Quantity:          float64(batch),
		Date:              midnightUTC(day),
	}
	var order db.ProductionOrder
	if err := s.c.Post("/api/production-orders", createReq, &order); err != nil {
		return fmt.Errorf("assembly %s: create production order for %s: %w", class, product.name, err)
	}
	s.stats.ProductionOrders++

	// Completing enforces a real availability check (line.TotalQuantity >
	// component.StockQuantity, db/production_order.go), unlike the old raw
	// "adjustment" movements this replaced, which had none. s.onHand is
	// only a local estimate (see stock.go) — a 409 here means it's drifted
	// from server truth, not that anything is actually broken, so this
	// batch is skipped (the draft order is left behind as a harmless
	// trace) rather than failing the whole run.
	if err := s.c.Patch("/api/production-orders/"+order.ID+"/status", map[string]string{"status": "completed"}, nil); err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.Status == http.StatusConflict {
			s.log.Printf("assembly %s: skipped completing %s for %s (on-hand estimate drifted from server stock): %v", class, order.OrderNumber, product.name, err)
			return nil
		}
		return fmt.Errorf("assembly %s: complete production order %s: %w", class, order.OrderNumber, err)
	}

	for _, line := range bom {
		s.adjustOnHand(line.ComponentProductID, -line.QuantityPerUnit*float64(batch))
	}
	s.adjustOnHand(product.id, float64(batch))

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
