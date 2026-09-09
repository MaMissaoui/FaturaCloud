package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// TestCreatePayment_RejectsDirectionPartnerMismatch confirms
// db/payment.go's direction<->partner validation (an inbound payment
// requires a client and forbids a vendor, and vice versa) surfaces as a
// clean 409 through writeMutationError rather than an opaque 500 from the
// table's own CHECK constraint — exactly the "wrong error codes" risk issue
// #152 names, and only reachable through the HTTP layer since the check
// runs before any SQL is issued.
func TestCreatePayment_RejectsDirectionPartnerMismatch(t *testing.T) {
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

	// direction "inbound" with a vendorId and no clientId — CreatePayment's
	// direction<->partner validation is enforced app-side first (returning a
	// clean *db.ValidationError), before this could ever reach the table's
	// own CHECK constraint and surface as an opaque 500 instead.
	reqBody, _ := json.Marshal(map[string]any{
		"id": "pay-1", "organizationId": org.ID, "direction": "inbound",
		"vendorId": vendor.ID, "bankAccountId": "bank-1", "amount": 1000,
		"currency": "EUR", "date": 1700000000000, "method": "bank_transfer",
		"applications": []map[string]any{},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/payments", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	authRequest(req, token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for an inbound payment carrying a vendor, got %d: %s", rec.Code, rec.Body.String())
	}
}
