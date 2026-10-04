package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// TestProductSummaryCounterpartiesByRole checks that the product panel's
// movements keep a client only for a role that sees client balances and a
// vendor (and the last vendor) only for one that sees vendor balances, while
// the route itself stays open to every member like the other product routes.
func TestProductSummaryCounterpartiesByRole(t *testing.T) {
	t.Parallel()
	mux, database, _, _ := newTestRouter(t)

	if _, err := database.CreateOrganization(db.CreateOrganizationRequest{ID: "org-ps"}); err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	for _, role := range []string{"sales", "purchasing", "accounting", "general"} {
		seedUser(t, database, role+"-user", "user", 1)
		if _, err := database.AddOrganizationUser("org-ps", role+"-user", role); err != nil {
			t.Fatalf("add %s-user: %v", role, err)
		}
	}
	for _, q := range []string{
		`INSERT INTO clients (id, organizationId, name) VALUES ('ps-c', 'org-ps', 'A Client')`,
		`INSERT INTO vendors (id, organizationId, name) VALUES ('ps-v', 'org-ps', 'A Vendor')`,
		`INSERT INTO products (id, organizationId, name, type, stockEnabled, stockQuantity) VALUES ('ps-p', 'org-ps', 'Fridge', 'product', 1, 1)`,
		`INSERT INTO invoices (id, organizationId, number, state, clientId, date) VALUES ('ps-inv', 'org-ps', 'FAC-1', 'paid', 'ps-c', 2000)`,
		`INSERT INTO inbound_deliveries (id, organizationId, vendorId, deliveryNumber, deliveryDate) VALUES ('ps-br', 'org-ps', 'ps-v', 'BR-1', 1000)`,
		`INSERT INTO stockMovements (id, organizationId, productId, type, quantity, reference, sourceDocumentId) VALUES
			('m-out', 'org-ps', 'ps-p', 'out', -1, 'FAC-1', 'ps-inv'), ('m-in', 'org-ps', 'ps-p', 'in', 2, 'BR-1', NULL)`,
		`INSERT INTO incoming_invoices (id, organizationId, vendorId, vendorInvoiceNumber, state, date, currency) VALUES ('ps-bill', 'org-ps', 'ps-v', 'V-1', 'paid', 1000, 'TND')`,
		`INSERT INTO incoming_invoice_line_items (id, incomingInvoiceId, productId, description, quantity, unitPrice) VALUES ('ps-bill-l', 'ps-bill', 'ps-p', 'Fridge', 2, 100)`,
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

	cases := []struct {
		role           string
		client, vendor bool
	}{
		{"sales", true, false},
		{"purchasing", false, true},
		{"accounting", true, true},
		{"general", true, true},
	}
	for _, c := range cases {
		t.Run(c.role, func(t *testing.T) {
			rec := get(t, c.role, "/api/products/ps-p/summary")
			if rec.Code != http.StatusOK {
				t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
			}
			var summary db.ProductSummary
			if err := json.Unmarshal(rec.Body.Bytes(), &summary); err != nil {
				t.Fatalf("decode: %v", err)
			}
			byID := map[string]db.ProductMovement{}
			for _, m := range summary.RecentMovements {
				byID[m.ID] = m
			}
			if got := byID["m-out"].CounterpartyName != nil; got != c.client {
				t.Errorf("client name shown = %v, want %v", got, c.client)
			}
			if got := byID["m-in"].CounterpartyName != nil; got != c.vendor {
				t.Errorf("vendor name shown = %v, want %v", got, c.vendor)
			}
			if got := summary.LastVendor != nil; got != c.vendor {
				t.Errorf("last vendor shown = %v, want %v", got, c.vendor)
			}
			// The documents themselves stay: the stock ledger already shows them.
			if byID["m-out"].DocumentKind != "invoice" || byID["m-in"].DocumentKind != "receipt" {
				t.Errorf("documents = %q / %q", byID["m-out"].DocumentKind, byID["m-in"].DocumentKind)
			}

			if rec := get(t, c.role, "/api/organizations/org-ps/products/summary"); rec.Code != http.StatusOK {
				t.Errorf("list: want 200, got %d", rec.Code)
			}
		})
	}

	off := false
	if _, err := database.UpdateOrganization("org-ps", db.UpdateOrganizationRequest{MasterDataSummaries: &off}); err != nil {
		t.Fatalf("UpdateOrganization: %v", err)
	}
	for _, path := range []string{"/api/products/ps-p/summary", "/api/organizations/org-ps/products/summary"} {
		if rec := get(t, "general", path); rec.Code != http.StatusConflict {
			t.Errorf("%s switched off: want 409, got %d", path, rec.Code)
		}
	}
}
