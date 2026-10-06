package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
		{"/api/organizations/{id}/reset", "reset", ""},
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
	// Newest first.
	del, state, create := events[0], events[1], events[2]
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
	if len(second.Events) != 1 || second.Events[0].EntityLabel != "Client A" || second.Next != "" {
		t.Errorf("second page = %+v, want only the oldest create", second)
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
