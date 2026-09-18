package main

// scenario is the one place that says what kind of business this run
// simulates. Everything else in this tool (the HTTP client, RNG/locale
// data, scheduler, totals math, org/tax-rate/fiscal-year setup, and the
// whole purchase-order -> receipt -> bill -> pay chain) has no business-type
// assumption baked in — only the catalog and which optional generators run
// actually differ between "a motorcycle assembler" and "a home-appliance
// retailer". Resolved once in NewSeeder from --scenario; Run() reads the
// booleans below instead of calling every generator unconditionally.
type scenario struct {
	name string

	productCatalog []productCatalogEntry
	serviceCatalog []productCatalogEntry

	// hasProduction gates setupBillsOfMaterials/maybeAssembleFinishedGoods/
	// maybeRestockAssemblyComponents (production.go) — meaningless for a
	// retailer that buys and resells finished goods rather than assembling
	// them. These would already no-op safely against a catalog with no
	// "finished"/"component" entries (every loop bails on an empty slice),
	// but a retail run skips calling them at all rather than relying on
	// that.
	hasProduction bool
	// hasImports gates setupForeignVendors/maybeStartImport/
	// maybeCreateImportLinkedPO (imports.go) — the retail scenario is
	// domestic-vendors-only for this first version (a deliberate scope
	// decision, not a gap: F114 imports are a realistic extension for a
	// later pass).
	hasImports bool
	// hasDirectInvoiceSales/hasOrderSales gate sales.go's two ordinary
	// B2B-style channels (createDirectInvoice, order->delivery->invoice).
	// The retail scenario's only sales channel is Cash Book
	// (hasCashBookSales, cash_book_sales.go) — a deliberate choice so this
	// dataset stays focused on exercising Cash Book/Cash Register at
	// realistic volume rather than diluting it with a second channel.
	hasDirectInvoiceSales bool
	hasOrderSales         bool
	hasCashBookSales      bool

	// vendors/purchaseOrdersPerWeek replace volumeProfile.Vendors/
	// PurchaseOrdersPerWeek for a scenario that doesn't use the small/busy
	// profile at all (retail has no "Clients" of its own — see
	// cash_book_sales.go's organic client-growth model instead).
	vendors               int
	purchaseOrdersPerWeek [2]int
}

// vendorCount and purchaseOrdersPerWeekRange resolve the effective value
// for either scenario: retail's own scenario.vendors/purchaseOrdersPerWeek
// when set, otherwise the small/busy volumeProfile every "moto" run has
// always used. Centralized here rather than an "if scenario == retail" at
// each call site (setupVendors, maybeStartPurchaseOrder). s.profile is
// already scaled by --volume-scale (NewSeeder); retail's own fixed
// scenario.vendors/purchaseOrdersPerWeek are scaled here instead, the same
// way, so --volume-scale reduces retail's master/transactional volume too.
func (s *Seeder) vendorCount() int {
	if s.scenario.vendors > 0 {
		return scaleCount(s.scenario.vendors, s.cfg.VolumeScale)
	}
	return s.profile.Vendors
}

func (s *Seeder) purchaseOrdersPerWeekRange() [2]int {
	if s.scenario.purchaseOrdersPerWeek != [2]int{} {
		return scaleCountRange(s.scenario.purchaseOrdersPerWeek, s.cfg.VolumeScale)
	}
	return s.profile.PurchaseOrdersPerWeek
}

func resolveScenario(name string) scenario {
	switch name {
	case "retail":
		return scenario{
			name:                  "retail",
			productCatalog:        buildApplianceCatalog(),
			serviceCatalog:        retailServiceCatalog,
			hasProduction:         false,
			hasImports:            false,
			hasDirectInvoiceSales: false,
			hasOrderSales:         false,
			hasCashBookSales:      true,
			vendors:               20,
			purchaseOrdersPerWeek: [2]int{4, 7},
		}
	default:
		return scenario{
			name:                  "moto",
			productCatalog:        productCatalog,
			serviceCatalog:        serviceCatalog,
			hasProduction:         true,
			hasImports:            true,
			hasDirectInvoiceSales: true,
			hasOrderSales:         true,
			hasCashBookSales:      false,
		}
	}
}
