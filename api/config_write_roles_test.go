package api

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// TestConfigurationWritesAreRoleGated pins audit F170 (owner decision
// 2026-10-17, option a) with real requests: organization-wide configuration
// is changed only by the org-wide roles plus the domain role that owns it —
// products by sales/purchasing, tax rates and payment terms by accounting,
// numbering, units of measure and product families by the org-wide roles
// alone. Every member still reads them. The route tables in cross_org_test.go
// check the wiring; this checks that a request is actually refused.
func TestConfigurationWritesAreRoleGated(t *testing.T) {
	t.Parallel()
	mux, database, _, _ := newTestRouter(t)
	roles := []string{"general", "sales", "purchasing", "accounting", "cashbook"}
	org, err := database.CreateOrganization(db.CreateOrganizationRequest{ID: "org-f170", Name: strPtr("Config Org")})
	if err != nil {
		t.Fatalf("seed CreateOrganization: %v", err)
	}
	token := map[string]string{}
	for _, role := range roles {
		user := "user-" + role
		seedUser(t, database, user, "user", 1)
		if _, err := database.AddOrganizationUser(org.ID, user, role); err != nil {
			t.Fatalf("seed membership %s: %v", user, err)
		}
		token[role] = mintTestJWT(t, user, "")
	}

	cases := []struct {
		name    string
		allowed map[string]bool
		request func(role string) (method, path string, body map[string]any)
	}{
		{
			name:    "create a product",
			allowed: map[string]bool{"general": true, "sales": true, "purchasing": true},
			request: func(role string) (string, string, map[string]any) {
				return http.MethodPost, "/api/products", map[string]any{
					"organizationId": org.ID, "name": "Kettle " + role, "sku": "K-" + role,
				}
			},
		},
		{
			name:    "create a tax rate",
			allowed: map[string]bool{"general": true, "accounting": true},
			request: func(role string) (string, string, map[string]any) {
				return http.MethodPost, "/api/tax-rates", map[string]any{
					"organizationId": org.ID, "name": "VAT " + role, "percentage": 7,
				}
			},
		},
		{
			name:    "create a payment term",
			allowed: map[string]bool{"general": true, "accounting": true},
			request: func(role string) (string, string, map[string]any) {
				return http.MethodPost, "/api/payment-terms", map[string]any{
					"organizationId": org.ID, "name": "Net " + role, "days": 30,
				}
			},
		},
		{
			name:    "create a unit of measure",
			allowed: map[string]bool{"general": true},
			request: func(role string) (string, string, map[string]any) {
				return http.MethodPost, "/api/units-of-measure", map[string]any{
					"organizationId": org.ID, "name": "box-" + role,
				}
			},
		},
		{
			name:    "create a product family",
			allowed: map[string]bool{"general": true},
			request: func(role string) (string, string, map[string]any) {
				return http.MethodPost, "/api/product-families", map[string]any{
					"organizationId": org.ID, "name": "Family " + role,
				}
			},
		},
		{
			name:    "set a document counter",
			allowed: map[string]bool{"general": true},
			request: func(role string) (string, string, map[string]any) {
				return http.MethodPut, fmt.Sprintf("/api/organizations/%s/document-number-settings/order", org.ID), map[string]any{
					"format": "ORD-{number}", "counter": 1,
				}
			},
		},
	}
	for _, tc := range cases {
		for _, role := range roles {
			method, path, body := tc.request(role)
			rec := doJSON(t, mux, token[role], method, path, body)
			if tc.allowed[role] {
				if rec.Code >= 300 {
					t.Errorf("%s as %s: status %d (%s); want success", tc.name, role, rec.Code, rec.Body.String())
				}
			} else if rec.Code != http.StatusForbidden {
				t.Errorf("%s as %s: status %d; want 403", tc.name, role, rec.Code)
			}
		}
	}

	// Reads stay open to every member: the screens a restricted role keeps
	// still list products, tax rates and the numbering format.
	for _, role := range roles {
		for _, path := range []string{
			"/api/organizations/" + org.ID + "/tax-rates",
			"/api/organizations/" + org.ID + "/payment-terms",
			"/api/organizations/" + org.ID + "/document-number-settings/order",
		} {
			if rec := doJSON(t, mux, token[role], http.MethodGet, path, nil); rec.Code != http.StatusOK {
				t.Errorf("GET %s as %s: status %d; want 200", path, role, rec.Code)
			}
		}
	}
}
