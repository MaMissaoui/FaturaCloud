package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MaMissaoui/fatura-cloud/db"
)

type recordedChange struct {
	Field  string `json:"field"`
	From   any    `json:"from"`
	To     any    `json:"to"`
	Masked bool   `json:"masked"`
}

// changesOf decodes the changes of the newest event for method and route.
func changesOf(t *testing.T, events []db.AuditEvent, method, route string) (db.AuditEvent, map[string]recordedChange) {
	t.Helper()
	for _, e := range events {
		if e.Method != method || e.Route != route {
			continue
		}
		out := map[string]recordedChange{}
		if e.Changes != "" {
			var list []recordedChange
			if err := json.Unmarshal([]byte(e.Changes), &list); err != nil {
				t.Fatalf("changes %q: %v", e.Changes, err)
			}
			for _, c := range list {
				out[c.Field] = c
			}
		}
		return e, out
	}
	t.Fatalf("no %s %s event", method, route)
	return db.AuditEvent{}, nil
}

// An edit records the fields it changed, before and after, with the bank
// account masked; a create records none.
func TestActivityHistoryRecordsChangedFields(t *testing.T) {
	t.Parallel()
	f := newAuditFixture(t)
	rec := f.do(t, f.admin, http.MethodPost, "/api/clients", `{"organizationId":"org-audit","name":"Fatma Trabelsi","iban":"DE89370400440532013000"}`)
	if rec.Code >= 300 {
		t.Fatalf("create client: %d %s", rec.Code, rec.Body)
	}
	var client struct{ ID string }
	_ = json.Unmarshal(rec.Body.Bytes(), &client)
	if rec := f.do(t, f.admin, http.MethodPut, "/api/clients/"+client.ID, `{"name":"Fatma Ben Salah","iban":"TN5910006035183598478831","city":"Sfax"}`); rec.Code != http.StatusOK {
		t.Fatalf("update client: %d %s", rec.Code, rec.Body)
	}
	events := f.events(t)
	update, changes := changesOf(t, events, "PUT", "/api/clients/{id}")
	if c := changes["name"]; c.From != "Fatma Trabelsi" || c.To != "Fatma Ben Salah" {
		t.Errorf("name change = %+v", c)
	}
	if c := changes["city"]; c.From != "" && c.From != nil || c.To != "Sfax" {
		t.Errorf("city change = %+v", c)
	}
	if c := changes["iban"]; !c.Masked || c.From != "DE••••3000" || c.To != "TN••••8831" {
		t.Errorf("iban change = %+v, want masked", c)
	}
	if strings.Contains(string(update.Changes), "TN5910006035183598478831") {
		t.Errorf("the full IBAN was recorded: %s", update.Changes)
	}
	for _, f := range []string{"id", "organizationId", "createdAt", "phone"} {
		if _, ok := changes[f]; ok {
			t.Errorf("%s recorded as changed", f)
		}
	}
	if create, _ := changesOf(t, events, "POST", "/api/clients"); create.Changes != "" {
		t.Errorf("a create recorded changes %s", create.Changes)
	}
}

// A member's role lives in organization_users, not the users row: it is
// recorded all the same.
func TestActivityHistoryRecordsMemberRole(t *testing.T) {
	t.Parallel()
	f := newAuditFixture(t)
	seedUser(t, f.d, "audit-member", "user", 1)
	if _, err := f.d.AddOrganizationUser("org-audit", "audit-member", "general"); err != nil {
		t.Fatal(err)
	}
	if rec := f.do(t, f.admin, http.MethodPut, "/api/organizations/org-audit/members/audit-member", `{"role":"sales"}`); rec.Code >= 300 {
		t.Fatalf("change role: %d %s", rec.Code, rec.Body)
	}
	e, changes := changesOf(t, f.events(t), "PUT", "/api/organizations/{orgId}/members/{userId}")
	if c := changes["organizationRole"]; c.From != "general" || c.To != "sales" {
		t.Errorf("organizationRole change = %+v", c)
	}
	// Exactly that one change: the users row's own (legacy) role column
	// must not be compared against the organization role.
	var list []recordedChange
	_ = json.Unmarshal([]byte(e.Changes), &list)
	if len(list) != 1 {
		t.Errorf("recorded %d changes, want 1: %s", len(list), e.Changes)
	}
}

