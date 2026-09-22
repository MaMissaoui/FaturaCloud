package api

import (
	"net/http"
	"strings"
)

// Focused per-role sections, enforced server-side (2026-09-22).
//
// src/layouts/role-menu.ts's ROLE_MENU is the frontend source of truth for
// which sidebar sections each organization role may see (and drives a redirect
// for a typed URL). This file mirrors that allow-list for the API so a
// restricted role can't reach a section its UI hides by calling the endpoint
// directly. admin and power_user are unrestricted everywhere and are therefore
// implicit in every entry below — see roleCanUseSection.
//
// Deliberately NOT section-scoped (membership-level for every role, the
// pre-existing rule), because a screen a restricted role *keeps* reads them:
//
//   - clients / vendors / products / tax-rates / payment-terms /
//     units-of-measure and the per-org settings, organization and membership
//     routes — shared master data and configuration.
//   - accounts reads (only accounts *writes* are section-scoped, via
//     sectionAccountingWrite): the invoice/incoming-invoice detail pages'
//     PaymentPanel and the Cash Book's withdraw modal both list accounts, and
//     the organization edit drawer picks default GL accounts.
//   - every payments route: PaymentPanel (sales/purchasing detail pages) and
//     the Cash Book's loan settlement both read and write payments.
//   - GET /api/{invoices,incoming-invoices}/{id}/payments — same reason.
//   - dashboard, exchange-rate, document templates/numbering, countries, auth,
//     users, backups, restore.
const (
	sectionSales            = "sales"
	sectionPurchasing       = "purchasing"
	sectionImports          = "imports"
	sectionInventory        = "inventory"
	sectionProductionOrders = "production-orders"
	sectionBillOfMaterials  = "bill-of-materials"
	sectionAccounting       = "accounting"
	sectionAccountingWrite  = "accounting-write"
	sectionCashbook         = "cashbook"
	sectionReportSales      = "report-sales"
	sectionReportPurchasing = "report-purchasing"
	sectionReportGL         = "report-gl"
	sectionReportTaxSummary = "report-tax-summary"
	sectionReportCashbook   = "report-cashbook"
	sectionReportLoan       = "report-loan"
)

// sectionRoles lists, per section, the org roles allowed to use it. admin and
// power_user are allowed everywhere and are omitted; an empty slice means
// "admin/power_user only". Mirrors src/layouts/role-menu.ts's ROLE_MENU.
var sectionRoles = map[string][]string{
	sectionSales:            {"general", "sales"},
	sectionPurchasing:       {"general", "purchasing"},
	sectionImports:          {"purchasing"},
	sectionInventory:        {"general"},
	sectionProductionOrders: {},
	sectionBillOfMaterials:  {},
	sectionAccounting:       {"accounting"},
	sectionAccountingWrite:  {"accounting"},
	sectionCashbook:         {"general", "cashbook"},
	// The Reporting group. general keeps every report (ROLE_MENU's
	// "group-reporting": null), accounting keeps Tax Summary, and the domain
	// roles keep only their own analytics.
	sectionReportSales:      {"general", "sales"},
	sectionReportPurchasing: {"general", "purchasing"},
	sectionReportGL:         {"accounting"},
	sectionReportTaxSummary: {"general", "accounting"},
	sectionReportCashbook:   {"general", "accounting", "cashbook"},
	sectionReportLoan:       {"general", "cashbook"},
}

// roleCanUseSection reports whether role may use a section. admin, power_user
// and an unresolved role ("") are unrestricted, matching the frontend's
// ROLE_MENU (an absent entry there means "full menu").
func roleCanUseSection(role, section string) bool {
	if role == "" || role == "admin" || role == "power_user" {
		return true
	}
	for _, r := range sectionRoles[section] {
		if r == role {
			return true
		}
	}
	return false
}

