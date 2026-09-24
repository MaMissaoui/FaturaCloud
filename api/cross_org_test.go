package api

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// --- Route-coverage tripwire (issue #141 Phase C) -------------------------
//
// This is the automated guarantee that a route can't silently ship without
// an org-scoping decision being made about it: every route registered in
// router.go must be accounted for in exactly one of the lists below, or
// this test fails the build. It parses router.go's actual source rather
// than re-typing the route table by hand a second time, so the two can
// never drift from each other silently — only from this list, which a
// route addition/removal will now force a decision on.
//
// A route gated via orgMemberProtected/orgAdminProtected/platformAdminProtected
// is self-evidently accounted for (that's what those wrappers exist for).
// A route still registered via the bare protected()/mux.Handle wrapper is
// either:
//   - exemptRoutes: intentionally never org-scoped (public, self-limiting,
//     or genuinely global — see each entry's own comment for why), or
//   - createRouteOrgChecks: a POST create route where organizationId lives
//     in the JSON body, so orgIDResolver-based middleware can't run before
//     decodeJSON — gated instead by an inline h.requireOrgMember call this
//     test verifies is actually present in the handler's source, or
//   - pendingPhaseCRoutes: known, tracked, not yet migrated — Phase C
//     (issue #141) ships across several PRs by design (see
//     /Users/mam/.claude/plans/tranquil-toasting-eagle.md); an entry here
//     is removed the same PR that adds its route to one of the lists above.
//
// Anything appearing in none of these, or in more than one, fails the test.

type routeKey struct {
	method  string
	pattern string
}

func (rk routeKey) String() string { return rk.method + " " + rk.pattern }

// discoverRoutes parses router.go's source (not the running mux — a raw
// AST walk is what lets this test see every registration call site
// directly, including the ones passed as unevaluated string literals to
// protected/orgMemberProtected/orgAdminProtected/platformAdminProtected/mux.Handle).
func discoverRoutes(t *testing.T) map[routeKey]string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filepath.Join(".", "router.go"), nil, 0)
	if err != nil {
		t.Fatalf("parse router.go: %v", err)
	}

	routes := map[routeKey]string{}
	wrapperNames := map[string]bool{
		"protected": true, "orgMemberProtected": true,
		"orgAdminProtected": true, "platformAdminProtected": true,
		"orgRoleProtected": true, "orgRoleAdminProtected": true,
	}
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fn := call.Fun.(type) {
		case *ast.Ident:
			if !wrapperNames[fn.Name] || len(call.Args) < 2 {
				return true
			}
			method, ok1 := stringLit(call.Args[0])
			pattern, ok2 := stringLit(call.Args[1])
			if ok1 && ok2 {
				routes[routeKey{method, pattern}] = fn.Name
			}
		case *ast.SelectorExpr:
			ident, ok := fn.X.(*ast.Ident)
			if !ok || ident.Name != "mux" || fn.Sel.Name != "Handle" || len(call.Args) < 2 {
				return true
			}
			combined, ok := stringLit(call.Args[0])
			if !ok {
				return true
			}
			parts := strings.SplitN(combined, " ", 2)
			if len(parts) != 2 {
				return true
			}
			// A handful of routes (LibreOffice-conversion exports, and the
			// two write-lock restore routes) are registered directly via
			// mux.Handle instead of one of the *Protected wrappers, for
			// reasons unrelated to authorization (see router.go's own
			// comments at each site) — but still wrap their handler in
			// h.orgMember(...)/h.platformAdmin(...) inline. Detect that
			// nested call so these don't all need a manually-maintained
			// exemptRoutes rationale forever; only genuinely bare
			// mux.Handle calls (the restore routes, already in
			// exemptRoutes) still need one.
			wrapper := "mux.Handle"
			ast.Inspect(call.Args[1], func(inner ast.Node) bool {
				innerCall, ok := inner.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := innerCall.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				recv, ok := sel.X.(*ast.Ident)
				if !ok || recv.Name != "h" {
					return true
				}
				switch sel.Sel.Name {
				case "orgMember":
					wrapper = "orgMemberProtected"
				case "orgAdmin":
					wrapper = "orgAdminProtected"
				}
				return true
			})
			routes[routeKey{parts[0], parts[1]}] = wrapper
		}
		return true
	})
	return routes
}

func stringLit(e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	v, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return v, true
}

// discoverDomainRoles parses router.go a second time and, for every
// orgRoleProtected/orgRoleAdminProtected call site, extracts the actual
// []string{...} roles argument passed (the 4th positional arg for both
// wrappers) — so TestDomainRoleRouteCoverage below compares the maintained
// domainRouteRoles table against what router.go *really* passes, not just
// against which wrapper name was used. A route whose role list drifts from
// the maintained table (added, removed, or reordered-but-different) fails
// the build, the same guarantee discoverRoutes already gives org-scoping.
func discoverDomainRoles(t *testing.T) map[routeKey][]string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filepath.Join(".", "router.go"), nil, 0)
	if err != nil {
		t.Fatalf("parse router.go: %v", err)
	}

	roles := map[routeKey][]string{}
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		ident, ok := call.Fun.(*ast.Ident)
		if !ok || (ident.Name != "orgRoleProtected" && ident.Name != "orgRoleAdminProtected") || len(call.Args) < 4 {
			return true
		}
		method, ok1 := stringLit(call.Args[0])
		pattern, ok2 := stringLit(call.Args[1])
		if !ok1 || !ok2 {
			return true
		}
		composite, ok := call.Args[3].(*ast.CompositeLit)
		if !ok {
			t.Fatalf("%s %s: 4th argument to %s isn't a []string{...} literal — discoverDomainRoles can't extract it", method, pattern, ident.Name)
		}
		var roleList []string
		for _, elt := range composite.Elts {
			s, ok := stringLit(elt)
			if !ok {
				t.Fatalf("%s %s: non-string-literal element in the roles argument to %s", method, pattern, ident.Name)
			}
			roleList = append(roleList, s)
		}
		roles[routeKey{method, pattern}] = roleList
		return true
	})
	return roles
}

func sameRoleSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	counts := map[string]int{}
	for _, r := range a {
		counts[r]++
	}
	for _, r := range b {
		counts[r]--
	}
	for _, c := range counts {
		if c != 0 {
			return false
		}
	}
	return true
}

// exemptRoutes are never org-scoped, on purpose.
var exemptRoutes = map[routeKey]string{
	{"GET", "/api/version"}:                       "public, pre-auth",
	{"POST", "/api/auth/login"}:                   "public, pre-auth",
	{"POST", "/api/auth/logout"}:                  "public, pre-auth",
	{"GET", "/api/auth/oidc/enabled"}:             "public, pre-auth",
	{"GET", "/api/auth/oidc/login"}:               "public, pre-auth",
	{"GET", "/api/auth/oidc/callback"}:            "public, pre-auth",
	{"GET", "/api/auth/me"}:                       "caller's own identity, no org context",
	{"POST", "/api/organizations"}:                "no existing org to check; creator is auto-granted membership",
	{"GET", "/api/organizations/{orgId}/my-role"}: "intentionally self-limiting — any authenticated user may ask their own role",
	{"GET", "/api/organizations/my-roles"}:        "batch counterpart to my-role (issue #147) — same self-limiting scope, across every organization the caller belongs to at once",
	{"GET", "/api/countries/active"}:              "global picklist, not per-org",
	// listOrganizations itself filters to the caller's own memberships
	// (GetUserOrganizations) — no single target org to resolve via
	// middleware, since the whole point of this route is "which orgs does
	// the caller belong to."
	{"GET", "/api/organizations"}: "handler filters to the caller's own memberships (GetUserOrganizations), no route-level resolver applies",
	// Both already platform-admin gated (auth(platformAdmin(csrf(...))) —
	// registered directly via mux.Handle instead of platformAdminProtected
	// only because they take dbMu's *write* lock themselves and must never
	// be wrapped in withDB's read lock (see router.go's own comment there).
	// Not org-scoped by nature — backup/restore is a global admin concern.
	{"POST", "/api/backups/{name}/restore"}: "platform-admin gated via direct mux.Handle (write-lock routes can't use withDB)",
	{"POST", "/api/restore"}:                "platform-admin gated via direct mux.Handle (write-lock routes can't use withDB)",
}

