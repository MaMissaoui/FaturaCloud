package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// TestDeleteOrganization_RequiresAdmin covers F17: deleting an organization
// cascade-deletes all of its clients/invoices/orders/deliveries, so it must
// be restricted to admins — a plain "user" role gets 403 and the
// organization must still exist afterward.
func TestDeleteOrganization_RequiresAdmin(t *testing.T) {
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "test-user", "user", 1)
	token := mintTestJWT(t, "test-user", "user")

	if _, err := database.CreateOrganization(db.CreateOrganizationRequest{ID: "org-1", Name: strPtr("ACME")}); err != nil {
		t.Fatalf("seed CreateOrganization: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/organizations/org-1", nil)
	authRequest(req, token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for a non-admin delete, got %d: %s", rec.Code, rec.Body.String())
	}

	orgs, err := database.GetOrganizations()
	if err != nil {
		t.Fatalf("GetOrganizations: %v", err)
	}
	if len(orgs) != 1 {
		t.Fatalf("expected the organization to survive a rejected delete, got %d orgs", len(orgs))
	}
}

// TestDeleteOrganization_AdminSucceeds is the positive counterpart — an
// admin can still delete an organization through the same route.
func TestDeleteOrganization_AdminSucceeds(t *testing.T) {
	mux, database, _, _ := newTestRouter(t)
	seedUser(t, database, "test-admin", "admin", 1)
	token := mintTestJWT(t, "test-admin", "admin")

	if _, err := database.CreateOrganization(db.CreateOrganizationRequest{ID: "org-1", Name: strPtr("ACME")}); err != nil {
		t.Fatalf("seed CreateOrganization: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/organizations/org-1", nil)
	authRequest(req, token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for an admin delete, got %d: %s", rec.Code, rec.Body.String())
	}

	orgs, err := database.GetOrganizations()
	if err != nil {
		t.Fatalf("GetOrganizations: %v", err)
	}
	if len(orgs) != 0 {
		t.Fatalf("expected the organization to be gone, got %d orgs", len(orgs))
	}
}
