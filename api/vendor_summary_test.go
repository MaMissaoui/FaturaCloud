package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// TestVendorSummaryRoutes exercises both vendor-summary endpoints for the
// section guard (purchasing → 200, sales → 403), the switched-off 409, and
// cross-org access denial.
func TestVendorSummaryRoutes(t *testing.T) {
	t.Parallel()
	mux, database, _, _ := newTestRouter(t)

	seedUser(t, database, "sales-user", "user", 1)
	seedUser(t, database, "purchasing-user", "user", 1)

	if _, err := database.CreateOrganization(db.CreateOrganizationRequest{ID: "org-vs"}); err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if _, err := database.AddOrganizationUser("org-vs", "sales-user", "sales"); err != nil {
		t.Fatalf("add sales-user: %v", err)
	}
	if _, err := database.AddOrganizationUser("org-vs", "purchasing-user", "purchasing"); err != nil {
		t.Fatalf("add purchasing-user: %v", err)
	}

	vendor, err := database.CreateVendor(db.CreateVendorRequest{
		ID: "vs-vendor", OrganizationID: "org-vs", Name: strPtr("Vendor Co"),
	})
	if err != nil {
		t.Fatalf("CreateVendor: %v", err)
	}

	salesToken := mintTestJWT(t, "sales-user", "user")
	purchasingToken := mintTestJWT(t, "purchasing-user", "user")

	// --- Purchasing member gets 200 on both routes ---
	t.Run("purchasing gets 200 list", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/organizations/org-vs/vendors/summary", nil)
		authRequest(req, purchasingToken)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
		}
		var list db.VendorSummaryList
		if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
			t.Fatalf("decode: %v", err)
		}
	})

	t.Run("purchasing gets 200 detail", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/vendors/"+vendor.ID+"/summary", nil)
		authRequest(req, purchasingToken)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	// --- Sales member gets 403 on both routes ---
	t.Run("sales gets 403 list", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/organizations/org-vs/vendors/summary", nil)
		authRequest(req, salesToken)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("want 403, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("sales gets 403 detail", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/vendors/"+vendor.ID+"/summary", nil)
		authRequest(req, salesToken)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("want 403, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	// --- Switch off summaries → purchasing gets 409 ---
	if _, err := database.UpdateOrganization("org-vs", db.UpdateOrganizationRequest{
		MasterDataSummaries: ptr(false),
	}); err != nil {
		t.Fatalf("switch off summaries: %v", err)
	}

	t.Run("purchasing gets 409 list when off", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/organizations/org-vs/vendors/summary", nil)
		authRequest(req, purchasingToken)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusConflict {
			t.Fatalf("want 409, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("purchasing gets 409 detail when off", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/vendors/"+vendor.ID+"/summary", nil)
		authRequest(req, purchasingToken)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusConflict {
			t.Fatalf("want 409, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	// --- Cross-org: a vendor from another org → 403 or 404 ---
	seedUser(t, database, "other-user", "user", 1)
	if _, err := database.CreateOrganization(db.CreateOrganizationRequest{ID: "org-other"}); err != nil {
		t.Fatalf("CreateOrganization other: %v", err)
	}
	if _, err := database.AddOrganizationUser("org-other", "other-user", "admin"); err != nil {
		t.Fatalf("add other-user: %v", err)
	}
	otherVendor, err := database.CreateVendor(db.CreateVendorRequest{
		ID: "other-vendor", OrganizationID: "org-other", Name: strPtr("OtherCo"),
	})
	if err != nil {
		t.Fatalf("CreateVendor other: %v", err)
	}

	// Re-enable summaries so the cross-org test isn't blocked by 409.
	if _, err := database.UpdateOrganization("org-vs", db.UpdateOrganizationRequest{
		MasterDataSummaries: ptr(true),
	}); err != nil {
		t.Fatalf("re-enable summaries: %v", err)
	}

	t.Run("cross-org vendor detail denied", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/vendors/"+otherVendor.ID+"/summary", nil)
		authRequest(req, purchasingToken)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden && rec.Code != http.StatusNotFound {
			t.Fatalf("want 403 or 404, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}