// createRouteOrgChecks are POST create routes gated by an inline
// h.requireOrgMember call inside the handler (organizationId lives in the
// JSON body, so router-level middleware can't check it before decodeJSON).
// file/fn name the handler this test parses to verify the call is actually
// present — catches a Create* handler that forgot the check as a build
// failure, not just a code-review miss.
var createRouteOrgChecks = map[routeKey]struct{ file, fn string }{
	{"POST", "/api/clients"}:            {"clients.go", "createClient"},
	{"POST", "/api/vendors"}:            {"vendors.go", "createVendor"},
	{"POST", "/api/invoices"}:           {"invoices.go", "createInvoice"},
	{"POST", "/api/orders"}:             {"orders.go", "createOrder"},
	{"POST", "/api/deliveries"}:         {"deliveries.go", "createDelivery"},
	{"POST", "/api/imports"}:            {"imports.go", "createImport"},
	{"POST", "/api/purchase-orders"}:    {"purchase_orders.go", "createPurchaseOrder"},
	{"POST", "/api/inbound-deliveries"}: {"inbound_deliveries.go", "createInboundDelivery"},
	{"POST", "/api/production-orders"}:  {"production_orders.go", "createProductionOrder"},
	{"POST", "/api/incoming-invoices"}:  {"incoming_invoices.go", "createIncomingInvoice"},
	{"POST", "/api/tax-rates"}:          {"tax_rates.go", "createTaxRate"},
	{"POST", "/api/payment-terms"}:      {"payment_terms.go", "createPaymentTerm"},
	{"POST", "/api/units-of-measure"}:   {"units_of_measure.go", "createUnitOfMeasure"},
	{"POST", "/api/products"}:           {"products.go", "createProduct"},
	{"POST", "/api/stock-movements"}:    {"stock.go", "createStockMovement"},
	{"POST", "/api/accounts"}:           {"accounts.go", "createAccount"},
	{"POST", "/api/journals"}:           {"journals.go", "createJournal"},
	{"POST", "/api/fiscal-years"}:       {"fiscal_periods.go", "createFiscalYear"},
	{"POST", "/api/fiscal-periods"}:     {"fiscal_periods.go", "createFiscalPeriod"},
	{"POST", "/api/journal-entries"}:    {"journal_entries.go", "createJournalEntry"},
	{"POST", "/api/payments"}:           {"payments.go", "createPayment"},
	{"POST", "/api/cash-sales"}:         {"cash_sale.go", "createCashSale"},
	{"POST", "/api/cash-movements"}:     {"cash_movement.go", "createCashMovement"},
}

// pendingPhaseCRoutes are known, tracked, not-yet-migrated routes — see the
// package comment above. Remove an entry the same PR its route moves to
// orgMemberProtected/orgAdminProtected or gains a createRouteOrgChecks
// entry. Empty as of PR6 (6/7) — every route this app registers now lands
// in exactly one of {an org-scoped wrapper, exemptRoutes,
// createRouteOrgChecks}. Kept (not deleted) as the live tripwire's
// documented third bucket, in case a future route needs a deliberate
// migration window again.
var pendingPhaseCRoutes = map[routeKey]bool{}

// TestPhaseCRouteCoverage is the tripwire described in the package comment
// above: every route router.go registers must land in exactly one of
// {gated by an org-scoped wrapper, exemptRoutes, createRouteOrgChecks,
// pendingPhaseCRoutes}, and every route named in those maintained lists
// must still exist in router.go.
func TestPhaseCRouteCoverage(t *testing.T) {
	discovered := discoverRoutes(t)
	if len(discovered) == 0 {
		t.Fatal("discovered zero routes — router.go parsing is broken, not that the app has no routes")
	}

	orgScopedWrappers := map[string]bool{
		"orgMemberProtected": true, "orgAdminProtected": true,
		// orgRoleProtected/orgRoleAdminProtected are self-evidently
		// org-scoped too (they're built on the same orgIDResolver
		// machinery, with a role check layered on top) — the role list
		// itself is verified separately by TestDomainRoleRouteCoverage,
		// not by this org-scoping tripwire.
		"orgRoleProtected": true, "orgRoleAdminProtected": true,
	}

	for key, wrapper := range discovered {
		if orgScopedWrappers[wrapper] || wrapper == "platformAdminProtected" {
			continue // self-evidently accounted for — that's what the wrapper does
		}
		// wrapper is "protected" or "mux.Handle": must be exempt, a
		// verified create-check, or a tracked pending route — never more
		// than one, never none.
		_, isExempt := exemptRoutes[key]
		_, isCreateCheck := createRouteOrgChecks[key]
		isPending := pendingPhaseCRoutes[key]
		count := 0
		for _, b := range []bool{isExempt, isCreateCheck, isPending} {
			if b {
				count++
			}
		}
		switch count {
		case 0:
			t.Errorf(
				"%s is registered via %s but appears in none of exemptRoutes/createRouteOrgChecks/pendingPhaseCRoutes — "+
					"classify it (see the file-level comment in cross_org_test.go) before this route can ship",
				key, wrapper,
			)
		case 1:
			// fine
		default:
			t.Errorf("%s appears in more than one of exemptRoutes/createRouteOrgChecks/pendingPhaseCRoutes", key)
		}
	}

	// Converse: every maintained-list entry must still be a real route —
	// catches a stale entry left behind after a route was renamed/removed,
	// same shape as TestVendorDocumentCountCoversEveryReference elsewhere.
	for key := range exemptRoutes {
		if _, ok := discovered[key]; !ok {
			t.Errorf("exemptRoutes lists %s, which router.go no longer registers", key)
		}
	}
	for key := range createRouteOrgChecks {
		if _, ok := discovered[key]; !ok {
			t.Errorf("createRouteOrgChecks lists %s, which router.go no longer registers", key)
		}
	}
	for key := range pendingPhaseCRoutes {
		if _, ok := discovered[key]; !ok {
			t.Errorf("pendingPhaseCRoutes lists %s, which router.go no longer registers", key)
		}
	}
}

