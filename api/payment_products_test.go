package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/MaMissaoui/fatura-cloud/db"
)

// TestPaymentProductsAreLimitedToSalesAndCashBook: what a payment paid for is
// sales content, so only Sales and Cash Book users see it — the invoice-lines
// panel, the payment-history export and the payments list's products field
// (audit F159, owner decision 2026-09-29). The payments list itself stays
// open to every member, since PaymentPanel on purchasing pages reads it.
func TestPaymentProductsAreLimitedToSalesAndCashBook(t *testing.T) {
	t.Parallel()
	mux, database, _, _ := newTestRouter(t)
	roles := map[string]string{
		"cashier": "cashbook", "seller": "sales", "clerk": "general",
		"buyer": "purchasing", "accountant": "accounting",
	}
	for user := range roles {
		seedUser(t, database, user, "user", 1)
	}
	org, err := database.CreateOrganization(db.CreateOrganizationRequest{ID: "org-pp", Name: strPtr("Counter Org")})
	if err != nil {
		t.Fatalf("seed CreateOrganization: %v", err)
	}
	for user, role := range roles {
		if _, err := database.AddOrganizationUser(org.ID, user, role); err != nil {
			t.Fatalf("seed membership %s: %v", user, err)
		}
	}
	if _, err := database.CreateFiscalYear(db.CreateFiscalYearRequest{
		OrganizationID: org.ID, Name: "2025", StartDate: 1735689600000, EndDate: 1767225599000,
	}); err != nil {
		t.Fatalf("seed CreateFiscalYear: %v", err)
	}
	client, err := database.CreateClient(db.CreateClientRequest{OrganizationID: org.ID, Name: strPtr("Walk-in")})
	if err != nil {
		t.Fatalf("seed CreateClient: %v", err)
	}
	accounts, err := database.GetAccounts(org.ID)
	if err != nil {
		t.Fatalf("GetAccounts: %v", err)
	}
	byCode := map[string]string{}
	for _, a := range accounts {
		byCode[a.Code] = a.ID
	}
	outputTax := byCode["2200"]
	taxRate, err := database.CreateTaxRate(db.CreateTaxRateRequest{
		OrganizationID: org.ID, Name: "VAT", Percentage: 20, OutputTaxAccountID: &outputTax,
	})
	if err != nil {
		t.Fatalf("seed CreateTaxRate: %v", err)
	}
	register := byCode["1010"]
	if _, err := database.UpdateOrganization(org.ID, db.UpdateOrganizationRequest{DefaultCashRegisterAccountID: &register}); err != nil {
		t.Fatalf("seed register account: %v", err)
	}

	token := map[string]string{}
	for user := range roles {
		token[roles[user]] = mintTestJWT(t, user, "")
	}
	rec := doJSON(t, mux, token["cashbook"], http.MethodPost, "/api/cash-sales", map[string]any{
		"organizationId": org.ID, "clientId": client.ID, "date": int64(1738368000000), "currency": "EUR",
		"lineItems": []map[string]any{
			{"description": "Fridge", "quantity": 1, "unitPrice": 1000, "taxRate": taxRate.ID},
		},
		"subTotal": 1000, "taxTotal": 200, "total": 1200, "amountReceived": 1200,
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/cash-sales: expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	listProducts := func(role string) (paymentID string, products []string) {
		t.Helper()
		rec := doJSON(t, mux, token[role], http.MethodGet, "/api/organizations/"+org.ID+"/payments", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s GET payments: expected 200, got %d: %s", role, rec.Code, rec.Body.String())
		}
		var payments []db.Payment
		if err := json.Unmarshal(rec.Body.Bytes(), &payments); err != nil {
			t.Fatalf("decode payments: %v", err)
		}
		if len(payments) != 1 {
			t.Fatalf("%s: %d payments, want 1", role, len(payments))
		}
		return payments[0].ID, payments[0].Products
	}

	paymentID, _ := listProducts("cashbook")
	for role, allowed := range map[string]bool{
		"cashbook": true, "sales": true, "general": true, "purchasing": false, "accounting": false,
	} {
		_, products := listProducts(role)
		if allowed && (len(products) != 1 || products[0] != "Fridge") {
			t.Errorf("%s: products = %v, want [Fridge]", role, products)
		}
		if !allowed && len(products) != 0 {
			t.Errorf("%s: products = %v, want none", role, products)
		}

		want := http.StatusOK
		if !allowed {
			want = http.StatusForbidden
		}
		rec := doJSON(t, mux, token[role], http.MethodGet, "/api/payments/"+paymentID+"/invoice-lines", nil)
		if rec.Code != want {
			t.Errorf("%s GET invoice-lines: expected %d, got %d: %s", role, want, rec.Code, rec.Body.String())
		}
		rec = doJSON(t, mux, token[role], http.MethodGet,
			"/api/organizations/"+org.ID+"/reports/payment-history/export?format=xlsx", nil)
		if rec.Code != want {
			t.Errorf("%s GET payment-history export: expected %d, got %d", role, want, rec.Code)
		}
	}
}
