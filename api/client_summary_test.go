package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// TestClientSummaryRoutes exercises both client-summary endpoints for the
// section guard (sales → 200, purchasing → 403), the switched-off 409,
// and cross-org access denial.
func TestClientSummaryRoutes(t *testing.T) {
	t.Parallel()
	mux, database, _, _ := newTestRouter(t)

	// Users with different org roles.
	seedUser(t, database, "sales-user", "user", 1)
	seedUser(t, database, "purchasing-user", "user", 1)

	_, err := database.CreateOrganization(db.CreateOrganizationRequest{ID: "org-cs"})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if _, err := database.AddOrganizationUser("org-cs", "sales-user", "sales"); err != nil {
		t.Fatalf("add sales-user: %v", err)
	}
	if _, err := database.AddOrganizationUser("org-cs", "purchasing-user", "purchasing"); err != nil {
		t.Fatalf("add purchasing-user: %v", err)
	}

	// A client so the single-client summary route has something to resolve.
	client, err := database.CreateClient(db.CreateClientRequest{
		ID: "cs-client", OrganizationID: "org-cs", Name: strPtr("TestCo"),
	})
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}

	salesToken := mintTestJWT(t, "sales-user", "user")
	purchasingToken := mintTestJWT(t, "purchasing-user", "user")

	// --- Sales member gets 200 on both routes ---
	t.Run("sales gets 200 list", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/organizations/org-cs/clients/summary", nil)
		authRequest(req, salesToken)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var list db.ClientSummaryList
		if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
			t.Fatalf("decode: %v", err)
		}
	})

	t.Run("sales gets 200 detail", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/clients/"+client.ID+"/summary", nil)
		authRequest(req, salesToken)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	// --- Purchasing member gets 403 on both routes ---
	t.Run("purchasing gets 403 list", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/organizations/org-cs/clients/summary", nil)
		authRequest(req, purchasingToken)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("want 403, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("purchasing gets 403 detail", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/clients/"+client.ID+"/summary", nil)
		authRequest(req, purchasingToken)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("want 403, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	// --- Switch off summaries → sales gets 409 ---
	if _, err := database.UpdateOrganization("org-cs", db.UpdateOrganizationRequest{
		MasterDataSummaries: ptr(false),
	}); err != nil {
		t.Fatalf("switch off summaries: %v", err)
	}

	t.Run("sales gets 409 list when off", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/organizations/org-cs/clients/summary", nil)
		authRequest(req, salesToken)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusConflict {
			t.Fatalf("want 409, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("sales gets 409 detail when off", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/clients/"+client.ID+"/summary", nil)
		authRequest(req, salesToken)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusConflict {
			t.Fatalf("want 409, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	// --- Cross-org: a client from another org → 403 or 404 ---
	// Create another org and client there.
	seedUser(t, database, "other-user", "user", 1)
	if _, err := database.CreateOrganization(db.CreateOrganizationRequest{ID: "org-other"}); err != nil {
		t.Fatalf("CreateOrganization other: %v", err)
	}
	if _, err := database.AddOrganizationUser("org-other", "other-user", "admin"); err != nil {
		t.Fatalf("add other-user: %v", err)
	}
	otherClient, err := database.CreateClient(db.CreateClientRequest{
		ID: "other-client", OrganizationID: "org-other", Name: strPtr("OtherCo"),
	})
	if err != nil {
		t.Fatalf("CreateClient other: %v", err)
	}

	// Re-enable summaries for org-cs so the cross-org test isn't blocked by 409.
	if _, err := database.UpdateOrganization("org-cs", db.UpdateOrganizationRequest{
		MasterDataSummaries: ptr(true),
	}); err != nil {
		t.Fatalf("re-enable summaries: %v", err)
	}

	t.Run("cross-org client detail denied", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/clients/"+otherClient.ID+"/summary", nil)
		authRequest(req, salesToken)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden && rec.Code != http.StatusNotFound {
			t.Fatalf("want 403 or 404, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func ptr[T any](v T) *T { return &v }