// --- Domain-role route-coverage tripwire (org role redesign) -------------
//
// domainRouteRoles is the maintained twin of the roles actually passed at
// each orgRoleProtected/orgRoleAdminProtected call site in router.go — "one
// of the sales/purchasing/accounting/cashbook domain roles" beyond the
// "admin"/"general" every such route implicitly already allows (see
// orgRole/orgRoleAdmin's own doc comments in api/middleware.go). Keeping
// this as a maintained table that discoverDomainRoles verifies against the
// real call sites (not just a comment) is what turns "did every domain
// mutation actually get the right role" into a build-time guarantee instead
// of a review question — the same shape TestPhaseCRouteCoverage already
// gives org-scoping itself.
var domainRouteRoles = map[routeKey][]string{
	{"PUT", "/api/clients/{id}"}:                          {"sales"},
	{"DELETE", "/api/clients/{id}"}:                       {"sales"},
	{"POST", "/api/organizations/{orgId}/clients/import"}: {"sales"},

	{"PUT", "/api/invoices/{id}"}:             {"sales"},
	{"PATCH", "/api/invoices/{id}/state"}:     {"sales"},
	{"POST", "/api/cash-sales/{id}/payments"}: {"cashbook"},
	{"DELETE", "/api/invoices/{id}"}:          {"sales"},

	{"PUT", "/api/orders/{id}"}:          {"sales"},
	{"PATCH", "/api/orders/{id}/status"}: {"sales"},
	{"DELETE", "/api/orders/{id}"}:       {"sales"},

	{"PUT", "/api/deliveries/{id}"}:          {"sales"},
	{"PATCH", "/api/deliveries/{id}/status"}: {"sales"},
	{"DELETE", "/api/deliveries/{id}"}:       {"sales"},

	{"PUT", "/api/vendors/{id}"}:                          {"purchasing"},
	{"DELETE", "/api/vendors/{id}"}:                       {"purchasing"},
	{"POST", "/api/organizations/{orgId}/vendors/import"}: {"purchasing"},

	{"PUT", "/api/imports/{id}"}:    {"purchasing"},
	{"DELETE", "/api/imports/{id}"}: {"purchasing"},

	{"PUT", "/api/purchase-orders/{id}"}:          {"purchasing"},
	{"PATCH", "/api/purchase-orders/{id}/status"}: {"purchasing"},
	{"DELETE", "/api/purchase-orders/{id}"}:       {"purchasing"},

	{"PUT", "/api/inbound-deliveries/{id}"}:          {"purchasing"},
	{"PATCH", "/api/inbound-deliveries/{id}/status"}: {"purchasing"},
	{"DELETE", "/api/inbound-deliveries/{id}"}:       {"purchasing"},

	{"PUT", "/api/incoming-invoices/{id}"}:         {"purchasing"},
	{"PATCH", "/api/incoming-invoices/{id}/state"}: {"purchasing"},
	{"DELETE", "/api/incoming-invoices/{id}"}:      {"purchasing"},

	{"PUT", "/api/accounts/{id}"}:                          {"accounting"},
	{"DELETE", "/api/accounts/{id}"}:                       {"accounting"},
	{"POST", "/api/organizations/{orgId}/accounts/import"}: {"accounting"},

	{"PUT", "/api/journals/{id}"}:    {"accounting"},
	{"DELETE", "/api/journals/{id}"}: {"accounting"},

	{"PATCH", "/api/fiscal-periods/{id}/status"}: {"accounting"},
	{"POST", "/api/fiscal-years/{id}/close"}:     {"accounting"},

	{"PATCH", "/api/journal-entries/{id}/post"}:   {"accounting"},
	{"POST", "/api/journal-entries/{id}/reverse"}: {"accounting"},
	{"DELETE", "/api/journal-entries/{id}"}:       {"accounting"},

	// F104 — voiding a payment reverses its posted GL entry, so it's gated
	// the same as journal-entry reversal.
	{"POST", "/api/payments/{id}/void"}: {"accounting"},

	{"GET", "/api/organizations/{orgId}/gl-export/fec"}:   {"accounting"},
	{"GET", "/api/organizations/{orgId}/gl-export/datev"}: {"accounting"},
}

// createRouteDomainRoles is domainRouteRoles' counterpart for the
// body-org Create* routes (organizationId lives in the JSON body, so
// there's no orgRoleProtected call site — the check is the inline
// h.requireOrgRole(...) call TestCreateRouteOrgChecksArePresent below
// verifies is actually present and passes these exact roles).
var createRouteDomainRoles = map[routeKey][]string{
	{"POST", "/api/clients"}:    {"sales"},
	{"POST", "/api/invoices"}:   {"sales"},
	{"POST", "/api/orders"}:     {"sales"},
	{"POST", "/api/deliveries"}: {"sales"},

	{"POST", "/api/vendors"}:            {"purchasing"},
	{"POST", "/api/imports"}:            {"purchasing"},
	{"POST", "/api/purchase-orders"}:    {"purchasing"},
	{"POST", "/api/inbound-deliveries"}: {"purchasing"},
	{"POST", "/api/incoming-invoices"}:  {"purchasing"},

	{"POST", "/api/accounts"}:        {"accounting"},
	{"POST", "/api/journals"}:        {"accounting"},
	{"POST", "/api/fiscal-years"}:    {"accounting"},
	{"POST", "/api/fiscal-periods"}:  {"accounting"},
	{"POST", "/api/journal-entries"}: {"accounting"},
	// F104 — payments settle AR/AP (body-carried organizationId, so the
	// check is the inline requireOrgRole in createPayment).
	{"POST", "/api/payments"}: {"accounting"},

	{"POST", "/api/cash-sales"}:     {"cashbook"},
	{"POST", "/api/cash-movements"}: {"cashbook"},
}

// TestDomainRoleRouteCoverage is TestPhaseCRouteCoverage's counterpart for
// the role dimension: every orgRoleProtected/orgRoleAdminProtected call
// site in router.go must appear in domainRouteRoles with the identical role
// list actually passed (not just documented), and every domainRouteRoles
// entry must still be a real orgRoleProtected/orgRoleAdminProtected route.
func TestDomainRoleRouteCoverage(t *testing.T) {
	discovered := discoverRoutes(t)
	actualRoles := discoverDomainRoles(t)
	if len(actualRoles) == 0 {
		t.Fatal("discovered zero domain-role routes — router.go parsing is broken, not that no route uses orgRoleProtected/orgRoleAdminProtected")
	}

	for key, roles := range actualRoles {
		want, ok := domainRouteRoles[key]
		if !ok {
			t.Errorf("%s is gated via orgRoleProtected/orgRoleAdminProtected with roles %v but has no domainRouteRoles entry — add one", key, roles)
			continue
		}
		if !sameRoleSet(want, roles) {
			t.Errorf("%s: domainRouteRoles says %v but router.go actually passes %v", key, want, roles)
		}
	}
	for key := range domainRouteRoles {
		wrapper, ok := discovered[key]
		if !ok {
			t.Errorf("domainRouteRoles lists %s, which router.go no longer registers", key)
			continue
		}
		if wrapper != "orgRoleProtected" && wrapper != "orgRoleAdminProtected" {
			t.Errorf("domainRouteRoles lists %s, but it's registered via %s, not an orgRole*Protected wrapper", key, wrapper)
		}
	}
}

