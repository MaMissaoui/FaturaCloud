package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MaMissaoui/fatura-cloud/db"
)

func seedOrgAndClient(t *testing.T, database *db.Database) (orgID, clientID string) {
	t.Helper()
	org, err := database.CreateOrganization(db.CreateOrganizationRequest{ID: "org-1"})
	if err != nil {
		t.Fatalf("seed CreateOrganization: %v", err)
	}
	client, err := database.CreateClient(db.CreateClientRequest{ID: "client-1", OrganizationID: org.ID, Name: strPtr("ACME")})
	if err != nil {
		t.Fatalf("seed CreateClient: %v", err)
	}
	// Every caller of this helper also seeds "test-user" via seedUser — this
	// grants that same, consistently-named user org membership, now
	// required (issue #141 Phase C) for the single-resource routes these
	// tests exercise.
	if _, err := database.AddOrganizationUser(org.ID, "test-user", "user"); err != nil {
		t.Fatalf("seed org membership: %v", err)
	}
	return org.ID, client.ID
}

// TestUpdateInvoice_CannotChangeStateViaPUT confirms state is settable only
// via PATCH /api/invoices/{id}/state — db.UpdateInvoiceRequest has no State
// field at all, so a PUT body carrying "state" must be silently ignored
// rather than changing the invoice's state. This is an API-boundary
// contract db/'s own tests can't see: they call UpdateInvoice directly with
// a typed struct that has no field to even attempt smuggling a state
// through, so only a real HTTP request against the JSON body proves it.
func TestUpdateInvoice_CannotChangeStateViaPUT(t *testing.T) {
	t.Parallel()
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "test-user", "user", 1)
	token := mintTestJWT(t, "test-user", "user")
	orgID, clientID := seedOrgAndClient(t, database)

	inv, err := database.CreateInvoice(db.CreateInvoiceRequest{
		ID: "inv-1", OrganizationID: orgID, Number: "INV-001", State: "draft", ClientID: clientID,
		Date: 1700000000000, Currency: "EUR",
	})
	if err != nil {
		t.Fatalf("seed CreateInvoice: %v", err)
	}

	body, _ := json.Marshal(map[string]any{"state": "paid", "customerNotes": "updated"})
	req := httptest.NewRequest(http.MethodPut, "/api/invoices/"+inv.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	authRequest(req, token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got db.Invoice
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.State != "draft" {
		t.Fatalf("expected state to stay %q (PUT can't change it), got %q", "draft", got.State)
	}
	if got.CustomerNotes == nil || *got.CustomerNotes != "updated" {
		t.Fatalf("expected customerNotes to update, got %v", got.CustomerNotes)
	}
}

// TestUpdateInvoiceState_RejectsUnknownState confirms PATCH .../state maps
// an invalid state to a 409 via writeMutationError, not a 500 or a silent
// no-op — the "wrong error codes" risk issue #152 names.
func TestUpdateInvoiceState_RejectsUnknownState(t *testing.T) {
	t.Parallel()
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "test-user", "user", 1)
	token := mintTestJWT(t, "test-user", "user")
	orgID, clientID := seedOrgAndClient(t, database)

	inv, err := database.CreateInvoice(db.CreateInvoiceRequest{
		ID: "inv-1", OrganizationID: orgID, Number: "INV-001", State: "draft", ClientID: clientID,
		Date: 1700000000000, Currency: "EUR",
	})
	if err != nil {
		t.Fatalf("seed CreateInvoice: %v", err)
	}

	body, _ := json.Marshal(map[string]string{"state": "bogus"})
	req := httptest.NewRequest(http.MethodPatch, "/api/invoices/"+inv.ID+"/state", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	authRequest(req, token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409 for an invalid state, got %d: %s", rec.Code, rec.Body.String())
	}
}
