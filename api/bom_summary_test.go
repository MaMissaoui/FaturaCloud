package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// TestBOMSummaryRoutes checks the Bill of Materials summaries end to end:
// admin and power_user get them, general (which has no Bill of Materials
// screen) is refused, and switching summaries off turns them into a 409.
func TestBOMSummaryRoutes(t *testing.T) {
	t.Parallel()
	mux, database, _, _ := newTestRouter(t)

	if _, err := database.CreateOrganization(db.CreateOrganizationRequest{ID: "org-bom"}); err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	for _, role := range []string{"admin", "power_user", "general"} {
		seedUser(t, database, role+"-user", "user", 1)
		if _, err := database.AddOrganizationUser("org-bom", role+"-user", role); err != nil {
			t.Fatalf("add %s-user: %v", role, err)
		}
	}
	for _, q := range []string{
		`INSERT INTO products (id, organizationId, name, type, category, price, stockEnabled, stockQuantity) VALUES
			('bom-f', 'org-bom', 'Bike', 'product', 'finished', 10000, 1, 0)`,
		`INSERT INTO products (id, organizationId, name, type, category, unitCost, stockEnabled, stockQuantity) VALUES
			('bom-c', 'org-bom', 'Wheel', 'product', 'component', 3000, 1, 9)`,
		`INSERT INTO bill_of_materials (id, organizationId, finishedProductId, componentProductId, quantityPerUnit) VALUES
			('bom-l', 'org-bom', 'bom-f', 'bom-c', 2)`,
	} {
		if _, err := database.DB.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}

	get := func(t *testing.T, role, path string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		authRequest(req, mintTestJWT(t, role+"-user", "user"))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	const list, detail = "/api/organizations/org-bom/products/bom-overview", "/api/products/bom-f/bom/summary"

	for _, role := range []string{"admin", "power_user"} {
		rec := get(t, role, detail)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s detail: want 200, got %d: %s", role, rec.Code, rec.Body.String())
		}
		var recipe db.BOMRecipeDetail
		if err := json.Unmarshal(rec.Body.Bytes(), &recipe); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if recipe.Buildable != 4 || recipe.Cost == nil || *recipe.Cost != 6000 || len(recipe.Lines) != 1 {
			t.Errorf("%s detail = %+v", role, recipe)
		}
		if rec := get(t, role, list); rec.Code != http.StatusOK {
			t.Errorf("%s list: want 200, got %d", role, rec.Code)
		}
	}
	for _, path := range []string{list, detail} {
		if rec := get(t, "general", path); rec.Code != http.StatusForbidden {
			t.Errorf("general %s: want 403, got %d", path, rec.Code)
		}
	}

	off := false
	if _, err := database.UpdateOrganization("org-bom", db.UpdateOrganizationRequest{MasterDataSummaries: &off}); err != nil {
		t.Fatalf("UpdateOrganization: %v", err)
	}
	for _, path := range []string{list, detail} {
		if rec := get(t, "admin", path); rec.Code != http.StatusConflict {
			t.Errorf("%s switched off: want 409, got %d", path, rec.Code)
		}
	}
}