// TestCreateRouteOrgChecksArePresent parses each createRouteOrgChecks
// handler's own source file and confirms its function body actually calls
// h.requireOrgMember (or, for a route also listed in createRouteDomainRoles,
// h.requireOrgRole with exactly that role list) — the thing that makes
// "POST /api/clients is in createRouteOrgChecks" a verified fact rather
// than an assertion the handler could silently stop satisfying (e.g. a
// future refactor that removes the check without anyone remembering to
// update this table).
func TestCreateRouteOrgChecksArePresent(t *testing.T) {
	for key, loc := range createRouteOrgChecks {
		t.Run(key.String(), func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, filepath.Join(".", loc.file), nil, 0)
			if err != nil {
				t.Fatalf("parse %s: %v", loc.file, err)
			}
			var found *ast.FuncDecl
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if ok && fn.Name.Name == loc.fn {
					found = fn
					break
				}
			}
			if found == nil {
				t.Fatalf("%s: handler func %q not found", loc.file, loc.fn)
			}

			wantRoles, isDomainRoute := createRouteDomainRoles[key]

			callsRequireOrgMember := false
			var requireOrgRoleCalls [][]string
			ast.Inspect(found, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				switch sel.Sel.Name {
				case "requireOrgMember":
					callsRequireOrgMember = true
				case "requireOrgRole":
					// Signature: h.requireOrgRole(w, r, orgID, roles...) —
					// the variadic roles start at the 4th positional arg.
					var roles []string
					for _, arg := range call.Args[3:] {
						if s, ok := stringLit(arg); ok {
							roles = append(roles, s)
						}
					}
					requireOrgRoleCalls = append(requireOrgRoleCalls, roles)
				}
				return true
			})

			if !isDomainRoute {
				if !callsRequireOrgMember {
					t.Errorf(
						"%s (%s) is listed in createRouteOrgChecks but its handler %s doesn't call h.requireOrgMember",
						key, loc.file, loc.fn,
					)
				}
				return
			}

			if len(requireOrgRoleCalls) == 0 {
				t.Errorf(
					"%s (%s) is listed in createRouteDomainRoles (wants %v) but its handler %s doesn't call h.requireOrgRole",
					key, loc.file, wantRoles, loc.fn,
				)
				return
			}
			matched := false
			for _, got := range requireOrgRoleCalls {
				if sameRoleSet(wantRoles, got) {
					matched = true
				}
			}
			if !matched {
				t.Errorf(
					"%s (%s): createRouteDomainRoles wants %v but handler %s calls h.requireOrgRole with %v",
					key, loc.file, wantRoles, loc.fn, requireOrgRoleCalls,
				)
			}
		})
	}

	// Converse: every createRouteDomainRoles entry must also be a
	// createRouteOrgChecks entry (the loop above is what actually verifies
	// it) — catches a route added to one table but not the other.
	for key := range createRouteDomainRoles {
		if _, ok := createRouteOrgChecks[key]; !ok {
			t.Errorf("createRouteDomainRoles lists %s, which has no createRouteOrgChecks entry", key)
		}
	}
}

// --- Cross-organization access denial (issue #141 Phase C) ---------------

