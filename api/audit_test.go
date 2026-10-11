package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/MaMissaoui/fatura-cloud/db"
)

func TestAuditTarget(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ path, resource, idParam string }{
		{"/api/invoices/{id}/state", "invoices", "id"},
		{"/api/clients", "clients", ""},
		{"/api/organizations/{id}", "organizations", "id"},
		{"/api/organizations/{id}/reset", "reset", "id"},
		{"/api/organizations/{id}/logo", "logo", "id"},
		{"/api/organizations/{orgId}/members/{userId}", "members", "userId"},
		{"/api/organizations/{orgId}/clients/import", "clients", ""},
		{"/api/backups/{name}/restore", "backups", "name"},
		{"/api/cash-sales/{id}/payments", "cash-sales", "id"},
	} {
		resource, idParam := auditTarget(c.path)
		if resource != c.resource || idParam != c.idParam {
			t.Errorf("auditTarget(%s) = %q, %q; want %q, %q", c.path, resource, idParam, c.resource, c.idParam)
		}
	}
}

type auditFixture struct {
	mux   http.Handler
	d     *db.Database
	admin string
}

func newAuditFixture(t *testing.T) auditFixture {
	t.Helper()
	mux, database, _, _ := newTestRouter(t)
	if _, err := database.CreateOrganization(db.CreateOrganizationRequest{ID: "org-audit"}); err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	seedUser(t, database, "audit-admin", "user", 1)
	if _, err := database.AddOrganizationUser("org-audit", "audit-admin", "admin"); err != nil {
		t.Fatalf("AddOrganizationUser: %v", err)
	}
	return auditFixture{mux: mux, d: database, admin: mintTestJWT(t, "audit-admin", "user")}
}

func (f auditFixture) do(t *testing.T, token, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	authRequest(req, token)
	rec := httptest.NewRecorder()
	f.mux.ServeHTTP(rec, req)
	return rec
}

func (f auditFixture) events(t *testing.T) []db.AuditEvent {
	t.Helper()
	events, err := f.d.ListAuditEvents(db.AuditEventFilter{OrganizationID: "org-audit"})
	if err != nil {
		t.Fatalf("ListAuditEvents: %v", err)
	}
	return events
}

// A create, a state change and a delete are each recorded with the user,
// the organization and the document; reads and refused changes are not.
func TestChangesAreRecordedInTheActivityHistory(t *testing.T) {
	t.Parallel()
	f := newAuditFixture(t)

	rec := f.do(t, f.admin, http.MethodPost, "/api/clients", `{"organizationId":"org-audit","name":"Fatma Trabelsi"}`)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("create client: %d %s", rec.Code, rec.Body)
	}
	var client struct{ ID string }
	_ = json.Unmarshal(rec.Body.Bytes(), &client)

	f.do(t, f.admin, http.MethodGet, "/api/organizations/org-audit/clients", "")
	if rec := f.do(t, f.admin, http.MethodDelete, "/api/clients/no-such-client", ""); rec.Code < 400 {
		t.Fatalf("deleting a missing client answered %d", rec.Code)
	}

	if _, err := f.d.DB.Exec(`INSERT INTO invoices (id, organizationId, number, state, clientId, date, dueDate, subTotal, taxTotal, total)
		VALUES ('inv-audit', 'org-audit', 'FAC-77', 'draft', ?, 1000, 2000, 0, 0, 0)`, client.ID); err != nil {
		t.Fatalf("insert invoice: %v", err)
	}
	if rec := f.do(t, f.admin, http.MethodPatch, "/api/invoices/inv-audit/state", `{"state":"cancelled"}`); rec.Code != http.StatusOK {
		t.Fatalf("cancel invoice: %d %s", rec.Code, rec.Body)
	}
	if _, err := f.d.DB.Exec(`DELETE FROM invoices WHERE id = 'inv-audit'`); err != nil {
		t.Fatalf("delete invoice: %v", err)
	}
	if rec := f.do(t, f.admin, http.MethodDelete, "/api/clients/"+client.ID, ""); rec.Code >= 300 {
		t.Fatalf("delete client: %d %s", rec.Code, rec.Body)
	}

	events := f.events(t)
	if len(events) != 3 {
		t.Fatalf("got %d events, want 3 (create, state change, delete): %+v", len(events), events)
	}
	// Picked by method: changes made in the same millisecond have no order
	// between them (the history sorts by time, then by random id).
	byMethod := map[string]db.AuditEvent{}
	for _, e := range events {
		byMethod[e.Method] = e
	}
	del, state, create := byMethod["DELETE"], byMethod["PATCH"], byMethod["POST"]
	for _, e := range events {
		if e.UserID == nil || *e.UserID != "audit-admin" || e.UserEmail != "audit-admin@test.local" ||
			e.OrganizationID == nil || *e.OrganizationID != "org-audit" || e.RequestID == "" {
			t.Errorf("event %s %s lacks who/where: %+v", e.Method, e.Route, e)
		}
	}
	if create.Method != "POST" || create.Resource != "clients" || create.EntityID != client.ID || create.EntityLabel != "Fatma Trabelsi" {
		t.Errorf("create = %+v", create)
	}
	if state.Route != "/api/invoices/{id}/state" || state.EntityLabel != "FAC-77" || state.FromState != "draft" || state.ToState != "cancelled" {
		t.Errorf("state change = %+v", state)
	}
	// The label is read before the delete: afterwards there's nothing to read.
	if del.Method != "DELETE" || del.EntityID != client.ID || del.EntityLabel != "Fatma Trabelsi" {
		t.Errorf("delete = %+v", del)
	}
}

