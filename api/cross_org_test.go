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
	{"POST", "/api/incoming-invoices"}:  {"incoming_invoices.go", "createIncomingInvoice"},
	{"POST", "/api/tax-rates"}:          {"tax_rates.go", "createTaxRate"},
	{"POST", "/api/payment-terms"}:      {"payment_terms.go", "createPaymentTerm"},
	{"POST", "/api/products"}:           {"products.go", "createProduct"},
	{"POST", "/api/stock-movements"}:    {"stock.go", "createStockMovement"},
	{"POST", "/api/accounts"}:           {"accounts.go", "createAccount"},
	{"POST", "/api/journals"}:           {"journals.go", "createJournal"},
	{"POST", "/api/fiscal-years"}:       {"fiscal_periods.go", "createFiscalYear"},
	{"POST", "/api/fiscal-periods"}:     {"fiscal_periods.go", "createFiscalPeriod"},
	{"POST", "/api/journal-entries"}:    {"journal_entries.go", "createJournalEntry"},
	{"POST", "/api/payments"}:           {"payments.go", "createPayment"},
}

// pendingPhaseCRoutes are known, tracked, not-yet-migrated routes — see the
// package comment above. Remove an entry the same PR its route moves to
// orgMemberProtected/orgAdminProtected or gains a createRouteOrgChecks entry.
var pendingPhaseCRoutes = map[routeKey]bool{
	// Create bucket (still POST-organizationId-in-body, no requireOrgMember
	// call yet) and single-resource/sub-resource/export buckets, grouped by
	// the domain-family PRs that will migrate them per the rollout plan.

	{"GET", "/api/tax-rates/{id}"}:             true,
	{"PUT", "/api/tax-rates/{id}"}:             true,
	{"DELETE", "/api/tax-rates/{id}"}:          true,
	{"GET", "/api/tax-rates/{id}/usage-count"}: true,

	{"PUT", "/api/payment-terms/{id}"}:    true,
	{"DELETE", "/api/payment-terms/{id}"}: true,

	{"GET", "/api/products/{id}"}:                 true,
	{"PUT", "/api/products/{id}"}:                 true,
	{"DELETE", "/api/products/{id}"}:              true,
	{"GET", "/api/products/{id}/stock-movements"}: true,
	{"GET", "/api/products/{id}/serial-numbers"}:  true,

	{"DELETE", "/api/stock-movements/{id}"}: true,

	{"GET", "/api/accounts/{id}"}:    true,
	{"PUT", "/api/accounts/{id}"}:    true,
	{"DELETE", "/api/accounts/{id}"}: true,

	{"PUT", "/api/journals/{id}"}:    true,
	{"DELETE", "/api/journals/{id}"}: true,

	{"PATCH", "/api/fiscal-periods/{id}/status"}: true,

	{"GET", "/api/journal-entries/{id}"}:          true,
	{"GET", "/api/journal-entries/{id}/lines"}:    true,
	{"PATCH", "/api/journal-entries/{id}/post"}:   true,
	{"POST", "/api/journal-entries/{id}/reverse"}: true,
	{"DELETE", "/api/journal-entries/{id}"}:       true,

	{"GET", "/api/payments/{id}"}:              true,
	{"GET", "/api/payments/{id}/applications"}: true,
	{"POST", "/api/payments/{id}/void"}:        true,
}

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

// TestCreateRouteOrgChecksArePresent parses each createRouteOrgChecks
// handler's own source file and confirms its function body actually calls
// h.requireOrgMember — the thing that makes "POST /api/clients is in
// createRouteOrgChecks" a verified fact rather than an assertion the
// handler could silently stop satisfying (e.g. a future refactor that
// removes the check without anyone remembering to update this table).
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
			callsRequireOrgMember := false
			ast.Inspect(found, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if ok && sel.Sel.Name == "requireOrgMember" {
					callsRequireOrgMember = true
				}
				return true
			})
			if !callsRequireOrgMember {
				t.Errorf(
					"%s (%s) is listed in createRouteOrgChecks but its handler %s doesn't call h.requireOrgMember",
					key, loc.file, loc.fn,
				)
			}
		})
	}
}

// --- Cross-organization access denial (issue #141 Phase C) ---------------

// crossOrgProof is a representative sample of already-gated routes verified
// end to end against a real cross-tenant request — grown alongside
// pendingPhaseCRoutes shrinking in later Phase C PRs, per the rollout plan
// (/Users/mam/.claude/plans/tranquil-toasting-eagle.md). Not exhaustive by
// design (that's TestPhaseCRouteCoverage's job, structurally): a list + a
// single-resource get + one mutation per newly-gated domain is enough to
// prove the mechanism actually denies a real request, not just that the
// router table claims it does. PR7's final regression sweep expands this to
// every route.
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
	if _, err := database.CreateOrganization(db.CreateOrganizationRequest{ID: "org-a"}); err != nil {
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
}