// crossOrgProof grew across Phase C's 7-PR rollout
// (/Users/mam/.claude/plans/tranquil-toasting-eagle.md): each domain PR
// added a list + a single-resource get + one mutation for its newly-gated
// routes — enough to prove the mechanism actually denies a real request,
// not just that the router table claims it does (that structural guarantee
// is TestPhaseCRouteCoverage's job). PR7's final sweep extended this to
// every remaining org-scoped route family — organization core fields,
// members, document templates, dashboard, exchange-rate, every GL/document
// report, GL export, fiscal-year close, and the next-number endpoints —
// including the orgAdminProtected (not just orgMemberProtected) routes,
// since those are exactly the highest-consequence ones (delete/reset an
// organization, close a fiscal year) to prove actually deny a non-admin of
// that org rather than trusting the router table's wrapper choice alone.
var crossOrgProof = []struct {
	name   string
	method string
	path   string
	body   []byte
}{
	{name: "list clients by org path", method: http.MethodGet, path: "/api/organizations/org-a/clients"},
	{name: "get client by id", method: http.MethodGet, path: "/api/clients/org-a-client"},
	{name: "update client by id", method: http.MethodPut, path: "/api/clients/org-a-client", body: []byte(`{"name":"hijacked"}`)},
	{name: "delete client by id", method: http.MethodDelete, path: "/api/clients/org-a-client"},
	{name: "client invoice count", method: http.MethodGet, path: "/api/clients/org-a-client/invoice-count"},

	{name: "list vendors by org path", method: http.MethodGet, path: "/api/organizations/org-a/vendors"},
	{name: "get vendor by id", method: http.MethodGet, path: "/api/vendors/org-a-vendor"},
	{name: "delete vendor by id", method: http.MethodDelete, path: "/api/vendors/org-a-vendor"},

	{name: "list invoices by org path", method: http.MethodGet, path: "/api/organizations/org-a/invoices"},
	{name: "get invoice by id", method: http.MethodGet, path: "/api/invoices/org-a-invoice"},
	{name: "invoice line items", method: http.MethodGet, path: "/api/invoices/org-a-invoice/line-items"},
	{name: "delete invoice by id", method: http.MethodDelete, path: "/api/invoices/org-a-invoice"},

	{name: "list orders by org path", method: http.MethodGet, path: "/api/organizations/org-a/orders"},
	{name: "get order by id", method: http.MethodGet, path: "/api/orders/org-a-order"},
	{name: "delete order by id", method: http.MethodDelete, path: "/api/orders/org-a-order"},

	{name: "list deliveries by org path", method: http.MethodGet, path: "/api/organizations/org-a/deliveries"},
	{name: "get delivery by id", method: http.MethodGet, path: "/api/deliveries/org-a-delivery"},
	{name: "delete delivery by id", method: http.MethodDelete, path: "/api/deliveries/org-a-delivery"},

	{name: "list imports by org path", method: http.MethodGet, path: "/api/organizations/org-a/imports"},
	{name: "import summaries by org path", method: http.MethodGet, path: "/api/organizations/org-a/imports/summaries"},
	{name: "get import by id", method: http.MethodGet, path: "/api/imports/org-a-import"},
	{name: "import summary", method: http.MethodGet, path: "/api/imports/org-a-import/summary"},
	{name: "delete import by id", method: http.MethodDelete, path: "/api/imports/org-a-import"},

	{name: "list purchase orders by org path", method: http.MethodGet, path: "/api/organizations/org-a/purchase-orders"},
	{name: "get purchase order by id", method: http.MethodGet, path: "/api/purchase-orders/org-a-po"},
	{name: "purchase order line items", method: http.MethodGet, path: "/api/purchase-orders/org-a-po/line-items"},
	{name: "purchase order export", method: http.MethodGet, path: "/api/purchase-orders/org-a-po/export"},
	{name: "delete purchase order by id", method: http.MethodDelete, path: "/api/purchase-orders/org-a-po"},

	{name: "list inbound deliveries by org path", method: http.MethodGet, path: "/api/organizations/org-a/inbound-deliveries"},
	{name: "get inbound delivery by id", method: http.MethodGet, path: "/api/inbound-deliveries/org-a-inbound-delivery"},
	{name: "inbound delivery export", method: http.MethodGet, path: "/api/inbound-deliveries/org-a-inbound-delivery/export"},
	{name: "delete inbound delivery by id", method: http.MethodDelete, path: "/api/inbound-deliveries/org-a-inbound-delivery"},

	{name: "list incoming invoices by org path", method: http.MethodGet, path: "/api/organizations/org-a/incoming-invoices"},
	{name: "get incoming invoice by id", method: http.MethodGet, path: "/api/incoming-invoices/org-a-incoming-invoice"},
	{name: "incoming invoice match", method: http.MethodGet, path: "/api/incoming-invoices/org-a-incoming-invoice/match"},
	{name: "incoming invoice payments", method: http.MethodGet, path: "/api/incoming-invoices/org-a-incoming-invoice/payments"},
	{name: "incoming invoice export", method: http.MethodGet, path: "/api/incoming-invoices/org-a-incoming-invoice/export"},
	{name: "delete incoming invoice by id", method: http.MethodDelete, path: "/api/incoming-invoices/org-a-incoming-invoice"},

	{name: "list tax rates by org path", method: http.MethodGet, path: "/api/organizations/org-a/tax-rates"},
	{name: "get tax rate by id", method: http.MethodGet, path: "/api/tax-rates/org-a-tax-rate"},
	{name: "tax rate usage count", method: http.MethodGet, path: "/api/tax-rates/org-a-tax-rate/usage-count"},
	{name: "delete tax rate by id", method: http.MethodDelete, path: "/api/tax-rates/org-a-tax-rate"},

	// Production orders and units of measure (audit 2026-09-14 F80): both
	// route families are orgMemberProtected with by-id resolvers, so
	// TestPhaseCRouteCoverage already accepts them as gated — but the deny
	// path itself was never exercised, which is what this list is for.
	{name: "list production orders by org path", method: http.MethodGet, path: "/api/organizations/org-a/production-orders"},
	{name: "next production order number by org path", method: http.MethodGet, path: "/api/organizations/org-a/production-orders/next-number"},
	{name: "get production order by id", method: http.MethodGet, path: "/api/production-orders/org-a-production-order"},
	{name: "production order component lines", method: http.MethodGet, path: "/api/production-orders/org-a-production-order/component-lines"},
	{name: "update production order status", method: http.MethodPatch, path: "/api/production-orders/org-a-production-order/status", body: []byte(`{"status":"cancelled"}`)},
	{name: "delete production order by id", method: http.MethodDelete, path: "/api/production-orders/org-a-production-order"},

	{name: "list units of measure by org path", method: http.MethodGet, path: "/api/organizations/org-a/units-of-measure"},
	{name: "update unit of measure by id", method: http.MethodPut, path: "/api/units-of-measure/org-a-unit-of-measure", body: []byte(`{"name":"hijacked"}`)},
	{name: "delete unit of measure by id", method: http.MethodDelete, path: "/api/units-of-measure/org-a-unit-of-measure"},

	{name: "list payment terms by org path", method: http.MethodGet, path: "/api/organizations/org-a/payment-terms"},
	{name: "update payment term by id", method: http.MethodPut, path: "/api/payment-terms/org-a-payment-term", body: []byte(`{"name":"hijacked"}`)},
	{name: "delete payment term by id", method: http.MethodDelete, path: "/api/payment-terms/org-a-payment-term"},

	{name: "list products by org path", method: http.MethodGet, path: "/api/organizations/org-a/products"},
	{name: "bom summaries by org path", method: http.MethodGet, path: "/api/organizations/org-a/products/bom-summaries"},
	{name: "get product by id", method: http.MethodGet, path: "/api/products/org-a-product"},
	{name: "product stock movements", method: http.MethodGet, path: "/api/products/org-a-product/stock-movements"},
	{name: "product serial numbers", method: http.MethodGet, path: "/api/products/org-a-product/serial-numbers"},
	{name: "get product bom", method: http.MethodGet, path: "/api/products/org-a-product/bom"},
	{name: "replace product bom", method: http.MethodPut, path: "/api/products/org-a-product/bom", body: []byte(`{"lines":[]}`)},
	{name: "list product bom versions", method: http.MethodGet, path: "/api/products/org-a-product/bom/versions"},
	{name: "get product bom version", method: http.MethodGet, path: "/api/products/org-a-product/bom/versions/fake-version-id"},
	{name: "restore product bom version", method: http.MethodPost, path: "/api/products/org-a-product/bom/versions/fake-version-id/restore"},
	{name: "delete product by id", method: http.MethodDelete, path: "/api/products/org-a-product"},

	{name: "delete stock movement by id", method: http.MethodDelete, path: "/api/stock-movements/org-a-stock-movement"},

	{name: "list accounts by org path", method: http.MethodGet, path: "/api/organizations/org-a/accounts"},
	{name: "get account by id", method: http.MethodGet, path: "/api/accounts/org-a-account"},
	{name: "delete account by id", method: http.MethodDelete, path: "/api/accounts/org-a-account"},

	{name: "list journals by org path", method: http.MethodGet, path: "/api/organizations/org-a/journals"},
	{name: "delete journal by id", method: http.MethodDelete, path: "/api/journals/org-a-journal"},

	{name: "list fiscal years by org path", method: http.MethodGet, path: "/api/organizations/org-a/fiscal-years"},
	{name: "list fiscal periods", method: http.MethodGet, path: "/api/fiscal-years/org-a-fiscal-year/periods"},
	{name: "fiscal period status update", method: http.MethodPatch, path: "/api/fiscal-periods/org-a-fiscal-period/status", body: []byte(`{"status":"closed"}`)},

	{name: "list journal entries by org path", method: http.MethodGet, path: "/api/organizations/org-a/journal-entries"},
	{name: "get journal entry by id", method: http.MethodGet, path: "/api/journal-entries/org-a-journal-entry"},
	{name: "journal entry lines", method: http.MethodGet, path: "/api/journal-entries/org-a-journal-entry/lines"},
	{name: "delete journal entry by id", method: http.MethodDelete, path: "/api/journal-entries/org-a-journal-entry"},

	{name: "list payments by org path", method: http.MethodGet, path: "/api/organizations/org-a/payments"},
	{name: "get payment by id", method: http.MethodGet, path: "/api/payments/org-a-payment"},
	{name: "payment applications", method: http.MethodGet, path: "/api/payments/org-a-payment/applications"},
	{name: "void payment by id", method: http.MethodPost, path: "/api/payments/org-a-payment/void"},

	// PR7's final sweep: every remaining org-scoped route family not
	// already exercised above — organization core fields/logo/usage-count,
	// members (admin-only), document templates, dashboard, exchange-rate
	// prefill, the GL/document-analytics report families, GL export
	// (admin-only), fiscal-year close (admin-only, irreversible — this is
	// exactly the kind of route where proving the deny-path actually denies
	// matters most), and the four next-number endpoints. orgAdminProtected
	// routes are included here too — orgAuthorized's 403-then-404 split
	// still falls inside the same "403 or 404 accepted" assertion below.
	{name: "get organization by id", method: http.MethodGet, path: "/api/organizations/org-a"},
	{name: "update organization by id", method: http.MethodPut, path: "/api/organizations/org-a", body: []byte(`{"name":"hijacked"}`)},
	{name: "organization usage count", method: http.MethodGet, path: "/api/organizations/org-a/usage-count"},
	{name: "organization logo", method: http.MethodGet, path: "/api/organizations/org-a/logo"},
	{name: "delete organization logo", method: http.MethodDelete, path: "/api/organizations/org-a/logo"},
	{name: "delete organization by id (admin)", method: http.MethodDelete, path: "/api/organizations/org-a"},
	{name: "reset organization data (admin)", method: http.MethodPost, path: "/api/organizations/org-a/reset"},

	{name: "list organization members (admin)", method: http.MethodGet, path: "/api/organizations/org-a/members"},
	{name: "add organization member (admin)", method: http.MethodPost, path: "/api/organizations/org-a/members", body: []byte(`{"email":"nobody@example.com","role":"user"}`)},
	{name: "update organization member role (admin)", method: http.MethodPut, path: "/api/organizations/org-a/members/org-a-admin", body: []byte(`{"role":"user"}`)},
	{name: "remove organization member (admin)", method: http.MethodDelete, path: "/api/organizations/org-a/members/org-a-admin"},

	{name: "list document templates", method: http.MethodGet, path: "/api/organizations/org-a/document-templates"},
	{name: "get document template", method: http.MethodGet, path: "/api/organizations/org-a/document-templates/invoice"},
	{name: "delete document template", method: http.MethodDelete, path: "/api/organizations/org-a/document-templates/invoice"},
	{name: "get document template orientation", method: http.MethodGet, path: "/api/organizations/org-a/document-templates/invoice/orientation"},
	{name: "update document template orientation", method: http.MethodPut, path: "/api/organizations/org-a/document-templates/invoice/orientation", body: []byte(`{"orientation":"landscape"}`)},
	{name: "delete document template orientation", method: http.MethodDelete, path: "/api/organizations/org-a/document-templates/invoice/orientation"},

	{name: "dashboard", method: http.MethodGet, path: "/api/organizations/org-a/dashboard"},
	{name: "exchange rate prefill", method: http.MethodGet, path: "/api/organizations/org-a/exchange-rate"},

	{name: "report: trial balance", method: http.MethodGet, path: "/api/organizations/org-a/reports/trial-balance"},
	{name: "report: profit and loss", method: http.MethodGet, path: "/api/organizations/org-a/reports/profit-and-loss"},
	{name: "report: balance sheet", method: http.MethodGet, path: "/api/organizations/org-a/reports/balance-sheet"},
	{name: "report: ar aging", method: http.MethodGet, path: "/api/organizations/org-a/reports/ar-aging"},
	{name: "report: ap aging", method: http.MethodGet, path: "/api/organizations/org-a/reports/ap-aging"},
	{name: "report: inventory valuation", method: http.MethodGet, path: "/api/organizations/org-a/reports/inventory-valuation"},

	{name: "reporting: revenue trend", method: http.MethodGet, path: "/api/organizations/org-a/reporting/revenue-trend"},
	{name: "reporting: sales by client", method: http.MethodGet, path: "/api/organizations/org-a/reporting/sales-by-client"},
	{name: "reporting: sales by product", method: http.MethodGet, path: "/api/organizations/org-a/reporting/sales-by-product"},
	{name: "reporting: purchases by vendor", method: http.MethodGet, path: "/api/organizations/org-a/reporting/purchases-by-vendor"},
	{name: "reporting: tax summary", method: http.MethodGet, path: "/api/organizations/org-a/reporting/tax-summary"},

	{name: "gl export: fec (admin)", method: http.MethodGet, path: "/api/organizations/org-a/gl-export/fec"},
	{name: "gl export: datev (admin)", method: http.MethodGet, path: "/api/organizations/org-a/gl-export/datev"},

	{name: "close fiscal year (admin, irreversible)", method: http.MethodPost, path: "/api/fiscal-years/org-a-fiscal-year/close"},

	{name: "imports next-number", method: http.MethodGet, path: "/api/organizations/org-a/imports/next-number"},
	{name: "purchase orders next-number", method: http.MethodGet, path: "/api/organizations/org-a/purchase-orders/next-number"},
	{name: "inbound deliveries next-number", method: http.MethodGet, path: "/api/organizations/org-a/inbound-deliveries/next-number"},
	{name: "deliveries next-number", method: http.MethodGet, path: "/api/organizations/org-a/deliveries/next-number"},
}

