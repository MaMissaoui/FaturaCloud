package api

import (
	"testing"
)

// wantSectionRoutes is the maintained mirror of api/sections.go's classifier:
// every route in router.go that is section-scoped, and the section it belongs
// to. TestSectionRouteCoverage below compares it against what routeSections
// actually returns for every route discoverRoutes parses out of router.go, in
// both directions — so a new route that gets classified must be listed here,
// a listed route that stops being classified fails, and a route that silently
// changes section fails. Same tripwire philosophy as domainRouteRoles in
// cross_org_test.go.
//
// It also deliberately does NOT list the shared (unrestricted) routes: those
// are asserted separately in TestSharedRoutesAreUnrestricted, so a route
// moving from shared to scoped (or back) is a decision, not an accident.
var wantSectionRoutes = map[string]string{
	// Sales
	"DELETE /api/deliveries/{id}":                           sectionSales,
	"DELETE /api/invoices/{id}":                             sectionSales,
	"DELETE /api/orders/{id}":                               sectionSales,
	"GET /api/deliveries/{id}":                              sectionSales,
	"GET /api/deliveries/{id}/export":                       sectionSales,
	"GET /api/deliveries/{id}/line-items":                   sectionSales,
	"GET /api/invoices/{id}":                                sectionSales,
	"GET /api/invoices/{id}/e-invoice":                      sectionSales,
	"GET /api/invoices/{id}/export":                         sectionSales,
	"GET /api/invoices/{id}/line-items":                     sectionSales,
	"GET /api/orders/{id}":                                  sectionSales,
	"GET /api/orders/{id}/delivered-quantities":             sectionSales,
	"GET /api/orders/{id}/export":                           sectionSales,
	"GET /api/orders/{id}/line-items":                       sectionSales,
	"GET /api/organizations/{orgId}/deliveries":             sectionSales,
	"GET /api/organizations/{orgId}/deliveries/next-number": sectionSales,
	"GET /api/organizations/{orgId}/invoices":               sectionSales,
	"GET /api/organizations/{orgId}/orders":                 sectionSales,
	"GET /api/organizations/{orgId}/orders/next-number":     sectionSales,
	"PATCH /api/deliveries/{id}/status":                     sectionSales,
	"PATCH /api/invoices/{id}/state":                        sectionSales,
	"PATCH /api/orders/{id}/status":                         sectionSales,
	"POST /api/deliveries":                                  sectionSales,
	"POST /api/invoices":                                    sectionSales,
	"POST /api/orders":                                      sectionSales,
	"PUT /api/deliveries/{id}":                              sectionSales,
	"PUT /api/invoices/{id}":                                sectionSales,
	"PUT /api/orders/{id}":                                  sectionSales,

	// Purchasing
	"DELETE /api/inbound-deliveries/{id}":                              sectionPurchasing,
	"DELETE /api/incoming-invoices/{id}":                               sectionPurchasing,
	"DELETE /api/purchase-orders/{id}":                                 sectionPurchasing,
	"GET /api/inbound-deliveries/{id}":                                 sectionPurchasing,
	"GET /api/inbound-deliveries/{id}/export":                          sectionPurchasing,
	"GET /api/inbound-deliveries/{id}/line-items":                      sectionPurchasing,
	"GET /api/incoming-invoices/{id}":                                  sectionPurchasing,
	"GET /api/incoming-invoices/{id}/export":                           sectionPurchasing,
	"GET /api/incoming-invoices/{id}/line-items":                       sectionPurchasing,
	"GET /api/incoming-invoices/{id}/match":                            sectionPurchasing,
	"GET /api/organizations/{orgId}/inbound-deliveries":                sectionPurchasing,
	"GET /api/organizations/{orgId}/inbound-deliveries/next-number":    sectionPurchasing,
	"GET /api/organizations/{orgId}/incoming-invoices":                 sectionPurchasing,
	"GET /api/organizations/{orgId}/incoming-invoices/match-summaries": sectionPurchasing,
	"GET /api/organizations/{orgId}/purchase-orders":                   sectionPurchasing,
	"GET /api/organizations/{orgId}/purchase-orders/next-number":       sectionPurchasing,
	"GET /api/purchase-orders/{id}":                                    sectionPurchasing,
	"GET /api/purchase-orders/{id}/export":                             sectionPurchasing,
	"GET /api/purchase-orders/{id}/line-items":                         sectionPurchasing,
	"GET /api/purchase-orders/{id}/received-quantities":                sectionPurchasing,
	"PATCH /api/inbound-deliveries/{id}/status":                        sectionPurchasing,
	"PATCH /api/incoming-invoices/{id}/state":                          sectionPurchasing,
	"PATCH /api/purchase-orders/{id}/status":                           sectionPurchasing,
	"POST /api/inbound-deliveries":                                     sectionPurchasing,
	"POST /api/incoming-invoices":                                      sectionPurchasing,
	"POST /api/purchase-orders":                                        sectionPurchasing,
	"PUT /api/inbound-deliveries/{id}":                                 sectionPurchasing,
	"PUT /api/incoming-invoices/{id}":                                  sectionPurchasing,
	"PUT /api/purchase-orders/{id}":                                    sectionPurchasing,

	// Imports
	"DELETE /api/imports/{id}":                           sectionImports,
	"GET /api/imports/{id}":                              sectionImports,
	"GET /api/imports/{id}/summary":                      sectionImports,
	"GET /api/organizations/{orgId}/imports":             sectionImports,
	"GET /api/organizations/{orgId}/imports/next-number": sectionImports,
	"GET /api/organizations/{orgId}/imports/summaries":   sectionImports,
	"POST /api/imports":                                  sectionImports,
	"PUT /api/imports/{id}":                              sectionImports,

	// Inventory
	"DELETE /api/stock-movements/{id}":               sectionInventory,
	"GET /api/organizations/{orgId}/stock-movements": sectionInventory,
	"POST /api/stock-movements":                      sectionInventory,

	// Production orders
	"DELETE /api/production-orders/{id}":                           sectionProductionOrders,
	"GET /api/organizations/{orgId}/production-orders":             sectionProductionOrders,
	"GET /api/organizations/{orgId}/production-orders/next-number": sectionProductionOrders,
	"GET /api/production-orders/{id}":                              sectionProductionOrders,
	"GET /api/production-orders/{id}/component-lines":              sectionProductionOrders,
	"PATCH /api/production-orders/{id}/status":                     sectionProductionOrders,
	"POST /api/production-orders":                                  sectionProductionOrders,

	// Bill of materials
	"GET /api/organizations/{orgId}/products/bom-summaries":    sectionBillOfMaterials,
	"GET /api/products/{id}/bom":                               sectionBillOfMaterials,
	"GET /api/products/{id}/bom/versions":                      sectionBillOfMaterials,
	"GET /api/products/{id}/bom/versions/{versionId}":          sectionBillOfMaterials,
	"POST /api/products/{id}/bom/versions/{versionId}/restore": sectionBillOfMaterials,
	"PUT /api/products/{id}/bom":                               sectionBillOfMaterials,

	// Accounting
	"DELETE /api/journal-entries/{id}":               sectionAccounting,
	"DELETE /api/journals/{id}":                      sectionAccounting,
	"GET /api/fiscal-years/{id}/periods":             sectionAccounting,
	"GET /api/journal-entries/{id}":                  sectionAccounting,
	"GET /api/journal-entries/{id}/lines":            sectionAccounting,
	"GET /api/journal-entries/{id}/reversal":         sectionAccounting,
	"GET /api/organizations/{orgId}/fiscal-years":    sectionAccounting,
	"GET /api/organizations/{orgId}/gl-export/datev": sectionAccounting,
	"GET /api/organizations/{orgId}/gl-export/fec":   sectionAccounting,
	"GET /api/organizations/{orgId}/journal-entries": sectionAccounting,
	"GET /api/organizations/{orgId}/journals":        sectionAccounting,
	"PATCH /api/fiscal-periods/{id}/status":          sectionAccounting,
	"PATCH /api/journal-entries/{id}/post":           sectionAccounting,
	"POST /api/fiscal-periods":                       sectionAccounting,
	"POST /api/fiscal-years":                         sectionAccounting,
	"POST /api/fiscal-years/{id}/close":              sectionAccounting,
	"POST /api/journal-entries":                      sectionAccounting,
	"POST /api/journal-entries/{id}/reverse":         sectionAccounting,
	"POST /api/journals":                             sectionAccounting,
	"PUT /api/journals/{id}":                         sectionAccounting,

	// Accounting — chart-of-accounts writes only (reads are shared)
	"DELETE /api/accounts/{id}":                       sectionAccountingWrite,
	"POST /api/accounts":                              sectionAccountingWrite,
	"POST /api/organizations/{orgId}/accounts/import": sectionAccountingWrite,
	"PUT /api/accounts/{id}":                          sectionAccountingWrite,

	// Cash Book
	"POST /api/cash-movements": sectionCashbook,
	"POST /api/cash-sales":     sectionCashbook,

	// Reporting — sales analytics
	"GET /api/organizations/{orgId}/reporting/revenue-trend":    sectionReportSales,
	"GET /api/organizations/{orgId}/reporting/sales-by-client":  sectionReportSales,
	"GET /api/organizations/{orgId}/reporting/sales-by-product": sectionReportSales,

	// Reporting — purchasing analytics
	"GET /api/organizations/{orgId}/reporting/purchases-by-vendor": sectionReportPurchasing,

	// Reporting — tax summary
	"GET /api/organizations/{orgId}/reporting/tax-summary": sectionReportTaxSummary,

	// Reports — GL-derived (Accounting group)
	"GET /api/organizations/{orgId}/reports/account-balance":     sectionReportGL,
	"GET /api/organizations/{orgId}/reports/ap-aging":            sectionReportGL,
	"GET /api/organizations/{orgId}/reports/ar-aging":            sectionReportGL,
	"GET /api/organizations/{orgId}/reports/balance-sheet":       sectionReportGL,
	"GET /api/organizations/{orgId}/reports/inventory-valuation": sectionReportGL,
	"GET /api/organizations/{orgId}/reports/profit-and-loss":     sectionReportGL,
	"GET /api/organizations/{orgId}/reports/trial-balance":       sectionReportGL,

	// Reports — cash book (Accounting group + Cash Book screen)
	"GET /api/organizations/{orgId}/reports/cash-movement-details":       sectionReportCashbook,
	"GET /api/organizations/{orgId}/reports/daily-cash-movements":        sectionReportCashbook,
	"GET /api/organizations/{orgId}/reports/daily-cash-movements/export": sectionReportCashbook,
	"GET /api/organizations/{orgId}/reports/payment-history/export":      sectionReportCashbook,

	// Reports — loan status (Cash Book screen)
	"GET /api/organizations/{orgId}/reports/loan-status":        sectionReportLoan,
	"GET /api/organizations/{orgId}/reports/loan-status/export": sectionReportLoan,
}