// routeSections maps a route's registered pattern (r.Pattern, e.g.
// "PUT /api/organizations/{orgId}/accounts/import") to the section(s) it
// belongs to. An empty result means the route is not section-scoped. A route
// may belong to more than one section (the caller is allowed if any matches).
func routeSections(pattern string) []string {
	method, path := splitPattern(pattern)
	// Normalize the two shapes an org-scoped route can take —
	// /api/invoices/{id} and /api/organizations/{orgId}/invoices — so both
	// classify identically.
	path = strings.TrimPrefix(path, "/api/")
	path = strings.TrimPrefix(path, "organizations/{orgId}/")

	switch {
	case resourceIs(path, "invoices"), resourceIs(path, "orders"), resourceIs(path, "deliveries"):
		// Payment reads on a detail page are shared (see the file comment) —
		// the Cash Book settles a loan sale through this same route.
		if strings.HasSuffix(path, "/payments") {
			return nil
		}
		return []string{sectionSales}
	case resourceIs(path, "purchase-orders"), resourceIs(path, "inbound-deliveries"),
		resourceIs(path, "incoming-invoices"):
		if strings.HasSuffix(path, "/payments") {
			return nil
		}
		return []string{sectionPurchasing}
	case resourceIs(path, "imports"):
		return []string{sectionImports}
	case resourceIs(path, "stock-movements"):
		return []string{sectionInventory}
	case resourceIs(path, "production-orders"):
		return []string{sectionProductionOrders}
	case resourceIs(path, "products/bom-summaries"), path == "products/{id}/bom",
		strings.HasPrefix(path, "products/{id}/bom/"):
		return []string{sectionBillOfMaterials}
	case resourceIs(path, "journals"), resourceIs(path, "journal-entries"),
		resourceIs(path, "fiscal-years"), resourceIs(path, "fiscal-periods"),
		strings.HasPrefix(path, "gl-export/"):
		return []string{sectionAccounting}
	case resourceIs(path, "accounts"):
		// Reads are shared (PaymentPanel, org edit drawer, Cash Book); writes
		// stay accounting-only, matching the hidden Chart of Accounts page.
		if method == http.MethodGet {
			return nil
		}
		return []string{sectionAccountingWrite}
	case resourceIs(path, "cash-sales"), resourceIs(path, "cash-movements"):
		return []string{sectionCashbook}
	case path == "reporting/revenue-trend", path == "reporting/sales-by-client",
		path == "reporting/sales-by-product":
		return []string{sectionReportSales}
	case path == "reporting/purchases-by-vendor":
		return []string{sectionReportPurchasing}
	case path == "reporting/tax-summary":
		return []string{sectionReportTaxSummary}
	case path == "reports/trial-balance", path == "reports/profit-and-loss",
		path == "reports/balance-sheet", path == "reports/ar-aging",
		path == "reports/ap-aging", path == "reports/inventory-valuation",
		path == "reports/account-balance":
		return []string{sectionReportGL}
	case path == "reports/daily-cash-movements", strings.HasPrefix(path, "reports/daily-cash-movements/"),
		path == "reports/cash-movement-details", path == "reports/payment-history/export":
		return []string{sectionReportCashbook}
	case path == "reports/loan-status", strings.HasPrefix(path, "reports/loan-status/"):
		return []string{sectionReportLoan}
	}
	return nil
}

// routeAllowedForRole reports whether role may call the route registered under
// pattern. A route in no section is always allowed here — membership is still
// enforced by the wrapping middleware.
func routeAllowedForRole(role, pattern string) bool {
	sections := routeSections(pattern)
	if len(sections) == 0 {
		return true
	}
	for _, s := range sections {
		if roleCanUseSection(role, s) {
			return true
		}
	}
	return false
}

// splitPattern separates r.Pattern's method from its path ("GET /api/x" ->
// "GET", "/api/x"). A pattern without a method (never produced by net/http's
// mux) yields an empty method and the pattern as-is.
func splitPattern(pattern string) (method, path string) {
	if i := strings.IndexByte(pattern, ' '); i >= 0 {
		return pattern[:i], pattern[i+1:]
	}
	return "", pattern
}

// resourceIs reports whether path addresses the resource name or a sub-path of
// it ("invoices" and "invoices/{id}" match "invoices"; "invoices-archive" does
// not).
func resourceIs(path, name string) bool {
	return path == name || strings.HasPrefix(path, name+"/")
}