// The history is for the organization's admins; a platform action (a new
// user) has no organization and is listed for platform admins.
func TestActivityHistoryRoutes(t *testing.T) {
	t.Parallel()
	f := newAuditFixture(t)
	seedUser(t, f.d, "audit-sales", "user", 1)
	if _, err := f.d.AddOrganizationUser("org-audit", "audit-sales", "sales"); err != nil {
		t.Fatalf("AddOrganizationUser: %v", err)
	}
	seedUser(t, f.d, "platform-admin", "admin", 1)
	platform := mintTestJWT(t, "platform-admin", "admin")

	for i, name := range []string{"A", "B", "C"} {
		body := `{"organizationId":"org-audit","name":"Client ` + name + `"}`
		if rec := f.do(t, f.admin, http.MethodPost, "/api/clients", body); rec.Code >= 300 {
			t.Fatalf("create client %d: %d %s", i, rec.Code, rec.Body)
		}
	}
	if rec := f.do(t, platform, http.MethodPost, "/api/users",
		`{"email":"new@test.local","password":"a-long-enough-password","displayName":"New","role":"user"}`); rec.Code >= 300 {
		t.Fatalf("create user: %d %s", rec.Code, rec.Body)
	}

	type page struct {
		Events []db.AuditEvent `json:"events"`
		Next   string          `json:"next"`
	}
	read := func(token, path string) (int, page) {
		rec := f.do(t, token, http.MethodGet, path, "")
		var p page
		_ = json.Unmarshal(rec.Body.Bytes(), &p)
		return rec.Code, p
	}

	code, first := read(f.admin, "/api/organizations/org-audit/audit-events?limit=2")
	if code != http.StatusOK || len(first.Events) != 2 || first.Next == "" {
		t.Fatalf("first page: %d %+v", code, first)
	}
	_, second := read(f.admin, "/api/organizations/org-audit/audit-events?limit=2&before="+first.Next)
	if len(second.Events) != 1 || second.Next != "" {
		t.Errorf("second page = %+v, want the one remaining create", second)
	}
	// Together the pages hold each create once. Which one comes last isn't
	// asserted: creates in the same millisecond have no order between them.
	seen := map[string]int{}
	for _, e := range append(first.Events, second.Events...) {
		seen[e.EntityLabel]++
	}
	if seen["Client A"] != 1 || seen["Client B"] != 1 || seen["Client C"] != 1 {
		t.Errorf("pages hold %v, want each client once", seen)
	}
	if code, _ := read(mintTestJWT(t, "audit-sales", "user"), "/api/organizations/org-audit/audit-events"); code != http.StatusForbidden {
		t.Errorf("a sales member read the history: %d", code)
	}

	code, platformPage := read(platform, "/api/audit-events")
	if code != http.StatusOK || len(platformPage.Events) != 1 ||
		platformPage.Events[0].Resource != "users" || platformPage.Events[0].EntityLabel != "new@test.local" ||
		platformPage.Events[0].OrganizationID != nil {
		t.Errorf("platform history = %d %+v", code, platformPage)
	}
	if code, _ := read(f.admin, "/api/audit-events"); code != http.StatusForbidden {
		t.Errorf("an organization admin read the platform history: %d", code)
	}
}