// TestCrossOrgAccessDenied is the fail-closed regression test the original
// Phase A/B/C design doc calls for: a member of org B must never be able to
// read or write org A's data through a route Phase C has already gated.
// Both 403 and 404 are accepted "denied" outcomes — see orgAuthorized's own
// comment in api/middleware.go for why orgMember deliberately collapses
// both cases to 404 (avoiding a cross-tenant existence oracle) while
// orgAdmin keeps them distinct.
func TestCrossOrgAccessDenied(t *testing.T) {
	t.Parallel()
	mux, database, _, _ := newTestRouter(t)

	seedUser(t, database, "org-a-admin", "user", 1)
	seedUser(t, database, "org-b-admin", "user", 1)
	orgA, err := database.CreateOrganization(db.CreateOrganizationRequest{ID: "org-a"})
	if err != nil {
		t.Fatalf("seed CreateOrganization org-a: %v", err)
	}
	if _, err := database.CreateOrganization(db.CreateOrganizationRequest{ID: "org-b"}); err != nil {
		t.Fatalf("seed CreateOrganization org-b: %v", err)
	}
	// Org-scoped admins, deliberately not platform admins — seedUser's
	// "admin" role sets isPlatformAdmin, a different actor than what this
	// test wants (see api/organizations_test.go's identical pattern).
	if _, err := database.AddOrganizationUser("org-a", "org-a-admin", "admin"); err != nil {
		t.Fatalf("seed org-a membership: %v", err)
	}
	if _, err := database.AddOrganizationUser("org-b", "org-b-admin", "admin"); err != nil {
		t.Fatalf("seed org-b membership: %v", err)
	}
	orgBToken := mintTestJWT(t, "org-b-admin", "user")

	client, err := database.CreateClient(db.CreateClientRequest{
		ID: "org-a-client", OrganizationID: "org-a", Name: strPtr("ACME"),
	})
	if err != nil {
		t.Fatalf("seed CreateClient: %v", err)
	}
	if _, err := database.CreateVendor(db.CreateVendorRequest{
		ID: "org-a-vendor", OrganizationID: "org-a", Name: strPtr("Acme Supplies"),
	}); err != nil {
		t.Fatalf("seed CreateVendor: %v", err)
	}
	if _, err := database.CreateInvoice(db.CreateInvoiceRequest{
		ID: "org-a-invoice", OrganizationID: "org-a", Number: "INV-001", State: "draft",
		ClientID: client.ID, Date: 1700000000000, Currency: "EUR",
	}); err != nil {
		t.Fatalf("seed CreateInvoice: %v", err)
	}
	if _, err := database.CreateOrder(db.CreateOrderRequest{
		ID: "org-a-order", OrganizationID: "org-a", OrderNumber: "ORD-001", OrderDate: 1700000000000,
	}); err != nil {
		t.Fatalf("seed CreateOrder: %v", err)
	}
	if _, err := database.CreateDelivery(db.CreateDeliveryRequest{
		ID: "org-a-delivery", OrganizationID: "org-a", DeliveryNumber: "DEL-001", DeliveryDate: 1700000000000,
	}); err != nil {
		t.Fatalf("seed CreateDelivery: %v", err)
	}
	if _, err := database.CreateImport(db.CreateImportRequest{
		ID: "org-a-import", OrganizationID: "org-a", ImportNumber: "IMP-001", Date: 1700000000000,
	}); err != nil {
		t.Fatalf("seed CreateImport: %v", err)
	}
	vendorID := "org-a-vendor"
	if _, err := database.CreatePurchaseOrder(db.CreatePurchaseOrderRequest{
		ID: "org-a-po", OrganizationID: "org-a", VendorID: &vendorID, OrderNumber: "PO-001", Status: "draft", OrderDate: 1700000000000,
	}); err != nil {
		t.Fatalf("seed CreatePurchaseOrder: %v", err)
	}
	if _, err := database.CreateInboundDelivery(db.CreateInboundDeliveryRequest{
		ID: "org-a-inbound-delivery", OrganizationID: "org-a", VendorID: &vendorID, DeliveryNumber: "GR-001", DeliveryDate: 1700000000000,
	}); err != nil {
		t.Fatalf("seed CreateInboundDelivery: %v", err)
	}
	if _, err := database.CreateIncomingInvoice(db.CreateIncomingInvoiceRequest{
		ID: "org-a-incoming-invoice", OrganizationID: "org-a", VendorID: vendorID, VendorInvoiceNumber: "BILL-001",
		State: "draft", Date: 1700000000000, Currency: "EUR",
	}); err != nil {
		t.Fatalf("seed CreateIncomingInvoice: %v", err)
	}
	if _, err := database.CreateTaxRate(db.CreateTaxRateRequest{
		ID: "org-a-tax-rate", OrganizationID: "org-a", Name: "Test Rate", Percentage: 10,
	}); err != nil {
		t.Fatalf("seed CreateTaxRate: %v", err)
	}
	paymentTermIsDefault := int64(0)
	if _, err := database.CreatePaymentTerm(db.CreatePaymentTermRequest{
		ID: "org-a-payment-term", OrganizationID: "org-a", Name: "Test Term", IsDefault: &paymentTermIsDefault,
	}); err != nil {
		t.Fatalf("seed CreatePaymentTerm: %v", err)
	}
	// "unit-of-measure-a" rather than a seeded default's name: creating an
	// organization already installs the default list (db/unit_of_measure.go's
	// SeedDefaultUnitsOfMeasure), so a common name like "kg" would collide
	// with the (organizationId, name) unique index.
	if _, err := database.CreateUnitOfMeasure(db.CreateUnitOfMeasureRequest{
		ID: "org-a-unit-of-measure", OrganizationID: "org-a", Name: "unit-of-measure-a",
	}); err != nil {
		t.Fatalf("seed CreateUnitOfMeasure: %v", err)
	}
	product, err := database.CreateProduct(db.CreateProductRequest{
		ID: "org-a-product", OrganizationID: "org-a", Name: "Test Product", Type: "product", Price: 1000,
	})
	if err != nil {
		t.Fatalf("seed CreateProduct: %v", err)
	}
	if _, err := database.CreateStockMovement(db.CreateStockMovementRequest{
		ID: "org-a-stock-movement", OrganizationID: "org-a", ProductID: product.ID, Type: "in", Quantity: 1,
	}); err != nil {
		t.Fatalf("seed CreateStockMovement: %v", err)
	}
	// A real production order for org-a to prove the deny path against.
	// Needs a finished product with a non-empty BOM, so it carries its own
	// component/finished pair rather than reusing org-a-product above (which
	// is deliberately uncategorized, and is what the product routes target).
	componentCategory, finishedCategory := "component", "finished"
	poComponent, err := database.CreateProduct(db.CreateProductRequest{
		ID: "org-a-bom-component", OrganizationID: "org-a", Name: "Bolt", SKU: strPtr("X-BLT-1"),
		Type: "product", StockEnabled: 1, Category: &componentCategory,
	})
	if err != nil {
		t.Fatalf("seed CreateProduct(component): %v", err)
	}
	poFinished, err := database.CreateProduct(db.CreateProductRequest{
		ID: "org-a-bom-finished", OrganizationID: "org-a", Name: "Frame", SKU: strPtr("X-FRM-1"),
		Type: "product", StockEnabled: 1, Category: &finishedCategory,
	})
	if err != nil {
		t.Fatalf("seed CreateProduct(finished): %v", err)
	}
	if _, err := database.ReplaceBillOfMaterials(poFinished.ID, []db.CreateBillOfMaterialsLineRequest{
		{ComponentProductID: poComponent.ID, QuantityPerUnit: 1},
	}, 1); err != nil {
		t.Fatalf("seed ReplaceBillOfMaterials: %v", err)
	}
	if _, err := database.CreateProductionOrder(db.CreateProductionOrderRequest{
		ID: "org-a-production-order", OrganizationID: "org-a", OrderNumber: "PRO-001",
		FinishedProductID: poFinished.ID, Quantity: 1, Date: 1700000000000,
	}); err != nil {
		t.Fatalf("seed CreateProductionOrder: %v", err)
	}
	// A fresh code well outside every seeded chart template's own numbering
	// (SKR04/PCG/generic — see db/account.go) to avoid colliding with the
	// organization's auto-seeded default chart of accounts.
	account, err := database.CreateAccount(db.CreateAccountRequest{
		ID: "org-a-account", OrganizationID: "org-a", Code: "9999", Name: "Test Account", Type: "asset",
	})
	if err != nil {
		t.Fatalf("seed CreateAccount: %v", err)
	}
	if _, err := database.CreateJournal(db.CreateJournalRequest{
		ID: "org-a-journal", OrganizationID: "org-a", Code: "TSTJ", Name: "Test Journal", Type: "miscellaneous",
	}); err != nil {
		t.Fatalf("seed CreateJournal: %v", err)
	}
	fiscalYear, err := database.CreateFiscalYear(db.CreateFiscalYearRequest{
		ID: "org-a-fiscal-year", OrganizationID: "org-a", Name: "FY2030",
		StartDate: 1893456000000, EndDate: 1924992000000, // 2030-01-01 .. 2030-12-31 (a range no other seed in this test touches)
	})
	if err != nil {
		t.Fatalf("seed CreateFiscalYear: %v", err)
	}
	if _, err := database.CreateFiscalPeriod(db.CreateFiscalPeriodRequest{
		ID: "org-a-fiscal-period", OrganizationID: "org-a", FiscalYearID: fiscalYear.ID, Name: "Q1 2030",
		StartDate: 1893456000000, EndDate: 1901318400000,
	}); err != nil {
		t.Fatalf("seed CreateFiscalPeriod: %v", err)
	}
	if orgA.DefaultRevenueAccountID == nil {
		t.Fatal("expected org-a's auto-seeded chart of accounts to set a default revenue account")
	}
	if _, err := database.CreateJournalEntry(db.CreateJournalEntryRequest{
		ID: "org-a-journal-entry", OrganizationID: "org-a", JournalID: "org-a-journal", Date: 1893456000000,
		Description: "Test entry",
		Lines: []db.CreateJournalLineRequest{
			{AccountID: account.ID, Debit: 100},
			{AccountID: *orgA.DefaultRevenueAccountID, Credit: 100},
		},
	}); err != nil {
		t.Fatalf("seed CreateJournalEntry: %v", err)
	}
	// CreatePayment requires a posted GL entry on the invoice it settles —
	// real business-rule setup this fixture has no need to satisfy just to
	// prove the route's org-membership gate. Inserted directly, the same
	// precedent api/auth_test.go and api/restore_test.go already use for a
	// fixture the public API can't (or shouldn't have to) produce.
	if _, err := database.DB.Exec(
		`INSERT INTO payments (id, organizationId, direction, clientId, bankAccountId, amount, currency, date, method)
		 VALUES (?, ?, 'inbound', ?, ?, 100, 'EUR', 1700000000000, 'bank_transfer')`,
		"org-a-payment", "org-a", client.ID, account.ID,
	); err != nil {
		t.Fatalf("seed payment insert: %v", err)
	}

	for _, tc := range crossOrgProof {
		t.Run(tc.name, func(t *testing.T) {
			var body *bytes.Buffer
			if tc.body != nil {
				body = bytes.NewBuffer(tc.body)
			} else {
				body = bytes.NewBuffer(nil)
			}
			req := httptest.NewRequest(tc.method, tc.path, body)
			req.Header.Set("Content-Type", "application/json")
			authRequest(req, orgBToken)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusForbidden && rec.Code != http.StatusNotFound {
				t.Fatalf("org-b-admin %s %s: expected 403 or 404, got %d: %s",
					tc.method, tc.path, rec.Code, rec.Body.String())
			}
		})
	}

	// Positive control: org-a's own admin must still be able to do all of
	// the above — a test that only ever asserts "denied" can't tell a
	// correctly-scoped check apart from one that denies everyone.
	orgAToken := mintTestJWT(t, "org-a-admin", "user")
	req := httptest.NewRequest(http.MethodGet, "/api/clients/"+client.ID, nil)
	authRequest(req, orgAToken)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("org-a-admin GET /api/clients/%s: expected 200, got %d: %s", client.ID, rec.Code, rec.Body.String())
	}
	var got db.Client
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ID != client.ID {
		t.Fatalf("expected client %q, got %q", client.ID, got.ID)
	}

	// Positive control for the route families added in the 2026-09-14 audit
	// (F80). The deny loop above accepts 404, which a *typo'd* path also
	// returns — for every caller, making the entry pass vacuously. These
	// assert org-a's own admin really can reach the same paths, so a 404 in
	// the loop above means "denied", not "no such route".
	//
	// Read-only routes only: the loop's destructive entries can't be
	// positively exercised without destroying the fixture the other subtests
	// share.
	for _, path := range []string{
		"/api/organizations/org-a/production-orders",
		"/api/organizations/org-a/production-orders/next-number",
		"/api/production-orders/org-a-production-order",
		"/api/production-orders/org-a-production-order/component-lines",
		"/api/organizations/org-a/units-of-measure",
	} {
		t.Run("reachable by its own org: "+path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, bytes.NewBuffer(nil))
			req.Header.Set("Content-Type", "application/json")
			authRequest(req, orgAToken)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("org-a-admin GET %s: expected 200, got %d: %s — the deny-path entry for this route would be passing vacuously",
					path, rec.Code, rec.Body.String())
			}
		})
	}
}