func TestSectionRouteCoverage(t *testing.T) {
	routes := discoverRoutes(t)

	for key := range routes {
		pattern := key.String()
		sections := routeSections(pattern)
		want, listed := wantSectionRoutes[pattern]

		switch {
		case !listed && len(sections) == 0:
			// Unrestricted (shared) route — expected, see the shared list test.
		case !listed:
			t.Errorf("%s is classified as %v by routeSections but isn't in wantSectionRoutes — add it (or fix the classifier)", pattern, sections)
		case len(sections) == 0:
			t.Errorf("%s is listed as %q but routeSections no longer classifies it", pattern, want)
		case len(sections) != 1 || sections[0] != want:
			t.Errorf("%s: want %q, got %v", pattern, want, sections)
		}
	}

	// The reverse direction: nothing listed that router.go doesn't register.
	for pattern, want := range wantSectionRoutes {
		method, path := splitPattern(pattern)
		if _, ok := routes[routeKey{method: method, pattern: path}]; !ok {
			t.Errorf("wantSectionRoutes lists %s (%q) but router.go has no such route", pattern, want)
		}
	}
}

// TestSharedRoutesAreUnrestricted pins the routes deliberately left at
// membership level because a screen a restricted role keeps reads them — see
// api/sections.go's file comment. Moving any of these into a section is a
// product decision, not an oversight.
func TestSharedRoutesAreUnrestricted(t *testing.T) {
	shared := []string{
		// Master data
		"GET /api/organizations/{orgId}/clients",
		"POST /api/clients",
		"GET /api/clients/{id}",
		"PUT /api/clients/{id}",
		"DELETE /api/clients/{id}",
		"GET /api/organizations/{orgId}/vendors",
		"POST /api/vendors",
		"GET /api/organizations/{orgId}/products",
		"POST /api/products",
		"GET /api/products/{id}",
		"PUT /api/products/{id}",
		"DELETE /api/products/{id}",
		"GET /api/products/{id}/stock-movements",
		"GET /api/products/{id}/serial-numbers",
		"GET /api/organizations/{orgId}/tax-rates",
		"GET /api/organizations/{orgId}/payment-terms",
		"GET /api/organizations/{orgId}/units-of-measure",
		// Accounts reads (writes are sectionAccountingWrite)
		"GET /api/organizations/{orgId}/accounts",
		"GET /api/accounts/{id}",
		"GET /api/organizations/{orgId}/accounts/export",
		// Payments — every route
		"GET /api/organizations/{orgId}/payments",
		"POST /api/payments",
		"GET /api/payments/{id}",
		"GET /api/payments/{id}/applications",
		"POST /api/payments/{id}/void",
		"GET /api/invoices/{id}/payments",
		"GET /api/incoming-invoices/{id}/payments",
		// Dashboard / shared config
		"GET /api/organizations/{orgId}/dashboard",
		"GET /api/organizations/{orgId}/exchange-rate",
		"GET /api/organizations/{orgId}/document-templates",
		"GET /api/organizations/{orgId}/document-number-settings/{documentType}",
		"GET /api/organizations/{orgId}/members",
	}
	for _, pattern := range shared {
		if sections := routeSections(pattern); len(sections) != 0 {
			t.Errorf("%s should be unrestricted, got %v", pattern, sections)
		}
	}
}