// A backup taken on demand is named by its file, and a restore by the file it
// was uploaded from: neither has a table the history could read a name from.
func TestBackupsAndRestoresAreNamedInTheHistory(t *testing.T) {
	t.Parallel()
	f := newAuditFixture(t)
	seedUser(t, f.d, "platform-admin", "admin", 1)
	platform := mintTestJWT(t, "platform-admin", "admin")

	platformEvents := func() []db.AuditEvent {
		rec := f.do(t, platform, http.MethodGet, "/api/audit-events", "")
		var p struct {
			Events []db.AuditEvent `json:"events"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &p)
		return p.Events
	}

	backup := f.do(t, platform, http.MethodPost, "/api/backups", "")
	if backup.Code != http.StatusOK {
		t.Fatalf("backup: %d %s", backup.Code, backup.Body)
	}
	name := downloadName(backup.Header())
	if !strings.HasPrefix(name, "fatura-backup-") {
		t.Fatalf("backup offered no file name: %q", backup.Header().Get("Content-Disposition"))
	}
	if events := platformEvents(); len(events) != 1 || events[0].Resource != "backups" || events[0].EntityLabel != name {
		t.Errorf("backup history = %+v, want one row named %s", events, name)
	}

	body, contentType := multipartDatabaseUpload(t, "monday.db", backup.Body.Bytes())
	req := httptest.NewRequest(http.MethodPost, "/api/restore", body)
	req.Header.Set("Content-Type", contentType)
	authRequest(req, platform)
	rec := httptest.NewRecorder()
	f.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("restore: %d %s", rec.Code, rec.Body)
	}
	// The restored database is the backup, taken before its own row was
	// written, plus the restore's row.
	if events := platformEvents(); len(events) != 1 || events[0].Resource != "restore" || events[0].EntityLabel != "monday.db" {
		t.Errorf("restore history = %+v, want one row named monday.db", events)
	}
}

// Every audited create route must say how its history row learns the
// document it made: a response that leads with the document's own id (the
// pattern api/audit.go falls back to), a response whose first field leads
// with it, an explicit setAuditEntity, or no single document at all. Audit
// F164: a cash sale answered client first and a member add with the
// membership row, so their rows named the wrong document. A new create
// route, or a response struct reordered so another id comes first, fails
// here.
func TestAuditCreateRoutesNameTheirDocument(t *testing.T) {
	t.Parallel()
	leadsWithID := map[string]any{
		"/api/organizations":      db.Organization{},
		"/api/clients":            db.Client{},
		"/api/vendors":            db.Vendor{},
		"/api/imports":            db.Import{},
		"/api/purchase-orders":    db.PurchaseOrder{},
		"/api/inbound-deliveries": db.InboundDelivery{},
		"/api/production-orders":  db.ProductionOrder{},
		"/api/incoming-invoices":  db.IncomingInvoice{},
		"/api/invoices":           db.Invoice{},
		"/api/tax-rates":          db.TaxRate{},
		"/api/payment-terms":      db.PaymentTerm{},
		"/api/units-of-measure":   db.UnitOfMeasure{},
		"/api/product-families":   db.ProductFamily{},
		"/api/products":           db.Product{},
		"/api/orders":             db.Order{},
		"/api/deliveries":         db.OutboundDelivery{},
		"/api/accounts":           db.Account{},
		"/api/journals":           db.Journal{},
		"/api/fiscal-years":       db.FiscalYear{},
		"/api/fiscal-periods":     db.FiscalPeriod{},
		"/api/journal-entries":    db.JournalEntry{},
		"/api/payments":           db.Payment{},
	}
	// The first field holds the document (or the first of several).
	leadsViaFirstField := map[string]any{
		"/api/stock-movements": db.CreateStockMovementResult{},
		"/api/cash-movements":  db.CashMovementResult{},
	}
	explicit := map[string]bool{
		"/api/cash-sales":                    true,
		"/api/organizations/{orgId}/members": true,
	}
	noSingleDocument := map[string]string{
		"/api/backups": "named by the download's file name",
		"/api/restore": "a database restore; the handler names the uploaded file",
		"/api/users":   "userToJSON is a flat map whose only id is the user's",
		"/api/organizations/{orgId}/loan-imports":            "a batch report; the handler sets the label",
		"/api/organizations/{orgId}/clients/import":          "a mass-data report",
		"/api/organizations/{orgId}/vendors/import":          "a mass-data report",
		"/api/organizations/{orgId}/tax-rates/import":        "a mass-data report",
		"/api/organizations/{orgId}/payment-terms/import":    "a mass-data report",
		"/api/organizations/{orgId}/units-of-measure/import": "a mass-data report",
		"/api/organizations/{orgId}/products/import":         "a mass-data report",
		"/api/organizations/{orgId}/stock-movements/import":  "a mass-data report",
		"/api/organizations/{orgId}/accounts/import":         "a mass-data report",
	}

	seen := map[string]bool{}
	for rk := range discoverRoutes(t) {
		if rk.method != http.MethodPost || auditSkipped(rk.pattern) {
			continue
		}
		if _, idParam := auditTarget(rk.pattern); idParam != "" {
			continue
		}
		seen[rk.pattern] = true
		lead, okLead := leadsWithID[rk.pattern]
		via, okVia := leadsViaFirstField[rk.pattern]
		_, okNone := noSingleDocument[rk.pattern]
		classes := 0
		for _, ok := range []bool{okLead, okVia, explicit[rk.pattern], okNone} {
			if ok {
				classes++
			}
		}
		if classes != 1 {
			t.Errorf("create route %s is in %d classes; put it in exactly one", rk, classes)
			continue
		}
		if okLead {
			if f := firstJSONField(reflect.TypeOf(lead)); f.name != "id" {
				t.Errorf("%s answers with %T, whose first JSON field is %q, not \"id\"", rk, lead, f.name)
			}
		}
		if okVia {
			f := firstJSONField(reflect.TypeOf(via))
			if inner := firstJSONField(f.typ); inner.name != "id" {
				t.Errorf("%s answers with %T: its first field %q leads with %q, not \"id\"", rk, via, f.name, inner.name)
			}
		}
	}
	for _, list := range []map[string]bool{keys(leadsWithID), keys(leadsViaFirstField), explicit, keys(noSingleDocument)} {
		for pattern := range list {
			if !seen[pattern] {
				t.Errorf("%s is listed but is not an audited create route in router.go", pattern)
			}
		}
	}
}

type jsonField struct {
	name string
	typ  reflect.Type
}

// firstJSONField is the first field t encodes to JSON, its pointer, slice
// or array element type unwrapped.
func firstJSONField(t reflect.Type) jsonField {
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Array {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return jsonField{}
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if !f.IsExported() || tag == "-" {
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if name == "" {
			name = f.Name
		}
		return jsonField{name: name, typ: f.Type}
	}
	return jsonField{}
}

func keys[V any](m map[string]V) map[string]bool {
	out := make(map[string]bool, len(m))
	for k := range m {
		out[k] = true
	}
	return out
}

// A cash sale's row names its invoice and a member add's the added user,
// although both responses lead with another id (audit F164).
func TestWrappedCreatesNameTheirDocument(t *testing.T) {
	t.Parallel()
	f := newAuditFixture(t)
	seedUser(t, f.d, "new-member", "user", 1)
	if rec := f.do(t, f.admin, http.MethodPost, "/api/organizations/org-audit/members",
		`{"email":"new-member@test.local","role":"general"}`); rec.Code != http.StatusCreated {
		t.Fatalf("add member: %d %s", rec.Code, rec.Body)
	}

	if _, err := f.d.CreateFiscalYear(db.CreateFiscalYearRequest{
		OrganizationID: "org-audit", Name: "2025", StartDate: 1735689600000, EndDate: 1767225599000,
	}); err != nil {
		t.Fatalf("CreateFiscalYear: %v", err)
	}
	register := ""
	accounts, err := f.d.GetAccounts("org-audit")
	if err != nil {
		t.Fatalf("GetAccounts: %v", err)
	}
	for _, a := range accounts {
		if a.Code == "1010" {
			register = a.ID
		}
	}
	if _, err := f.d.UpdateOrganization("org-audit", db.UpdateOrganizationRequest{DefaultCashRegisterAccountID: &register}); err != nil {
		t.Fatalf("set register account: %v", err)
	}
	rec := f.do(t, f.admin, http.MethodPost, "/api/cash-sales", `{"organizationId":"org-audit",
		"newClient":{"name":"Walk-in"},"date":1738368000000,"currency":"EUR",
		"lineItems":[{"quantity":1,"unitPrice":1000,"description":"Kettle"}],
		"subTotal":1000,"taxTotal":0,"total":1000,"amountReceived":1000}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("cash sale: %d %s", rec.Code, rec.Body)
	}
	var sale db.CashSaleResult
	if err := json.Unmarshal(rec.Body.Bytes(), &sale); err != nil {
		t.Fatalf("decode cash sale: %v", err)
	}

	byResource := map[string]db.AuditEvent{}
	for _, e := range f.events(t) {
		if e.Method == http.MethodPost {
			byResource[e.Resource] = e
		}
	}
	if e := byResource["members"]; e.EntityID != "new-member" || e.EntityLabel != "new-member@test.local" {
		t.Errorf("member add row = %q %q, want the user and their email", e.EntityID, e.EntityLabel)
	}
	if e := byResource["cash-sales"]; e.EntityID != sale.Invoice.ID || e.EntityLabel != sale.Invoice.Number {
		t.Errorf("cash sale row = %q %q, want invoice %q %q", e.EntityID, e.EntityLabel, sale.Invoice.ID, sale.Invoice.Number)
	}
}