// --- Domain-role enforcement (org role redesign) --------------------------

// TestDomainRoleEnforcement is real, not just AST-verified, evidence that
// the domain roles introduced by the org role redesign actually gate what
// they claim to: a member scoped to one domain can write within it and is
// denied outside it, while "general" keeps full access to both — proving
// the mechanism itself, not just that the router table claims it (that
// structural guarantee is TestDomainRoleRouteCoverage's job, the same
// division TestPhaseCRouteCoverage/TestCrossOrgAccessDenied already have
// for org-scoping).
func TestDomainRoleEnforcement(t *testing.T) {
	t.Parallel()
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "org-r-admin", "user", 1)
	seedUser(t, database, "sales-1", "user", 1)
	seedUser(t, database, "purchasing-1", "user", 1)
	seedUser(t, database, "general-1", "user", 1)

	if _, err := database.CreateOrganization(db.CreateOrganizationRequest{ID: "org-r"}); err != nil {
		t.Fatalf("seed CreateOrganization: %v", err)
	}
	if _, err := database.AddOrganizationUser("org-r", "org-r-admin", "admin"); err != nil {
		t.Fatalf("seed org-r-admin membership: %v", err)
	}
	if _, err := database.AddOrganizationUser("org-r", "sales-1", "sales"); err != nil {
		t.Fatalf("seed sales-1 membership: %v", err)
	}
	if _, err := database.AddOrganizationUser("org-r", "purchasing-1", "purchasing"); err != nil {
		t.Fatalf("seed purchasing-1 membership: %v", err)
	}
	if _, err := database.AddOrganizationUser("org-r", "general-1", "general"); err != nil {
		t.Fatalf("seed general-1 membership: %v", err)
	}

	client, err := database.CreateClient(db.CreateClientRequest{
		ID: "org-r-client", OrganizationID: "org-r", Name: strPtr("ACME"),
	})
	if err != nil {
		t.Fatalf("seed CreateClient: %v", err)
	}
	newVendor := func(id string) *db.Vendor {
		v, err := database.CreateVendor(db.CreateVendorRequest{ID: id, OrganizationID: "org-r", Name: strPtr("Vendor " + id)})
		if err != nil {
			t.Fatalf("seed CreateVendor %s: %v", id, err)
		}
		return v
	}

	salesToken := mintTestJWT(t, "sales-1", "")
	purchasingToken := mintTestJWT(t, "purchasing-1", "")
	generalToken := mintTestJWT(t, "general-1", "")

	// sales-1 can write its own domain (clients)...
	rec := doJSON(t, mux, salesToken, http.MethodPut, "/api/clients/"+client.ID, map[string]any{"name": "Sales Edit"})
	if rec.Code != http.StatusOK {
		t.Fatalf("sales-1 PUT client: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	// ...but not purchasing's.
	vendorA := newVendor("org-r-vendor-a")
	rec = doJSON(t, mux, salesToken, http.MethodDelete, "/api/vendors/"+vendorA.ID, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("sales-1 DELETE vendor: expected 403, got %d: %s", rec.Code, rec.Body.String())
	}

	// purchasing-1 is the mirror image: vendors yes, clients no.
	rec = doJSON(t, mux, purchasingToken, http.MethodDelete, "/api/vendors/"+vendorA.ID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("purchasing-1 DELETE vendor: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, mux, purchasingToken, http.MethodPut, "/api/clients/"+client.ID, map[string]any{"name": "Purchasing Edit"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("purchasing-1 PUT client: expected 403, got %d: %s", rec.Code, rec.Body.String())
	}

	// general-1 keeps full access to both — unchanged from before domain
	// roles existed, confirming "general" wasn't accidentally narrowed.
	rec = doJSON(t, mux, generalToken, http.MethodPut, "/api/clients/"+client.ID, map[string]any{"name": "General Edit"})
	if rec.Code != http.StatusOK {
		t.Fatalf("general-1 PUT client: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	vendorB := newVendor("org-r-vendor-b")
	rec = doJSON(t, mux, generalToken, http.MethodDelete, "/api/vendors/"+vendorB.ID, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("general-1 DELETE vendor: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestOrgRoleAdminEnforcement covers orgRoleAdmin — the modeAdminStrict
// counterpart to orgRole, used for the two actions the org role redesign
// folded "accounting" into alongside admin (fiscal-year close, GL export).
// GL export is the cheaper of the two to exercise here: passing the role
// gate with no fiscalYearId query param reaches the handler's own 400
// ("fiscalYearId is required"), which is enough to prove the request got
// *past* authorization — distinct from the 403 a denied role/non-member
// gets, without needing a fully valid FEC-generation fixture.
func TestOrgRoleAdminEnforcement(t *testing.T) {
	t.Parallel()
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "org-x-admin", "user", 1)
	seedUser(t, database, "accounting-1", "user", 1)
	seedUser(t, database, "sales-1", "user", 1)
	seedUser(t, database, "outsider", "user", 1)

	if _, err := database.CreateOrganization(db.CreateOrganizationRequest{ID: "org-x"}); err != nil {
		t.Fatalf("seed CreateOrganization: %v", err)
	}
	if _, err := database.AddOrganizationUser("org-x", "org-x-admin", "admin"); err != nil {
		t.Fatalf("seed org-x-admin membership: %v", err)
	}
	if _, err := database.AddOrganizationUser("org-x", "accounting-1", "accounting"); err != nil {
		t.Fatalf("seed accounting-1 membership: %v", err)
	}
	if _, err := database.AddOrganizationUser("org-x", "sales-1", "sales"); err != nil {
		t.Fatalf("seed sales-1 membership: %v", err)
	}
	// outsider is deliberately not a member of org-x at all.

	get := func(actor string) *httptest.ResponseRecorder {
		token := mintTestJWT(t, actor, "")
		req := httptest.NewRequest(http.MethodGet, "/api/organizations/org-x/gl-export/fec", nil)
		authRequest(req, token)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	// accounting-1 passes the role gate — reaches the handler's own
	// validation (400, not 403/404).
	if rec := get("accounting-1"); rec.Code != http.StatusBadRequest {
		t.Fatalf("accounting-1 GL export: expected 400 (past the role gate), got %d: %s", rec.Code, rec.Body.String())
	}
	// sales-1 is a member of org-x, just the wrong role — denied.
	if rec := get("sales-1"); rec.Code != http.StatusForbidden {
		t.Fatalf("sales-1 GL export: expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
	// outsider isn't a member of org-x at all — orgRoleAdmin's
	// modeAdminStrict shape means this is also 403, not 404 (see
	// orgRoleAdmin's doc comment in api/middleware.go).
	if rec := get("outsider"); rec.Code != http.StatusForbidden {
		t.Fatalf("outsider GL export: expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}
