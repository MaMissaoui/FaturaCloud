package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// TestUpdateIncomingInvoice_CannotChangeStateViaPUT mirrors
// TestUpdateInvoice_CannotChangeStateViaPUT for vendor bills —
// db.UpdateIncomingInvoiceRequest has no State field either, so state stays
// settable only through PATCH .../state, where 3-way matching is enforced.
func TestUpdateIncomingInvoice_CannotChangeStateViaPUT(t *testing.T) {
	t.Parallel()
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "test-user", "user", 1)
	token := mintTestJWT(t, "test-user", "user")
	org, err := database.CreateOrganization(db.CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("seed CreateOrganization: %v", err)
	}
	vendor, err := database.CreateVendor(db.CreateVendorRequest{ID: "vendor-1", OrganizationID: org.ID, Name: strPtr("Acme Supplies")})
	if err != nil {
		t.Fatalf("seed CreateVendor: %v", err)
	}
	if _, err := database.AddOrganizationUser(org.ID, "test-user", "user"); err != nil {
		t.Fatalf("seed org membership: %v", err)
	}

	inv, err := database.CreateIncomingInvoice(db.CreateIncomingInvoiceRequest{
		ID: "bill-1", OrganizationID: org.ID, VendorID: vendor.ID, VendorInvoiceNumber: "V-001",
		State: "draft", Date: 1700000000000, Currency: "EUR",
	})
	if err != nil {
		t.Fatalf("seed CreateIncomingInvoice: %v", err)
	}

	body, _ := json.Marshal(map[string]any{"state": "paid", "notes": "updated"})
	req := httptest.NewRequest(http.MethodPut, "/api/incoming-invoices/"+inv.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	authRequest(req, token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got db.IncomingInvoice
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.State != "draft" {
		t.Fatalf("expected state to stay %q (PUT can't change it), got %q", "draft", got.State)
	}
	if got.Notes == nil || *got.Notes != "updated" {
		t.Fatalf("expected notes to update, got %v", got.Notes)
	}
}
