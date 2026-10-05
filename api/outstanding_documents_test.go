package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// TestOutstandingInvoicesRoute checks that the Invoices list's outstanding
// set follows payments, not the state: a sent invoice paid in full drops out,
// a part-paid one shows its balance. It sits in the Sales section.
func TestOutstandingInvoicesRoute(t *testing.T) {
	t.Parallel()
	mux, database, _, _ := newTestRouter(t)

	if _, err := database.CreateOrganization(db.CreateOrganizationRequest{ID: "org-od"}); err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	for _, role := range []string{"sales", "purchasing"} {
		seedUser(t, database, role+"-user", "user", 1)
		if _, err := database.AddOrganizationUser("org-od", role+"-user", role); err != nil {
			t.Fatalf("add %s-user: %v", role, err)
		}
	}
	for _, q := range []string{
		`INSERT INTO clients (id, organizationId, name) VALUES ('od-c', 'org-od', 'A Client')`,
		`INSERT INTO invoices (id, organizationId, number, state, clientId, date, dueDate, total) VALUES
			('od-part', 'org-od', 'FAC-1', 'sent', 'od-c', 1000, 2000, 10000),
			('od-full', 'org-od', 'FAC-2', 'sent', 'od-c', 1000, 2000, 5000),
			('od-draft', 'org-od', 'FAC-3', 'draft', 'od-c', 1000, 2000, 7000)`,
		`INSERT INTO accounts (id, organizationId, code, name, type) VALUES ('od-bank', 'org-od', '1999', 'Test bank', 'asset')`,
		`INSERT INTO payments (id, organizationId, direction, clientId, bankAccountId, amount, currency, date, method) VALUES
			('od-pay', 'org-od', 'inbound', 'od-c', 'od-bank', 9000, 'EUR', 1500, 'bank_transfer')`,
		`INSERT INTO payment_applications (id, paymentId, documentType, documentId, amount) VALUES
			('od-pa1', 'od-pay', 'invoice', 'od-part', 4000), ('od-pa2', 'od-pay', 'invoice', 'od-full', 5000)`,
	} {
		if _, err := database.DB.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}

	get := func(role, path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		authRequest(req, mintTestJWT(t, role+"-user", "user"))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	rec := get("sales", "/api/organizations/org-od/invoices/outstanding")
	if rec.Code != http.StatusOK {
		t.Fatalf("sales: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var docs []outstandingDocument
	if err := json.Unmarshal(rec.Body.Bytes(), &docs); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(docs) != 1 || docs[0].ID != "od-part" || docs[0].Outstanding != 6000 || docs[0].Bucket == "current" {
		t.Errorf("outstanding = %+v, want only od-part with 6000 left, overdue", docs)
	}

	if rec := get("purchasing", "/api/organizations/org-od/invoices/outstanding"); rec.Code != http.StatusForbidden {
		t.Errorf("purchasing invoices: want 403, got %d", rec.Code)
	}
	if rec := get("purchasing", "/api/organizations/org-od/incoming-invoices/outstanding"); rec.Code != http.StatusOK {
		t.Errorf("purchasing bills: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec := get("sales", "/api/organizations/org-od/incoming-invoices/outstanding"); rec.Code != http.StatusForbidden {
		t.Errorf("sales bills: want 403, got %d", rec.Code)
	}
}