// TestSectionRoleAccess exercises the role matrix end to end through
// routeAllowedForRole: the roles that may call a representative route per
// section, and the roles that may not.
func TestSectionRoleAccess(t *testing.T) {
	allRoles := []string{"admin", "power_user", "general", "sales", "purchasing", "accounting", "cashbook"}

	cases := []struct {
		pattern string
		allowed map[string]bool
	}{
		// general loses Accounting, Imports, Bill of Materials, Production Orders.
		{"GET /api/organizations/{orgId}/journals", map[string]bool{"accounting": true}},
		{"GET /api/organizations/{orgId}/journal-entries", map[string]bool{"accounting": true}},
		{"GET /api/organizations/{orgId}/fiscal-years", map[string]bool{"accounting": true}},
		{"POST /api/journal-entries", map[string]bool{"accounting": true}},
		{"GET /api/organizations/{orgId}/gl-export/fec", map[string]bool{"accounting": true}},
		{"GET /api/organizations/{orgId}/imports", map[string]bool{"purchasing": true}},
		{"POST /api/imports", map[string]bool{"purchasing": true}},
		{"GET /api/organizations/{orgId}/products/bom-summaries", map[string]bool{}},
		{"GET /api/products/{id}/bom", map[string]bool{}},
		{"PUT /api/products/{id}/bom", map[string]bool{}},
		{"GET /api/organizations/{orgId}/production-orders", map[string]bool{}},
		{"POST /api/production-orders", map[string]bool{}},

		// The domain sections.
		{"GET /api/organizations/{orgId}/invoices", map[string]bool{"general": true, "sales": true}},
		{"POST /api/orders", map[string]bool{"general": true, "sales": true}},
		{"GET /api/organizations/{orgId}/purchase-orders", map[string]bool{"general": true, "purchasing": true}},
		{"POST /api/purchase-orders", map[string]bool{"general": true, "purchasing": true}},
		{"GET /api/organizations/{orgId}/stock-movements", map[string]bool{"general": true}},
		{"POST /api/stock-movements", map[string]bool{"general": true}},
		{"POST /api/cash-sales", map[string]bool{"general": true, "cashbook": true}},
		{"POST /api/cash-movements", map[string]bool{"general": true, "cashbook": true}},

		// Chart-of-accounts writes only; reads are shared.
		{"POST /api/accounts", map[string]bool{"accounting": true}},
		{"PUT /api/accounts/{id}", map[string]bool{"accounting": true}},
		{"GET /api/organizations/{orgId}/accounts", map[string]bool{"general": true, "sales": true, "purchasing": true, "accounting": true, "cashbook": true}},

		// Reports.
		{"GET /api/organizations/{orgId}/reporting/revenue-trend", map[string]bool{"general": true, "sales": true}},
		{"GET /api/organizations/{orgId}/reporting/purchases-by-vendor", map[string]bool{"general": true, "purchasing": true}},
		{"GET /api/organizations/{orgId}/reporting/tax-summary", map[string]bool{"general": true, "accounting": true}},
		{"GET /api/organizations/{orgId}/reports/trial-balance", map[string]bool{"accounting": true}},
		{"GET /api/organizations/{orgId}/reports/inventory-valuation", map[string]bool{"accounting": true}},
		{"GET /api/organizations/{orgId}/reports/daily-cash-movements", map[string]bool{"general": true, "accounting": true, "cashbook": true}},
		{"GET /api/organizations/{orgId}/reports/cash-movement-details", map[string]bool{"general": true, "accounting": true, "cashbook": true}},
		{"GET /api/organizations/{orgId}/reports/loan-status", map[string]bool{"general": true, "cashbook": true}},

		// Shared routes are open to every role at the section level.
		{"GET /api/organizations/{orgId}/dashboard", map[string]bool{"general": true, "sales": true, "purchasing": true, "accounting": true, "cashbook": true}},
		{"GET /api/organizations/{orgId}/clients", map[string]bool{"general": true, "sales": true, "purchasing": true, "accounting": true, "cashbook": true}},
		{"POST /api/payments", map[string]bool{"general": true, "sales": true, "purchasing": true, "accounting": true, "cashbook": true}},
		{"GET /api/invoices/{id}/payments", map[string]bool{"general": true, "sales": true, "purchasing": true, "accounting": true, "cashbook": true}},
	}

	for _, tc := range cases {
		for _, role := range allRoles {
			want := role == "admin" || role == "power_user" || tc.allowed[role]
			if got := routeAllowedForRole(role, tc.pattern); got != want {
				t.Errorf("routeAllowedForRole(%q, %q) = %v, want %v", role, tc.pattern, got, want)
			}
		}
		// An unresolved role ("") and admin/power_user are unrestricted.
		for _, role := range []string{"", "admin", "power_user"} {
			if !routeAllowedForRole(role, tc.pattern) {
				t.Errorf("routeAllowedForRole(%q, %q) = false, want true", role, tc.pattern)
			}
		}
	}
}

// TestRouteSectionsMatchesResourceBoundaries guards resourceIs against
// prefix-collision bugs: a route that merely starts with a section resource
// name must not be swept in.
func TestRouteSectionsMatchesResourceBoundaries(t *testing.T) {
	unrelated := []string{
		"GET /api/invoices-archive",
		"GET /api/orders-report",
		"GET /api/organizations/{orgId}/imports-archive",
		"GET /api/journal-entries-draft",
		"GET /api/products/{id}/bom-copy",
	}
	for _, pattern := range unrelated {
		if sections := routeSections(pattern); len(sections) != 0 {
			t.Errorf("%s: expected no section, got %v", pattern, sections)
		}
	}
}