// Password hashes and token versions are never recorded.
func TestActivityHistoryNeverRecordsSecrets(t *testing.T) {
	t.Parallel()
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "root", "admin", 1)
	seedUser(t, database, "someone", "user", 1)
	req := `{"displayName":"Someone Else","password":"a-new-password-123"}`
	f := auditFixture{mux: mux, d: database}
	if rec := f.do(t, mintTestJWT(t, "root", "admin"), http.MethodPut, "/api/users/someone", req); rec.Code != http.StatusOK {
		t.Fatalf("update user: %d %s", rec.Code, rec.Body)
	}
	events, err := database.ListAuditEvents(db.AuditEventFilter{Platform: true})
	if err != nil {
		t.Fatal(err)
	}
	e, changes := changesOf(t, events, "PUT", "/api/users/{id}")
	if c := changes["displayName"]; c.To != "Someone Else" {
		t.Errorf("displayName change = %+v", c)
	}
	lower := strings.ToLower(string(e.Changes))
	if strings.Contains(lower, "hash") || strings.Contains(lower, "token") || strings.Contains(lower, "password") {
		t.Errorf("a secret was recorded: %s", e.Changes)
	}
}

// A logo upload and removal change their organization: recorded as hasLogo
// on the logo's own events, named as the organization. The logo's bytes
// never are.
func TestActivityHistoryRecordsLogoAsHasLogo(t *testing.T) {
	t.Parallel()
	f := newAuditFixture(t)
	if _, err := f.d.DB.Exec(`UPDATE organizations SET name = 'Audit SARL' WHERE id = 'org-audit'`); err != nil {
		t.Fatalf("name organization: %v", err)
	}
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, _ := w.CreateFormFile("file", "logo.png")
	part.Write([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0, 0, 0, 0})
	w.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/organizations/org-audit/logo", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	authRequest(req, f.admin)
	rec := httptest.NewRecorder()
	f.mux.ServeHTTP(rec, req)
	if rec.Code >= 300 {
		t.Fatalf("upload logo: %d %s", rec.Code, rec.Body)
	}
	if rec := f.do(t, f.admin, http.MethodDelete, "/api/organizations/org-audit/logo", ""); rec.Code >= 300 {
		t.Fatalf("delete logo: %d %s", rec.Code, rec.Body)
	}
	events := f.events(t)
	// Both rows are named as the organization the logo belongs to.
	for _, method := range []string{"POST", "DELETE"} {
		if e, _ := changesOf(t, events, method, "/api/organizations/{id}/logo"); e.EntityID != "org-audit" || e.EntityLabel != "Audit SARL" {
			t.Errorf("%s logo row names %q / %q, want org-audit / Audit SARL", method, e.EntityID, e.EntityLabel)
		}
	}
	if _, c := changesOf(t, events, "POST", "/api/organizations/{id}/logo"); c["hasLogo"].From != float64(0) || c["hasLogo"].To != float64(1) {
		t.Errorf("upload: hasLogo change = %+v", c["hasLogo"])
	}
	if _, c := changesOf(t, events, "DELETE", "/api/organizations/{id}/logo"); c["hasLogo"].From != float64(1) || c["hasLogo"].To != float64(0) {
		t.Errorf("removal: hasLogo change = %+v", c["hasLogo"])
	}
	for _, e := range events {
		if strings.Contains(string(e.Changes), `"logo"`) {
			t.Errorf("the logo itself was recorded: %s", e.Changes)
		}
	}
}
